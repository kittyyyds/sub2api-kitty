//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestAccountFeatures24DatabaseIntegration(t *testing.T) {
	ctx := context.Background()
	schema := fmt.Sprintf("account_features24_%d", time.Now().UnixNano())
	_, err := integrationDB.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	defer func() {
		_, e := integrationDB.ExecContext(ctx, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE")
		require.NoError(t, e)
	}()
	parsed, err := url.Parse(integrationDatabaseDSN)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		_, e := db.ExecContext(ctx, q, args...)
		require.NoError(t, e)
	}
	exec(`CREATE TABLE accounts(id bigint PRIMARY KEY,platform text,type text,credentials jsonb DEFAULT '{}',parent_account_id bigint,deleted_at timestamptz);
 INSERT INTO accounts(id,platform,type,credentials,parent_account_id,deleted_at) VALUES
 (1,'openai','oauth','{"plan_type":"Pro_100"}',NULL,NULL),
 (2,'openai','oauth','{}',1,NULL),
 (3,'openai','oauth','{"plan_type":"pro-200"}',NULL,NULL),
 (4,'openai','apikey','{"plan_type":"prolite"}',NULL,NULL),
 (5,'anthropic','oauth','{"plan_type":"prolite"}',NULL,NULL),
 (6,'openai','oauth','{"plan_type":"prolite"}',NULL,NOW()),
 (7,'openai','oauth','{}',6,NULL),
 (8,'openai','oauth','{"plan_type":"Plus"}',1,NULL);`)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()
	planCtx, err := service.WithAccountPlanFilter(ctx, "Pro 100")
	require.NoError(t, err)
	q := applyAccountPlanFilter(planCtx, client.Account.Query())
	total, err := q.Clone().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	var ids []int64
	require.NoError(t, q.Order(dbent.Asc("id")).Select("id").Scan(ctx, &ids))
	require.Equal(t, []int64{1, 2}, ids)
	var paged []int64
	require.NoError(t, applyAccountPlanFilter(planCtx, client.Account.Query()).Order(dbent.Asc("id")).Offset(1).Limit(1).Select("id").Scan(ctx, &paged))
	require.Equal(t, []int64{2}, paged)
	planCtx, err = service.WithAccountPlanFilter(ctx, "plus")
	require.NoError(t, err)
	ids = nil // Ent's Scan appends to a provided slice.
	require.NoError(t, applyAccountPlanFilter(planCtx, client.Account.Query()).Select("id").Scan(ctx, &ids))
	require.Equal(t, []int64{8}, ids)
	_, err = service.WithAccountPlanFilter(ctx, "invalid OR true")
	require.Error(t, err)

	migration, err := dbmigrations.FS.ReadFile("361_account_intelligence_tests.sql")
	require.NoError(t, err)
	exec(string(migration))
	exec(string(migration))
	repo := newAccountRepositoryWithSQL(client, db, nil)
	testIntelligenceScheduleDatabase(t, ctx, db, repo)
	item := &service.AccountIntelligenceTest{ID: uuid.NewString(), RequestID: uuid.NewString(), AccountID: 1, Kind: "candy", Prompt: "historical prompt", ExpectedAnswer: "21", ScoringVersion: "candy-21-v1", Status: "running", CreatedAt: time.Now().UTC()}
	saved, inserted, err := repo.IntelligenceClaim(ctx, item)
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, item.ID, saved.ID)
	_, inserted, err = repo.IntelligenceClaim(ctx, item)
	require.NoError(t, err)
	require.False(t, inserted)
	busy := *item
	busy.ID = uuid.NewString()
	busy.RequestID = uuid.NewString()
	_, _, err = repo.IntelligenceClaim(ctx, &busy)
	require.ErrorIs(t, err, service.ErrAccountIntelligenceBusy)
	item.Status = "incorrect"
	item.Response = "29"
	now := time.Now().UTC()
	item.FinishedAt = &now
	require.NoError(t, repo.IntelligenceFinish(ctx, item))
	second := busy
	second.CreatedAt = now.Add(time.Millisecond)
	_, inserted, err = repo.IntelligenceClaim(ctx, &second)
	require.NoError(t, err)
	require.True(t, inserted)
	second.Status = "correct"
	second.Response = "21"
	second.FinishedAt = &now
	require.NoError(t, repo.IntelligenceFinish(ctx, &second))
	history, err := repo.IntelligenceHistory(ctx, []int64{1, 3}, 1, 0)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, second.ID, history[0].ID)
	history, err = repo.IntelligenceHistory(ctx, []int64{1}, 1, 1)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "historical prompt", history[0].Prompt)
	require.Equal(t, "29", history[0].Response)
	stale := *item
	stale.ID = uuid.NewString()
	stale.RequestID = uuid.NewString()
	stale.Status = "running"
	stale.CreatedAt = now.Add(-7 * time.Minute)
	stale.FinishedAt = nil
	_, _, err = repo.IntelligenceClaim(ctx, &stale)
	require.NoError(t, err)
	expired, err := repo.IntelligenceFind(ctx, 1, stale.RequestID)
	require.NoError(t, err)
	require.Equal(t, "error", expired.Status)
	require.NotNil(t, expired.FinishedAt)
	raw, err := json.Marshal(expired)
	require.NoError(t, err)
	require.Contains(t, string(raw), "candy-21-v1")
	_, inserted, err = repo.IntelligenceClaim(ctx, &busy)
	require.NoError(t, err)
	require.False(t, inserted, "existing request must remain idempotent after finish")
}
