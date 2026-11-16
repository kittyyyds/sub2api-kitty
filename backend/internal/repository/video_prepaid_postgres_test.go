//go:build video_postgres

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"testing"
	"time"
)

func newVideoPrepaidFixture(t *testing.T) (*sql.DB, *usageLogRepository, *usageBillingRepository) {
	db, billing := newVideoUsagePostgresFixture(t)
	_, err := db.Exec(`ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active',ADD COLUMN frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0;
 ALTER TABLE api_keys ADD COLUMN user_id BIGINT NOT NULL DEFAULT 1,ADD COLUMN group_id BIGINT DEFAULT 25,ADD COLUMN expires_at TIMESTAMPTZ,
 ADD COLUMN rate_limit_5h NUMERIC NOT NULL DEFAULT 0,ADD COLUMN rate_limit_1d NUMERIC NOT NULL DEFAULT 0,ADD COLUMN rate_limit_7d NUMERIC NOT NULL DEFAULT 0,
 ADD COLUMN usage_5h NUMERIC NOT NULL DEFAULT 0,ADD COLUMN usage_1d NUMERIC NOT NULL DEFAULT 0,ADD COLUMN usage_7d NUMERIC NOT NULL DEFAULT 0,
 ADD COLUMN window_5h_start TIMESTAMPTZ,ADD COLUMN window_1d_start TIMESTAMPTZ,ADD COLUMN window_7d_start TIMESTAMPTZ;
 CREATE TABLE groups(id BIGINT PRIMARY KEY,status TEXT DEFAULT 'active',platform TEXT DEFAULT 'video',subscription_type TEXT DEFAULT 'balance',deleted_at TIMESTAMPTZ);
 INSERT INTO groups(id)VALUES(25);
 CREATE TABLE accounts(id BIGINT PRIMARY KEY,status TEXT DEFAULT 'active',platform TEXT DEFAULT 'video',type TEXT DEFAULT 'apikey',credentials JSONB,deleted_at TIMESTAMPTZ);
 INSERT INTO accounts(id,credentials)VALUES(205,'{"base_url":"https://fixture.invalid","api_key":"fixture-only","grok_media_provider":"suanli_special"}');
 CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);INSERT INTO account_groups VALUES(205,25);
 CREATE TABLE user_subscriptions(id BIGINT PRIMARY KEY,user_id BIGINT,group_id BIGINT,status TEXT,starts_at TIMESTAMPTZ,expires_at TIMESTAMPTZ,deleted_at TIMESTAMPTZ);
 CREATE TABLE user_platform_quotas(platform TEXT);CREATE TABLE composite_model_routes(target_platform TEXT);
 CREATE TABLE payment_orders(id BIGINT PRIMARY KEY);INSERT INTO payment_orders VALUES(1);`)
	require.NoError(t, err)
	for _, name := range []string{"182_recharge_bonus_credits.sql", "254_recharge_bonus_permanent.sql", "252_video_tasks.sql", "268_kitty_video_prepaid.sql"} {
		raw, e := os.ReadFile("../../migrations/" + name)
		require.NoError(t, e)
		_, e = db.Exec(string(raw))
		require.NoError(t, e, name)
	}
	billing.bonus = newRechargeBonusRepository(db)
	return db, &usageLogRepository{db: db, sql: db}, billing
}
func prepaidFixtureTask(id string) *service.VideoTask {
	return &service.VideoTask{ID: id, UserID: 1, APIKeyID: 166, GroupID: 25, AccountID: 205, Status: "submitting", IdempotencyKey: id, RequestHash: "same-input", CreatedAt: time.Now(), Pending: service.GrokVideoPendingBilling{Unified: true, Model: "exact-video", PrepaidAmount: 3.75, PriceSnapshot: &service.VideoPriceSnapshot{PrepaidTaskID: id, Model: "exact-video", UnitPrice: .75, BillingMode: service.BillingModeVideo, DurationSeconds: 5, FinalUnitPrice: true, RateMultiplier: 1}}}
}
func assertPrepaidBalance(t *testing.T, db *sql.DB, balance, frozen float64) {
	t.Helper()
	var b, f float64
	require.NoError(t, db.QueryRow(`SELECT balance,frozen_balance FROM users WHERE id=1`).Scan(&b, &f))
	require.Equal(t, balance, b)
	require.Equal(t, frozen, f)
}

func TestVideoPrepaidPostgresIdempotencyAndRefund(t *testing.T) {
	db, tasks, _ := newVideoPrepaidFixture(t)
	ctx := context.Background()
	task := prepaidFixtureTask("a")
	_, created, err := tasks.CreatePrepaidVideoTask(ctx, task, 2)
	require.NoError(t, err)
	require.True(t, created)
	assertPrepaidBalance(t, db, 6.25, 3.75)
	again := prepaidFixtureTask("b")
	again.IdempotencyKey = "a"
	old, created, err := tasks.CreatePrepaidVideoTask(ctx, again, 2)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, "a", old.ID)
	again.RequestHash = "different"
	_, _, err = tasks.CreatePrepaidVideoTask(ctx, again, 2)
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	require.NoError(t, tasks.MarkVideoRefundPending(ctx, "a"))
	leased, err := tasks.LeaseVideoTasks(ctx, 5)
	require.NoError(t, err)
	require.Len(t, leased, 1)
	require.Equal(t, "refund_pending", leased[0].BillingState)
	require.NoError(t, tasks.RefundVideoTask(ctx, "a", "fixture"))
	require.NoError(t, tasks.RefundVideoTask(ctx, "a", "fixture"))
	assertPrepaidBalance(t, db, 10, 0)
}
func TestVideoPrepaidPostgresConcurrentAdmission(t *testing.T) {
	db, tasks, _ := newVideoPrepaidFixture(t)
	var wg sync.WaitGroup
	out := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := tasks.CreatePrepaidVideoTask(context.Background(), prepaidFixtureTask(fmt.Sprint(i)), 2)
			out <- err
		}(i)
	}
	wg.Wait()
	close(out)
	accepted := 0
	for e := range out {
		if e == nil {
			accepted++
		}
	}
	require.Equal(t, 2, accepted)
	assertPrepaidBalance(t, db, 2.5, 7.5)
}
func TestVideoPrepaidPostgresCaptureExactlyOnceWithBonus(t *testing.T) {
	db, tasks, billing := newVideoPrepaidFixture(t)
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at)VALUES(1,1,2,2,NULL)`)
	require.NoError(t, err)
	task := prepaidFixtureTask("a")
	_, _, err = tasks.CreatePrepaidVideoTask(ctx, task, 2)
	require.NoError(t, err)
	task.UpstreamTaskID = "task"
	require.NoError(t, tasks.AcceptVideoTask(ctx, task))
	var wg sync.WaitGroup
	out := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd, log := videoPostgresCommand()
			cmd.PrepaidVideoTaskID = task.ID
			_, err := billing.ApplyVideoUsage(ctx, cmd, log)
			out <- err
		}()
	}
	wg.Wait()
	close(out)
	for e := range out {
		require.NoError(t, e)
	}
	assertPrepaidBalance(t, db, 6.25, 0)
	assertVideoPostgresState(t, db, 6.25, 3.75, 1, 1)
	var amount float64
	require.NoError(t, db.QueryRow(`SELECT SUM(amount) FROM recharge_bonus_ledger WHERE entry_type='consume'`).Scan(&amount))
	require.Equal(t, 2.0, amount)
	require.Error(t, tasks.RefundVideoTask(ctx, "a", "late failure"))
	assertPrepaidBalance(t, db, 6.25, 0)
}
func TestVideoPrepaidPostgresRefundRestoresPermanentBonus(t *testing.T) {
	db, tasks, _ := newVideoPrepaidFixture(t)
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at)VALUES(1,1,2,2,NULL)`)
	require.NoError(t, err)
	_, _, err = tasks.CreatePrepaidVideoTask(ctx, prepaidFixtureTask("a"), 2)
	require.NoError(t, err)
	require.NoError(t, tasks.RefundVideoTask(ctx, "a", "failed"))
	assertPrepaidBalance(t, db, 10, 0)
	var left float64
	require.NoError(t, db.QueryRow(`SELECT remaining_amount FROM recharge_bonus_grants`).Scan(&left))
	require.Equal(t, 2.0, left)
}
func TestVideoPrepaidPostgresReceiptFailureRollsBackCapture(t *testing.T) {
	db, tasks, billing := newVideoPrepaidFixture(t)
	ctx := context.Background()
	task := prepaidFixtureTask("a")
	_, _, err := tasks.CreatePrepaidVideoTask(ctx, task, 2)
	require.NoError(t, err)
	task.UpstreamTaskID = "task"
	require.NoError(t, tasks.AcceptVideoTask(ctx, task))
	_, err = db.Exec(`ALTER TABLE usage_logs ADD CONSTRAINT reject_fixture CHECK(model<>'exact-video')`)
	require.NoError(t, err)
	cmd, log := videoPostgresCommand()
	cmd.PrepaidVideoTaskID = task.ID
	_, err = billing.ApplyVideoUsage(ctx, cmd, log)
	require.Error(t, err)
	assertPrepaidBalance(t, db, 6.25, 3.75)
	var state string
	require.NoError(t, db.QueryRow(`SELECT billing_state FROM video_tasks WHERE id='a'`).Scan(&state))
	require.Equal(t, "held", state)
}

func TestVideoPrepaidPostgresKeyBudgetIncludesOtherHeldTasks(t *testing.T) {
	for _, limit := range []string{"quota=6", "rate_limit_5h=6", "rate_limit_1d=6", "rate_limit_7d=6"} {
		t.Run(limit, func(t *testing.T) {
			db, tasks, _ := newVideoPrepaidFixture(t)
			ctx := context.Background()
			_, err := db.Exec(`UPDATE api_keys SET ` + limit)
			require.NoError(t, err)
			_, _, err = tasks.CreatePrepaidVideoTask(ctx, prepaidFixtureTask("one"), 10)
			require.NoError(t, err)
			_, _, err = tasks.CreatePrepaidVideoTask(ctx, prepaidFixtureTask("two"), 10)
			require.Error(t, err)
			assertPrepaidBalance(t, db, 6.25, 3.75)
			require.NoError(t, tasks.RefundVideoTask(ctx, "one", "failed"))
			_, _, err = tasks.CreatePrepaidVideoTask(ctx, prepaidFixtureTask("two"), 10)
			require.NoError(t, err)
		})
	}
}
func TestVideoPrepaidPostgresZeroPriceHasDurableRefundReceipt(t *testing.T) {
	db, tasks, _ := newVideoPrepaidFixture(t)
	task := prepaidFixtureTask("free")
	task.Pending.PrepaidAmount = 0
	_, created, err := tasks.CreatePrepaidVideoTask(context.Background(), task, 2)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, tasks.RefundVideoTask(context.Background(), task.ID, "failed"))
	assertPrepaidBalance(t, db, 10, 0)
	var state string
	require.NoError(t, db.QueryRow(`SELECT billing_state FROM video_tasks WHERE id='free'`).Scan(&state))
	require.Equal(t, "refunded", state)
}
func TestVideoPrepaidPostgresCaptureRefundRace(t *testing.T) {
	db, tasks, billing := newVideoPrepaidFixture(t)
	ctx := context.Background()
	task := prepaidFixtureTask("race")
	_, _, err := tasks.CreatePrepaidVideoTask(ctx, task, 2)
	require.NoError(t, err)
	task.UpstreamTaskID = "task"
	require.NoError(t, tasks.AcceptVideoTask(ctx, task))
	start := make(chan struct{})
	out := make(chan error, 2)
	go func() {
		<-start
		cmd, log := videoPostgresCommand()
		cmd.PrepaidVideoTaskID = task.ID
		_, e := billing.ApplyVideoUsage(ctx, cmd, log)
		out <- e
	}()
	go func() { <-start; out <- tasks.RefundVideoTask(ctx, task.ID, "failed") }()
	close(start)
	a, b := <-out, <-out
	require.NotEqual(t, a == nil, b == nil, "exactly one terminal outcome must win")
	var state string
	require.NoError(t, db.QueryRow(`SELECT billing_state FROM video_tasks WHERE id='race'`).Scan(&state))
	if state == "refunded" {
		assertPrepaidBalance(t, db, 10, 0)
		assertVideoPostgresState(t, db, 10, 0, 0, 0)
	} else {
		require.Equal(t, "captured", state)
		assertPrepaidBalance(t, db, 6.25, 0)
		assertVideoPostgresState(t, db, 6.25, 3.75, 1, 1)
	}
}
func TestVideoPrepaidPostgresRefundDoesNotReviveExpiredBonus(t *testing.T) {
	db, tasks, _ := newVideoPrepaidFixture(t)
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at) VALUES(1,1,2,2,NOW()+INTERVAL '1 hour')`)
	require.NoError(t, err)
	_, _, err = tasks.CreatePrepaidVideoTask(ctx, prepaidFixtureTask("a"), 2)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE recharge_bonus_grants SET expires_at=NOW()-INTERVAL '1 minute'`)
	require.NoError(t, err)
	require.NoError(t, tasks.RefundVideoTask(ctx, "a", "failed"))
	assertPrepaidBalance(t, db, 8, 0)
}
