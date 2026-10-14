package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskRepositoryOwnerAndPricingSurviveReload(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageLogRepository{db: db, sql: db}
	pending := service.GrokVideoPendingBilling{Model: "exact-video", VideoDurationSeconds: 30,
		PriceSnapshot: &service.VideoPriceSnapshot{Model: "exact-video", BillingMode: service.BillingModePerRequest, UnitPrice: 1.3, RateMultiplier: 1, DurationSeconds: 30}}
	raw, err := json.Marshal(pending)
	require.NoError(t, err)
	mock.ExpectQuery(`(?s)SELECT .* FROM video_tasks.*WHERE \(upstream_task_id=\$1 OR id=\$1\) AND user_id=\$2 AND api_key_id=\$3`).
		WithArgs("upstream-task", int64(1), int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "upstream_task_id", "user_id", "api_key_id", "group_id", "account_id", "subscription_id", "pending", "status", "attempts", "created_at", "billing_state"}).
			AddRow("local-task", "upstream-task", 1, 6, 25, 205, nil, raw, "pending", 2, time.Now(), "legacy"))
	task, err := repo.FindVideoTask(context.Background(), "upstream-task", 1, 6)
	require.NoError(t, err)
	require.Equal(t, int64(205), task.AccountID)
	require.Equal(t, 30, task.Pending.PriceSnapshot.DurationSeconds)
	require.Equal(t, 1.3, task.Pending.PriceSnapshot.UnitPrice)
	mock.ExpectQuery(`(?s)SELECT .* FROM video_tasks.*WHERE \(upstream_task_id=\$1 OR id=\$1\) AND user_id=\$2 AND api_key_id=\$3`).
		WithArgs("upstream-task", int64(2), int64(6)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	other, err := repo.FindVideoTask(context.Background(), "upstream-task", 2, 6)
	require.NoError(t, err)
	require.Nil(t, other)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVideoTaskRepositoryAcceptanceRequiresExistingSubmission(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageLogRepository{db: db, sql: db}
	mock.ExpectExec(`(?s)UPDATE video_tasks SET upstream_task_id=.*WHERE id=\$1 AND status IN`).
		WithArgs("missing", "remote", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	err = repo.AcceptVideoTask(context.Background(), &service.VideoTask{ID: "missing", UpstreamTaskID: "remote"})
	require.ErrorContains(t, err, "state conflict")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVideoTaskRepositoryLeasesAreRecoverableAndBillingChecksArchive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageLogRepository{db: db, sql: db}
	mock.ExpectQuery(`(?s)WITH due AS .*FOR UPDATE SKIP LOCKED.*UPDATE video_tasks SET next_poll_at=NOW\(\)\+INTERVAL '3 minutes'.*RETURNING`).
		WithArgs(5).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	tasks, err := repo.LeaseVideoTasks(context.Background(), 5)
	require.NoError(t, err)
	require.Empty(t, tasks)
	mock.ExpectQuery(`(?s)SELECT EXISTS.*usage_billing_dedup WHERE request_id=\$1 AND api_key_id=\$2.*UNION ALL SELECT 1 FROM usage_billing_dedup_archive`).
		WithArgs("grok-video:stable", int64(6)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	billed, err := repo.VideoTaskWasBilled(context.Background(), "grok-video:stable", 6)
	require.NoError(t, err)
	require.True(t, billed)
	require.NoError(t, mock.ExpectationsWereMet())
}
