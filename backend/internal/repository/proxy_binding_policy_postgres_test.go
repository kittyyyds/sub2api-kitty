package repository

import (
	"context"
	"database/sql"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"testing"
	"time"
)

func TestProxyBindingAutomationPostgres(t *testing.T) {
	dsn := os.Getenv("KITTY_ACCOUNT_AUTOMATION_TEST_DSN")
	if dsn == "" {
		t.Skip("dedicated fixture required")
	}
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	defer db.Close()
	var name string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
	require.Equal(t, "kitty_account_automation_fixture", name)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, ApplyMigrations(ctx, db))
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	r := newAccountRepositoryWithSQL(client, db, nil)
	policy := service.ProxyBindingPolicy{Enabled: true, OnlyTwoFA: true, Platforms: []string{"openai"}, HistoryLimit: 2, AutoAssign: true}
	require.NoError(t, r.SaveProxyBindingPolicy(ctx, policy))
	defer r.SaveProxyBindingPolicy(context.Background(), service.ProxyBindingPolicy{OnlyTwoFA: true, Platforms: []string{"openai"}, HistoryLimit: 5})
	suffix := fmt.Sprint(time.Now().UnixNano())
	proxy := func(ip string) int64 {
		var id int64
		var revision time.Time
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO proxies(name,protocol,host,port,status) VALUES($1,'http','fixture.invalid',8080,'active') RETURNING id,updated_at`, suffix).Scan(&id, &revision))
		require.NoError(t, r.RememberProxyBindingExit(ctx, id, ip, revision))
		return id
	}
	p1 := proxy("192.0.2.1")
	p2 := proxy("192.0.2.1")
	tickets := make(chan *service.AccountImportTicket, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			t, e := r.BeginAccountImport(ctx, "openai", "twofa", nil, fmt.Sprintf("a%d-%s@test.invalid", i, suffix))
			if e != nil {
				errs <- e
			} else {
				tickets <- t
			}
		}(i)
	}
	wg.Wait()
	close(tickets)
	close(errs)
	require.Len(t, tickets, 1, "same real exit cannot be reserved twice")
	require.Len(t, errs, 7)
	reserved := <-tickets
	require.True(t, reserved.AutoAssigned)
	require.NoError(t, r.ReleaseAccountImport(ctx, reserved.ID))
	add := func(email string, pid *int64) *service.Account {
		receipt, e := r.BeginAccountImport(ctx, "openai", "twofa", pid, email)
		require.NoError(t, e)
		credentials := map[string]any{"access_token": "fixture-token-" + email, "email": email}
		require.NoError(t, r.ReadyAccountImport(ctx, receipt.ID, service.ImportCredentialFingerprint(credentials)))
		a := &service.Account{Name: email, Platform: "openai", Type: "oauth", Status: "active", Concurrency: 1, Schedulable: true, Credentials: credentials, ImportTicket: receipt.ID, ProxyID: receipt.ProxyID}
		require.NoError(t, r.Create(ctx, a))
		return a
	}
	first := add("first-"+suffix+"@test.invalid", nil)
	_, e = r.BeginAccountImport(ctx, "openai", "twofa", nil, "busy-"+suffix)
	require.Error(t, e, "occupied exit must be skipped")
	_, e = db.ExecContext(ctx, `UPDATE accounts SET proxy_id=NULL WHERE id=$1`, first.ID)
	require.NoError(t, e)
	second := add("second-"+suffix+"@test.invalid", nil)
	_, e = db.ExecContext(ctx, `UPDATE accounts SET proxy_id=NULL WHERE id=$1`, second.ID)
	require.NoError(t, e)
	_, e = r.BeginAccountImport(ctx, "openai", "twofa", nil, "full-"+suffix)
	require.Error(t, e)
	summaries, e := r.ProxyBindingSummaries(ctx)
	require.NoError(t, e)
	for _, s := range summaries {
		if s.ProxyID == p1 || s.ProxyID == p2 {
			require.EqualValues(t, 2, s.History)
			require.Zero(t, s.Current)
		}
	}
	require.NoError(t, r.AllowOneProxyBinding(ctx, p1))
	require.NoError(t, r.AllowOneProxyBinding(ctx, p2))
	_, e = r.BeginAccountImport(ctx, "openai", "twofa", nil, "auto-no-extra-"+suffix)
	require.Error(t, e, "automatic allocation never consumes the exception")
	policy.AutoAssign = false
	require.NoError(t, r.SaveProxyBindingPolicy(ctx, policy))
	third := add("third-"+suffix+"@test.invalid", &p2)
	_, e = db.ExecContext(ctx, `UPDATE accounts SET proxy_id=NULL WHERE id=$1`, third.ID)
	require.NoError(t, e)
	_, e = r.BeginAccountImport(ctx, "openai", "twofa", &p1, "fourth-"+suffix)
	require.Error(t, e, "repeated confirmation must not stack allowances")
	// Rebinding a previously counted identity is not a new historical account.
	_, e = db.ExecContext(ctx, `UPDATE accounts SET proxy_id=$2 WHERE id=$1`, first.ID, p1)
	require.NoError(t, e)
	// A nonparticipating API key may use this proxy but still blocks automatic selection.
	api := &service.Account{Name: "api-" + suffix, Platform: "openai", Type: "apikey", Status: "active", Concurrency: 1, Schedulable: true, Credentials: map[string]any{"api_key": "fixture-only"}, ProxyID: &p2}
	require.NoError(t, r.Create(ctx, api))
	summaries, e = r.ProxyBindingSummaries(ctx)
	require.NoError(t, e)
	for _, s := range summaries {
		if s.ProxyID == p1 || s.ProxyID == p2 {
			require.EqualValues(t, 3, s.History)
			require.EqualValues(t, 2, s.Current)
			require.False(t, s.ExtraAllowed)
		}
	}
	// Removing an account never deletes historical bindings.
	_, e = db.ExecContext(ctx, `UPDATE accounts SET deleted_at=NOW() WHERE id IN ($1,$2,$3,$4)`, first.ID, second.ID, third.ID, api.ID)
	require.NoError(t, e)
}
