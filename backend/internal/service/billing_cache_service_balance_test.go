//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type balanceEligibilityCacheStub struct {
	billingCacheWorkerStub

	balance                  float64
	cacheMissAfterInvalidate bool
	invalidated              atomic.Bool
	deductCalls              atomic.Int64
	invalidateCalls          atomic.Int64
}

type mediaAdmissionBalanceReader struct {
	view *MediaWalletBalance
	err  error
}

func (r *mediaAdmissionBalanceReader) Balance(context.Context, int64) (*MediaWalletBalance, error) {
	return r.view, r.err
}

func TestMediaBalanceAuthorityRejectsStaleCacheAndQueuedRefill(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 100}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)
	reader := &mediaAdmissionBalanceReader{view: &MediaWalletBalance{Currency: "USD", Denomination: "cny_pegged_site_credit", AvailableUSD: "0.00000000"}}
	require.NoError(t, svc.RequireMediaWalletBalanceAuthority(reader))
	// Simulate a stale fill that survived invalidation and committed after a
	// media hold. It must not restore text admission, even after draining workers.
	require.True(t, svc.enqueueCacheWrite(cacheWriteTask{kind: cacheWriteSetBalance, userID: 1, balance: 100}))
	svc.Stop()
	require.ErrorIs(t, svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, ""), ErrInsufficientBalance)
	reader.view.AvailableUSD = "0.10000000"
	require.NoError(t, svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, ""))
	reader.view.AvailableUSD = "-0.25000000"
	require.ErrorIs(t, svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, ""), ErrInsufficientBalance)
}

func TestMediaBalanceAuthorityFailsClosedWithoutCacheFallback(t *testing.T) {
	cases := []struct {
		name string
		view *MediaWalletBalance
		err  error
	}{
		{name: "database unavailable", err: errors.New("database unavailable")},
		{name: "missing view"},
		{name: "wrong denomination", view: &MediaWalletBalance{Currency: "USD", AvailableUSD: "100"}},
		{name: "invalid amount", view: &MediaWalletBalance{Currency: "USD", Denomination: "cny_pegged_site_credit", AvailableUSD: "NaN"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewBillingCacheService(&balanceEligibilityCacheStub{balance: 100}, nil, nil, nil, nil, nil, &config.Config{}, nil)
			t.Cleanup(svc.Stop)
			require.NoError(t, svc.RequireMediaWalletBalanceAuthority(&mediaAdmissionBalanceReader{view: tt.view, err: tt.err}))
			err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
			require.ErrorIs(t, err, ErrBillingServiceUnavailable)
		})
	}
}

func (s *balanceEligibilityCacheStub) GetUserBalance(context.Context, int64) (float64, error) {
	if s.cacheMissAfterInvalidate && s.invalidated.Load() {
		return 0, errors.New("cache miss")
	}
	return s.balance, nil
}

func (s *balanceEligibilityCacheStub) DeductUserBalance(context.Context, int64, float64) error {
	s.deductCalls.Add(1)
	return nil
}

func (s *balanceEligibilityCacheStub) InvalidateUserBalance(context.Context, int64) error {
	s.invalidateCalls.Add(1)
	s.invalidated.Store(true)
	return nil
}

func TestCheckBillingEligibility_RejectsBalanceBelowMinimumReserve(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 0.005}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestCheckBillingEligibility_AllowsBalanceAtMinimumReserve(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 0.01}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.NoError(t, err)
}

func TestSyncBalanceCacheAfterDeduction_InvalidatesExhaustedBalance(t *testing.T) {
	cache := &balanceEligibilityCacheStub{
		balance:                  0.50,
		cacheMissAfterInvalidate: true,
	}
	userRepo := &balanceLoadUserRepoStub{balance: -0.25}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, userRepo, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	newBalance := -0.25
	syncBalanceCacheAfterDeduction(context.Background(), &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 0.75},
		User: &User{ID: 1},
	}, &billingDeps{billingCacheService: svc}, &UsageBillingApplyResult{
		NewBalance:         &newBalance,
		BalanceOverdrafted: true,
	})

	require.Equal(t, int64(1), cache.invalidateCalls.Load())
	require.Equal(t, int64(0), cache.deductCalls.Load())

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
	require.Equal(t, int64(1), userRepo.calls.Load())
}

func TestSyncBalanceCacheAfterDeduction_InvalidatesWhenBalanceFallsBelowReserve(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 0.50}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	newBalance := 0.005
	syncBalanceCacheAfterDeduction(context.Background(), &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 0.495},
		User: &User{ID: 1},
	}, &billingDeps{billingCacheService: svc}, &UsageBillingApplyResult{NewBalance: &newBalance})

	require.Equal(t, int64(1), cache.invalidateCalls.Load())
	require.Equal(t, int64(0), cache.deductCalls.Load())
}

func TestSyncBalanceCacheAfterDeduction_QueuesDeductWhenBalanceStillEligible(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 1}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	newBalance := 0.75
	syncBalanceCacheAfterDeduction(context.Background(), &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 0.25},
		User: &User{ID: 1},
	}, &billingDeps{billingCacheService: svc}, &UsageBillingApplyResult{NewBalance: &newBalance})

	require.Equal(t, int64(0), cache.invalidateCalls.Load())
	require.Eventually(t, func() bool {
		return cache.deductCalls.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}
