package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func videoBillingReceiptFixture() (*service.UsageBillingCommand, *service.UsageLog) {
	mode, resolution, seconds, group := "video", "720p", 5, int64(25)
	cmd := &service.UsageBillingCommand{RequestID: "grok-video:task", APIKeyID: 166,
		UserID: 1, AccountID: 205, AccountType: "apikey", Model: "exact-video", BalanceCost: 3.75}
	cmd.Normalize()
	log := &service.UsageLog{RequestID: cmd.RequestID, APIKeyID: cmd.APIKeyID,
		UserID: cmd.UserID, AccountID: cmd.AccountID, GroupID: &group, Model: cmd.Model,
		RequestedModel: cmd.Model, TotalCost: 3.75, ActualCost: 3.75, RateMultiplier: 1,
		VideoCount: 1, VideoResolution: &resolution, VideoDurationSeconds: &seconds, BillingMode: &mode}
	return cmd, log
}

func expectVideoClaim(mock sqlmock.Sqlmock, cmd *service.UsageBillingCommand, existing, archived bool) {
	if existing {
		mock.ExpectQuery("INSERT INTO usage_billing_dedup").WithArgs(cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint).WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("SELECT request_fingerprint.*").WithArgs(cmd.RequestID, cmd.APIKeyID).
			WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}).AddRow(cmd.RequestFingerprint))
		return
	}
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").WithArgs(cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	query := mock.ExpectQuery("(?s)SELECT request_fingerprint.*usage_billing_dedup_archive").WithArgs(cmd.RequestID, cmd.APIKeyID)
	if archived {
		query.WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}).AddRow(cmd.RequestFingerprint))
	} else {
		query.WillReturnError(sql.ErrNoRows)
	}
}

func TestVideoUsageBillingChargeAndReceiptAreAtomic(t *testing.T) {
	for _, failReceipt := range []bool{false, true} {
		t.Run(map[bool]string{false: "commits_both", true: "receipt_failure_rolls_back_charge"}[failReceipt], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			cmd, log := videoBillingReceiptFixture()
			chargedAt := time.Date(2026, 9, 20, 14, 38, 31, 0, time.UTC)
			mock.ExpectBegin()
			expectVideoClaim(mock, cmd, false, false)
			mock.ExpectQuery("(?s)UPDATE users.*SET balance = balance -").WithArgs(3.75, int64(1)).
				WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(6.25))
			mock.ExpectQuery("(?s)SELECT MIN\\(created_at\\).*usage_billing_dedup_archive").WithArgs(cmd.RequestID, cmd.APIKeyID).
				WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(chargedAt))
			insert := mock.ExpectQuery("INSERT INTO usage_logs")
			if failReceipt {
				insert.WillReturnError(errors.New("receipt unavailable"))
				mock.ExpectRollback()
			} else {
				insert.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(42, chargedAt))
				mock.ExpectCommit()
			}
			result, err := (&usageBillingRepository{db: db}).ApplyVideoUsage(context.Background(), cmd, log)
			if failReceipt {
				require.ErrorContains(t, err, "receipt unavailable")
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.True(t, result.Applied)
				require.Equal(t, chargedAt, log.CreatedAt)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestVideoUsageBillingRepairsMissingReceiptWithoutChargingAgain(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "hot_dedup", true: "archived_dedup"}[archived], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			cmd, log := videoBillingReceiptFixture()
			chargedAt := time.Date(2026, 9, 20, 14, 38, 31, 0, time.UTC)
			mock.ExpectBegin()
			expectVideoClaim(mock, cmd, !archived, archived)
			mock.ExpectQuery("SELECT MIN").WithArgs(cmd.RequestID, cmd.APIKeyID).
				WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(chargedAt))
			mock.ExpectQuery("INSERT INTO usage_logs").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(42, chargedAt))
			mock.ExpectCommit()
			result, err := (&usageBillingRepository{db: db}).ApplyVideoUsage(context.Background(), cmd, log)
			require.NoError(t, err)
			require.False(t, result.Applied, "no UPDATE users/quotas is permitted by the mock on a replay")
			require.Equal(t, chargedAt, log.CreatedAt)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestVideoUsageBillingCorrectsOnlyMatchingZeroCostPlaceholder(t *testing.T) {
	for _, matches := range []bool{true, false} {
		t.Run(map[bool]string{true: "matching_placeholder", false: "different_paid_receipt"}[matches], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			cmd, log := videoBillingReceiptFixture()
			chargedAt := time.Now().UTC()
			mock.ExpectBegin()
			expectVideoClaim(mock, cmd, true, false)
			mock.ExpectQuery("SELECT MIN").WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(chargedAt))
			mock.ExpectQuery("INSERT INTO usage_logs").WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("SELECT id, created_at FROM usage_logs").WithArgs(cmd.RequestID, cmd.APIKeyID).
				WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(42, chargedAt))
			mock.ExpectExec("(?s)UPDATE usage_logs SET actual_cost=.*actual_cost=0 AND total_cost=\\$9.*user_id=\\$10.*video_count>0.*group_id IS NOT DISTINCT FROM").
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("(?s)SELECT actual_cost=.*FROM usage_logs WHERE").
				WillReturnRows(sqlmock.NewRows([]string{"matches", "created_at"}).AddRow(matches, chargedAt))
			if matches {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			result, err := (&usageBillingRepository{db: db}).ApplyVideoUsage(context.Background(), cmd, log)
			if matches {
				require.NoError(t, err)
				require.False(t, result.Applied)
			} else {
				require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestVideoUsageBillingRejectsUnprovenHistoricalAmount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	cmd, log := videoBillingReceiptFixture()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT request_fingerprint").WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}).AddRow("different-original-bill"))
	mock.ExpectRollback()
	_, err = (&usageBillingRepository{db: db}).ApplyVideoUsage(context.Background(), cmd, log)
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	require.NoError(t, mock.ExpectationsWereMet(), "neither money nor receipt may change on fingerprint mismatch")
}

func TestVideoUsageBillingRetriesUnchargedPlaceholderThenDeduplicates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageBillingRepository{db: db}
	chargedAt := time.Date(2026, 9, 20, 14, 38, 31, 0, time.UTC)
	for _, replay := range []bool{false, true} {
		cmd, log := videoBillingReceiptFixture()
		mock.ExpectBegin()
		expectVideoClaim(mock, cmd, replay, false)
		if !replay {
			mock.ExpectQuery("(?s)UPDATE users.*SET balance = balance -").WithArgs(3.75, int64(1)).
				WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(6.25))
		}
		mock.ExpectQuery("SELECT MIN").WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(chargedAt))
		mock.ExpectQuery("INSERT INTO usage_logs").WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("SELECT id, created_at FROM usage_logs").
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(42, chargedAt.Add(-time.Hour)))
		mock.ExpectExec("(?s)UPDATE usage_logs SET actual_cost=.*created_at=\\$15.*actual_cost=0 AND total_cost=\\$9").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("(?s)SELECT actual_cost=.*FROM usage_logs WHERE").
			WillReturnRows(sqlmock.NewRows([]string{"matches", "created_at"}).AddRow(true, chargedAt))
		mock.ExpectCommit()
		result, err := repo.ApplyVideoUsage(context.Background(), cmd, log)
		require.NoError(t, err)
		require.Equal(t, !replay, result.Applied)
		require.Equal(t, chargedAt, log.CreatedAt)
	}
	require.NoError(t, mock.ExpectationsWereMet(), "the placeholder is repaired, and the second poll has no balance update")
}
