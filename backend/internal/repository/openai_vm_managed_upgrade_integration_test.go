//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// A deployment that already has token-fed dedicated slots must come through
// the managed migration unchanged: the slot stays sub_token, the account keeps
// its token, and it can still be detached. This builds such a database at the
// previous schema version, in a scratch database of its own, then upgrades it.
func TestOpenAIVMManagedMigrationUpgradesExistingBindings(t *testing.T) {
	const managedMigration = "353_openai_vm_managed.sql"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	name := fmt.Sprintf("mig353_%d", time.Now().UnixNano())
	_, err := integrationDB.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	dsn, err := url.Parse(integrationDatabaseDSN)
	require.NoError(t, err)
	dsn.Path = "/" + name
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = integrationDB.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
	})

	// Everything this build ships except the managed migration.
	previous := fstest.MapFS{}
	files, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, file := range files {
		if file == managedMigration {
			continue
		}
		data, readErr := fs.ReadFile(migrations.FS, file)
		require.NoError(t, readErr)
		previous[file] = &fstest.MapFile{Data: data}
	}
	require.Len(t, previous, len(files)-1)
	require.NoError(t, applyMigrationsFS(ctx, db, previous))

	// A bound account exactly as the previous version's Reserve left it.
	var original, bridge, parent, shadow int64
	require.NoError(t, db.QueryRow(`INSERT INTO proxies(name,protocol,host,port,status,fallback_mode) VALUES('exit','socks5','127.0.0.1',1080,'active','none') RETURNING id`).Scan(&original))
	require.NoError(t, db.QueryRow(`INSERT INTO accounts(name,platform,type,credentials,extra,proxy_id) VALUES('bound','openai','oauth','{"access_token":"at-before","refresh_token":"rt-before"}','{"kept":true}',$1) RETURNING id`, original).Scan(&parent))
	require.NoError(t, db.QueryRow(`INSERT INTO accounts(name,platform,type,credentials,extra,parent_account_id,proxy_id,quota_dimension) VALUES('bound-shadow','openai','oauth','{}','{}',$1,$2,'spark') RETURNING id`, parent, original).Scan(&shadow))
	require.NoError(t, db.QueryRow(`INSERT INTO proxies(name,protocol,host,port,username,password,status,fallback_mode) VALUES('bridge','http','127.0.0.1',8081,$1,'synthetic','active','none') RETURNING id`, fmt.Sprint(parent)).Scan(&bridge))
	_, err = db.Exec(`INSERT INTO openai_vm_bindings(account_id,managed_proxy_id,original_proxy_id,runtime_type,slot_id,status) VALUES($1,$2,$3,'docker','s2a-abcdefabcdef-1','ready')`, parent, bridge, original)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE accounts SET proxy_id=$2,extra=extra||'{"openai_vm_bound":true}'::jsonb WHERE id=$1 OR parent_account_id=$1`, parent, bridge)
	require.NoError(t, err)
	const accountState = `SELECT md5(string_agg(a::text, '|' ORDER BY id)) FROM accounts a`
	const legacyBinding = `SELECT ROW(account_id,managed_proxy_id,original_proxy_id,original_proxy_ids,original_fallback_id,runtime_type,slot_id,status,last_error,created_at,updated_at)::text FROM openai_vm_bindings WHERE account_id=$1`
	var accountsBefore, bindingBefore string
	require.NoError(t, db.QueryRow(accountState).Scan(&accountsBefore))
	require.NoError(t, db.QueryRow(legacyBinding, parent).Scan(&bindingBefore))

	require.NoError(t, ApplyMigrations(ctx, db), "upgrade with a live binding")

	var ledger int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&ledger))
	require.Equal(t, len(files), ledger)
	var accountsAfter, bindingAfter string
	require.NoError(t, db.QueryRow(accountState).Scan(&accountsAfter))
	require.NoError(t, db.QueryRow(legacyBinding, parent).Scan(&bindingAfter))
	require.Equal(t, accountsBefore, accountsAfter, "the upgrade must not touch account rows")
	require.Equal(t, bindingBefore, bindingAfter)

	repo := &openAIVMRepository{db: db}
	b, err := repo.GetBinding(ctx, parent)
	require.NoError(t, err)
	require.Equal(t, service.OpenAIVMModeSubToken, b.Mode)
	require.False(t, b.Managed())
	require.Equal(t, "ready", b.Status)
	require.Empty(t, b.ManagedStatus)
	require.Nil(t, b.ManagedDetail)
	require.Nil(t, b.ActivatedAt)
	listed, err := repo.ListManaged(ctx)
	require.NoError(t, err)
	require.Empty(t, listed, "an upgraded slot is not picked up by the managed state sync")

	// The replaced guard still accepts what it accepted and refuses what it refused.
	_, err = db.Exec(`UPDATE accounts SET credentials=credentials||'{"access_token":"at-rotated"}'::jsonb,name='renamed' WHERE id=$1`, parent)
	require.NoError(t, err, "token refresh on an upgraded account")
	_, err = db.Exec(`UPDATE accounts SET name='renamed-shadow' WHERE id=$1`, shadow)
	require.NoError(t, err)
	for _, statement := range []string{
		`UPDATE accounts SET proxy_id=NULL WHERE id=$1`,
		`UPDATE accounts SET extra=extra-'openai_vm_bound' WHERE id=$1`,
		`UPDATE accounts SET extra=extra||'{"openai_vm_managed":true}'::jsonb WHERE id=$1`,
		`UPDATE accounts SET deleted_at=NOW() WHERE id=$1`,
	} {
		_, err = db.Exec(statement, parent)
		require.Errorf(t, err, statement)
	}

	// And it detaches the way it always did.
	require.NoError(t, repo.Restore(ctx, parent))
	var proxyID int64
	var token string
	var bound, kept bool
	for _, id := range []int64{parent, shadow} {
		require.NoError(t, db.QueryRow(`SELECT proxy_id,COALESCE(extra->>'openai_vm_bound','')='true' FROM accounts WHERE id=$1`, id).Scan(&proxyID, &bound))
		require.Equal(t, original, proxyID)
		require.False(t, bound)
	}
	require.NoError(t, db.QueryRow(`SELECT credentials->>'access_token',(extra->>'kept')::boolean FROM accounts WHERE id=$1`, parent).Scan(&token, &kept))
	require.Equal(t, "at-rotated", token)
	require.True(t, kept)
}
