package migrations

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetiredGrokActivationRemainsAuditableButDoesNotRun(t *testing.T) {
	const name = "188_enable_grok_enhancement_group.sql"
	_, err := FS.ReadFile(name)
	require.Error(t, err, "startup must not reactivate the retired prompt campaign")
	content, err := HistoryFS.ReadFile("history/" + name)
	require.NoError(t, err)
	require.Equal(t, "d53c9ab1f36d012ae13dfdb22b7303c94d6d9ebe01704430b681f9c6fc24fbe1",
		fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(string(content))))))
}
