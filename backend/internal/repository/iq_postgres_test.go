//go:build unit && linux

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIQIntervalPostgresLeaseAndHistory(t *testing.T) {
	dsn := os.Getenv("KITTY_ISOLATED_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated fixture only")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	var name string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
	require.Equal(t, "fixture", name)
	schema := fmt.Sprintf("iq_fixture_%d", time.Now().UnixNano())
	_, err = db.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	isolated, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	defer isolated.Close()
	isolated.SetMaxOpenConns(16)
	_, err = isolated.Exec(`CREATE TABLE accounts(id BIGINT PRIMARY KEY,deleted_at TIMESTAMPTZ,type TEXT);
 CREATE TABLE scheduled_test_plans(LIKE public.scheduled_test_plans INCLUDING DEFAULTS INCLUDING GENERATED);
 CREATE TABLE scheduled_test_results(LIKE public.scheduled_test_results INCLUDING DEFAULTS INCLUDING GENERATED);
 INSERT INTO accounts SELECT generate_series(100,109),NULL,'apikey';`)
	require.NoError(t, err)
	repo := NewScheduledTestPlanRepository(isolated)
	iq := repo.(service.IQIntervalRepository)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	due := now.Add(-time.Minute)
	until := now.Add(10 * time.Minute)
	plans := make([]*service.ScheduledTestPlan, 10)
	for i := range plans {
		plans[i], err = repo.Create(ctx, &service.ScheduledTestPlan{AccountID: int64(100 + i), ModelID: "fixture", CronExpression: "* * * * *", Enabled: true, MaxResults: 20, NextRunAt: &due, PelicanConfig: &service.PelicanTestConfig{ScheduleMode: service.IQCompletionInterval, IntervalSeconds: 60}})
		require.NoError(t, err)
	}
	var claimed atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errors := make(chan error, 10)
	for _, p := range plans {
		wg.Add(1)
		go func(p *service.ScheduledTestPlan) {
			defer wg.Done()
			<-start
			ok, e := repo.ClaimPelican(ctx, p, now, until, until)
			if e != nil {
				errors <- e
			}
			if ok {
				claimed.Add(1)
			}
		}(p)
	}
	close(start)
	wg.Wait()
	close(errors)
	for e := range errors {
		require.NoError(t, e)
	}
	require.Equal(t, int32(8), claimed.Load())
	var winner *service.ScheduledTestPlan
	for _, p := range plans {
		saved, e := repo.GetByID(ctx, p.ID)
		require.NoError(t, e)
		if saved.RunningUntil != nil {
			winner = p
			break
		}
	}
	require.NotNil(t, winner)
	finished := now.Add(20 * time.Second)
	require.NoError(t, iq.FinishIQ(ctx, winner, until, finished))
	saved, err := repo.GetByID(ctx, winner.ID)
	require.NoError(t, err)
	require.Nil(t, saved.RunningUntil)
	require.True(t, saved.NextRunAt.Equal(finished.Add(time.Minute)))
	// An old completion must not release a newly assigned lease.
	newLease := until.Add(time.Minute)
	_, err = isolated.Exec("UPDATE scheduled_test_plans SET running_until=$2 WHERE id=$1", winner.ID, newLease)
	require.NoError(t, err)
	require.NoError(t, iq.FinishIQ(ctx, winner, until, finished))
	saved, err = repo.GetByID(ctx, winner.ID)
	require.NoError(t, err)
	require.True(t, saved.RunningUntil.Equal(newLease))
	// Active account leases must be omitted from the due queue.
	rows, err := iq.ListDueIQ(ctx, now)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	resultRepo := NewScheduledTestResultRepository(isolated)
	svc := service.NewScheduledTestService(repo, resultRepo)
	for i := 0; i < 25; i++ {
		old := now.Add(-10 * 24 * time.Hour)
		require.NoError(t, svc.SaveResult(ctx, winner.ID, 20, &service.ScheduledTestResult{Status: "success", ResponseText: "21", StartedAt: old, FinishedAt: old, PelicanConfig: &service.PelicanTestConfig{IQVerdict: "correct", ModelID: "fixture"}}))
	}
	_, err = isolated.Exec("UPDATE scheduled_test_results SET created_at=now()-interval '10 days'")
	require.NoError(t, err)
	require.NoError(t, resultRepo.PruneExpiredPelican(ctx, now.Add(-7*24*time.Hour)))
	summaries, err := iq.IQSummaries(ctx, []int64{winner.AccountID})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Len(t, summaries[0].Results, 20)
}
