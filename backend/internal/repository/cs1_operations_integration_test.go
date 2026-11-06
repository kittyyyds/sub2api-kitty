//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCS1ProviderAtomicWalletCASAndImport(t *testing.T) {
	billing, cmd, _, client := durableBillingFixture(t)
	r := &channelRepository{db: billing.db}
	ctx := context.Background()
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
	input := json.RawMessage(`{"name":"Upstream A","site_type":"sub2api","base_url":"https://upstream.example","auth_mode":"token","secret_encrypted":"test-cipher-only","metrics":{"balance":120,"currency":"USD"}}`)
	raw, e := r.SaveCS1Provider(ctx, id, 0, input)
	require.NoError(t, e)
	var state struct {
		WalletID   int64 `json:"wallet_id"`
		SupplierID int64 `json:"supplier_id"`
		Revision   int64 `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(raw, &state))
	require.EqualValues(t, 1, state.Revision)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := r.SaveCS1Provider(ctx, id, 1, raw); errs <- e }()
	}
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, service.ErrProviderHubConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflicts)
	var wallets, suppliers int
	require.NoError(t, billing.db.QueryRow(`SELECT (SELECT count(*) FROM kitty_supplier_wallets),(SELECT count(*) FROM kitty_suppliers)`).Scan(&wallets, &suppliers))
	require.Equal(t, 1, wallets)
	require.Equal(t, 1, suppliers)
	account := &service.Account{Name: "Imported through provider", Platform: "openai", Type: "apikey", Status: "active", Schedulable: true, Concurrency: 1, Credentials: map[string]any{"base_url": "https://upstream.example", "api_key": "synthetic-key"}, Extra: map[string]any{"provider_binding": map[string]any{"provider_id": id}}}
	accounts := newAccountRepositoryWithSQL(client, billing.db, nil)
	require.NoError(t, accounts.Create(ctx, account))
	require.NotZero(t, account.ID)
	var wallet int64
	require.NoError(t, billing.db.QueryRow(`SELECT wallet_id FROM kitty_supplier_bindings WHERE account_id=$1 AND ended_at IS NULL`, account.ID).Scan(&wallet))
	require.Equal(t, state.WalletID, wallet)
	public, e := r.ReadProviderHub(ctx)
	require.NoError(t, e)
	require.NotContains(t, string(public), "test-cipher-only")
	require.NotContains(t, string(public), "synthetic-key")
	require.NoError(t, r.DeleteCS1Provider(ctx, id, 2))
	visible, e := r.ReadCS1Providers(ctx)
	require.NoError(t, e)
	require.JSONEq(t, "[]", string(visible))
	var secretExists bool
	require.NoError(t, billing.db.QueryRow(`SELECT value::jsonb ? 'secret_encrypted' FROM settings WHERE key=$1`, cs1ProviderPrefix+id).Scan(&secretExists))
	require.False(t, secretExists)
	var before, after int
	require.NoError(t, billing.db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&before))
	account.ID = 0
	account.Name = "must rollback"
	require.ErrorIs(t, accounts.Create(ctx, account), service.ErrProviderHubConflict)
	require.NoError(t, billing.db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&after))
	require.Equal(t, before, after)
	require.NoError(t, billing.db.QueryRow(`SELECT count(*) FROM kitty_supplier_bindings WHERE wallet_id=$1`, wallet).Scan(&after))
	require.Equal(t, 1, after)
	var balance float64
	require.NoError(t, billing.db.QueryRow(`SELECT balance FROM users WHERE id=$1`, cmd.UserID).Scan(&balance))
	require.Equal(t, 100.0, balance)
}

func TestCS1OperationsCostsCoverageGroupingAndRetention(t *testing.T) {
	billing, cmd, _, client := durableBillingFixture(t)
	db := billing.db
	ctx := context.Background()
	r := &channelRepository{db: db}
	usage := newUsageLogRepositoryWithSQL(client, db)
	start := time.Now().UTC().Truncate(24 * time.Hour)
	at := start.Add(time.Hour)
	insert := func(request string, total, actual, rate float64) {
		t.Helper()
		_, e := db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,requested_model,total_cost,actual_cost,account_rate_multiplier,created_at) VALUES($1,$2,$3,$4,'upstream-model','customer-model',$5,$6,$7,$8)`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, request, total, actual, rate, at)
		require.NoError(t, e)
	}
	insert("legacy", 5, 3, .2)
	mutate := func(c service.ProviderHubCommand) int64 {
		t.Helper()
		b, e := r.MutateProviderHub(ctx, c)
		require.NoError(t, e)
		var x struct{ ID int64 }
		require.NoError(t, json.Unmarshal(b, &x))
		return x.ID
	}
	s := mutate(service.ProviderHubCommand{Action: "supplier", Name: "Source", SiteType: "manual"})
	w := mutate(service.ProviderHubCommand{Action: "wallet", SupplierID: s, Name: "Wallet", Identity: "1", Currency: "USD", CreditCurrency: "USD"})
	mutate(service.ProviderHubCommand{Action: "bindings", ID: w, Revision: 1, AccountIDs: []int64{cmd.AccountID}})
	mutate(service.ProviderHubCommand{Action: "rule", ID: w, Revision: 2, Model: "*", Method: "standard_multiplier", Multiplier: .3, Currency: "USD"})
	// The business timestamp is after the rule starts, so it is eligible.
	at = time.Now().UTC()
	insert("frozen", 10, 5, 1)
	mutate(service.ProviderHubCommand{Action: "adjustment", APIKeyID: cmd.APIKeyID, RequestKey: "request:frozen", OperationID: uuid.NewString(), Amount: 2, Currency: "USD", Reference: "invoice-1"})
	mutate(service.ProviderHubCommand{Action: "rule", ID: w, Revision: 3, Model: "*", Method: "standard_multiplier", Multiplier: .5, Currency: "CNY"})
	at = time.Now().UTC()
	insert("unknown-fx", 4, 4, 1)
	end := start.Add(24 * time.Hour)
	get := func() *service.OperationsDashboardData {
		t.Helper()
		d, e := usage.GetOperationsDashboardData(ctx, start, end, start, end, start, end, "UTC", 0)
		require.NoError(t, e)
		return d
	}
	d := get()
	require.EqualValues(t, 3, d.CostCoverage.Requests)
	require.EqualValues(t, 1, d.CostCoverage.Reconciled)
	require.EqualValues(t, 1, d.CostCoverage.AccountEstimated)
	require.EqualValues(t, 1, d.CostCoverage.Unknown)
	require.InDelta(t, 12, d.Metrics.TodayUserConsumption, 1e-9)
	require.Nil(t, d.Metrics.TodayRevenue["USD"])
	require.EqualValues(t, 1, d.Metrics.TodayCostUnknown)
	require.Len(t, d.Models, 1)
	require.Equal(t, "customer-model", d.Models[0].Model)
	require.InDelta(t, 3, d.Models[0].UpstreamCost, 1e-9)
	require.EqualValues(t, 1, d.Models[0].UnknownCost)
	require.Len(t, d.Groups, 2)
	require.EqualValues(t, 0, d.Groups[0].GroupID)
	require.Equal(t, d.Models[0].UpstreamCost, d.Groups[0].UpstreamCost)
	users, e := usage.GetOperationsUsersData(ctx, start, end, "", cmd.UserID)
	require.NoError(t, e)
	require.Len(t, users.Users, 1)
	require.EqualValues(t, 1, users.Users[0].UnknownCost)
	require.NotNil(t, users.Detail)
	require.Equal(t, d.Models[0].UserCost, users.Detail.Models[0].UserCost)
	// Bill correction with an explicit conversion resolves the unknown cost.
	mutate(service.ProviderHubCommand{Action: "adjustment", APIKeyID: cmd.APIKeyID, RequestKey: "request:unknown-fx", OperationID: uuid.NewString(), Amount: 2, Currency: "CNY", FXToUSD: func() *float64 { v := .14; return &v }(), Reference: "invoice-2"})
	d = get()
	require.Zero(t, d.CostCoverage.Unknown)
	require.InDelta(t, 8.72, *d.Metrics.TodayRevenue["USD"], 1e-8)
	// A later account price change cannot change historical snapshots.
	_, e = db.Exec(`UPDATE accounts SET rate_multiplier=90 WHERE id=$1`, cmd.AccountID)
	require.NoError(t, e)
	d = get()
	require.InDelta(t, 8.72, *d.Metrics.TodayRevenue["USD"], 1e-8)
	// Removing detailed logs keeps lifetime consumption, but suppresses profit
	// whose cost source has been retained outside this detail query.
	_, e = db.Exec(`DELETE FROM usage_logs WHERE request_id='legacy'`)
	require.NoError(t, e)
	d = get()
	require.InDelta(t, 12, d.Metrics.TotalUserConsumption, 1e-9)
	require.Nil(t, d.Metrics.TodayRevenue["USD"])
	var balance float64
	require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=$1`, cmd.UserID).Scan(&balance))
	require.Equal(t, 100.0, balance)
}

func TestCS1OperationsRechargeOnlyCompletedCreditAndNoPayDuplicate(t *testing.T) {
	billing, cmd, _, client := durableBillingFixture(t)
	db := billing.db
	ctx := context.Background()
	now := time.Now().UTC()
	start := now.Truncate(24 * time.Hour)
	for _, q := range []string{
		`INSERT INTO payment_orders(user_id,amount,pay_amount,status,expires_at,paid_at,completed_at) VALUES($1,100,70,'COMPLETED',NOW()+INTERVAL '1 day',NOW(),NOW())`,
		`INSERT INTO payment_orders(user_id,amount,pay_amount,status,expires_at,paid_at) VALUES($1,200,140,'PAID',NOW()+INTERVAL '1 day',NOW())`,
		`INSERT INTO redeem_codes(code,type,value,status,used_by,used_at) VALUES('PAY-duplicate','balance',100,'used',$1,NOW()),('MANUAL-test','admin_balance',10,'used',$1,NOW())`,
	} {
		_, e := db.Exec(q, cmd.UserID)
		require.NoError(t, e)
	}
	usage := newUsageLogRepositoryWithSQL(client, db)
	d, e := usage.GetOperationsDashboardData(ctx, start, start.Add(24*time.Hour), start, now.Add(time.Minute), start, now.Add(time.Minute), "UTC", 0)
	require.NoError(t, e)
	require.Equal(t, 110.0, d.Metrics.TodayUserRecharge)
	require.Empty(t, d.Models)
	require.Empty(t, d.Trend)
}
