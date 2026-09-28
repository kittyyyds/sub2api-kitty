package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPrismOAuthRefreshKeepsDurableBindingSQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	mock.ExpectBegin()
	query := `(?s)UPDATE accounts.*` + regexp.QuoteMeta("credentials = ($1::jsonb - 'prism_cookie' - 'prism_cookie_encrypted' - 'prism_user_id')") + `.*` +
		regexp.QuoteMeta("CASE WHEN credentials ? 'prism_cookie_encrypted'") + `.*` +
		regexp.QuoteMeta("jsonb_build_object('prism_cookie_encrypted', credentials -> 'prism_cookie_encrypted')") + `.*` +
		regexp.QuoteMeta("CASE WHEN credentials ? 'prism_user_id'") + `.*` +
		regexp.QuoteMeta("jsonb_build_object('prism_user_id', credentials -> 'prism_user_id')") + `.*WHERE id = \$2 AND deleted_at IS NULL`
	mock.ExpectExec(query).WithArgs(`{"access_token":"new-oauth","prism_cookie_encrypted":"stale-bound-cookie","prism_user_id":"old-user"}`, int64(27)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WithArgs(service.SchedulerOutboxEventAccountChanged, int64(27), nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, db, nil)
	require.NoError(t, repo.UpdateCredentials(context.Background(), 27, map[string]any{"access_token": "new-oauth", service.PrismCookieEncryptedKey: "stale-bound-cookie", service.PrismUserIDKey: "old-user"}))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Uses a dedicated local PostgreSQL DSN and connection-local temporary tables;
// it neither requires Docker nor reads or changes an application database.
func TestPrismOAuthRefreshPreservesCurrentBindingPostgres(t *testing.T) {
	dsn := os.Getenv("PRISM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set PRISM_TEST_POSTGRES_DSN to run the isolated PostgreSQL receipt")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TEMP TABLE accounts (id bigint PRIMARY KEY, platform text NOT NULL, type text NOT NULL,
		credentials jsonb NOT NULL DEFAULT '{}'::jsonb, extra jsonb, updated_at timestamptz, deleted_at timestamptz);
		CREATE TEMP TABLE scheduler_outbox (event_type text, account_id bigint, group_id bigint, payload jsonb, dedup_key text);
		CREATE UNIQUE INDEX ON scheduler_outbox(dedup_key) WHERE dedup_key IS NOT NULL`)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newAccountRepositoryWithSQL(client, db, nil)
	for _, tc := range []struct {
		name                 string
		current, stale, want map[string]any
	}{
		{"concurrent-rebind", map[string]any{"access_token": "old", service.PrismCookieEncryptedKey: "new-bound-cookie", service.PrismUserIDKey: "new-user"},
			map[string]any{"access_token": "fresh", service.PrismCookieEncryptedKey: "old-bound-cookie", service.PrismUserIDKey: "old-user"},
			map[string]any{"access_token": "fresh", service.PrismCookieEncryptedKey: "new-bound-cookie", service.PrismUserIDKey: "new-user"}},
		{"concurrent-unlink", map[string]any{"access_token": "old"},
			map[string]any{"access_token": "fresh", service.PrismCookieEncryptedKey: "old-bound-cookie", service.PrismUserIDKey: "old-user"},
			map[string]any{"access_token": "fresh"}},
		{"refresh-omits-prism", map[string]any{"access_token": "old", service.PrismCookieEncryptedKey: "bound", service.PrismUserIDKey: "user"},
			map[string]any{"access_token": "fresh"}, map[string]any{"access_token": "fresh", service.PrismCookieEncryptedKey: "bound", service.PrismUserIDKey: "user"}},
		{"ordinary-oauth", map[string]any{"access_token": "old"}, map[string]any{"access_token": "fresh", "refresh_token": "fresh-refresh"}, map[string]any{"access_token": "fresh", "refresh_token": "fresh-refresh"}},
		{"plaintext-rejected", map[string]any{"access_token": "old"}, map[string]any{"access_token": "fresh", service.PrismCookieKey: "must-not-persist"}, map[string]any{"access_token": "fresh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, err := json.Marshal(tc.current)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO accounts(id,platform,type,credentials) VALUES (27,'openai','oauth',$1::jsonb) ON CONFLICT(id) DO UPDATE SET credentials=excluded.credentials`, string(current))
			require.NoError(t, err)
			require.NoError(t, repo.UpdateCredentials(context.Background(), 27, tc.stale))
			var raw []byte
			require.NoError(t, db.QueryRow(`SELECT credentials FROM accounts WHERE id=27`).Scan(&raw))
			var got map[string]any
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Equal(t, tc.want, got)
		})
	}
}
