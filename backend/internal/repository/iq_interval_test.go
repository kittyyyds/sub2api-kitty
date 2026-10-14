package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIQIntervalGlobalCapacityAndCompletionCAS(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewScheduledTestPlanRepository(db)
	now := time.Now().Truncate(time.Microsecond)
	p := &service.ScheduledTestPlan{ID: 1, AccountID: 2, UpdatedAt: now, NextRunAt: &now, PelicanConfig: &service.PelicanTestConfig{ScheduleMode: service.IQCompletionInterval, IntervalSeconds: 60}}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT count").WithArgs(now).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))
	mock.ExpectRollback()
	claimed, err := repo.ClaimPelican(context.Background(), p, now, now.Add(time.Minute), now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, claimed)
	until := now.Add(15 * time.Minute)
	finished := now.Add(20 * time.Second)
	mock.ExpectExec(`UPDATE scheduled_test_plans SET running_until=NULL,last_run_at=\$3,next_run_at=CASE WHEN updated_at=\$5 THEN \$4 ELSE next_run_at END WHERE id=\$1 AND running_until=\$2`).WithArgs(p.ID, until, finished, finished.Add(time.Minute), p.UpdatedAt).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.(service.IQIntervalRepository).FinishIQ(context.Background(), p, until, finished))
	require.NoError(t, mock.ExpectationsWereMet())
}
