package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/stretchr/testify/require"
)

func TestManagedMigrationsReadOnlyValidation(t *testing.T) {
	content := "CREATE TABLE example(id BIGINT);"
	h := sha256.Sum256([]byte(content))
	checksum := hex.EncodeToString(h[:])
	fsys := fstest.MapFS{"001.sql": &fstest.MapFile{Data: []byte(content)}}
	for _, tc := range []struct {
		name, checksum, extraName, extraSum string
		missing, ok                         bool
	}{
		{name: "exact", checksum: checksum, ok: true},
		{name: "known_history", checksum: checksum, extraName: "231_disable_long_context_pricing.sql", extraSum: managedHistoricalMigrations["231_disable_long_context_pricing.sql"], ok: true},
		{name: "retired_grok_data_operation", checksum: checksum, extraName: "188_enable_grok_enhancement_group.sql", extraSum: "d53c9ab1f36d012ae13dfdb22b7303c94d6d9ebe01704430b681f9c6fc24fbe1", ok: true},
		{name: "retired_grok_checksum_changed", checksum: checksum, extraName: "188_enable_grok_enhancement_group.sql", extraSum: "bad"},
		{name: "exact_ticket_learning", checksum: checksum, extraName: "239_codex_harvest_node_learning.sql", extraSum: "40a2d7a2d40a9d29cfd77781c7f70ff96ef95e8824b03dc6e06cf4bc891636d9", ok: true},
		{name: "exact_ticket_events", checksum: checksum, extraName: "240_codex_harvest_flow_events.sql", extraSum: "5a438fe8536068659b50d4d03d24fd57da21f1f9e226ac920332506e985d173e", ok: true},
		{name: "ticket_learning_changed", checksum: checksum, extraName: "239_codex_harvest_node_learning.sql", extraSum: "bad"},
		{name: "ticket_events_changed", checksum: checksum, extraName: "240_codex_harvest_flow_events.sql", extraSum: "bad"},
		{name: "mismatch", checksum: "bad"},
		{name: "missing", missing: true},
		{name: "unknown_extra", checksum: checksum, extraName: "999.sql", extraSum: checksum},
		{name: "history_changed", checksum: checksum, extraName: "231_disable_long_context_pricing.sql", extraSum: "bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
			rows := sqlmock.NewRows([]string{"filename", "checksum"})
			if !tc.missing {
				rows.AddRow("001.sql", tc.checksum)
			}
			if tc.extraName != "" {
				rows.AddRow(tc.extraName, tc.extraSum)
			}
			mock.ExpectQuery("SELECT filename, checksum FROM schema_migrations ORDER BY filename").WillReturnRows(rows)
			if tc.ok {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = validateManagedMigrations(context.Background(), db, fsys)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestManagedBootstrapSecretsLoadsWithoutInsert(t *testing.T) {
	role := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(role, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "readonly-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", role)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	t.Cleanup(func() { _ = os.Setenv("KITTY_MANAGED_INSTANCE", "false"); _ = managedinstance.ConfigureFromEnv() })
	client := newSecuritySecretTestClient(t)
	cfg := &config.Config{}
	require.Error(t, ensureBootstrapSecrets(context.Background(), client, cfg))
	count, err := client.SecuritySecret.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	_, err = client.SecuritySecret.Create().SetKey(securitySecretKeyJWT).SetValue("existing-jwt-secret-32bytes-long!!!!").Save(context.Background())
	require.NoError(t, err)
	cfg.JWT.Secret = "different-configured-secret-32bytes-long"
	require.NoError(t, ensureBootstrapSecrets(context.Background(), client, cfg))
	require.Equal(t, "existing-jwt-secret-32bytes-long!!!!", cfg.JWT.Secret)
	count, err = client.SecuritySecret.Query().Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
