package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplicationCenterMigrationCreatesVersionedRunState(t *testing.T) {
	content, err := FS.ReadFile("194_application_center.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS application_runs",
		"CREATE TABLE IF NOT EXISTS application_run_steps",
		"id UUID PRIMARY KEY",
		"user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"planner_api_key_id BIGINT NOT NULL REFERENCES api_keys(id)",
		"media_api_key_id BIGINT NOT NULL REFERENCES api_keys(id)",
		"input JSONB NOT NULL",
		"request JSONB NOT NULL",
		"version BIGINT NOT NULL DEFAULT 1",
		"idx_application_runs_user_status_created",
		"idx_application_runs_session",
		"idx_application_run_steps_run_sequence",
	} {
		require.Contains(t, sql, required)
	}
	require.NotContains(t, strings.ToLower(sql), "bytea")
	require.NotContains(t, strings.ToLower(sql), "base64")
}

func TestApplicationCenterOptionalPlannerMigrationDropsNotNull(t *testing.T) {
	content, err := FS.ReadFile("195_application_center_optional_planner.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE application_runs ALTER COLUMN planner_api_key_id DROP NOT NULL")
}
