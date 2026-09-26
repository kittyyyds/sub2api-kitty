package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokMediaHistoryLookupIndexesMigration(t *testing.T) {
	content, err := FS.ReadFile("197_grok_media_history_lookup_indexes_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_application_run_steps_grok_video_request")
	require.Contains(t, sql, "ON application_run_steps (gateway_request_id, run_id)")
	require.Contains(t, sql, "executor IN ('video_generation', 'video_edit', 'video_extension')")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_grok_video_history_owner")
	require.Contains(t, sql, "ON usage_logs (session_id, user_id, api_key_id, group_id, account_id)")
	require.Contains(t, sql, "video_count > 0")
}
