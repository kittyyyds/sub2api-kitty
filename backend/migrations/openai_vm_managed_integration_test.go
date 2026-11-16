//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// These tests run against the database named by MIGRATION_TEST_DATABASE_URL:
// either an empty one (the whole migration chain is applied) or a copy of a
// deployed one (only the pending migrations are applied). They use the same
// entry point as server start-up, repository.ApplyMigrations.

const managedMigrationFile = "353_openai_vm_managed.sql"

// Columns openai_vm_bindings had before the managed migration.
const bindingLegacyColumns = "ROW(account_id,managed_proxy_id,original_proxy_id,original_proxy_ids,original_fallback_id,runtime_type,slot_id,status,last_error,created_at,updated_at)::text"

func managedMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("MIGRATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIGRATION_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	return db
}

type tableState struct {
	Rows int64  `json:"rows"`
	Hash string `json:"hash"`
}

func publicTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		out = append(out, name)
	}
	require.NoError(t, rows.Err())
	return out
}

// fingerprint is the row count plus an order-independent hash of every row.
func fingerprint(t *testing.T, db *sql.DB, table string) tableState {
	t.Helper()
	expression := "t::text"
	if table == "openai_vm_bindings" {
		expression = bindingLegacyColumns
	}
	var state tableState
	query := fmt.Sprintf(`SELECT count(*), COALESCE(md5(string_agg(h, '' ORDER BY h)), '') FROM (SELECT md5(%s) AS h FROM public.%q t) s`, expression, table)
	require.NoErrorf(t, db.QueryRow(query).Scan(&state.Rows, &state.Hash), "fingerprint %s", table)
	return state
}

func readLedger(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	ledger := map[string]string{}
	var exists bool
	require.NoError(t, db.QueryRow(`SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists))
	if !exists {
		return ledger
	}
	rows, err := db.Query(`SELECT filename, checksum FROM schema_migrations`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, sum string
		require.NoError(t, rows.Scan(&name, &sum))
		ledger[name] = sum
	}
	require.NoError(t, rows.Err())
	return ledger
}

func TestOpenAIVMManagedMigrationKeepsLedgerAndData(t *testing.T) {
	db := managedMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	ledgerBefore := readLedger(t, db)
	tablesBefore := map[string]tableState{}
	for _, table := range publicTables(t, db) {
		tablesBefore[table] = fingerprint(t, db, table)
	}
	_, alreadyApplied := ledgerBefore[managedMigrationFile]

	require.NoError(t, repository.ApplyMigrations(ctx, db), "the migration chain must apply cleanly")

	// 1. Nothing that was already applied changed, and its checksum still
	// matches the file shipped with this build (ApplyMigrations refuses to run
	// otherwise, so reaching this line already proves the second half).
	ledgerAfter := readLedger(t, db)
	for name, sum := range ledgerBefore {
		require.Equalf(t, sum, ledgerAfter[name], "ledger row for %s changed", name)
	}
	// 2. Exactly the files this build ships and the database had not seen were added.
	files, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	var expected, added []string
	for _, name := range files {
		if _, seen := ledgerBefore[name]; !seen {
			expected = append(expected, name)
		}
	}
	for name := range ledgerAfter {
		if _, seen := ledgerBefore[name]; !seen {
			added = append(added, name)
		}
	}
	sort.Strings(expected)
	sort.Strings(added)
	require.Equal(t, expected, added)
	require.Contains(t, ledgerAfter, managedMigrationFile)

	// 3. No existing row of any existing table changed. openai_vm_bindings is
	// compared on the columns it had before; the ledger is covered above.
	tablesAfter := publicTables(t, db)
	var changed []string
	for table, before := range tablesBefore {
		if table == "schema_migrations" {
			continue
		}
		if after := fingerprint(t, db, table); after != before {
			changed = append(changed, fmt.Sprintf("%s: %+v -> %+v", table, before, after))
		}
	}
	sort.Strings(changed)
	require.Empty(t, changed, "applying migrations changed existing data")
	var addedTables []string
	for _, table := range tablesAfter {
		if _, existed := tablesBefore[table]; !existed {
			addedTables = append(addedTables, table)
		}
	}

	// 4. The new schema is there, and existing bindings read as token-fed slots.
	var bindings, unexpected int
	require.NoError(t, db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE mode<>'sub_token' OR managed_status<>'' OR managed_detail<>'{}'::jsonb OR activated_at IS NOT NULL OR synced_at IS NOT NULL) FROM openai_vm_bindings`).Scan(&bindings, &unexpected))
	var batchJobs, batchItems int
	require.NoError(t, db.QueryRow(`SELECT (SELECT count(*) FROM openai_vm_batch_jobs), (SELECT count(*) FROM openai_vm_batch_items)`).Scan(&batchJobs, &batchItems))
	if !alreadyApplied {
		require.Zero(t, unexpected, "an existing binding did not default to sub_token")
		require.Zero(t, batchJobs+batchItems)
	}
	var modeChecks int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM pg_constraint WHERE conrelid='public.openai_vm_bindings'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%vm_managed%'`).Scan(&modeChecks))
	require.Equal(t, 1, modeChecks, "openai_vm_bindings.mode must be constrained")
	var guardKnowsManaged bool
	require.NoError(t, db.QueryRow(`SELECT pg_get_functiondef('public.guard_openai_vm_account()'::regprocedure) LIKE '%openai_vm_managed%'`).Scan(&guardKnowsManaged))
	require.True(t, guardKnowsManaged, "the account guard was not replaced")
	var guardTriggers int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM pg_trigger WHERE tgrelid='public.accounts'::regclass AND tgname='openai_vm_account_guard' AND NOT tgisinternal`).Scan(&guardTriggers))
	require.Equal(t, 1, guardTriggers, "the guard trigger must exist exactly once")

	// 5. Applying again is a no-op.
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	require.Equal(t, ledgerAfter, readLedger(t, db))

	summary, _ := json.Marshal(map[string]any{
		"ledger_before": len(ledgerBefore), "ledger_after": len(ledgerAfter), "added": added,
		"tables_before": len(tablesBefore), "tables_after": len(tablesAfter), "tables_added": addedTables,
		"tables_compared": len(tablesBefore), "tables_changed": len(changed), "existing_bindings": bindings,
		"accounts_rows": tablesBefore["accounts"].Rows, "already_applied": alreadyApplied,
	})
	t.Logf("MIGCHECK %s", summary)
}

type managedDBFixture struct {
	t        *testing.T
	db       *sql.DB
	repo     service.OpenAIVMRepository
	managed  service.OpenAIVMManagedRepository
	accounts []int64
	proxies  []int64
}

func newManagedDBFixture(t *testing.T) *managedDBFixture {
	db := managedMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	repo := repository.NewOpenAIVMRepository(db)
	managed, ok := repo.(service.OpenAIVMManagedRepository)
	require.True(t, ok, "the binding repository must also store the managed columns")
	f := &managedDBFixture{t: t, db: db, repo: repo, managed: managed}
	t.Cleanup(f.cleanup)
	return f
}

func (f *managedDBFixture) cleanup() {
	for _, id := range f.accounts {
		for _, statement := range []string{
			`DELETE FROM openai_vm_bindings WHERE account_id=$1`,
			`DELETE FROM account_proxies WHERE account_id IN(SELECT id FROM accounts WHERE id=$1 OR parent_account_id=$1)`,
			`DELETE FROM scheduler_outbox WHERE account_id IN(SELECT id FROM accounts WHERE id=$1 OR parent_account_id=$1)`,
			`DELETE FROM accounts WHERE parent_account_id=$1`,
			`DELETE FROM accounts WHERE id=$1`,
		} {
			_, _ = f.db.Exec(statement, id)
		}
		_, _ = f.db.Exec(`DELETE FROM proxies WHERE name=$1`, fmt.Sprintf("OpenAI VM account %d", id))
	}
	for _, id := range f.proxies {
		_, _ = f.db.Exec(`DELETE FROM proxies WHERE id=$1`, id)
	}
}

func (f *managedDBFixture) proxy(protocol string) int64 {
	f.t.Helper()
	var id int64
	require.NoError(f.t, f.db.QueryRow(`INSERT INTO proxies(name,protocol,host,port,status,fallback_mode) VALUES($1,$2,'127.0.0.1',1080,'active','none') RETURNING id`, fmt.Sprintf("vm-managed-it-%d-%d", time.Now().UnixNano(), len(f.proxies)), protocol).Scan(&id))
	f.proxies = append(f.proxies, id)
	return id
}

func (f *managedDBFixture) account(marked bool, proxyID *int64) int64 {
	f.t.Helper()
	extra := `{"preserved":true}`
	credentials := `{"access_token":"synthetic-test-token"}`
	if marked {
		extra, credentials = `{"preserved":true,"openai_vm_managed":true}`, `{"email":"managed@example.test"}`
	}
	var id int64
	name := fmt.Sprintf("vm-managed-it-%d-%d", time.Now().UnixNano(), len(f.accounts))
	require.NoError(f.t, f.db.QueryRow(`INSERT INTO accounts(name,platform,type,credentials,extra,proxy_id) VALUES($4,'openai','oauth',$1::jsonb,$2::jsonb,$3) RETURNING id`, credentials, extra, proxyID, name).Scan(&id))
	f.accounts = append(f.accounts, id)
	return id
}

func (f *managedDBFixture) shadow(parent int64) error {
	_, err := f.db.Exec(`INSERT INTO accounts(name,platform,type,credentials,extra,parent_account_id,quota_dimension) VALUES('vm-managed-it-shadow','openai','oauth','{}','{}',$1,'spark')`, parent)
	return err
}

func (f *managedDBFixture) reserve(id int64, managed bool) (*service.OpenAIVMBinding, error) {
	var binding *service.OpenAIVMBinding
	err := f.repo.WithLock(context.Background(), id, func() error {
		bridge := &service.Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8081, Username: fmt.Sprint(id), Password: "synthetic-proxy-auth"}
		var err error
		if managed {
			binding, err = f.managed.ReserveManaged(context.Background(), id, "kvm", bridge)
		} else {
			binding, err = f.repo.Reserve(context.Background(), id, "docker", bridge)
		}
		return err
	})
	return binding, err
}

func (f *managedDBFixture) accountRow(id int64) (proxyID *int64, schedulable, bound, marked bool, name string) {
	f.t.Helper()
	require.NoError(f.t, f.db.QueryRow(`SELECT proxy_id,schedulable,COALESCE(extra->>'openai_vm_bound','')='true',COALESCE(extra->>'openai_vm_managed','')='true',name FROM accounts WHERE id=$1`, id).Scan(&proxyID, &schedulable, &bound, &marked, &name))
	return
}

func TestOpenAIVMManagedRepositoryLifecycle(t *testing.T) {
	f := newManagedDBFixture(t)
	ctx := context.Background()
	exit, other := f.proxy("socks5"), f.proxy("socks5h")
	id := f.account(true, &exit)

	b, err := f.reserve(id, true)
	require.NoError(t, err)
	require.Equal(t, service.OpenAIVMModeManaged, b.Mode)
	require.True(t, b.Managed())
	require.Equal(t, "provisioning", b.Status)
	require.Equal(t, "kvm", b.RuntimeType)
	require.Equal(t, exit, *b.OriginalProxyID)
	require.Empty(t, b.ManagedStatus)
	require.Nil(t, b.ManagedDetail)
	proxyID, _, bound, marked, _ := f.accountRow(id)
	require.Equal(t, b.ManagedProxyID, *proxyID, "the account now routes through the bridge")
	require.True(t, bound)
	require.True(t, marked)

	require.NoError(t, f.repo.SetState(ctx, id, "awaiting_login", "s2a-abcdefabcdef-1", ""))
	require.NoError(t, f.managed.SetManaged(ctx, id, "awaiting_login", nil))
	require.NoError(t, f.managed.SetManaged(ctx, id, "available", json.RawMessage(`{"plan_type":"plus","has_token":true}`)))
	// A reading without a snapshot keeps the previous one.
	require.NoError(t, f.managed.SetManaged(ctx, id, "restricted", nil))
	b, err = f.repo.GetBinding(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "restricted", b.ManagedStatus)
	require.JSONEq(t, `{"plan_type":"plus","has_token":true}`, string(b.ManagedDetail))
	require.NotNil(t, b.SyncedAt)
	require.Nil(t, b.ActivatedAt)
	require.Equal(t, "s2a-abcdefabcdef-1", b.SlotID)

	first, err := f.managed.MarkActivated(ctx, id)
	require.NoError(t, err)
	require.True(t, first)
	again, err := f.managed.MarkActivated(ctx, id)
	require.NoError(t, err)
	require.False(t, again, "activation is reported exactly once")

	_, err = f.db.Exec(`UPDATE openai_vm_bindings SET mode='something_else' WHERE account_id=$1`, id)
	require.Error(t, err, "an unknown mode must be rejected by the database")
	require.NoError(t, f.managed.SetOriginalProxy(ctx, id, other))
	b, err = f.repo.GetBinding(ctx, id)
	require.NoError(t, err)
	require.Equal(t, other, *b.OriginalProxyID)
	require.NotNil(t, b.ActivatedAt)

	// A token-fed slot next to it is untouched by every managed operation.
	plainExit := f.proxy("socks5")
	plain := f.account(false, &plainExit)
	pb, err := f.reserve(plain, false)
	require.NoError(t, err)
	require.Equal(t, service.OpenAIVMModeSubToken, pb.Mode)
	require.False(t, pb.Managed())
	require.NoError(t, f.managed.SetManaged(ctx, plain, "available", json.RawMessage(`{"x":1}`)))
	require.NoError(t, f.managed.SetOriginalProxy(ctx, plain, other))
	activated, err := f.managed.MarkActivated(ctx, plain)
	require.NoError(t, err)
	require.False(t, activated)
	pb, err = f.repo.GetBinding(ctx, plain)
	require.NoError(t, err)
	require.Empty(t, pb.ManagedStatus)
	require.Nil(t, pb.ManagedDetail)
	require.Equal(t, plainExit, *pb.OriginalProxyID)
	listed, err := f.managed.ListManaged(ctx)
	require.NoError(t, err)
	var ids []int64
	for _, item := range listed {
		ids = append(ids, item.AccountID)
	}
	require.Contains(t, ids, id)
	require.NotContains(t, ids, plain)

	// Release: the slot is gone, the account keeps its mark and stops serving.
	require.NoError(t, f.managed.ReleaseManaged(ctx, plain), "releasing a token-fed slot is a no-op")
	still, err := f.repo.GetBinding(ctx, plain)
	require.NoError(t, err)
	require.NotNil(t, still)
	require.NoError(t, f.managed.ReleaseManaged(ctx, id))
	require.NoError(t, f.managed.ReleaseManaged(ctx, id), "release is idempotent")
	gone, err := f.repo.GetBinding(ctx, id)
	require.NoError(t, err)
	require.Nil(t, gone)
	proxyID, schedulable, bound, marked, _ := f.accountRow(id)
	require.Equal(t, other, *proxyID)
	require.False(t, schedulable, "an account with neither slot nor token must not be scheduled")
	require.False(t, bound)
	require.True(t, marked, "the mark keeps it out of every token path until it is deleted")
	var removed bool
	require.NoError(t, f.db.QueryRow(`SELECT deleted_at IS NOT NULL AND status='disabled' FROM proxies WHERE id=$1`, b.ManagedProxyID).Scan(&removed))
	require.True(t, removed)
	_, err = f.db.Exec(`UPDATE accounts SET deleted_at=NOW() WHERE id=$1`, id)
	require.NoError(t, err, "a released account can be deleted")

	// The token-fed slot still detaches the old way.
	require.NoError(t, f.repo.Restore(ctx, plain))
	proxyID, _, bound, _, _ = f.accountRow(plain)
	require.Equal(t, plainExit, *proxyID)
	require.False(t, bound)
	var token string
	require.NoError(t, f.db.QueryRow(`SELECT credentials->>'access_token' FROM accounts WHERE id=$1`, plain).Scan(&token))
	require.Equal(t, "synthetic-test-token", token)
}

func TestOpenAIVMManagedReserveChecksCredentialOwnership(t *testing.T) {
	f := newManagedDBFixture(t)
	exit := f.proxy("socks5")

	plain := f.account(false, &exit)
	_, err := f.reserve(plain, true)
	require.Error(t, err, "an account that holds its own token cannot get a managed slot")
	marked := f.account(true, &exit)
	_, err = f.reserve(marked, false)
	require.Error(t, err, "a managed account cannot get a token-fed slot")
	direct := f.account(true, nil)
	_, err = f.reserve(direct, true)
	require.Error(t, err, "a managed slot needs an exit")
	withShadow := f.account(true, &exit)
	require.NoError(t, f.shadow(withShadow))
	_, err = f.reserve(withShadow, true)
	require.Error(t, err, "a managed account has no token for a shadow to borrow")
	for _, id := range []int64{plain, marked, direct, withShadow} {
		b, getErr := f.repo.GetBinding(context.Background(), id)
		require.NoError(t, getErr)
		require.Nil(t, b, "a refused reservation must leave nothing behind")
		_, _, bound, _, _ := f.accountRow(id)
		require.False(t, bound)
	}
}

func TestOpenAIVMManagedAccountGuard(t *testing.T) {
	f := newManagedDBFixture(t)
	exit := f.proxy("socks5")
	managed := f.account(true, &exit)
	_, err := f.reserve(managed, true)
	require.NoError(t, err)
	plain := f.account(false, &exit)
	_, err = f.reserve(plain, false)
	require.NoError(t, err)

	for name, statement := range map[string]string{
		"drop the mark":        `UPDATE accounts SET extra=extra-'openai_vm_managed' WHERE id=$1`,
		"switch the mark off":  `UPDATE accounts SET extra=extra||'{"openai_vm_managed":false}'::jsonb WHERE id=$1`,
		"replace extra":        `UPDATE accounts SET extra='{"openai_vm_bound":true}'::jsonb WHERE id=$1`,
		"change the route":     `UPDATE accounts SET proxy_id=NULL WHERE id=$1`,
		"change the type":      `UPDATE accounts SET type='apikey' WHERE id=$1`,
		"delete while bound":   `UPDATE accounts SET deleted_at=NOW() WHERE id=$1`,
		"drop the bound mark":  `UPDATE accounts SET extra=extra-'openai_vm_bound' WHERE id=$1`,
		"set a fallback route": `UPDATE accounts SET proxy_fallback_origin_id=proxy_id WHERE id=$1`,
	} {
		_, err = f.db.Exec(statement, managed)
		require.Errorf(t, err, "managed account: %s", name)
	}
	// Ordinary edits, including what the state sync writes, still work.
	_, err = f.db.Exec(`UPDATE accounts SET name='renamed',schedulable=FALSE,concurrency=5,credentials=credentials||'{"plan_type":"plus"}'::jsonb,extra=extra||'{"openai_vm_managed_status":"available","codex_5h_used_percent":12.5}'::jsonb WHERE id=$1`, managed)
	require.NoError(t, err)
	_, _, bound, marked, name := f.accountRow(managed)
	require.True(t, bound)
	require.True(t, marked)
	require.Equal(t, "renamed", name)
	require.Error(t, f.shadow(managed), "a managed account cannot get a shadow")

	// A token-fed slot cannot be re-labelled as managed, and keeps its old rules.
	_, err = f.db.Exec(`UPDATE accounts SET extra=extra||'{"openai_vm_managed":true}'::jsonb WHERE id=$1`, plain)
	require.Error(t, err)
	_, err = f.db.Exec(`UPDATE accounts SET extra=extra-'openai_vm_bound' WHERE id=$1`, plain)
	require.Error(t, err)
	_, err = f.db.Exec(`UPDATE accounts SET name='renamed-plain',credentials=credentials||'{"access_token":"rotated"}'::jsonb WHERE id=$1`, plain)
	require.NoError(t, err)
	require.NoError(t, f.shadow(plain), "token-fed slots keep supporting shadows")
	var shadowProxy *int64
	var shadowBound, shadowMarked bool
	require.NoError(t, f.db.QueryRow(`SELECT proxy_id,COALESCE(extra->>'openai_vm_bound','')='true',COALESCE(extra->>'openai_vm_managed','')='true' FROM accounts WHERE parent_account_id=$1`, plain).Scan(&shadowProxy, &shadowBound, &shadowMarked))
	plainProxy, _, _, _, _ := f.accountRow(plain)
	require.Equal(t, *plainProxy, *shadowProxy, "the shadow inherits the bridge route")
	require.True(t, shadowBound)
	require.False(t, shadowMarked)

	// An account without a slot is not constrained by the guard at all.
	free := f.account(false, &exit)
	_, err = f.db.Exec(`UPDATE accounts SET proxy_id=NULL,extra='{}'::jsonb WHERE id=$1`, free)
	require.NoError(t, err)
}
