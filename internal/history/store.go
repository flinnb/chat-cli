package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type Session struct {
	ID        string    `bun:"id,pk"`
	ModelID   string    `bun:"model_id,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}

type Message struct {
	ID        int64     `bun:"id,pk,autoincrement"`
	SessionID string    `bun:"session_id,notnull"`
	Role      string    `bun:"role,notnull"`
	Content   string    `bun:"content,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}

type MCPServer struct {
	ID        int64     `bun:"id,pk,autoincrement"`
	Name      string    `bun:"name,notnull,unique"`
	Transport string    `bun:"transport,notnull"`
	Endpoint  string    `bun:"endpoint,notnull"`
	Args      string    `bun:"args,notnull,default:''"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}

type MCPRegistration struct {
	Name      string
	Transport string
	Endpoint  string
	Args      []string
}

type Store struct {
	db *bun.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	store := &Store{db: db}
	if _, err := db.NewCreateTable().Model((*Session)(nil)).IfNotExists().Exec(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("create sessions table: %w", err)
	}
	if _, err := db.NewCreateTable().Model((*Message)(nil)).IfNotExists().Exec(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("create messages table: %w", err)
	}
	if _, err := db.NewCreateTable().Model((*MCPServer)(nil)).IfNotExists().Exec(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("create MCP servers table: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateSession(ctx context.Context, modelID string) (*Session, error) {
	session := &Session{ID: fmt.Sprintf("%d", time.Now().UnixNano()), ModelID: modelID, CreatedAt: time.Now().UTC()}
	_, err := s.db.NewInsert().Model(session).Exec(ctx)
	return session, err
}

func (s *Store) AddMessage(ctx context.Context, sessionID, role, content string) error {
	_, err := s.db.NewInsert().Model(&Message{SessionID: sessionID, Role: role, Content: content, CreatedAt: time.Now().UTC()}).Exec(ctx)
	return err
}

func (s *Store) AddMCPServer(ctx context.Context, registration MCPRegistration) error {
	args, err := json.Marshal(registration.Args)
	if err != nil {
		return err
	}
	_, err = s.db.NewInsert().Model(&MCPServer{
		Name: registration.Name, Transport: registration.Transport,
		Endpoint: registration.Endpoint, Args: string(args), CreatedAt: time.Now().UTC(),
	}).Exec(ctx)
	return err
}

func (s *Store) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	servers := make([]MCPServer, 0)
	err := s.db.NewSelect().Model(&servers).Order("created_at ASC").Scan(ctx)
	return servers, err
}

func DecodeMCPArgs(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(value), &args); err != nil {
		return nil, err
	}
	return args, nil
}
