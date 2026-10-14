//go:build bonus_postgres

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// This suite accepts a dedicated localhost database only and always works in a
// disposable synthetic schema. It never accesses public/production app tables.
func newRechargeBonusPostgresFixture(t *testing.T, upgrade bool) (*sql.DB, *rechargeBonusRepository) {
	t.Helper()
	dsn := os.Getenv("RECHARGE_BONUS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "set a dedicated localhost RECHARGE_BONUS_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname())
	query := parsed.Query()
	query.Set("connect_timeout", "5")
	parsed.RawQuery = query.Encode()
	admin, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	require.NoError(t, admin.Ping())
	schema := fmt.Sprintf("recharge_bonus_test_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	query.Set("search_path", schema+",pg_catalog")
	query.Set("statement_timeout", "15000")
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(12)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		_, cleanupErr := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE")
		require.NoError(t, cleanupErr)
		require.NoError(t, admin.Close())
	})
	_, err = db.Exec(`
		CREATE TABLE users (id BIGINT PRIMARY KEY, balance NUMERIC(20,8) NOT NULL DEFAULT 0,
		 frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0, updated_at TIMESTAMPTZ, deleted_at TIMESTAMPTZ);
		CREATE TABLE payment_orders (id BIGINT PRIMARY KEY);
		INSERT INTO users(id,balance) VALUES (42,100);
		INSERT INTO payment_orders(id) SELECT generate_series(1,100);
	`)
	require.NoError(t, err)
	for _, migration := range []string{"182_recharge_bonus_credits.sql", "071_add_usage_billing_dedup.sql", "073_add_usage_billing_dedup_archive.sql"} {
		runRechargeBonusSQL(t, db, "../../migrations/"+migration)
	}
	if upgrade {
		runRechargeBonusSQL(t, db, "../../migrations/254_recharge_bonus_permanent.sql")
	}
	return db, newRechargeBonusRepository(db)
}

func runRechargeBonusSQL(t *testing.T, db *sql.DB, filename string) {
	t.Helper()
	ddl, err := os.ReadFile(filename)
	require.NoError(t, err)
	_, err = db.Exec(string(ddl))
	require.NoError(t, err, filename)
}

func rechargeBonusTestRule(t *testing.T, db *sql.DB, amount, bonus float64, days int) *service.RechargeBonusRule {
	t.Helper()
	rule := &service.RechargeBonusRule{RechargeAmount: amount, BonusAmount: bonus, ValidityDays: days}
	err := db.QueryRow(`INSERT INTO recharge_bonus_rules(recharge_amount,bonus_amount,validity_days)
		VALUES($1,$2,$3) ON CONFLICT(recharge_amount) DO UPDATE
		SET bonus_amount=EXCLUDED.bonus_amount, validity_days=EXCLUDED.validity_days
		RETURNING id`, amount, bonus, days).Scan(&rule.ID)
	require.NoError(t, err)
	return rule
}

func grantRechargeBonusTestOrder(t *testing.T, db *sql.DB, repo *rechargeBonusRepository, order int64, rule *service.RechargeBonusRule, now time.Time) *service.RechargeBonusGrantResult {
	t.Helper()
	require.NoError(t, repo.SnapshotOrder(context.Background(), db, order, rule))
	result, err := repo.GrantForOrder(context.Background(), order, 42, now)
	require.NoError(t, err)
	require.True(t, result.Applied)
	return result
}

func assertRechargeBonusBalances(t *testing.T, db *sql.DB, balance, frozen float64) {
	t.Helper()
	var gotBalance, gotFrozen float64
	require.NoError(t, db.QueryRow("SELECT balance, frozen_balance FROM users WHERE id=42").Scan(&gotBalance, &gotFrozen))
	require.InDelta(t, balance, gotBalance, 0.0000001)
	require.InDelta(t, frozen, gotFrozen, 0.0000001)
}

func TestRechargeBonusPostgresMigrationAndCampaignPreserveHistory(t *testing.T) {
	db, repo := newRechargeBonusPostgresFixture(t, false)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	old50, err := repo.MatchRule(ctx, 50, now)
	require.NoError(t, err)
	require.NoError(t, repo.SnapshotOrder(ctx, db, 1, old50)) // pending before campaign
	oldGrant := grantRechargeBonusTestOrder(t, db, repo, 2, old50, now)
	require.NotNil(t, oldGrant.ExpiresAt)
	var rulesBefore string
	require.NoError(t, db.QueryRow(`SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM recharge_bonus_rules r`).Scan(&rulesBefore))
	// Upgrades are repeatable and never mutate historical records.
	runRechargeBonusSQL(t, db, "../../migrations/254_recharge_bonus_permanent.sql")
	runRechargeBonusSQL(t, db, "../../migrations/254_recharge_bonus_permanent.sql")
	apply := "../../../deploy/recharge-bonus/20260926-permanent-campaign.sql"
	runRechargeBonusSQL(t, db, apply)
	runRechargeBonusSQL(t, db, apply)
	for amount, bonus := range map[float64]float64{30: 1, 50: 1.99, 100: 4.99, 500: 29.99, 1000: 49.99, 5000: 249.99} {
		rule, matchErr := repo.MatchRule(ctx, amount, now.Add(time.Minute))
		require.NoError(t, matchErr)
		require.NotNil(t, rule)
		require.Zero(t, rule.ValidityDays)
		require.InDelta(t, bonus, rule.BonusAmount, 0.0000001)
	}
	for _, amount := range []float64{5, 10} {
		rule, matchErr := repo.MatchRule(ctx, amount, now)
		require.NoError(t, matchErr)
		require.Nil(t, rule)
	}
	old300, err := repo.MatchRule(ctx, 300, now)
	require.NoError(t, err)
	require.Equal(t, 15, old300.ValidityDays)
	require.Equal(t, 15.0, old300.BonusAmount)
	// Paying an old pending order after the switch keeps its original 2 / 3 days.
	pending, err := repo.GrantForOrder(ctx, 1, 42, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, 2.0, pending.BonusAmount)
	require.Equal(t, now.Add(time.Hour).AddDate(0, 0, 3), *pending.ExpiresAt)
	new50, err := repo.MatchRule(ctx, 50, now)
	require.NoError(t, err)
	permanent := grantRechargeBonusTestOrder(t, db, repo, 3, new50, now)
	require.Nil(t, permanent.ExpiresAt)
	// Snapshot insertion is immutable even when called again with another rule.
	require.NoError(t, repo.SnapshotOrder(ctx, db, 1, new50))
	var oldDays int
	require.NoError(t, db.QueryRow("SELECT validity_days FROM recharge_bonus_order_snapshots WHERE payment_order_id=1").Scan(&oldDays))
	require.Equal(t, 3, oldDays)
	revert := "../../../deploy/recharge-bonus/20260926-revert-campaign.sql"
	runRechargeBonusSQL(t, db, revert)
	runRechargeBonusSQL(t, db, revert)
	var restored string
	require.NoError(t, db.QueryRow(`SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM recharge_bonus_rules r WHERE recharge_amount<>5000`).Scan(&restored))
	require.JSONEq(t, rulesBefore, restored)
	var originalExpiry time.Time
	require.NoError(t, db.QueryRow("SELECT expires_at FROM recharge_bonus_grants WHERE id=$1", oldGrant.GrantID).Scan(&originalExpiry))
	require.True(t, oldGrant.ExpiresAt.Equal(originalExpiry))
	var permanentExpiry sql.NullTime
	require.NoError(t, db.QueryRow("SELECT expires_at FROM recharge_bonus_grants WHERE id=$1", permanent.GrantID).Scan(&permanentExpiry))
	require.False(t, permanentExpiry.Valid)
	assertRechargeBonusBalances(t, db, 105.99, 0)
	_, err = db.Exec("UPDATE recharge_bonus_rules SET validity_days=-1 WHERE recharge_amount=50")
	require.Error(t, err)
	// A reverted campaign cannot silently reactivate from an obsolete backup.
	script, err := os.ReadFile(apply)
	require.NoError(t, err)
	_, err = db.Exec(string(script))
	require.ErrorContains(t, err, "was reverted")
}

func TestRechargeBonusPostgresPermanentConcurrentFulfillmentAndRefund(t *testing.T) {
	db, repo := newRechargeBonusPostgresFixture(t, true)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	rule := rechargeBonusTestRule(t, db, 50, 1.99, 0)
	require.NoError(t, repo.SnapshotOrder(ctx, db, 1, rule))
	var applied atomic.Int32
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repo.GrantForOrder(ctx, 1, 42, now)
			if err != nil {
				errors <- err
				return
			}
			if result.ExpiresAt != nil {
				errors <- fmt.Errorf("permanent expiry is not nil")
				return
			}
			if result.Applied {
				applied.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, applied.Load())
	assertRechargeBonusBalances(t, db, 101.99, 0)
	summary, err := repo.GetUserSummary(ctx, 42, now.AddDate(100, 0, 0))
	require.NoError(t, err)
	require.Equal(t, 1.99, summary.AvailableAmount)
	require.Equal(t, 1, summary.ActiveGrants)
	require.Nil(t, summary.NextExpiresAt)
	expiry, err := repo.ExpireDue(ctx, now.AddDate(100, 0, 0), 500)
	require.NoError(t, err)
	require.Zero(t, expiry.ExpiredGrants)
	// Consuming part of permanent credit still limits what a refund may revoke.
	billing := &usageBillingRepository{db: db, bonus: repo}
	for i := 0; i < 2; i++ {
		result, err := billing.Apply(ctx, &service.UsageBillingCommand{RequestID: "perm-consume", APIKeyID: 7, UserID: 42, BalanceCost: 0.99})
		require.NoError(t, err)
		require.Equal(t, i == 0, result.Applied)
	}
	for i := 0; i < 2; i++ {
		revoked, err := repo.RevokeForRefund(ctx, 1, 42, 50, 50, now)
		require.NoError(t, err)
		if i == 0 {
			require.Equal(t, 1.0, revoked)
		} else {
			require.Zero(t, revoked)
		}
	}
	assertRechargeBonusBalances(t, db, 100, 0)
	var ledgerCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM recharge_bonus_ledger").Scan(&ledgerCount))
	require.Equal(t, 3, ledgerCount) // grant, consume, revoke exactly once
}

func TestRechargeBonusPostgresMixedExpirySummaryAndConsumption(t *testing.T) {
	db, repo := newRechargeBonusPostgresFixture(t, true)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	permanent := grantRechargeBonusTestOrder(t, db, repo, 1, rechargeBonusTestRule(t, db, 5000, 10, 0), now)
	finite := grantRechargeBonusTestOrder(t, db, repo, 2, rechargeBonusTestRule(t, db, 50, 2, 3), now)
	grantRechargeBonusTestOrder(t, db, repo, 3, rechargeBonusTestRule(t, db, 30, 1, 1), now.AddDate(0, 0, -2))
	summary, err := repo.GetUserSummary(ctx, 42, now)
	require.NoError(t, err)
	require.Equal(t, 12.0, summary.AvailableAmount)
	require.Equal(t, 2, summary.ActiveGrants)
	require.True(t, finite.ExpiresAt.Equal(*summary.NextExpiresAt))
	assertRechargeBonusBalances(t, db, 112, 0) // old expired dollar removed once
	billing := &usageBillingRepository{db: db, bonus: repo}
	result, err := billing.Apply(ctx, &service.UsageBillingCommand{RequestID: "mixed-consume", APIKeyID: 7, UserID: 42, BalanceCost: 3})
	require.NoError(t, err)
	require.True(t, result.Applied)
	var finiteRemaining, permanentRemaining float64
	require.NoError(t, db.QueryRow("SELECT remaining_amount FROM recharge_bonus_grants WHERE id=$1", finite.GrantID).Scan(&finiteRemaining))
	require.NoError(t, db.QueryRow("SELECT remaining_amount FROM recharge_bonus_grants WHERE id=$1", permanent.GrantID).Scan(&permanentRemaining))
	require.Zero(t, finiteRemaining)
	require.Equal(t, 9.0, permanentRemaining)
	assertRechargeBonusBalances(t, db, 109, 0)
}

func TestRechargeBonusPostgresBatchCaptureReleaseAndIdempotency(t *testing.T) {
	for _, capture := range []bool{false, true} {
		t.Run(fmt.Sprintf("capture_%v", capture), func(t *testing.T) {
			db, repo := newRechargeBonusPostgresFixture(t, true)
			ctx := context.Background()
			now := time.Now().UTC().Truncate(time.Microsecond)
			grantRechargeBonusTestOrder(t, db, repo, 1, rechargeBonusTestRule(t, db, 5000, 10, 0), now)
			finite := grantRechargeBonusTestOrder(t, db, repo, 2, rechargeBonusTestRule(t, db, 50, 2, 3), now)
			billing := &usageBillingRepository{db: db, bonus: repo}
			batch := "permanent-batch"
			for i := 0; i < 2; i++ {
				result, err := billing.ReserveBatchImageBalance(ctx, &service.BatchImageBalanceHoldCommand{RequestID: service.BatchImageHoldRequestID(batch), APIKeyID: 7, UserID: 42, BatchID: batch, HoldAmount: 5})
				require.NoError(t, err)
				require.Equal(t, i == 0, result.Applied)
			}
			assertRechargeBonusBalances(t, db, 107, 5)
			// Model a finite source expiring while the asynchronous job runs.
			_, err := db.Exec("UPDATE recharge_bonus_grants SET expires_at=$1 WHERE id=$2", now.Add(-time.Hour), finite.GrantID)
			require.NoError(t, err)
			for i := 0; i < 2; i++ {
				cmd := &service.BatchImageBalanceHoldCommand{RequestID: "batch-settle", APIKeyID: 7, UserID: 42, BatchID: batch, HoldAmount: 5}
				var result *service.BatchImageBalanceHoldResult
				if capture {
					cmd.ActualAmount = 1
					result, err = billing.CaptureBatchImageBalance(ctx, cmd)
				} else {
					result, err = billing.ReleaseBatchImageBalance(ctx, cmd)
				}
				require.NoError(t, err)
				require.Equal(t, i == 0, result.Applied)
			}
			assertRechargeBonusBalances(t, db, 110, 0)
			summary, err := repo.GetUserSummary(ctx, 42, now.AddDate(100, 0, 0))
			require.NoError(t, err)
			require.Equal(t, 10.0, summary.AvailableAmount)
			require.Nil(t, summary.NextExpiresAt)
			var restored, expired float64
			require.NoError(t, db.QueryRow("SELECT SUM(restored_amount),SUM(expired_amount) FROM recharge_bonus_batch_holds").Scan(&restored, &expired))
			require.Equal(t, 3.0, restored)
			if capture {
				require.Equal(t, 1.0, expired)
			} else {
				require.Equal(t, 2.0, expired)
			}
		})
	}
}
