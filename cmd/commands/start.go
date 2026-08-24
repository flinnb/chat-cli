package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/flinnb/chat-cli/internal/bedrock"
	"github.com/flinnb/chat-cli/internal/history"
)

func newStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start an interactive Bedrock chat",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runChat(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), os.Stderr)
		},
	}
}

func runChat(ctx context.Context, in io.Reader, out, errOut io.Writer) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS configuration: %w", err)
	}

	modelsClient := bedrock.NewAWSModelsClient(cfg)
	models, err := modelsClient.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("list Bedrock models: %w", err)
	}
	modelID, err := chooseModelScrollable(in, out, models)
	if err != nil {
		return err
	}

	chatClient := bedrock.NewAWSChatClient(cfg)
	store, err := history.Open(filepath.Join("data", "chat.db"))
	if err != nil {
		return fmt.Errorf("open chat history: %w", err)
	}
	defer store.Close()

	session, err := store.CreateSession(ctx, modelID)
	if err != nil {
		return fmt.Errorf("create chat session: %w", err)
	}
	fmt.Fprintf(out, "\nChatting with %s. Type /help for commands, /exit to quit.\n\n", modelID)
	return repl(ctx, in, out, errOut, chatClient, store, session.ID, modelID)
}

type modelPicker struct {
	models        []bedrock.Model
	selectedIndex int
	offset        int
	height        int
	selected      string
}

func chooseModelScrollable(in io.Reader, out io.Writer, models []bedrock.Model) (string, error) {
	if len(models) == 0 {
		return "", errors.New("no Bedrock models supporting Converse are available")
	}
	picker := modelPicker{models: models, height: 10}
	finalModel, err := tea.NewProgram(picker, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return "", err
	}
	result, ok := finalModel.(modelPicker)
	if !ok || result.selected == "" {
		return "", io.EOF
	}
	return result.selected, nil
}

func (m modelPicker) Init() tea.Cmd { return nil }

func (m modelPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = max(1, size.Height-4)
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			m.selected = m.models[m.selectedIndex].ID
			return m, tea.Quit
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				if m.selectedIndex < m.offset {
					m.offset = m.selectedIndex
				}
			}
			return m, nil
		case "down", "j", "mousewheel down":
			if m.selectedIndex < len(m.models)-1 {
				m.selectedIndex++
				if m.selectedIndex >= m.offset+m.height {
					m.offset = m.selectedIndex - m.height + 1
				}
			}
			return m, nil
		case "pgup", "mousewheel up":
			m.selectedIndex = max(0, m.selectedIndex-m.height)
			m.offset = max(0, m.offset-m.height)
			return m, nil
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m modelPicker) View() string {
	var output strings.Builder
	output.WriteString("Choose a Bedrock model (↑/↓ to scroll, Enter to select)\n\n")
	end := min(len(m.models), m.offset+m.height)
	for i := m.offset; i < end; i++ {
		marker := "  "
		if i == m.selectedIndex {
			marker = "> "
		}
		fmt.Fprintf(&output, "%s%d) %s (%s)\n", marker, i+1, m.models[i].Name, m.models[i].ID)
	}
	if end < len(m.models) {
		output.WriteString("\n  ↓ more models")
	}
	return output.String()
}

func chooseModel(in io.Reader, out io.Writer, models []bedrock.Model) (string, error) {
	if len(models) == 0 {
		return "", errors.New("no Bedrock models supporting Converse are available")
	}
	fmt.Fprintln(out, "Available Bedrock models:")
	for i, model := range models {
		fmt.Fprintf(out, "  %d) %s (%s)\n", i+1, model.Name, model.ID)
	}
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprintf(out, "Select a model [1-%d]: ", len(models))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}
		n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err == nil && n >= 1 && n <= len(models) {
			return models[n-1].ID, nil
		}
		fmt.Fprintln(out, "Please enter a valid model number.")
	}
}

func repl(ctx context.Context, in io.Reader, out, _ io.Writer, client bedrock.ChatClient, store *history.Store, sessionID, modelID string) error {
	model := newShellModel(ctx, client, store, sessionID, modelID)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	if shell, ok := finalModel.(shellModel); ok && shell.err != nil {
		return shell.err
	}
	return nil
}

type shellModel struct {
	ctx       context.Context
	client    bedrock.ChatClient
	store     *history.Store
	sessionID string
	modelID   string
	viewport  viewport.Model
	input     textarea.Model
	messages  []bedrock.Message
	body      []string
	width     int
	quitting  bool
	err       error
}

type responseMsg struct {
	text string
	err  error
}

func newShellModel(ctx context.Context, client bedrock.ChatClient, store *history.Store, sessionID, modelID string) shellModel {
	input := textarea.New()
	input.Placeholder = "Message Bedrock..."
	input.Focus()
	input.CharLimit = 0
	input.SetHeight(3)
	return shellModel{
		ctx: ctx, client: client, store: store, sessionID: sessionID, modelID: modelID,
		viewport: viewport.New(0, 0), input: input,
		body: []string{"Type /help for commands. Use the viewport's arrow keys or mouse wheel to scroll."},
	}
}

func (m shellModel) Init() tea.Cmd { return textarea.Blink }

func (m shellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.input.SetWidth(msg.Width)
		m.viewport.Width = msg.Width
		m.viewport.Height = max(1, msg.Height-6)
		m.viewport.SetContent(strings.Join(m.body, "\n"))
		m.viewport.GotoBottom()
	case responseMsg:
		if msg.err != nil {
			m.body = append(m.body, wrapForScreen("error> "+msg.err.Error(), m.width))
		} else {
			m.messages = append(m.messages, bedrock.Message{Role: "assistant", Content: msg.text})
			if err := m.store.AddMessage(m.ctx, m.sessionID, "assistant", msg.text); err != nil {
				m.err = fmt.Errorf("save assistant message: %w", err)
				return m, tea.Quit
			}
			m.body = append(m.body, wrapForScreen("assistant> "+msg.text, m.width), "")
		}
		m.viewport.SetContent(strings.Join(m.body, "\n"))
		m.viewport.GotoBottom()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			if text == "" {
				return m, nil
			}
			if text == "/exit" || text == "/quit" {
				m.quitting = true
				return m, tea.Quit
			}
			if text == "/help" {
				m.body = append(m.body, "/help  Show commands", "/exit  Exit the chat", "/quit  Exit the chat", "")
				m.viewport.SetContent(strings.Join(m.body, "\n"))
				m.viewport.GotoBottom()
				return m, nil
			}
			if strings.HasPrefix(text, "/mcp ") {
				m.handleMCPCommand(text)
				m.viewport.SetContent(strings.Join(m.body, "\n"))
				m.viewport.GotoBottom()
				return m, nil
			}
			if err := m.store.AddMessage(m.ctx, m.sessionID, "user", text); err != nil {
				m.err = fmt.Errorf("save user message: %w", err)
				return m, tea.Quit
			}

			m.body = append(m.body, wrapForScreen("you> "+text, m.width))
			pending := append(append([]bedrock.Message(nil), m.messages...), bedrock.Message{Role: "user", Content: text})
			m.messages = pending
			m.viewport.SetContent(strings.Join(m.body, "\n"))
			m.viewport.GotoBottom()
			return m, func() tea.Msg {
				response, err := m.client.Converse(m.ctx, m.modelID, pending)
				return responseMsg{text: response, err: err}
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.viewport, _ = m.viewport.Update(msg)
	return m, cmd
}

func (m *shellModel) handleMCPCommand(text string) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		m.body = append(m.body, "usage> /mcp add <name> <url|command> [args...]", "        /mcp ls", "")
		return
	}
	switch parts[1] {
	case "ls":
		servers, err := m.store.ListMCPServers(m.ctx)
		if err != nil {
			m.body = append(m.body, wrapForScreen("error> "+err.Error(), m.width))
			return
		}
		if len(servers) == 0 {
			m.body = append(m.body, "mcp> no registered MCP servers", "")
			return
		}
		m.body = append(m.body, "mcp> registered servers:")
		for _, server := range servers {
			m.body = append(m.body, wrapForScreen(fmt.Sprintf("  %s [%s] %s", server.Name, server.Transport, server.Endpoint), m.width))
		}
		m.body = append(m.body, "")
	case "add":
		if len(parts) < 4 {
			m.body = append(m.body, "usage> /mcp add <name> <url|command> [args...]", "")
			return
		}
		registration := history.MCPRegistration{
			Name: parts[2], Endpoint: parts[3], Transport: "stdio", Args: parts[4:],
		}
		if strings.HasPrefix(strings.ToLower(registration.Endpoint), "http://") ||
			strings.HasPrefix(strings.ToLower(registration.Endpoint), "https://") {
			registration.Transport = "http"
			registration.Args = nil
		}
		if err := m.store.AddMCPServer(m.ctx, registration); err != nil {
			m.body = append(m.body, wrapForScreen("error> "+err.Error(), m.width))
			return
		}
		m.body = append(m.body, "mcp> registered "+registration.Name, "")
	default:
		m.body = append(m.body, "usage> /mcp add <name> <url|command> [args...]", "        /mcp ls", "")
	}
}

func (m shellModel) View() string {
	if m.quitting {
		return "Goodbye.\n"
	}
	header := lipgloss.NewStyle().Bold(true).Render("chat-cli · " + m.modelID)
	return header + "\n\n" + m.viewport.View() + "\n\n" + m.input.View()
}

func wrapForScreen(text string, width int) string {
	if width <= 0 {
		width = 80
	}
	return lipgloss.NewStyle().Width(width).Render(text)
}
