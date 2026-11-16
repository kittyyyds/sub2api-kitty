//go:build schema_postgres

package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// A standalone, read-only rehearsal of the managed startup ledger check. The
// same test is compiled against the candidate and its narrowly patched fallback.
func TestManagedSchemaRestoredPostgres(t *testing.T) {
	dsn := os.Getenv("KITTY_SCHEMA_TEST_DSN")
	if dsn == "" { t.Skip("set KITTY_SCHEMA_TEST_DSN for restored-backup rehearsal") }
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	var name string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
	require.Equal(t, "absorb_rehearsal", name)
	require.NoError(t, validateManagedMigrations(context.Background(), db, migrations.FS))
}
