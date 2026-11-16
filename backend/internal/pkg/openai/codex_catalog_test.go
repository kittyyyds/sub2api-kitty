package openai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCodexCatalogPinnedHash fails when the embedded models.json is refreshed
// without updating CodexCatalogPinnedHash. Regenerate with
// `go run ./tools/codexprompts -models <path> -write`, which prints the hash.
func TestCodexCatalogPinnedHash(t *testing.T) {
	// The pinned hash is over the LF bytes stored in git; a Windows checkout
	// with core.autocrlf may materialize CRLF in the working tree.
	sum := sha256.Sum256(bytes.ReplaceAll(codexCatalogModelsJSON, []byte("\r\n"), []byte("\n")))
	require.Equal(t, CodexCatalogPinnedHash, hex.EncodeToString(sum[:]))
}

func TestCodexCatalogModelResolution(t *testing.T) {
	model, ok := CodexCatalogModelForSlug("gpt-5.6-sol-max")
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", model.Slug)
	require.NotEmpty(t, model.ModelMessages.InstructionsTemplate)

	_, ok = CodexCatalogModelForSlug("definitely-not-a-codex-model")
	require.False(t, ok)
}
