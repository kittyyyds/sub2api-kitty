//go:build kitty_cross_version_canary && media_postgres && bonus_postgres && video_postgres

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const crossVersionBuildKind = "candidate"

// This test deliberately leaves its synthetic schemas in place so a different
// executable can settle the candidate's durable rows after the first exits.
// It rejects public/production names and never drops schemas or touches Redis.
func TestCrossVersionMediaBillingCanary(t *testing.T) {
	phase := os.Getenv("KITTY_CROSS_VERSION_PHASE")
	require.Contains(t, []string{"candidate", "fallback", "fallback_replay"}, phase)
	if phase == "candidate" {
		require.Equal(t, "candidate", crossVersionBuildKind)
	} else {
		require.Equal(t, "fallback", crossVersionBuildKind)
	}
	base := os.Getenv("KITTY_CROSS_VERSION_SCHEMA")
	require.Regexp(t, regexp.MustCompile(`^kitty_cross_20261101_[0-9]{1,20}$`), base)
	t.Logf("CANARY build=%s phase=%s schema_base=%s", crossVersionBuildKind, phase, base)
	for _, part := range []string{"wallet", "video"} {
		t.Run(part, func(t *testing.T) {
			db := crossVersionDB(t, base+"_"+part, phase == "candidate")
			if part == "wallet" {
				crossVersionWallet(t, db, phase)
			} else {
				crossVersionVideo(t, db, phase)
			}
		})
	}
}

func crossVersionDB(t *testing.T, schema string, create bool) *sql.DB {
	t.Helper()
	raw := os.Getenv("KITTY_CROSS_VERSION_DATABASE_URL")
	require.NotEmpty(t, raw)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "postgres", u.Scheme)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, u.Hostname())
	require.Equal(t, "/kitty_media_wallet_test", u.Path)
	require.Empty(t, u.Fragment)
	q := u.Query()
	q.Del("search_path")
	q.Del("options")
	q.Set("connect_timeout", "5")
	q.Set("statement_timeout", "15000")
	u.RawQuery = q.Encode()
	admin, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	require.NoError(t, admin.Ping())
	var actual string
	require.NoError(t, admin.QueryRow(`SELECT current_database()`).Scan(&actual))
	require.Equal(t, "kitty_media_wallet_test", actual)
	if create {
		_, err = admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema))
		require.NoError(t, err, "refuse overwriting previous canary evidence")
	} else {
		var exists bool
		require.NoError(t, admin.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, schema).Scan(&exists))
		require.True(t, exists, "candidate process must create this exact schema first")
	}
	require.NoError(t, admin.Close())
	q.Set("search_path", schema+",pg_catalog")
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.QueryRow(`SELECT current_schema()`).Scan(&actual))
	require.Equal(t, schema, actual)
	if create {
		_, err = db.Exec(`CREATE TABLE canary_marker(id INTEGER PRIMARY KEY CHECK(id=1),producer TEXT NOT NULL,phase TEXT NOT NULL);
            INSERT INTO canary_marker VALUES(1,'candidate','creating')`)
		require.NoError(t, err)
	} else {
		var producer, phase string
		require.NoError(t, db.QueryRow(`SELECT producer,phase FROM canary_marker WHERE id=1`).Scan(&producer, &phase))
		require.Equal(t, "candidate", producer)
		expected := "candidate_ready"
		if os.Getenv("KITTY_CROSS_VERSION_PHASE") == "fallback_replay" {
			expected = "fallback_complete"
		}
		require.Equal(t, expected, phase)
	}
	return db
}

func crossMigrations(t *testing.T, db *sql.DB, names ...string) {
	t.Helper()
	for _, name := range names {
		raw, err := os.ReadFile("../../migrations/" + name)
		require.NoError(t, err)
		_, err = db.Exec(string(raw))
		require.NoError(t, err, name)
	}
}
func crossMark(t *testing.T, db *sql.DB, phase string) {
	t.Helper()
	_, err := db.Exec(`UPDATE canary_marker SET phase=$1 WHERE id=1`, phase)
	require.NoError(t, err)
}
func crossBonus(t *testing.T, db *sql.DB) {
	t.Helper()
	var left float64
	var expiry sql.NullTime
	require.NoError(t, db.QueryRow(`SELECT remaining_amount,expires_at FROM recharge_bonus_grants WHERE user_id=1`).Scan(&left, &expiry))
	require.Equal(t, 2.0, left)
	require.False(t, expiry.Valid)
}
func crossVersionWallet(t *testing.T, db *sql.DB, phase string) {
	ctx := context.Background()
	wallet := NewMediaWalletRepository(db)
	if phase == "candidate" {
		_, err := db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY,balance NUMERIC(20,8) NOT NULL,frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0,status TEXT NOT NULL DEFAULT 'active',deleted_at TIMESTAMPTZ,updated_at TIMESTAMPTZ);
          INSERT INTO users(id,balance) VALUES(1,10),(2,10);
          CREATE TABLE payment_orders(id BIGINT PRIMARY KEY);INSERT INTO payment_orders VALUES(1),(2);`)
		require.NoError(t, err)
		crossMigrations(t, db, "182_recharge_bonus_credits.sql", "254_recharge_bonus_permanent.sql", "286_kitty_media_wallet.sql", "287_kitty_media_identities.sql")
		_, err = db.Exec(`INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at)VALUES(1,1,2,2,NULL)`)
		require.NoError(t, err)
		for _, item := range []struct{ id, amount string }{{"cross-refund", "2"}, {"cross-capture", "4"}} {
			_, created, err := wallet.Reserve(ctx, mediaReserveFixture(t, item.id, item.amount))
			require.NoError(t, err)
			require.True(t, created)
		}
		acceptMedia(t, wallet, service.MediaWalletKey{UserID: 1, OrderID: "cross-capture"})
		assertMediaBalance(t, db, "4.00000000", "6.00000000")
		crossMark(t, db, "candidate_ready")
		t.Log("PERSISTED wallet balance=4 frozen=6 capture_hold=4 refund_hold=2 bonus_allocated=2")
		return
	}
	captureKey := service.MediaWalletKey{UserID: 1, OrderID: "cross-capture"}
	refundKey := service.MediaWalletKey{UserID: 1, OrderID: "cross-refund"}
	saved, err := wallet.Get(ctx, captureKey)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "4.00000000", saved.HoldUSD)
	require.Equal(t, "accepted", saved.SubmissionState)
	require.JSONEq(t, string(mediaReserveFixture(t, "cross-capture", "4").Quote), string(saved.Quote))
	require.JSONEq(t, string(mediaReserveFixture(t, "cross-capture", "4").PriceContract), string(saved.PriceContract))
	if phase == "fallback" {
		assertMediaBalance(t, db, "4.00000000", "6.00000000")
		require.Equal(t, "held", saved.BillingState)
	}
	for repeat := 0; repeat < 2; repeat++ {
		_, err = wallet.Release(ctx, refundKey, mediaEvidence("cross-refund-confirmed"))
		require.NoError(t, err)
		order, err := wallet.Capture(ctx, captureKey, "3.5", mediaEvidence("cross-capture-confirmed"))
		require.NoError(t, err)
		require.Equal(t, "3.50000000", order.ActualUSD)
		assertMediaBalance(t, db, "6.50000000", "0.00000000")
		crossBonus(t, db)
	}
	var count int
	var actual, released, held string
	require.NoError(t, db.QueryRow(`SELECT count(*),sum(actual_amount)::text,sum(released_amount)::text,sum(held_amount)::text FROM kitty_media_wallet_ledger`).Scan(&count, &actual, &released, &held))
	require.Equal(t, 4, count)
	require.Equal(t, "3.50000000", actual)
	require.Equal(t, "2.50000000", released)
	require.Equal(t, "6.00000000", held)
	_, err = wallet.Capture(ctx, captureKey, "3.6", mediaEvidence("cross-capture-confirmed"))
	require.ErrorIs(t, err, service.ErrMediaWalletConflict)
	assertMediaBalance(t, db, "6.50000000", "0.00000000")
	crossMark(t, db, "fallback_complete")
	t.Log("VERIFIED wallet candidate->fallback balance=6.5 frozen=0 actual=3.5 restored=2.5 ledger_count=4 permanent_bonus=2 replay_idempotent=true conflicting_replay_rejected=true")
}

func crossVideoTask(id string, amount float64) *service.VideoTask {
	task := prepaidFixtureTask(id)
	task.Pending.PrepaidAmount = amount
	task.Pending.VideoDurationSeconds = 30
	task.Pending.VideoResolution = "720p"
	task.Pending.PriceSnapshot = &service.VideoPriceSnapshot{PrepaidTaskID: id, Model: task.Pending.Model, UnitPrice: amount, BillingMode: service.BillingModePerRequest, DurationSeconds: 30, FinalUnitPrice: true, RateMultiplier: 7}
	return task
}
func crossVersionVideo(t *testing.T, db *sql.DB, phase string) {
	ctx := context.Background()
	tasks := &usageLogRepository{db: db, sql: db}
	billing := &usageBillingRepository{db: db, bonus: newRechargeBonusRepository(db)}
	if phase == "candidate" {
		_, err := db.Exec(videoUsagePostgresSchema)
		require.NoError(t, err)
		crossMigrations(t, db, "071_add_usage_billing_dedup.sql", "073_add_usage_billing_dedup_archive.sql", "321_usage_log_queue_wait.sql", "322_usage_log_queue_timing.sql")
		_, err = db.Exec(`ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active',ADD COLUMN frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0;
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
		crossMigrations(t, db, "182_recharge_bonus_credits.sql", "254_recharge_bonus_permanent.sql", "252_video_tasks.sql", "268_kitty_video_prepaid.sql")
		_, err = db.Exec(`UPDATE users SET balance=20 WHERE id=1;
          ALTER TABLE groups ADD COLUMN model_pricing JSONB;
          UPDATE groups SET model_pricing='[{"models":["exact-video"],"billing_mode":"per_request","per_request_price":8.35}]'::jsonb;
          INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at)VALUES(1,1,2,2,NULL)`)
		require.NoError(t, err)
		for _, item := range []struct {
			id     string
			amount float64
		}{{"cross-video-refund", 2}, {"cross-video-capture", 8.35}} {
			task := crossVideoTask(item.id, item.amount)
			_, created, err := tasks.CreatePrepaidVideoTask(ctx, task, 10)
			require.NoError(t, err)
			require.True(t, created)
			if item.id == "cross-video-capture" {
				task.UpstreamTaskID = "cross-upstream-task"
				require.NoError(t, tasks.AcceptVideoTask(ctx, task))
			}
		}
		// A later current price must not replace the already accepted snapshot.
		_, err = db.Exec(`UPDATE groups SET model_pricing='[{"models":["exact-video"],"billing_mode":"per_request","per_request_price":99}]'::jsonb`)
		require.NoError(t, err)
		assertPrepaidBalance(t, db, 9.65, 10.35)
		crossMark(t, db, "candidate_ready")
		t.Log("PERSISTED video balance=9.65 frozen=10.35 locked_price=8.35 mode=per_request duration=30 final_unit=true multiplier=7 current_group_price=99")
		return
	}
	task, err := tasks.FindVideoTask(ctx, "cross-video-capture", 1, 166)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, "cross-upstream-task", task.UpstreamTaskID)
	require.Equal(t, "720p", task.Pending.VideoResolution)
	snapshot := task.Pending.PriceSnapshot
	require.NotNil(t, snapshot)
	require.Equal(t, 8.35, snapshot.UnitPrice)
	require.Equal(t, service.BillingModePerRequest, snapshot.BillingMode)
	require.Equal(t, 30, snapshot.DurationSeconds)
	require.True(t, snapshot.FinalUnitPrice)
	require.Equal(t, 7.0, snapshot.RateMultiplier)
	var price float64
	require.NoError(t, db.QueryRow(`SELECT (model_pricing->0->>'per_request_price')::numeric FROM groups WHERE id=25`).Scan(&price))
	require.Equal(t, 99.0, price)
	cost, err := service.CrossVersionCanaryVideoCost(snapshot, 1)
	require.NoError(t, err)
	require.Equal(t, 8.35, cost.ActualCost)
	require.Equal(t, 8.35, cost.TotalCost)
	require.True(t, cost.FinalPriceApplied)
	if phase == "fallback" {
		assertPrepaidBalance(t, db, 9.65, 10.35)
		require.Equal(t, "held", task.BillingState)
	}
	for repeat := 0; repeat < 2; repeat++ {
		cmd, receipt := videoPostgresCommand()
		cmd.RequestID = service.StableGrokVideoBillingRequestID(task.UpstreamTaskID)
		cmd.PrepaidVideoTaskID = task.ID
		cmd.BalanceCost = cost.ActualCost
		cmd.APIKeyQuotaCost = cost.ActualCost
		cmd.RequestFingerprint = ""
		cmd.Normalize()
		receipt.RequestID = cmd.RequestID
		receipt.TotalCost = cost.TotalCost
		receipt.ActualCost = cost.ActualCost
		mode, resolution, duration := cost.BillingMode, task.Pending.VideoResolution, snapshot.DurationSeconds
		receipt.BillingMode = &mode
		receipt.VideoResolution = &resolution
		receipt.VideoDurationSeconds = &duration
		result, err := billing.ApplyVideoUsage(ctx, cmd, receipt)
		require.NoError(t, err)
		require.Equal(t, phase == "fallback" && repeat == 0, result.Applied)
		require.NoError(t, tasks.RefundVideoTask(ctx, "cross-video-refund", "fixture failure"))
		assertPrepaidBalance(t, db, 11.65, 0)
		assertVideoPostgresState(t, db, 11.65, 8.35, 1, 1)
		crossBonus(t, db)
	}
	saved, err := tasks.FindVideoTask(ctx, task.ID, 1, 166)
	require.NoError(t, err)
	require.Equal(t, "captured", saved.BillingState)
	before, _ := json.Marshal(snapshot)
	after, _ := json.Marshal(saved.Pending.PriceSnapshot)
	require.JSONEq(t, string(before), string(after))
	refunded, err := tasks.FindVideoTask(ctx, "cross-video-refund", 1, 166)
	require.NoError(t, err)
	require.Equal(t, "refunded", refunded.BillingState)
	var actual float64
	var mode string
	var duration int
	require.NoError(t, db.QueryRow(`SELECT actual_cost,billing_mode,video_duration_seconds FROM usage_logs`).Scan(&actual, &mode, &duration))
	require.Equal(t, 8.35, actual)
	require.Equal(t, "per_request", mode)
	require.Equal(t, 30, duration)
	require.Error(t, tasks.RefundVideoTask(ctx, task.ID, "late failure"))
	assertPrepaidBalance(t, db, 11.65, 0)
	crossMark(t, db, "fallback_complete")
	t.Log("VERIFIED video candidate->fallback balance=11.65 frozen=0 quota=8.35 receipts=1 dedup=1 permanent_bonus=2 locked_per_request_price=8.35 unchanged_after_group_price=99 replay_idempotent=true")
}
