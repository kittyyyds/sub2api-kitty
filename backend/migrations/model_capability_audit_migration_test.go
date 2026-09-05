package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelCapabilityAuditBaseMigrationIncludesCompatibilityColumns(t *testing.T) {
	content, err := FS.ReadFile("234_model_capability_audit.sql")
	require.NoError(t, err)

	sql := string(content)
	digest := sha256.Sum256(content)
	require.Equal(t, "bf52527402cae107ffded148eeb06ee2b8a228d464c62bc947a91dca2cbac78b", hex.EncodeToString(digest[:]))
	require.Contains(t, sql, "ALTER TABLE model_capability_probe_results")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS upstream_model VARCHAR(200)")
	require.Contains(t, sql, "response_summary VARCHAR(512)")
}

func TestModelCapabilityAuditResilienceUsesAdditiveMigration(t *testing.T) {
	content, err := FS.ReadFile("235_model_capability_audit_resilience.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS request_key VARCHAR(64)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ")
	require.Contains(t, sql, "ALTER COLUMN response_summary TYPE TEXT")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS model_behavior_audit_results")
	require.Contains(t, sql, "idx_model_capability_probe_runs_running_request")
	require.Contains(t, sql, "WHERE status = 'running' AND request_key IS NOT NULL")
}
