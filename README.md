# chat-cli

`chat-cli` is an interactive terminal chat client for Amazon Bedrock. It uses
the Bedrock Converse API and stores chat history locally with SQLite and Bun.

## Requirements

- Go 1.26 or newer
- AWS credentials configured through the AWS SDK's standard credential chain
- An AWS region with Bedrock model access
- Access to at least one supported Bedrock Converse model

For example:

```sh
export AWS_REGION=us-east-1
aws configure
```

## Run

Run directly from the project:

```sh
go run . start
```

Or build an executable:

```sh
go build -o chat-cli .
./chat-cli start
```

When the session starts, use the arrow keys to scroll through available models
and press Enter to select one. The chat body is scrollable with the arrow keys
or mouse wheel.

## Chat commands

```text
/help          Show available commands
/exit          Exit the chat
/quit          Exit the chat
/mcp add ...   Register an MCP server
/mcp ls        List registered MCP servers
```

## MCP servers

Register a local stdio MCP server by providing its command and optional
arguments:

```text
/mcp add filesystem npx -y @modelcontextprotocol/server-filesystem /tmp
```

Register an HTTP MCP server by providing its URL:

```text
/mcp add my-server https://example.com/mcp
```

Registrations are stored in `data/chat.db`. The CLI currently records MCP
servers for management; MCP tool discovery and invocation are not yet wired
into chat requests.

## Development

Run tests and static analysis:

```sh
go test ./...
go vet ./...
```

