package commands

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flinnb/chat-cli/internal/bedrock"
)

func TestChooseModel(t *testing.T) {
	models := []bedrock.Model{
		{ID: "model-a", Name: "Alpha"},
		{ID: "model-b", Name: "Beta"},
	}

	selected, err := chooseModel(strings.NewReader("invalid\n2\n"), new(strings.Builder), models)

	require.NoError(t, err)
	require.Equal(t, "model-b", selected)
}

func TestChooseModelReturnsEOF(t *testing.T) {
	_, err := chooseModel(strings.NewReader(""), new(strings.Builder), []bedrock.Model{{ID: "model-a", Name: "Alpha"}})

	require.ErrorIs(t, err, io.EOF)
}
