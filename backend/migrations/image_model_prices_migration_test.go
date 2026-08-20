package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupImageModelPricesMigration(t *testing.T) {
	migration, err := FS.ReadFile("222_group_image_model_prices.sql")
	require.NoError(t, err)
	require.Contains(t, string(migration), "ADD COLUMN IF NOT EXISTS image_model_prices JSONB")
	require.Contains(t, string(migration), "COMMENT ON COLUMN groups.image_model_prices")
	require.Contains(t, string(migration), "CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()")
	require.Contains(t, string(migration), "OLD.image_model_prices IS NOT DISTINCT FROM NEW.image_model_prices")

	previous, err := FS.ReadFile("196_group_auth_cache_profit_grok_prompt.sql")
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
		"grok_prompt_enabled",
		"grok_prompt_mode",
		"grok_prompt_content",
		"deleted_at",
	} {
		require.Contains(t, string(previous), "OLD."+field)
		require.Contains(t, string(migration), "OLD."+field)
	}
}
