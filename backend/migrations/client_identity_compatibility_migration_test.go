package migrations

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Run against an isolated PostgreSQL instance. The temp table and transaction
// keep this test independent of application tables and persistent test data.
func TestClientIdentityCompatibilityMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_MIGRATION_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set TEST_MIGRATION_DATABASE_DSN to run PostgreSQL migration regression")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	tx, err := db.Begin()
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.Exec(`CREATE TEMP TABLE accounts (id bigint PRIMARY KEY, platform text, type text, extra jsonb)`)
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO accounts VALUES
	 (811,'openai','oauth','{"fingerprint_v2":{"enabled":true,"schema_version":2,"identity_epoch":7,"ws_mode":"ctx_pool"},"fingerprint_v2_seed":"11111111-1111-4111-8111-111111111111"}'),
	 (812,'openai','oauth','{"fingerprint_v2":{"enabled":false,"schema_version":2},"codex_fingerprint_seed":"11111111-1111-4111-8111-111111111111"}'),
	 (813,'openai','oauth','{"fingerprint_v2":{"enabled":true,"schema_version":2},"client_identity_compatibility":"integrated","client_installation_id":"keep-new-installation"}'),
	 (814,'openai','apikey','{"fingerprint_v2":{"enabled":true,"schema_version":2}}'),
	 (815,'openai','oauth','{"client_identity":{"enabled":true,"schema_version":2},"fingerprint_v2":{"enabled":true,"schema_version":2}}'),
	 (816,'openai','oauth','{"fingerprint_v2":"invalid-old-value"}'),
	 (817,'openai','oauth','{"client_identity":null,"fingerprint_v2":{"enabled":false,"schema_version":2}}')`)
	require.NoError(t, err)
	readRows := func() map[int64]map[string]any {
		rows, err := tx.Query(`SELECT id,extra FROM accounts ORDER BY id`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[int64]map[string]any{}
		for rows.Next() {
			var id int64
			var raw []byte
			require.NoError(t, rows.Scan(&id, &raw))
			var extra map[string]any
			require.NoError(t, json.Unmarshal(raw, &extra))
			out[id] = extra
		}
		require.NoError(t, rows.Err())
		return out
	}
	before := readRows()
	freeze, err := FS.ReadFile("249_client_installation_identity.sql")
	require.NoError(t, err)
	_, err = tx.Exec(string(freeze))
	require.NoError(t, err)
	compatibility, err := FS.ReadFile("250_client_identity_compatibility.sql")
	require.NoError(t, err)
	_, err = tx.Exec(string(compatibility))
	require.NoError(t, err)
	after := readRows()
	require.Equal(t, "024b0247-6ee7-4ad5-8118-a198ca6221da", after[811]["client_installation_id"])
	require.Equal(t, "ctx_pool", after[811]["openai_oauth_responses_websockets_v2_mode"])
	for _, id := range []int64{811, 812, 817} {
		require.Equal(t, "identity_v2", after[id]["client_identity_compatibility"])
	}
	require.Equal(t, "integrated", after[813]["client_identity_compatibility"])
	require.Equal(t, "keep-new-installation", after[813]["client_installation_id"])
	for _, id := range []int64{814, 815, 816} {
		require.NotContains(t, after[id], "client_identity_compatibility")
	}
	require.Equal(t, before[814], after[814], "API-key accounts are outside this migration")
	for id, extra := range before {
		for key, value := range extra {
			require.Equal(t, value, after[id][key], "existing field %s on account %d changed", key, id)
		}
	}
	_, err = tx.Exec(string(freeze))
	require.NoError(t, err)
	_, err = tx.Exec(string(compatibility))
	require.NoError(t, err)
	require.Equal(t, after, readRows(), "replaying migrations must not rotate identity or change compatibility")
}
