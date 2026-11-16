//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

type contextRulesStub struct {
	rule *GlobalContextRule
	err  error
}

func (s contextRulesStub) Find(context.Context, string) (*GlobalContextRule, error) {
	return s.rule, s.err
}
func TestGlobalContextStrictBoundaryAndIndependentItems(t *testing.T) {
	bs := NewBillingService(&config.Config{}, nil)
	bs.fallbackPrices["gpt-5.4"] = &ModelPricing{InputPricePerToken: 1, OutputPricePerToken: 2, CacheReadPricePerToken: 3, CacheCreationPricePerToken: 4, LongContextInputThreshold: 10, LongContextInputMultiplier: 9, LongContextOutputMultiplier: 9}
	rule := &GlobalContextRule{ID: 7, Revision: 2, Model: "gpt-5.4", Platform: "openai", Enabled: true, ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 1.5, CacheReadMultiplier: 3, CacheWriteMultiplier: 4}
	bs.globalContextRules = contextRulesStub{rule: rule}
	for _, n := range []int{499999, 500000, 500001} {
		tokens := UsageTokens{InputTokens: n - 200, CacheReadTokens: 100, CacheCreationTokens: 100, OutputTokens: 1000000}
		cost, err := bs.CalculateCostUnified(CostInput{Model: "gpt-5.4", Tokens: tokens, RateMultiplier: 0.2})
		require.NoError(t, err)
		m := 1.
		if n > 500000 {
			m = 2
		}
		require.Equal(t, float64(n-200)*m, cost.InputCost)
		if n > 500000 {
			require.Equal(t, 3000000., cost.OutputCost)
			require.Equal(t, 900., cost.CacheReadCost)
			require.Equal(t, 1600., cost.CacheCreationCost)
		} else {
			require.Equal(t, 2000000., cost.OutputCost)
		}
		require.InDelta(t, cost.TotalCost*0.2, cost.ActualCost, 1e-8)
	}
	// Customer policy must never change upstream/account statistics.
	with, err := bs.CalculateCostUnified(CostInput{Model: "gpt-5.4", Tokens: UsageTokens{InputTokens: 500001}, RateMultiplier: 1, SkipGlobalContextRules: true})
	require.NoError(t, err)
	require.Equal(t, 500001.*9, with.InputCost)
	// Rule read failures must not silently charge the old policy.
	bs.globalContextRules = contextRulesStub{err: errors.New("offline")}
	_, err = bs.CalculateCostUnified(CostInput{Model: "gpt-5.4"})
	require.Error(t, err)
}
func TestGlobalContextOverridesIntervalsEvenWhenGroupDisabled(t *testing.T) {
	base, high := 1e-6, 100e-6
	bs, resolver := newTokenCostTestEnv(t, PlatformOpenAI, []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-5.4"}, BillingMode: BillingModeToken, InputPrice: &base, Intervals: []PricingInterval{{MinTokens: 1000, InputPrice: &high}}}}, nil)
	bs.globalContextRules = contextRulesStub{rule: &GlobalContextRule{Enabled: true, ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 2, CacheReadMultiplier: 2, CacheWriteMultiplier: 2}}
	group := &Group{ID: 100, Platform: PlatformOpenAI}
	for _, n := range []int{500000, 500001} {
		got, err := bs.CalculateTokenCostForRequest(TokenCostRequest{Model: "gpt-5.4", Group: group, Resolver: resolver, Tokens: UsageTokens{InputTokens: n}, RateMultiplier: 1})
		require.NoError(t, err)
		m := 1.
		if n > 500000 {
			m = 2
		}
		require.InDelta(t, float64(n)*base*m, got.InputCost, 1e-8)
	}
	schedule, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{Model: "gpt-5.4", Group: group, Platform: PlatformOpenAI})
	require.NoError(t, err)
	require.Len(t, schedule.Tiers, 2)
	require.Equal(t, 500000, *schedule.Tiers[0].MaxTokens)
}

func TestGlobalContextCacheBreakdownThreshold(t *testing.T) {
	rule := &GlobalContextRule{ID: 1, ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 2, CacheReadMultiplier: 2, CacheWriteMultiplier: 2}
	for _, tc := range []struct {
		t    UsageTokens
		want float64
	}{
		{UsageTokens{InputTokens: 499999, CacheCreation5mTokens: 1}, 1},
		{UsageTokens{InputTokens: 499999, CacheCreation5mTokens: 1, CacheCreation1hTokens: 1}, 2},
		{UsageTokens{InputTokens: 499999, CacheCreationTokens: 1, CacheCreation5mTokens: 10}, 1},
	} {
		b := &CostBreakdown{InputCost: 1, TotalCost: 1, ActualCost: 1}
		applyGlobalContextRule(b, rule, tc.t, 1)
		require.Equal(t, tc.want, b.InputCost)
	}
}
