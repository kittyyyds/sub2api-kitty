//go:build video_postgres

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestKittyV219UsageAuditAndQueueTimingPostgresRoundTrip(t *testing.T) {
	db, _ := newVideoUsagePostgresFixture(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	wait, duration, first := 125, 500, 100
	serverFirst, forwardFirst := int64(225), int64(100)
	audit := &service.UsageAudit{Version: 1, Status: "observed", Protocol: "responses", Reasons: []string{"isolated_release_test"}}
	timing := &service.QueueTiming{Status: "measured", Source: "http_ingress_responses", Scope: "pre_final_forward", ServerFirstTokenMs: &serverFirst, ServerDurationMs: 625, ForwardFirstTokenMs: &forwardFirst, ForwardDurationMs: 500}
	for _, batched := range []bool{false, true} {
		row := &service.UsageLog{UserID: 1, APIKeyID: 166, AccountID: 205, Model: "isolated-release-fixture", RequestedModel: "isolated-release-fixture", InputTokens: 3, OutputTokens: 4, TotalCost: 0.125, ActualCost: 0.0875, RateMultiplier: 0.7, Stream: true, DurationMs: &duration, FirstTokenMs: &first, QueueWaitMs: &wait, QueueTiming: timing, UsageAudit: audit, CreatedAt: time.Now().UTC()}
		row.RequestID = "isolated-v219-single"
		var inserted bool
		var err error
		if batched {
			row.RequestID = "isolated-v219-batched"
			inserted, err = repo.Create(ctx, row)
		} else {
			inserted, err = repo.createSingle(ctx, db, row)
		}
		require.NoError(t, err)
		require.True(t, inserted)
		saved, err := repo.GetByID(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, row.QueueWaitMs, saved.QueueWaitMs)
		require.Equal(t, timing, saved.QueueTiming)
		require.Equal(t, audit, saved.UsageAudit)
		require.Equal(t, row.TotalCost, saved.TotalCost)
		require.Equal(t, row.ActualCost, saved.ActualCost)
		require.Equal(t, 3, saved.InputTokens)
		require.Equal(t, 4, saved.OutputTokens)
		if batched {
			inserted, err = repo.Create(ctx, row)
			require.NoError(t, err)
			require.False(t, inserted)
		}
	}
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM usage_logs").Scan(&count))
	require.Equal(t, 2, count)
}
