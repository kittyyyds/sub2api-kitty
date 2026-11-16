package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type selectedTodayStatsRepo struct {
	usageBatchLogRepoStub
	ids   []int64
	stats map[int64]*AccountTodayStats
	err   error
}

func (r *selectedTodayStatsRepo) GetAccountTodayStatsBatch(_ context.Context, ids []int64, today time.Time) (map[int64]*AccountTodayStats, error) {
	r.ids = append([]int64(nil), ids...)
	return r.stats, r.err
}

func TestSelectedAccountTodayStatsCombinedReader(t *testing.T) {
	repo := &selectedTodayStatsRepo{stats: map[int64]*AccountTodayStats{7: {
		WindowStats:      WindowStats{Requests: 2, Tokens: 50, Cost: 1.5, UserCost: .3},
		LifetimeRequests: 9, LifetimeTokens: 800, LifetimeCost: 5.5, LifetimeUserCost: 1.1,
	}}}
	svc := &AccountUsageService{usageLogRepo: repo}
	batch, err := svc.GetTodayStatsBatch(context.Background(), []int64{7, 7, 0, -1, 8})
	require.NoError(t, err)
	require.Equal(t, []int64{7, 8}, repo.ids)
	require.Equal(t, int64(9), batch[7].LifetimeRequests)
	require.Equal(t, int64(800), batch[7].LifetimeTokens)
	require.Equal(t, 5.5, batch[7].LifetimeCost)
	require.Equal(t, 1.1, batch[7].LifetimeUserCost)
	require.Equal(t, int64(2), batch[7].Requests)
	require.Equal(t, &AccountTodayStats{}, batch[8])
	single, err := svc.GetTodayStats(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, batch[7], single)
	data, err := json.Marshal(single)
	require.NoError(t, err)
	for _, key := range []string{"requests", "tokens", "cost", "user_cost", "lifetime_requests", "lifetime_tokens", "lifetime_cost", "lifetime_user_cost"} {
		require.Contains(t, string(data), `"`+key+`":`)
	}
}

func TestSelectedAccountTodayStatsFailureIsNotZero(t *testing.T) {
	repo := &selectedTodayStatsRepo{err: errors.New("stats unavailable")}
	svc := &AccountUsageService{usageLogRepo: repo}
	stats, err := svc.GetTodayStatsBatch(context.Background(), []int64{7})
	require.ErrorContains(t, err, "stats unavailable")
	require.Nil(t, stats)
	_, err = svc.GetTodayStats(context.Background(), 7)
	require.ErrorContains(t, err, "stats unavailable")
}

type selectedLegacyTodayStatsRepo struct{ usageBatchLogRepoStub }

func (r *selectedLegacyTodayStatsRepo) GetAccountWindowStats(_ context.Context, _ int64, start time.Time) (*usagestats.AccountStats, error) {
	if start.Equal(time.Unix(0, 0)) {
		return &usagestats.AccountStats{Requests: 20, Tokens: 200, Cost: 5, UserCost: 2}, nil
	}
	return &usagestats.AccountStats{Requests: 3, Tokens: 30, Cost: .5, UserCost: .2}, nil
}

func TestSelectedAccountTodayStatsLegacyReaderIncludesTotals(t *testing.T) {
	svc := &AccountUsageService{usageLogRepo: &selectedLegacyTodayStatsRepo{}}
	stats, err := svc.GetTodayStats(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.Requests)
	require.Equal(t, int64(20), stats.LifetimeRequests)
	require.Equal(t, int64(200), stats.LifetimeTokens)
	require.Equal(t, 5.0, stats.LifetimeCost)
	require.Equal(t, 2.0, stats.LifetimeUserCost)
}
