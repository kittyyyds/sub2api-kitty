//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestProviderHubFrozenCostsReplayCorrectionAndReconciliation(t *testing.T) {
	billing, cmd, _, _ := durableBillingFixture(t)
	db := billing.db
	repo := &channelRepository{db: db}
	ctx := context.Background()
	mutate := func(c service.ProviderHubCommand) int64 {
		t.Helper()
		raw, e := repo.MutateProviderHub(ctx, c)
		require.NoError(t, e)
		var result struct {
			ID int64 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(raw, &result))
		return result.ID
	}
	supplier := mutate(service.ProviderHubCommand{Action: "supplier", Name: "Fixture supplier", SiteType: "manual"})
	wallet := mutate(service.ProviderHubCommand{Action: "wallet", SupplierID: supplier, Name: "One shared wallet", Identity: "explicit-user-1", Currency: "USD", CreditCurrency: "USD"})
	mutate(service.ProviderHubCommand{Action: "bindings", ID: wallet, Revision: 1, AccountIDs: []int64{cmd.AccountID}})
	mutate(service.ProviderHubCommand{Action: "rule", ID: wallet, Revision: 2, Model: "*", Method: "standard_multiplier", Multiplier: 0.2, Currency: "USD"})
	insert := func(request string, total float64) {
		t.Helper()
		_, e := db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,input_tokens,output_tokens,total_cost,actual_cost,created_at) VALUES($1,$2,$3,$4,'fixture-model',10,20,$5,3,clock_timestamp())`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, request, total)
		require.NoError(t, e)
	}
	insert(cmd.RequestID, 5)
	cost := func(request string) float64 {
		t.Helper()
		var n float64
		require.NoError(t, db.QueryRow(`SELECT estimated_cost FROM kitty_procurement_receipts WHERE api_key_id=$1 AND request_key=$2`, cmd.APIKeyID, "request:"+request).Scan(&n))
		return n
	}
	require.Equal(t, 1.0, cost(cmd.RequestID))
	mutate(service.ProviderHubCommand{Action: "rule", ID: wallet, Revision: 3, Model: "*", Method: "standard_multiplier", Multiplier: 0.9, Currency: "USD"})
	require.Equal(t, 1.0, cost(cmd.RequestID), "a new price cannot rewrite an old receipt")
	_, e := db.Exec(`UPDATE usage_logs SET total_cost=6 WHERE request_id=$1`, cmd.RequestID)
	require.NoError(t, e)
	require.Equal(t, 1.2, cost(cmd.RequestID), "correction must reuse the old 0.2 rule")
	_, e = db.Exec(`DELETE FROM usage_logs WHERE request_id=$1`, cmd.RequestID)
	require.NoError(t, e)
	insert(cmd.RequestID, 99)
	require.Equal(t, 1.2, cost(cmd.RequestID), "replayed log cannot overwrite the canonical receipt")
	newer := uuid.NewString()
	insert(newer, 5)
	require.Equal(t, 4.5, cost(newer))
	adjustment := service.ProviderHubCommand{Action: "adjustment", APIKeyID: cmd.APIKeyID, RequestKey: "request:" + cmd.RequestID, OperationID: uuid.NewString(), Amount: 0.7, Currency: "USD", Reference: "supplier-invoice-1"}
	mutate(adjustment)
	mutate(adjustment)
	var adjustmentCount int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM kitty_procurement_adjustments`).Scan(&adjustmentCount))
	require.Equal(t, 1, adjustmentCount)
	for _, dimension := range []string{"supplier", "wallet", "account", "group", "model"} {
		raw, e := repo.ReadProviderOperations(ctx, service.ProviderOperationsFilter{From: time.Now().Add(-time.Hour), Until: time.Now().Add(time.Minute), Dimension: dimension, Page: 1, PageSize: 20, Timezone: "Asia/Shanghai"})
		require.NoError(t, e)
		var state struct {
			Summary struct {
				Requests   int     `json:"requests"`
				Cost       float64 `json:"usd_cost"`
				Reconciled int     `json:"reconciled"`
				Profit     float64 `json:"comparable_profit"`
			} `json:"summary"`
		}
		require.NoError(t, json.Unmarshal(raw, &state))
		require.Equal(t, 2, state.Summary.Requests)
		require.InDelta(t, 5.2, state.Summary.Cost, 1e-9)
		require.Equal(t, 1, state.Summary.Reconciled)
		require.InDelta(t, 0.8, state.Summary.Profit, 1e-9)
	}
	var balance float64
	require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=$1`, cmd.UserID).Scan(&balance))
	require.Equal(t, 100.0, balance)
	raw, e := repo.ReadProviderHub(ctx)
	require.NoError(t, e)
	require.NotContains(t, string(raw), "secret_cipher")
	require.NotContains(t, string(raw), "api_key\"")
}

func TestProviderHubWeightedPurchaseIdempotencyAndCurrencyCoverage(t *testing.T) {
	billing, cmd, _, _ := durableBillingFixture(t)
	db := billing.db
	repo := &channelRepository{db: db}
	ctx := context.Background()
	write := func(c service.ProviderHubCommand) int64 {
		t.Helper()
		b, e := repo.MutateProviderHub(ctx, c)
		require.NoError(t, e)
		var r struct {
			ID int64 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(b, &r))
		return r.ID
	}
	s := write(service.ProviderHubCommand{Action: "supplier", Name: "Weighted supplier", SiteType: "manual"})
	w := write(service.ProviderHubCommand{Action: "wallet", SupplierID: s, Name: "Wallet", Identity: "user", Currency: "CNY", CreditCurrency: "USD"})
	write(service.ProviderHubCommand{Action: "bindings", ID: w, Revision: 1, AccountIDs: []int64{cmd.AccountID}})
	purchase := service.ProviderHubCommand{Action: "purchase", ID: w, OperationID: uuid.NewString(), Kind: "topup", Cash: 100, Currency: "CNY", FXRate: 1, Credits: 900, Bonus: 100, OccurredAt: time.Now().Add(-time.Minute)}
	write(purchase)
	write(purchase)
	write(service.ProviderHubCommand{Action: "rule", ID: w, Revision: 2, Model: "*", Method: "account_credits", Currency: "CNY"})
	_, e := db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,total_cost,actual_cost,account_rate_multiplier,created_at) VALUES($1,$2,$3,$4,'fixture',10,5,2,clock_timestamp())`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, cmd.RequestID)
	require.NoError(t, e)
	var price, cost float64
	require.NoError(t, db.QueryRow(`SELECT credit_cost,estimated_cost FROM kitty_procurement_receipts WHERE api_key_id=$1`, cmd.APIKeyID).Scan(&price, &cost))
	require.Equal(t, 0.1, price)
	require.Equal(t, 2.0, cost)
	write(service.ProviderHubCommand{Action: "purchase", ID: w, OperationID: uuid.NewString(), Kind: "refund", Cash: 10, Currency: "CNY", FXRate: 1, Credits: 100, OccurredAt: time.Now()})
	require.NoError(t, db.QueryRow(`SELECT credit_cost,estimated_cost FROM kitty_procurement_receipts WHERE api_key_id=$1`, cmd.APIKeyID).Scan(&price, &cost))
	require.Equal(t, 2.0, cost)
	raw, e := repo.ReadProviderOperations(ctx, service.ProviderOperationsFilter{From: time.Now().Add(-time.Hour), Until: time.Now().Add(time.Minute), Dimension: "supplier", Page: 1, PageSize: 20, Timezone: "UTC"})
	require.NoError(t, e)
	var result struct {
		Summary struct {
			Priced, Unknown int
			USD             int `json:"usd_priced"`
			Comparable      int `json:"comparable_count"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	require.Equal(t, 1, result.Summary.Priced)
	require.Zero(t, result.Summary.USD)
	require.Zero(t, result.Summary.Comparable)
	purchase.Cash = 101
	_, e = repo.MutateProviderHub(ctx, purchase)
	require.ErrorIs(t, e, service.ErrProviderHubConflict)
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM kitty_supplier_purchases`).Scan(&n))
	require.Equal(t, 2, n)
}

func TestProviderHubAssignmentConflictAndExplicitZero(t *testing.T) {
	billing, cmd, _, _ := durableBillingFixture(t)
	repo := &channelRepository{db: billing.db}
	ctx := context.Background()
	var supplier, w1, w2 int64
	require.NoError(t, repo.db.QueryRow(`INSERT INTO kitty_suppliers(name,site_type) VALUES('Concurrent','manual') RETURNING id`).Scan(&supplier))
	require.NoError(t, repo.db.QueryRow(`INSERT INTO kitty_supplier_wallets(supplier_id,name,identity,currency,credit_currency) VALUES($1,'A','a','USD','USD') RETURNING id`, supplier).Scan(&w1))
	require.NoError(t, repo.db.QueryRow(`INSERT INTO kitty_supplier_wallets(supplier_id,name,identity,currency,credit_currency) VALUES($1,'B','b','USD','USD') RETURNING id`, supplier).Scan(&w2))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []int64{w1, w2} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, e := repo.MutateProviderHub(ctx, service.ProviderHubCommand{Action: "bindings", ID: id, Revision: 1, AccountIDs: []int64{cmd.AccountID}})
			errs <- e
		}(id)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if e == service.ErrProviderHubConflict {
			conflict++
		} else {
			require.NoError(t, e)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	var wallet, revision int64
	require.NoError(t, repo.db.QueryRow(`SELECT b.wallet_id,w.revision FROM kitty_supplier_bindings b JOIN kitty_supplier_wallets w ON w.id=b.wallet_id WHERE b.account_id=$1 AND b.ended_at IS NULL`, cmd.AccountID).Scan(&wallet, &revision))
	_, e := repo.MutateProviderHub(ctx, service.ProviderHubCommand{Action: "rule", ID: wallet, Revision: revision, Model: "*", Method: "request", UnitPrice: 0, Currency: "USD"})
	require.NoError(t, e)
	_, e = repo.db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,total_cost,actual_cost,created_at) VALUES($1,$2,$3,$4,'fixture',1,1,clock_timestamp())`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, cmd.RequestID)
	require.NoError(t, e)
	var cost float64
	var status string
	require.NoError(t, repo.db.QueryRow(`SELECT estimated_cost,cost_status FROM kitty_procurement_receipts WHERE api_key_id=$1`, cmd.APIKeyID).Scan(&cost, &status))
	require.Zero(t, cost)
	require.Equal(t, "estimated", status)
	tx, e := repo.db.Begin()
	require.NoError(t, e)
	_, e = tx.Exec(`UPDATE usage_logs SET total_cost=8 WHERE request_id=$1`, cmd.RequestID)
	require.NoError(t, e)
	require.NoError(t, tx.Rollback())
	require.NoError(t, repo.db.QueryRow(`SELECT estimated_cost FROM kitty_procurement_receipts WHERE api_key_id=$1`, cmd.APIKeyID).Scan(&cost))
	require.Zero(t, cost)
}

func TestProviderHubMediaCostMethods(t *testing.T) {
	billing, _, _, _ := durableBillingFixture(t)
	for _, tc := range []struct {
		method   string
		expected float64
	}{{"tokens", 0.0006}, {"image", 0.3}, {"video_second", 0.8}, {"video_item", 0.2}, {"request", 0.1}} {
		m := `{"input":10,"output":20,"cache_read":30,"cache_write":40,"images":0,"videos":0,"seconds":4}`
		if tc.method == "image" {
			m = `{"images":3,"videos":0}`
		}
		if tc.method == "video_second" || tc.method == "video_item" {
			m = `{"images":0,"videos":2,"seconds":4}`
		}
		var value float64
		require.NoError(t, billing.db.QueryRow(`SELECT kitty_procurement_cost(jsonb_build_object('method',$1::text,'input_price',2,'output_price',3,'cache_read_price',4,'cache_write_price',10,'unit_price',0.1),$2::jsonb,NULL)`, tc.method, m).Scan(&value))
		require.InDelta(t, tc.expected, value, 1e-12, tc.method)
	}
}

func TestProviderHubSyncClaimsTimestampsAndRemoteOperationIdempotency(t *testing.T) {
	billing, _, _, _ := durableBillingFixture(t)
	repo := &channelRepository{db: billing.db}
	ctx := context.Background()
	var supplier, wallet int64
	require.NoError(t, repo.db.QueryRow(`INSERT INTO kitty_suppliers(name,site_type) VALUES('Sync fixture','manual') RETURNING id`).Scan(&supplier))
	require.NoError(t, repo.db.QueryRow(`INSERT INTO kitty_supplier_wallets(supplier_id,name,identity,currency,credit_currency,sync_interval_minutes,secret_cipher) VALUES($1,'Wallet','identity','USD','USD',5,'synthetic-encrypted-only') RETURNING id`, supplier).Scan(&wallet))
	authorization, e := repo.ProviderHubAuthorization(ctx, wallet)
	require.NoError(t, e)
	observed := time.Now().Add(-45 * time.Minute)
	balance := 50.0
	require.NoError(t, repo.SaveProviderHubObservation(ctx, authorization, &balance, "USD", "linked_account_snapshot", "", &observed))
	var actual time.Time
	var amount float64
	require.NoError(t, repo.db.QueryRow(`SELECT observed_at,balance FROM kitty_supplier_wallets WHERE id=$1`, wallet).Scan(&actual, &amount))
	require.WithinDuration(t, observed, actual, time.Millisecond)
	require.Equal(t, 50.0, amount)
	require.NoError(t, repo.SaveProviderHubObservation(ctx, authorization, nil, "", "", "connection_failed", nil))
	require.NoError(t, repo.db.QueryRow(`SELECT observed_at,balance FROM kitty_supplier_wallets WHERE id=$1`, wallet).Scan(&actual, &amount))
	require.WithinDuration(t, observed, actual, time.Millisecond)
	require.Equal(t, 50.0, amount)
	_, e = repo.db.Exec(`UPDATE kitty_supplier_wallets SET attempted_at=NULL WHERE id=$1`, wallet)
	require.NoError(t, e)
	ids, e := repo.ClaimProviderHubSync(ctx, 3)
	require.NoError(t, e)
	require.Equal(t, []int64{wallet}, ids)
	ids, e = repo.ClaimProviderHubSync(ctx, 3)
	require.NoError(t, e)
	require.Empty(t, ids)
	start, e := repo.BeginProviderHubKeyCreation(ctx, wallet, "op-fixture-create", "name", "group")
	require.NoError(t, e)
	require.True(t, start)
	_, e = repo.BeginProviderHubKeyCreation(ctx, wallet, "op-fixture-create", "name", "group")
	require.ErrorIs(t, e, service.ErrProviderHubConflict)
	require.NoError(t, repo.FinishProviderHubKeyCreation(ctx, "op-fixture-create", true))
	start, e = repo.BeginProviderHubKeyCreation(ctx, wallet, "op-fixture-create", "name", "group")
	require.NoError(t, e)
	require.False(t, start)
	_, e = repo.BeginProviderHubKeyCreation(ctx, wallet, "op-fixture-create", "changed", "group")
	require.ErrorIs(t, e, service.ErrProviderHubConflict)
	_, e = repo.db.Exec(`UPDATE kitty_supplier_wallets SET revision=revision+1 WHERE id=$1`, wallet)
	require.NoError(t, e)
	require.ErrorIs(t, repo.SaveProviderHubObservation(ctx, authorization, &balance, "USD", "source", "", nil), service.ErrProviderHubConflict)
	raw, e := repo.ReadProviderHub(ctx)
	require.NoError(t, e)
	require.NotContains(t, string(raw), "synthetic-encrypted-only")
	for i := 0; i < 7; i++ {
		_, e = repo.MutateProviderHub(ctx, service.ProviderHubCommand{Action: "purchase", ID: wallet, OperationID: uuid.NewString(), Kind: "topup", Cash: 1, Currency: "USD", FXRate: 1, Credits: 10, OccurredAt: time.Now()})
		require.NoError(t, e)
	}
	raw, e = repo.ReadProviderHubPurchases(ctx, wallet, 2, 3)
	require.NoError(t, e)
	var p struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	require.NoError(t, json.Unmarshal(raw, &p))
	require.Len(t, p.Items, 3)
	require.Equal(t, 7, p.Total)
}

func TestProviderHubCaptureBulkTiming(t *testing.T) {
	billing, cmd, _, _ := durableBillingFixture(t)
	db := billing.db
	_, e := db.Exec(`INSERT INTO kitty_suppliers(name,site_type) VALUES('Timing fixture','manual');
 INSERT INTO kitty_supplier_wallets(supplier_id,name,identity,currency,credit_currency) SELECT id,'Wallet','identity','USD','USD' FROM kitty_suppliers;
 INSERT INTO kitty_supplier_cost_rules(wallet_id,model,method,multiplier,currency,fx_to_usd) SELECT id,'*','standard_multiplier',0.1,'USD',1 FROM kitty_supplier_wallets;`)
	require.NoError(t, e)
	_, e = db.Exec(`INSERT INTO kitty_supplier_bindings(account_id,wallet_id) SELECT $1,id FROM kitty_supplier_wallets`, cmd.AccountID)
	require.NoError(t, e)
	started := time.Now()
	_, e = db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,total_cost,actual_cost,created_at)
 SELECT $1,$2,$3,'timing-'||n,'fixture',1,1,clock_timestamp() FROM generate_series(1,500)n`, cmd.UserID, cmd.APIKeyID, cmd.AccountID)
	require.NoError(t, e)
	elapsed := time.Since(started)
	var count int
	var cost float64
	require.NoError(t, db.QueryRow(`SELECT count(*),sum(estimated_cost) FROM kitty_procurement_receipts`).Scan(&count, &cost))
	require.Equal(t, 500, count)
	require.Equal(t, 50.0, cost)
	t.Logf("500 synthetic receipts including permanent/procurement triggers: %s (mean %.3fms per row; database-only, not an API latency claim)", elapsed, float64(elapsed.Microseconds())/500/1000)
}
