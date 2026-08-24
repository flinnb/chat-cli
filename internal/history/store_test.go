package history_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flinnb/chat-cli/internal/history"
)

func TestStorePersistsSessionAndMessages(t *testing.T) {
	store, err := history.Open(filepath.Join(t.TempDir(), "chat.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	session, err := store.CreateSession(context.Background(), "model-id")
	require.NoError(t, err)
	require.NotEmpty(t, session.ID)

	require.NoError(t, store.AddMessage(context.Background(), session.ID, "user", "hello"))
	require.NoError(t, store.AddMessage(context.Background(), session.ID, "assistant", "hi"))
}
