//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestPermanentUsageRetentionReplayCorrectionsAndTransactions(t *testing.T) {
	r, cmd, u, _ := durableBillingFixture(t)
	ctx := context.Background()
	db := r.db
	_, err := db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,input_tokens,output_tokens,actual_cost,total_cost) VALUES($1,$2,$3,$4,'fixture',10,20,1,2)`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, cmd.RequestID)
	require.NoError(t, err)
	read := func() (int, float64) {
		var n int
		var amount float64
		require.NoError(t, db.QueryRow(`SELECT count(*),COALESCE(sum(actual_cost),0) FROM kitty_permanent_usage`).Scan(&n, &amount))
		return n, amount
	}
	n, amount := read()
	require.Equal(t, 1, n)
	require.Equal(t, 1.0, amount)
	_, err = db.Exec(`UPDATE usage_logs SET actual_cost=3 WHERE request_id=$1`, cmd.RequestID)
	require.NoError(t, err)
	n, amount = read()
	require.Equal(t, 1, n)
	require.Equal(t, 3.0, amount)
	tx, err := db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`UPDATE usage_logs SET actual_cost=99 WHERE request_id=$1`, cmd.RequestID)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	_, amount = read()
	require.Equal(t, 3.0, amount)
	_, err = db.Exec(`DELETE FROM usage_logs WHERE request_id=$1`, cmd.RequestID)
	require.NoError(t, err)
	n, amount = read()
	require.Equal(t, 1, n)
	require.Equal(t, 3.0, amount)
	_, err = db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,input_tokens,actual_cost,total_cost) VALUES($1,$2,$3,$4,'fixture',10,1,2)`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, cmd.RequestID)
	require.NoError(t, err)
	n, amount = read()
	require.Equal(t, 1, n)
	require.Equal(t, 3.0, amount, "replayed stale receipt must not undo a correction")
	// A second canonical durable delivery is billed once, including two writers.
	cmd.RequestID += "-durable"
	u.RequestID = cmd.RequestID
	require.NoError(t, r.StageUsageBilling(ctx, &cmd, &u))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := r.ApplyStagedUsageBilling(ctx, service.UsageBillingEventKey{RequestID: cmd.RequestID, APIKeyID: cmd.APIKeyID})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	n, amount = read()
	require.Equal(t, 2, n)
	require.Equal(t, 4.0, amount)
	_, err = db.Exec(`UPDATE api_keys SET deleted_at=NOW() WHERE id=$1`, cmd.APIKeyID)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM usage_logs`)
	require.NoError(t, err)
	n, amount = read()
	require.Equal(t, 2, n)
	require.Equal(t, 4.0, amount)
}

func TestPermanentUsageBackfillLiveUpdatesDeletesAndLateReceipts(t *testing.T) {
	r, cmd, _, _ := durableBillingFixture(t)
	db := r.db
	// Simulate pre-migration retained rows, then enable the exact trigger.
	_, err := db.Exec(`ALTER TABLE usage_logs DISABLE TRIGGER kitty_permanent_usage_write`)
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		_, err = db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,input_tokens,actual_cost,total_cost,created_at) VALUES($1,$2,$3,$4,'fixture',10,1,1,'2025-01-01')`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, fmt.Sprintf("baseline-%d", i))
		require.NoError(t, err)
	}
	_, err = db.Exec(`ALTER TABLE usage_logs ENABLE TRIGGER kitty_permanent_usage_write; UPDATE kitty_permanent_usage_state SET baseline_highwater=(SELECT max(id) FROM usage_logs),baseline_cursor=0,baseline_complete=FALSE`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE usage_logs SET actual_cost=2 WHERE request_id='baseline-0'; DELETE FROM usage_logs WHERE request_id='baseline-1'`)
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		var n int
		require.NoError(t, db.QueryRow(`SELECT kitty_backfill_permanent_usage(2)`).Scan(&n))
	}
	var n int
	var cost float64
	var complete bool
	require.NoError(t, db.QueryRow(`SELECT count(*),sum(actual_cost) FROM kitty_permanent_usage`).Scan(&n, &cost))
	require.Equal(t, 4, n)
	require.Equal(t, 5.0, cost)
	require.NoError(t, db.QueryRow(`SELECT baseline_complete FROM kitty_permanent_usage_state`).Scan(&complete))
	require.True(t, complete)
	// A late receipt keeps its real observation date; no date watermark loss.
	_, err = db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,input_tokens,actual_cost,total_cost,created_at) VALUES($1,$2,$3,'late','fixture',1,-0.25,-0.25,'2024-01-01')`, cmd.UserID, cmd.APIKeyID, cmd.AccountID)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM usage_logs`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT count(*),sum(actual_cost) FROM kitty_permanent_usage`).Scan(&n, &cost))
	require.Equal(t, 5, n)
	require.Equal(t, 4.75, cost)
}

func TestFinanceOverviewSeparatesCashCurrenciesRefundUnitsAndNonCash(t *testing.T) {
	r, cmd, _, _ := durableBillingFixture(t)
	db := r.db
	now := time.Now().UTC()
	from := now.Add(-time.Hour)
	until := now.Add(time.Hour)
	_, err := db.Exec(`INSERT INTO payment_orders(user_id,amount,pay_amount,status,expires_at,paid_at,provider_snapshot) VALUES($1,100,70,'COMPLETED',NOW()+INTERVAL '1 day',NOW(),'{"currency":"CNY"}'),($1,100,10,'COMPLETED',NOW()+INTERVAL '1 day',NOW(),'{"currency":"USD"}')`, cmd.UserID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO payment_orders(user_id,amount,pay_amount,status,expires_at,paid_at,refund_at,refund_amount,provider_snapshot) VALUES($1,100,700,'REFUNDED',NOW()+INTERVAL '1 day',NOW()-INTERVAL '2 days',NOW(),100,'{"currency":"CNY"}')`, cmd.UserID)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE users SET total_recharged=9999 WHERE id=$1;`, cmd.UserID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO user_affiliate_ledger(user_id,action,amount) VALUES($1,'transfer',22)`, cmd.UserID)
	require.NoError(t, err)
	out, err := (&channelRepository{db: db}).ReadFinanceOverview(context.Background(), from, until)
	require.NoError(t, err)
	var cash []struct {
		Currency        string
		Received        float64
		RefundedCredits float64 `json:"refunded_credits"`
	}
	require.NoError(t, json.Unmarshal(out.Cash, &cash))
	require.Len(t, cash, 2)
	require.Equal(t, "CNY", cash[0].Currency)
	require.Equal(t, 70.0, cash[0].Received)
	require.Equal(t, 100.0, cash[0].RefundedCredits)
	require.Equal(t, 10.0, cash[1].Received)
	require.NotContains(t, string(out.Cash), "9999")
	require.Contains(t, string(out.Credits), `"affiliate_transfers": 22`)
}

func TestSupplierBalancesNarrowProjectionAndSharedWalletNotSummed(t *testing.T) {
	r, cmd, _, _ := durableBillingFixture(t)
	db := r.db
	ctx := context.Background()
	_, err := db.Exec(`UPDATE accounts SET credentials='{"base_url":"https://user:credential@fixture.example/path?secret=hidden","api_key":"business-secret"}',extra=jsonb_build_object('unrelated','private','upstream_billing_probe',jsonb_build_object('balance',jsonb_build_object('status','ok','received_at',NOW()-INTERVAL '1 minute','fresh_until',NOW()+INTERVAL '1 hour','data',jsonb_build_object('unit','USD','wallet_balance',3,'remaining',10,'is_valid',true)))) WHERE id=$1`, cmd.AccountID)
	require.NoError(t, err)
	repo := &channelRepository{db: db}
	v := 4.0
	require.NoError(t, repo.SaveSupplierBalanceThresholds(ctx, service.SupplierBalanceThresholds{"USD": {Wallet: &v}}))
	out, err := repo.ReadSupplierBalances(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, out.Rows, 1)
	require.Equal(t, "fixture.example", out.Rows[0].Host)
	require.Equal(t, []string{"wallet"}, out.Rows[0].Alerts)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	for _, secret := range []string{"business-secret", "credential", "hidden", "unrelated"} {
		require.NotContains(t, string(raw), secret)
	}
	page, err := repo.ReadSupplierBalances(ctx, 2, 20)
	require.NoError(t, err)
	require.Empty(t, page.Rows)
	require.Equal(t, out.Total, page.Total)
}

func TestPermanentUsageConcurrentBaselineAndReceiptCorrections(t *testing.T) {
	r, cmd, _, _ := durableBillingFixture(t)
	db := r.db
	_, err := db.Exec(`ALTER TABLE usage_logs DISABLE TRIGGER kitty_permanent_usage_write`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,input_tokens,actual_cost,total_cost)
 SELECT $1,$2,$3,'concurrent-'||n,'fixture',10,1,1 FROM generate_series(1,120)n`, cmd.UserID, cmd.APIKeyID, cmd.AccountID)
	require.NoError(t, err)
	_, err = db.Exec(`ALTER TABLE usage_logs ENABLE TRIGGER kitty_permanent_usage_write;
 UPDATE kitty_permanent_usage_state SET baseline_highwater=(SELECT max(id) FROM usage_logs),baseline_cursor=0,baseline_complete=FALSE`)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 120; i++ {
			_, e := db.Exec(`UPDATE usage_logs SET actual_cost=2 WHERE request_id=$1`, fmt.Sprintf("concurrent-%d", i))
			if e != nil {
				errs <- e
				return
			}
			if i%2 == 0 {
				_, e = db.Exec(`DELETE FROM usage_logs WHERE request_id=$1`, fmt.Sprintf("concurrent-%d", i))
				if e != nil {
					errs <- e
					return
				}
			}
		}
		errs <- nil
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			var n int
			e := db.QueryRow(`SELECT kitty_backfill_permanent_usage(7)`).Scan(&n)
			if e != nil {
				errs <- e
				return
			}
			if n == 0 {
				errs <- nil
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	var n int
	var amount float64
	require.NoError(t, db.QueryRow(`SELECT count(*),sum(actual_cost) FROM kitty_permanent_usage`).Scan(&n, &amount))
	require.Equal(t, 120, n)
	require.Equal(t, 240.0, amount)
}

func TestPermanentUsageVideoPlaceholderRepairChargesAndCountsOnce(t *testing.T) {
	r, cmd, u, _ := durableBillingFixture(t)
	db := r.db
	ctx := context.Background()
	cmd.RequestID = "grok-video:" + cmd.RequestID
	u.RequestID = cmd.RequestID
	u.VideoCount = 1
	u.User = nil
	u.APIKey = nil
	_, err := db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,video_count,actual_cost,total_cost) VALUES($1,$2,$3,$4,$5,1,0,1)`, cmd.UserID, cmd.APIKeyID, cmd.AccountID, cmd.RequestID, u.Model)
	require.NoError(t, err)
	first, err := r.ApplyVideoUsage(ctx, &cmd, &u)
	require.NoError(t, err)
	require.True(t, first.Applied)
	second, err := r.ApplyVideoUsage(ctx, &cmd, &u)
	require.NoError(t, err)
	require.False(t, second.Applied)
	var n int
	var amount, balance float64
	require.NoError(t, db.QueryRow(`SELECT count(*),sum(actual_cost) FROM kitty_permanent_usage`).Scan(&n, &amount))
	require.Equal(t, 1, n)
	require.Equal(t, 1.0, amount)
	require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=$1`, cmd.UserID).Scan(&balance))
	require.Equal(t, 99.0, balance)
}
