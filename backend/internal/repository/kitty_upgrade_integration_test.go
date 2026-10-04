//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// A frozen prior release is supplied by the release harness. The fixture is
// synthetic, localhost-only, and has its own schema; no production rows.
func TestKitty291UpgradePreservesVideoAndRetiresOnlyManualPrism(t *testing.T) {
	baseline := os.Getenv("KITTY_TEST_BASELINE_MIGRATIONS")
	if baseline == "" {
		t.Skip("release harness must supply frozen 2.9.1 migrations")
	}
	dsn := os.Getenv("KITTY_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, u.Hostname())
	schema := fmt.Sprintf("kitty_upgrade_%d", time.Now().UnixNano())
	_, err = integrationDB.Exec("CREATE DATABASE " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	u.Path = "/" + schema
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		_, e := integrationDB.Exec("DROP DATABASE " + pq.QuoteIdentifier(schema))
		require.NoError(t, e)
	})
	ctx := context.Background()
	require.NoError(t, applyMigrationsFS(ctx, db, os.DirFS(baseline)))
	_, err = db.Exec(`
INSERT INTO users(id,email,password_hash,balance) VALUES(9991,'upgrade@example.invalid','fixture',123.45);
INSERT INTO groups(id,name,platform) VALUES(9991,'upgrade-video','video');
INSERT INTO user_platform_quotas(user_id,platform,daily_usage_usd) VALUES(9991,'video',12.34);
INSERT INTO composite_model_routes(group_id,public_model,target_platform) VALUES(9991,'fixture-video','video');
INSERT INTO accounts(id,name,platform,type,credentials,extra) VALUES(9991,'fixture-pro','openai','oauth',
 '{"access_token":"fixture-oauth","prism_cookie_encrypted":"fixture-retired","prism_user_id":"fixture-user","prism_project_id":"fixture-project"}',
 '{"openai_prism":true,"openai_prism_models":["old-model"],"openai_prism_browser_models":["new-model"],"unrelated":true}');`)
	require.NoError(t, err)
	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, ApplyMigrations(ctx, db), "upgrade must be repeatable")
	var balance, usage float64
	require.NoError(t, db.QueryRow("SELECT balance FROM users WHERE id=9991").Scan(&balance))
	require.Equal(t, 123.45, balance)
	require.NoError(t, db.QueryRow("SELECT daily_usage_usd FROM user_platform_quotas WHERE user_id=9991 AND platform='video'").Scan(&usage))
	require.Equal(t, 12.34, usage)
	var intact bool
	require.NoError(t, db.QueryRow(`SELECT credentials->>'access_token'='fixture-oauth'
 AND NOT credentials ? 'prism_cookie_encrypted' AND NOT credentials ? 'prism_user_id'
 AND NOT extra ? 'openai_prism' AND NOT extra ? 'openai_prism_models'
 AND extra->'openai_prism_browser_models'='["new-model"]'::jsonb AND extra->>'unrelated'='true'
 FROM accounts WHERE id=9991`).Scan(&intact))
	require.True(t, intact)
	_, err = db.Exec(`INSERT INTO user_platform_quotas(user_id,platform) VALUES(9991,'typesafe');
INSERT INTO composite_model_routes(group_id,public_model,target_platform) VALUES(9991,'fixture-typesafe','typesafe')`)
	require.NoError(t, err)
}
