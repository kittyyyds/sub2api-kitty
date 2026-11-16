package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// This opt-in test uses transaction-local temporary tables and never calls a
// video supplier. Point VIDEO_STATS_TEST_DSN at an isolated PostgreSQL instance.
func TestVideoTaskStatsPostgres(t *testing.T) {
	dsn := os.Getenv("VIDEO_STATS_TEST_DSN")
	if dsn == "" {
		t.Skip("VIDEO_STATS_TEST_DSN is required for PostgreSQL aggregation verification")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE video_tasks(id text,upstream_task_id text,user_id bigint,api_key_id bigint,group_id bigint,account_id bigint,pending jsonb,status text,created_at timestamptz,updated_at timestamptz) ON COMMIT DROP;
 CREATE TEMP TABLE usage_logs(id bigint,api_key_id bigint,request_id text,actual_cost numeric,total_cost numeric,rate_multiplier numeric,video_count int,created_at timestamptz) ON COMMIT DROP;
 CREATE TEMP TABLE usage_billing_dedup(api_key_id bigint,request_id text) ON COMMIT DROP;
 CREATE TEMP TABLE usage_billing_dedup_archive(api_key_id bigint,request_id text) ON COMMIT DROP;
 CREATE TEMP TABLE api_keys(id bigint,name text) ON COMMIT DROP;
 CREATE TEMP TABLE groups(id bigint,name text) ON COMMIT DROP;
 INSERT INTO video_tasks SELECT id,id,owner,9,8,3,jsonb_build_object('model',model,'video_duration_seconds',5),state,'2026-09-01T12:00:00Z','2026-09-01T12:01:00Z'
 FROM (VALUES ('done-paid',42,'model-a','done'),('done-missing',42,'model-a','done'),('failed',42,'model-a','failed'),('pending',42,'model-b','pending'),('review',42,'model-b','needs_review'),('foreign',99,'model-a','done'),('free',42,'model-b','done')) AS t(id,owner,model,state);
 INSERT INTO video_tasks VALUES('end-boundary','end-boundary',42,9,8,3,'{"model":"model-a"}','done','2026-09-02T00:00:00Z','2026-09-02T00:01:00Z');
 INSERT INTO usage_logs VALUES(1,9,'grok-video:done-paid',3.75,3.75,1,1,'2026-09-01T12:01:00Z'),(2,9,'grok-video:foreign',99,99,1,1,'2026-09-01T12:01:00Z'),(3,9,'grok-video:free',0,0,1,1,'2026-09-01T12:01:00Z'),(4,9,'grok-video:done-missing',0,10,1,1,'2026-09-01T12:01:00Z');
 INSERT INTO usage_billing_dedup VALUES(9,'grok-video:done-paid'),(9,'grok-video:done-missing'),(9,'grok-video:foreign');
 INSERT INTO usage_billing_dedup_archive VALUES(9,'grok-video:free'),(9,'grok-video:done-paid');`)
	require.NoError(t, err)
	indexMigration, err := os.ReadFile("../../migrations/253_video_task_stats_indexes.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(indexMigration))
	require.NoError(t, err)
	repo := &usageLogRepository{sql: tx}
	f := service.VideoTaskListFilter{UserID: 42, StartAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), Limit: 20}
	stats, err := repo.GetVideoTaskStats(ctx, f)
	require.NoError(t, err)
	require.EqualValues(t, 6, stats.Total)
	require.EqualValues(t, 3, stats.Succeeded)
	require.EqualValues(t, 1, stats.Failed)
	require.EqualValues(t, 1, stats.InProgress)
	require.EqualValues(t, 1, stats.NeedsReview)
	require.InDelta(t, 75, *stats.SuccessRate, 0.001)
	require.EqualValues(t, 15, stats.SuccessfulSeconds)
	require.Equal(t, 3.75, stats.ActualCost)
	require.EqualValues(t, 1, stats.Unreconciled)
	require.Equal(t, 60.0, *stats.AverageCompletionSeconds)
	require.Len(t, stats.Models, 2)
	items, err := repo.ListVideoTaskSummaries(ctx, f)
	require.NoError(t, err)
	require.Len(t, items, int(stats.Total))
	for _, item := range items {
		require.EqualValues(t, 42, item.UserID)
		require.Zero(t, item.AccountID)
	}
	f.Model = "model-b"
	stats, err = repo.GetVideoTaskStats(ctx, f)
	require.NoError(t, err)
	require.EqualValues(t, 3, stats.Total)
	require.Zero(t, stats.ActualCost)
	require.Zero(t, stats.Unreconciled)
	f.Status = "pending"
	stats, err = repo.GetVideoTaskStats(ctx, f)
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.Total)
	require.Nil(t, stats.SuccessRate)
	require.Nil(t, stats.AverageCompletionSeconds)
	f.GroupID = 999
	stats, err = repo.GetVideoTaskStats(ctx, f)
	require.NoError(t, err)
	require.Zero(t, stats.Total)
	require.Empty(t, stats.Models)
	f.Admin, f.UserID, f.GroupID, f.Status, f.Model = true, 0, 0, "", ""
	stats, err = repo.GetVideoTaskStats(ctx, f)
	require.NoError(t, err)
	require.EqualValues(t, 7, stats.Total)
	require.Equal(t, 102.75, stats.ActualCost)
}
