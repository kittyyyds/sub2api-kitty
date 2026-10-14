package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskListKeepsOwnerScopeAndSeparatesChargeEvidence(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(map[bool]string{false: "customer", true: "admin"}[admin], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			filter := service.VideoTaskListFilter{UserID: 42, APIKeyID: 9, Status: "done", Admin: admin, Limit: 20, Offset: 0, GroupID: 8, Model: "video-1", StartAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
			// SQL pagination must retain the caller's user/key/status scope; a done task
			// with a dedup but no receipt is returned without an invented cost.
			mock.ExpectQuery(`(?s)SELECT jsonb_build_object.*FROM \(SELECT \* FROM video_tasks WHERE \(\$1::bigint=0 OR user_id=\$1\).*api_key_id=\$2.*status=\$3.*group_id=\$4.*pending->>'model'=\$5.*created_at >= \$6 AND created_at < \$7.*LIMIT \$8 OFFSET \$9`).
				WithArgs(int64(42), int64(9), "done", int64(8), "video-1", filter.StartAt, filter.EndAt, 20, 0).
				WillReturnRows(sqlmock.NewRows([]string{"summary"}).AddRow(`{"id":"local-id","task_id":"remote-id","user_id":42,"api_key_id":9,"account_id":205,"status":"done","billed":true,"actual_cost":null,"usage_id":null}`))
			items, err := (&usageLogRepository{sql: db}).ListVideoTaskSummaries(context.Background(), filter)
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.True(t, items[0].Billed)
			require.Nil(t, items[0].ActualCost)
			if admin {
				require.EqualValues(t, 205, items[0].AccountID)
			} else {
				require.Zero(t, items[0].AccountID)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestVideoTaskStatsScopeAndMissingReceipts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	f := service.VideoTaskListFilter{UserID: 42, APIKeyID: 9, GroupID: 8, Model: "video-' OR 1=1 --", StartAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	mock.ExpectQuery(`(?s)WITH matched AS.*FROM video_tasks WHERE \(\$1::bigint=0 OR user_id=\$1\).*pending->>'model'=\$5.*created_at >= \$6 AND created_at < \$7.*sum\(recorded_cost\) FILTER \(WHERE billed AND usage_recorded\).*GROUPING SETS`).
		WithArgs(int64(42), int64(9), "", int64(8), f.Model, f.StartAt, f.EndAt).
		WillReturnRows(sqlmock.NewRows([]string{"total", "metrics"}).
			AddRow(true, `{"total":4,"succeeded":2,"failed":1,"needs_review":1,"success_rate":66.6667,"actual_cost":3.75,"unreconciled":1,"successful_seconds":10,"average_completion_seconds":60}`).
			AddRow(false, `{"model":"video-1","total":4,"succeeded":2,"actual_cost":3.75,"unreconciled":1}`))
	stats, err := (&usageLogRepository{sql: db}).GetVideoTaskStats(context.Background(), f)
	require.NoError(t, err)
	require.EqualValues(t, 4, stats.Total)
	require.Equal(t, 3.75, stats.ActualCost)
	require.EqualValues(t, 1, stats.Unreconciled)
	require.Len(t, stats.Models, 1)
	require.Equal(t, f.StartAt, stats.StartAt)
	require.NoError(t, mock.ExpectationsWereMet())

	f.UserID = 0
	_, err = (&usageLogRepository{sql: db}).GetVideoTaskStats(context.Background(), f)
	require.Error(t, err)
}

func TestVideoTaskListRejectsUnscopedCustomerBeforeQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageLogRepository{sql: db}
	for _, f := range []service.VideoTaskListFilter{{Limit: 20}, {UserID: 1, Limit: 101}, {Admin: true, Limit: 20, Offset: 10001}} {
		_, err = repo.ListVideoTaskSummaries(context.Background(), f)
		require.Error(t, err)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
