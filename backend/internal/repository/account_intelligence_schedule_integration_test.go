//go:build integration

package repository

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testIntelligenceScheduleDatabase(t *testing.T, ctx context.Context, db *sql.DB, r *accountRepository) {
	// This helper shares the isolated fixture schema with the older history
	// checks. Remove only its account-3 fixtures before those assertions run.
	defer func() {
		_, e := db.ExecContext(ctx, `DELETE FROM account_intelligence_tests WHERE account_id=3`)
		require.NoError(t, e)
	}()
	_, err := db.ExecContext(ctx, `CREATE TABLE settings(key text PRIMARY KEY,value text NOT NULL,updated_at timestamptz DEFAULT NOW())`)
	require.NoError(t, err)
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	next := now.Add(30 * time.Minute)
	p := &service.AccountIntelligenceSchedule{AccountID: 1, Revision: 1, Enabled: true, Kind: "candy", Model: "gpt-6-astra", CronExpression: "*/30 * * * *", Timezone: "UTC", MaxResults: 2, NextRunAt: &due}
	ok, err := r.IntelligenceScheduleSave(ctx, p, 0)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = r.IntelligenceScheduleSave(ctx, p, 0)
	require.NoError(t, err)
	require.False(t, ok)
	read, err := r.IntelligenceScheduleGet(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, read.Revision)
	var wg sync.WaitGroup
	var mu sync.Mutex
	claims := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, e := r.IntelligenceScheduleClaim(ctx, read, now, next)
			require.NoError(t, e)
			if ok {
				mu.Lock()
				claims++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 1, claims)
	// A second occurrence must still claim after empty last_error fields and
	// PostgreSQL jsonb normalization have round-tripped through text storage.
	read, err = r.IntelligenceScheduleGet(ctx, 1)
	require.NoError(t, err)
	ok, err = r.IntelligenceScheduleClaim(ctx, read, next, next.Add(30*time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
	read.Enabled = false
	read.Revision++
	read.NextRunAt = nil
	ok, err = r.IntelligenceScheduleSave(ctx, read, 1)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = r.IntelligenceScheduleClaim(ctx, p, now, next)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, r.IntelligenceScheduleError(ctx, p, now, "stale execution"))
	read, err = r.IntelligenceScheduleGet(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, read.LastError)
	require.False(t, read.Enabled)
	plans, err := r.IntelligenceSchedules(ctx)
	require.NoError(t, err)
	require.Empty(t, plans)
	for i := 0; i < 5; i++ {
		source := "scheduled"
		if i == 0 {
			source = "manual"
		}
		item := &service.AccountIntelligenceTest{ID: uuid.NewString(), RequestID: uuid.NewString(), AccountID: 3, Source: source, Status: "running", CreatedAt: now.Add(time.Duration(i) * time.Second)}
		_, inserted, e := r.IntelligenceClaim(ctx, item)
		require.NoError(t, e)
		require.True(t, inserted)
		item.Status = "correct"
		require.NoError(t, r.IntelligenceFinish(ctx, item))
	}
	require.NoError(t, r.IntelligencePruneScheduled(ctx, 3, 2))
	history, err := r.IntelligenceHistory(ctx, []int64{3}, 100, 0)
	require.NoError(t, err)
	require.Len(t, history, 3)
	manual := 0
	for _, v := range history {
		if v.Source == "manual" {
			manual++
		}
	}
	require.Equal(t, 1, manual)
}
