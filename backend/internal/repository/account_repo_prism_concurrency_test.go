package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Two connections cannot share a PostgreSQL TEMP table. This fixture therefore
// owns a unique short-lived schema in the local maintenance database, never an
// application's schema. Every pooled connection has that exact search_path.
func TestPrismManualEditConcurrentRefreshPostgres(t *testing.T) {
	dsn := os.Getenv("PRISM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set PRISM_TEST_POSTGRES_DSN for isolated local PostgreSQL concurrency tests")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, u.Hostname())
	require.Equal(t, "/postgres", u.Path, "use the local maintenance database, not application data")
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := fmt.Sprintf("prism_manual_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(fixture))
	require.NoError(t, err)
	defer func() {
		_, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+pq.QuoteIdentifier(fixture)+" CASCADE")
		require.NoError(t, err)
	}()
	q := u.Query()
	q.Set("search_path", fixture)
	q.Set("application_name", fixture)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(5)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	accountTable := *migrate.AccountsTable
	accountTable.ForeignKeys = nil
	require.NoError(t, migrate.Create(ctx, client.Schema, []*schema.Table{&accountTable}, migrate.WithForeignKeys(false)))
	_, err = db.ExecContext(ctx, `CREATE TABLE scheduler_outbox (event_type text, account_id bigint, group_id bigint, payload jsonb, dedup_key text);
		CREATE UNIQUE INDEX ON scheduler_outbox(dedup_key) WHERE dedup_key IS NOT NULL`)
	require.NoError(t, err)
	repo := newAccountRepositoryWithSQL(client, db, nil)
	copyCreds := func(m map[string]any) map[string]any {
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	newAccount := func(t *testing.T) *service.Account {
		creds := map[string]any{"access_token": "old-access", "refresh_token": "old-refresh", "expires_at": "old-expiry", service.PrismCookieEncryptedKey: "old-cookie"}
		a, err := client.Account.Create().SetName("synthetic-prism").SetPlatform(service.PlatformOpenAI).
			SetType(service.AccountTypeOAuth).SetCredentials(creds).SetExtra(map[string]any{service.PrismEnabledExtraKey: true}).Save(ctx)
		require.NoError(t, err)
		return &service.Account{ID: a.ID, Name: a.Name, Platform: a.Platform, Type: a.Type, Credentials: copyCreds(creds),
			Extra: map[string]any{service.PrismEnabledExtraKey: true}, Status: service.StatusActive, Concurrency: 1, Priority: 1, Schedulable: true}
	}
	readCreds := func(t *testing.T, id int64) map[string]any {
		var raw []byte
		require.NoError(t, db.QueryRowContext(ctx, `SELECT credentials FROM accounts WHERE id=$1`, id).Scan(&raw))
		var got map[string]any
		require.NoError(t, json.Unmarshal(raw, &got))
		return got
	}
	waitBlocked := func(t *testing.T) {
		require.Eventually(t, func() bool {
			var blocked bool
			err := admin.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')`, fixture).Scan(&blocked)
			return err == nil && blocked
		}, 3*time.Second, 10*time.Millisecond, "the competing writer must actually wait for the row lock")
	}
	t.Run("refresh_then_manual_bind_keeps_fresh_tokens_and_expiry", func(t *testing.T) {
		a := newAccount(t)
		edit := service.PrismCredentialsUpdate{Original: copyCreds(a.Credentials), ExplicitKeys: []string{service.PrismCookieEncryptedKey}}
		a.Credentials[service.PrismCookieEncryptedKey] = "new-cookie"
		// Cookie-only API updates previously omitted the old expiry field.
		delete(a.Credentials, "expires_at")
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		defer tx.Rollback()
		fresh := map[string]any{"access_token": "fresh-access", "refresh_token": "fresh-refresh", "expires_at": "fresh-expiry"}
		require.NoError(t, repo.UpdateCredentials(dbent.NewTxContext(ctx, tx), a.ID, fresh))
		done := make(chan error, 1)
		go func() { done <- repo.UpdateWithPrismAccountSettings(ctx, a, edit, nil, nil, nil) }()
		waitBlocked(t)
		require.NoError(t, tx.Commit())
		require.NoError(t, <-done)
		fresh[service.PrismCookieEncryptedKey] = "new-cookie"
		require.Equal(t, fresh, readCreds(t, a.ID))
		require.Equal(t, fresh, a.Credentials, "returned account must contain committed fresh credentials")
	})
	t.Run("manual_bind_then_stale_refresh_keeps_new_cookie", func(t *testing.T) {
		a := newAccount(t)
		stale := copyCreds(a.Credentials)
		edit := service.PrismCredentialsUpdate{Original: copyCreds(a.Credentials), ExplicitKeys: []string{service.PrismCookieEncryptedKey}}
		a.Credentials[service.PrismCookieEncryptedKey] = "new-cookie"
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		defer tx.Rollback()
		require.NoError(t, repo.UpdateWithPrismAccountSettings(dbent.NewTxContext(ctx, tx), a, edit, nil, nil, nil))
		stale["access_token"] = "fresh-access"
		done := make(chan error, 1)
		go func() { done <- repo.UpdateCredentials(ctx, a.ID, stale) }()
		waitBlocked(t)
		require.NoError(t, tx.Commit())
		require.NoError(t, <-done)
		stale[service.PrismCookieEncryptedKey] = "new-cookie"
		require.Equal(t, stale, readCreds(t, a.ID))
	})
	t.Run("unrelated_edit_preserves_concurrent_rebind", func(t *testing.T) {
		a := newAccount(t)
		edit := service.PrismCredentialsUpdate{Original: copyCreds(a.Credentials)}
		_, err := db.ExecContext(ctx, `UPDATE accounts SET credentials=jsonb_set(credentials,'{prism_cookie_encrypted}','"new-cookie"') WHERE id=$1`, a.ID)
		require.NoError(t, err)
		a.Name = "renamed"
		require.NoError(t, repo.UpdateWithPrismAccountSettings(ctx, a, edit, nil, nil, nil))
		require.Equal(t, "new-cookie", readCreds(t, a.ID)[service.PrismCookieEncryptedKey])
	})
	t.Run("explicit_unlink_removes_binding_missing_from_original", func(t *testing.T) {
		a := newAccount(t)
		delete(a.Credentials, service.PrismCookieEncryptedKey)
		a.Extra[service.PrismEnabledExtraKey] = false
		edit := service.PrismCredentialsUpdate{Original: copyCreds(a.Credentials), ExplicitKeys: []string{service.PrismCookieKey, service.PrismCookieEncryptedKey}}
		require.NoError(t, repo.UpdateWithPrismAccountSettings(ctx, a, edit, nil, nil, nil))
		require.NotContains(t, readCreds(t, a.ID), service.PrismCookieEncryptedKey)
	})
	t.Run("explicit_oauth_value_wins_even_if_equal_to_original", func(t *testing.T) {
		a := newAccount(t)
		edit := service.PrismCredentialsUpdate{Original: copyCreds(a.Credentials), ExplicitKeys: []string{"access_token"}}
		require.NoError(t, repo.UpdateCredentials(ctx, a.ID, map[string]any{"access_token": "fresh-access", "refresh_token": "fresh-refresh"}))
		require.NoError(t, repo.UpdateWithPrismAccountSettings(ctx, a, edit, nil, nil, nil))
		got := readCreds(t, a.ID)
		require.Equal(t, "old-access", got["access_token"])
		require.Equal(t, "fresh-refresh", got["refresh_token"])
	})
	t.Run("concurrent_unlink_does_not_restore_stale_enabled_binding", func(t *testing.T) {
		a := newAccount(t)
		edit := service.PrismCredentialsUpdate{Original: copyCreds(a.Credentials)}
		_, err := db.ExecContext(ctx, `UPDATE accounts SET credentials=credentials-'prism_cookie_encrypted', extra='{"openai_prism":false}' WHERE id=$1`, a.ID)
		require.NoError(t, err)
		require.ErrorContains(t, repo.UpdateWithPrismAccountSettings(ctx, a, edit, nil, nil, nil), "session changed")
		require.NotContains(t, readCreds(t, a.ID), service.PrismCookieEncryptedKey)
	})
	t.Run("outbox_failure_rolls_back_cookie_and_settings", func(t *testing.T) {
		a := newAccount(t)
		before := copyCreds(a.Credentials)
		edit := service.PrismCredentialsUpdate{Original: before, ExplicitKeys: []string{service.PrismCookieEncryptedKey}}
		a.Credentials[service.PrismCookieEncryptedKey] = "must-rollback"
		a.Name = "must-rollback"
		_, err := db.ExecContext(ctx, `ALTER TABLE scheduler_outbox ADD CONSTRAINT reject_test_event CHECK(false) NOT VALID`)
		require.NoError(t, err)
		defer db.ExecContext(ctx, `ALTER TABLE scheduler_outbox DROP CONSTRAINT reject_test_event`)
		require.Error(t, repo.UpdateWithPrismAccountSettings(ctx, a, edit, nil, nil, nil))
		require.Equal(t, before, readCreds(t, a.ID))
		var name string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT name FROM accounts WHERE id=$1`, a.ID).Scan(&name))
		require.Equal(t, "synthetic-prism", name)
	})
}
