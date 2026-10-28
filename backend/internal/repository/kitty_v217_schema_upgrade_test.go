//go:build media_postgres

package repository

import (
	"context"
	"database/sql"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"testing"
)

func TestKittyV2177SchemaUpgrade(t *testing.T) {
	dsn := os.Getenv("KITTY_SCHEMA_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", u.Hostname())
	require.Equal(t, "/kitty_v217_schema_test", u.Path)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	before := map[string]string{}
	rows, err := db.QueryContext(ctx, "SELECT filename,checksum FROM schema_migrations")
	require.NoError(t, err)
	for rows.Next() {
		var n, c string
		require.NoError(t, rows.Scan(&n, &c))
		before[n] = c
	}
	require.NoError(t, rows.Close())
	require.Len(t, before, 413)
	require.NotEmpty(t, before["298_kitty_retained_media.sql"])
	require.NotEmpty(t, before["299_quality_rule_presets.sql"])
	require.Empty(t, before["300_account_relogin_budget.sql"])
	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, validateManagedMigrations(ctx, db, migrations.FS))
	for n, c := range before {
		var got string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename=$1", n).Scan(&got))
		require.Equal(t, c, got)
	}
	for _, name := range []string{"300_account_relogin_budget.sql", "301_content_moderation_email_exemption.sql", "302_usage_billing_quotes.sql", "303_usage_billing_delivery.sql", "304_usage_platform_quota_deltas.sql", "305_batch_image_owned_money.sql", "306_usage_billing_quote_precision.sql", "307_user_traffic.sql", "308_user_traffic_cursor_indexes_notx.sql", "309_channel_operations.sql", "309_credential_operation_refresh_failure.sql"} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE filename=$1", name).Scan(&count))
		require.Equal(t, 1, count, name)
	}
	for _, table := range []string{"usage_billing_events", "usage_billing_quotes", "usage_platform_quota_totals", "usage_platform_quota_effective", "batch_image_money_holds"} {
		var got string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT to_regclass($1)::text", table).Scan(&got))
		require.Equal(t, table, got)
	}
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count))
	require.Equal(t, len(before)+11, count)
	_, err = db.ExecContext(ctx, "SELECT keyword_exempt,api_audit_exempt FROM content_moderation_user_state LIMIT 0;SELECT keyword_exempt,api_audit_exempt FROM content_moderation_user_actions LIMIT 0;SELECT id,pelican_config FROM quality_rule_presets LIMIT 0")
	require.NoError(t, err)
}
