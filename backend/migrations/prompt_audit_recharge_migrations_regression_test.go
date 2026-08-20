package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptAuditAndRechargeBonusMigrationsCoexist(t *testing.T) {
	files := []string{
		"182_prompt_audit_full_prompt.sql",
		"182_recharge_bonus_credits.sql",
	}

	for _, name := range files {
		content, err := FS.ReadFile(name)
		require.NoError(t, err)
		require.NotEmpty(t, content)
	}

	require.NotEqual(t, files[0], files[1])
}
