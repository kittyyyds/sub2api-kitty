//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestRechargeBonusGrantForOrderIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newRechargeBonusRepository(db)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT rule_id, bonus_amount::double precision, validity_days`).
		WithArgs(int64(88)).
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "bonus_amount", "validity_days"}).AddRow(3, 5.0, 10))
	mock.ExpectQuery(`INSERT INTO recharge_bonus_grants`).
		WithArgs(int64(42), int64(88), int64(3), 5.0, now.AddDate(0, 0, 10)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(901))
	mock.ExpectQuery(`UPDATE users\s+SET balance = balance \+ \$1`).
		WithArgs(5.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(105.0))
	mock.ExpectExec(`INSERT INTO recharge_bonus_ledger`).
		WithArgs(int64(901), int64(42), "payment:88", 5.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	result, err := repo.GrantForOrder(ctx, 88, 42, now)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.InDelta(t, 5, result.BonusAmount, 0.000001)
	require.InDelta(t, 105, result.NewBalance, 0.000001)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT rule_id, bonus_amount::double precision, validity_days`).
		WithArgs(int64(88)).
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "bonus_amount", "validity_days"}).AddRow(3, 5.0, 10))
	mock.ExpectQuery(`INSERT INTO recharge_bonus_grants`).
		WithArgs(int64(42), int64(88), int64(3), 5.0, now.AddDate(0, 0, 10)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT id, granted_amount::double precision, remaining_amount::double precision, expires_at`).
		WithArgs(int64(88)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "granted_amount", "remaining_amount", "expires_at"}).
			AddRow(901, 5.0, 5.0, now.AddDate(0, 0, 10)))
	mock.ExpectRollback()

	result, err = repo.GrantForOrder(ctx, 88, 42, now)
	require.NoError(t, err)
	require.False(t, result.Applied)
	require.Equal(t, int64(901), result.GrantID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRechargeBonusSnapshotAcceptsPermanentButRejectsNegativeDuration(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newRechargeBonusRepository(db)
	rule := &service.RechargeBonusRule{ID: 3, RechargeAmount: 50, BonusAmount: 1.99, ValidityDays: 0}
	mock.ExpectExec(`INSERT INTO recharge_bonus_order_snapshots`).
		WithArgs(int64(88), int64(3), 50.0, 1.99, 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.SnapshotOrder(ctx, db, 88, rule))
	rule.ValidityDays = -1
	require.NoError(t, repo.SnapshotOrder(ctx, db, 89, rule))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRechargeBonusPermanentGrantAndDuplicateReadNullExpiry(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newRechargeBonusRepository(db)
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT rule_id, bonus_amount::double precision, validity_days`).
			WithArgs(int64(88)).
			WillReturnRows(sqlmock.NewRows([]string{"rule_id", "bonus_amount", "validity_days"}).AddRow(3, 1.99, 0))
		insert := mock.ExpectQuery(`INSERT INTO recharge_bonus_grants`).
			WithArgs(int64(42), int64(88), int64(3), 1.99, nil)
		if i == 0 {
			insert.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(901))
			mock.ExpectQuery(`UPDATE users\s+SET balance = balance \+ \$1`).
				WithArgs(1.99, int64(42)).
				WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(101.99))
			mock.ExpectExec(`INSERT INTO recharge_bonus_ledger`).
				WithArgs(int64(901), int64(42), "payment:88", 1.99).
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
		} else {
			insert.WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`SELECT id, granted_amount::double precision, remaining_amount::double precision, expires_at`).
				WithArgs(int64(88)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "granted_amount", "remaining_amount", "expires_at"}).AddRow(901, 1.99, 1.99, nil))
			mock.ExpectRollback()
		}
		result, err := repo.GrantForOrder(ctx, 88, 42, now)
		require.NoError(t, err)
		require.Equal(t, i == 0, result.Applied)
		require.Nil(t, result.ExpiresAt)
		require.Equal(t, 1.99, result.BonusAmount)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRechargeBonusConsumeUsesEarliestExpiryFirst(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newRechargeBonusRepository(db)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT id, remaining_amount::double precision\s+FROM recharge_bonus_grants\s+WHERE user_id = \$1.*expires_at <= \$2`).
		WithArgs(int64(42), now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "remaining_amount"}))
	mock.ExpectQuery(`SELECT id, remaining_amount::double precision\s+FROM recharge_bonus_grants\s+WHERE user_id = \$1.*expires_at > \$2`).
		WithArgs(int64(42), now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "remaining_amount"}).AddRow(1, 2.0).AddRow(2, 5.0))
	mock.ExpectExec(`UPDATE recharge_bonus_grants\s+SET remaining_amount = \$1, status = \$2`).
		WithArgs(0.0, service.RechargeBonusStatusExhausted, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO recharge_bonus_ledger`).
		WithArgs(int64(1), int64(42), "request-1", 2.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE recharge_bonus_grants\s+SET remaining_amount = \$1, status = \$2`).
		WithArgs(4.0, service.RechargeBonusStatusActive, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO recharge_bonus_ledger`).
		WithArgs(int64(2), int64(42), "request-1", 1.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	consumed, expired, err := repo.consumeInTx(ctx, tx, 42, "request-1", 3, now)
	require.NoError(t, err)
	require.InDelta(t, 3, consumed, 0.000001)
	require.Zero(t, expired)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRechargeBonusExpiryRemovesAvailableBalance(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newRechargeBonusRepository(db)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, remaining_amount::double precision`).
		WithArgs(now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "remaining_amount"}).AddRow(9, 42, 2.5))
	mock.ExpectExec(`UPDATE recharge_bonus_grants\s+SET remaining_amount = 0, status = 'expired'`).
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET balance = balance - \$1`).
		WithArgs(2.5, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO recharge_bonus_ledger`).
		WithArgs(int64(9), int64(42), "expire:9", 2.5).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	result, err := repo.ExpireDue(ctx, now, 500)
	require.NoError(t, err)
	require.Equal(t, 1, result.ExpiredGrants)
	require.Equal(t, []int64{42}, result.UserIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRechargeBonusRefundRevokesOnlyRemainingProratedCredit(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newRechargeBonusRepository(db)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, granted_amount::double precision, remaining_amount::double precision`).
		WithArgs(int64(88), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "granted_amount", "remaining_amount"}).AddRow(9, 10.0, 3.0))
	mock.ExpectQuery(`INSERT INTO recharge_bonus_ledger`).
		WithArgs(int64(9), int64(42), "refund:88", 3.0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(101))
	mock.ExpectExec(`UPDATE recharge_bonus_grants\s+SET remaining_amount = \$1, revoked_amount = revoked_amount \+ \$2`).
		WithArgs(0.0, 3.0, service.RechargeBonusStatusRevoked, now, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET balance = balance - \$1`).
		WithArgs(3.0, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	revoked, err := repo.RevokeForRefund(ctx, 88, 42, 50, 100, now)
	require.NoError(t, err)
	require.InDelta(t, 3, revoked, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchImageBonusCaptureRestoresOnlyUnusedValidCredit(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	bonusRepo := newRechargeBonusRepository(db)
	repo := &usageBillingRepository{db: db, bonus: bonusRepo}
	now := time.Now().UTC()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT id, remaining_amount::double precision.*expires_at <= \$2`).
		WithArgs(int64(42), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "remaining_amount"}))
	mock.ExpectQuery(`SELECT id, remaining_amount::double precision.*expires_at > \$2`).
		WithArgs(int64(42), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "remaining_amount"}).AddRow(7, 3.0))
	mock.ExpectExec(`UPDATE recharge_bonus_grants\s+SET remaining_amount = \$1, status = \$2`).
		WithArgs(0.0, service.RechargeBonusStatusExhausted, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO recharge_bonus_batch_holds`).
		WithArgs("batch-1", int64(42), int64(7), 3.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(reserveBatchImageHoldSQL).
		WithArgs(5.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "frozen_balance"}).AddRow(95.0, 5.0))

	_, err = repo.reserveUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{
		UserID: 42, BatchID: "batch-1", HoldAmount: 5,
	})
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT h.id, h.grant_id, h.held_amount::double precision, g.expires_at`).
		WithArgs("batch-1", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "grant_id", "held_amount", "expires_at"}).
			AddRow(11, 7, 3.0, now.Add(time.Hour)))
	mock.ExpectExec(`UPDATE recharge_bonus_grants\s+SET remaining_amount = remaining_amount \+ \$1`).
		WithArgs(2.0, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO recharge_bonus_ledger`).
		WithArgs(int64(7), int64(42), "batch:batch-1:capture", 1.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE recharge_bonus_batch_holds`).
		WithArgs(1.0, 2.0, 0.0, sqlmock.AnyArg(), int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(captureBatchImageHoldSQL).
		WithArgs(4.0, 5.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "frozen_balance"}).AddRow(99.0, 0.0))
	mock.ExpectCommit()

	result, err := repo.captureUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{
		UserID: 42, BatchID: "batch-1", HoldAmount: 5, ActualAmount: 1,
	})
	require.NoError(t, err)
	require.InDelta(t, 99, *result.NewBalance, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchImageBonusSettlementDoesNotRestoreExpiredCredit(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newRechargeBonusRepository(db)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT h.id, h.grant_id, h.held_amount::double precision, g.expires_at`).
		WithArgs("batch-expired", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "grant_id", "held_amount", "expires_at"}).
			AddRow(12, 8, 2.0, now.Add(-time.Minute)))
	mock.ExpectExec(`INSERT INTO recharge_bonus_ledger`).
		WithArgs(int64(8), int64(42), "batch:batch-expired:expire", 2.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE recharge_bonus_batch_holds`).
		WithArgs(0.0, 0.0, 2.0, now, int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.settleBatchInTx(ctx, tx, 42, "batch-expired", 0, now)
	require.NoError(t, err)
	require.Zero(t, result.RestoredAmount)
	require.InDelta(t, 2, result.ExpiredAmount, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
