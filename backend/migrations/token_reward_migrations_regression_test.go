package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenRewardCycleMigrationUsesStableWeeklyAnchor(t *testing.T) {
	base, err := FS.ReadFile("192_token_reward_claims.sql")
	require.NoError(t, err)
	require.Contains(t, string(base), "CREATE TABLE IF NOT EXISTS token_reward_claims")

	content, err := FS.ReadFile("193_token_reward_claim_cycle_start.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS cycle_start TIMESTAMPTZ")
	require.Contains(t, sql, "SET cycle_start = period_start WHERE cycle_start IS NULL")
	require.Contains(t, sql, "ALTER COLUMN cycle_start SET NOT NULL")
	require.Contains(t, sql, "ON token_reward_claims (user_id, cycle_start, period_type, tier_key)")
}
