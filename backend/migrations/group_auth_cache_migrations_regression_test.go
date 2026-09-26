package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupAuthCacheMigrationPreservesProfitAndGrokPromptFields(t *testing.T) {
	upstream, err := FS.ReadFile("193_group_profit_control_auth_cache_invalidation.sql")
	require.NoError(t, err)

	merged, err := FS.ReadFile("196_group_auth_cache_profit_grok_prompt.sql")
	require.NoError(t, err)

	for _, field := range []string{
		"status",
		"is_exclusive",
		"allow_image_generation",
		"platform",
		"subscription_type",
		"rate_multiplier",
		"peak_rate_enabled",
		"peak_start",
		"peak_end",
		"peak_rate_multiplier",
		"profit_control_enabled",
		"profit_min_margin",
		"profit_safety_buffer",
		"deleted_at",
	} {
		require.Contains(t, string(upstream), "OLD."+field)
		require.Contains(t, string(merged), "OLD."+field)
	}

	for _, field := range []string{
		"grok_prompt_enabled",
		"grok_prompt_mode",
		"grok_prompt_content",
	} {
		require.Contains(t, string(merged), "OLD."+field)
	}
}
