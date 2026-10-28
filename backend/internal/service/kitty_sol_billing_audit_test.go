//go:build unit

package service

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKittySolGlobalContextWithoutGroupRules(t *testing.T) {
	s := NewBillingService(&config.Config{}, nil)
	s.globalContextRules = contextRulesStub{rule: &GlobalContextRule{ID: 1, Model: "gpt-6.1-sol", Enabled: true, ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 2, CacheReadMultiplier: 2, CacheWriteMultiplier: 2}}
	s.modelBillingSettings = modelBillingSettings(t, DefaultModelBillingConfig())
	p, err := s.GetModelPricing("gpt-6.1-sol")
	require.NoError(t, err)
	gid := int64(2)
	for _, n := range []int{272001, 499999, 500000, 500001} {
		tokens := UsageTokens{InputTokens: n - 200, CacheReadTokens: 100, CacheCreationTokens: 100, OutputTokens: 1000000}
		baseline := s.computeTokenBreakdown(p, tokens, .18, "", false)
		got, err := s.CalculateCostUnified(CostInput{Model: "gpt-6.1-sol", GroupID: &gid, Tokens: tokens, RateMultiplier: .18})
		require.NoError(t, err)
		want := 1.
		if n > 500000 {
			want = 2
		}
		require.InDelta(t, baseline.ActualCost*want, got.ActualCost, 1e-10)
		t.Logf("group_rules_off context=%d multiplier=%.1f", n, got.ActualCost/baseline.ActualCost)
	}
}

// Regression for the live overlapping 500K policies; customer prices must not stack.
func TestKittySolConcurrentPoliciesDoNotStack(t *testing.T) {
	s := NewBillingService(&config.Config{}, nil)
	s.globalContextRules = contextRulesStub{rule: &GlobalContextRule{ID: 1, Model: "gpt-6.1-sol", Enabled: true, ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 2, CacheReadMultiplier: 2, CacheWriteMultiplier: 2}}
	cfg := scopedBillingFixture("gpt-6.1-sol", 2, 2)
	cfg.Rules[0].Mode = ModelBillingContext
	cfg.Rules[0].ThresholdTokens = 500000
	s.modelBillingSettings = modelBillingSettings(t, cfg)
	p, err := s.GetModelPricing("gpt-6.1-sol")
	require.NoError(t, err)
	gid := int64(2)
	for _, n := range []int{500000, 500001} {
		tokens := UsageTokens{InputTokens: n - 200, CacheReadTokens: 100, CacheCreationTokens: 100, OutputTokens: 100}
		base := s.computeTokenBreakdown(p, tokens, .18, "", false)
		cost, err := s.CalculateCostUnified(CostInput{Model: "gpt-6.1-sol", GroupID: &gid, Tokens: tokens, RateMultiplier: .18})
		require.NoError(t, err)
		want := 1.
		if n > 500000 {
			want = 2
		}
		require.InDelta(t, base.ActualCost*want, cost.ActualCost, 1e-10)
		require.Zero(t, cost.ContextRuleID)
		t.Logf("both_rules_on context=%d effective_multiplier=%.1f global_rule=%d group_multiplier=%.1f", n, cost.ActualCost/base.ActualCost, cost.ContextRuleID, costModelBillingMultiplier(cost))
	}
}

func TestKittySolPolicyPrecedenceAndPriceSchedule(t *testing.T) {
	for _, tc := range []struct {
		name, mode   string
		threshold, n int
		enabled      bool
		gid          int64
		want         float64
	}{
		{"group wins below its higher threshold", ModelBillingContext, 600000, 500001, true, 2, 1},
		{"group lower threshold", ModelBillingContext, 50000, 50001, true, 2, 2},
		{"strict group boundary", ModelBillingContext, 50000, 50000, true, 2, 1},
		{"always does not stack", ModelBillingAlways, 0, 500001, true, 2, 2},
		{"disabled rule uses global", ModelBillingContext, 600000, 500001, false, 2, 2},
		{"unmatched group uses global", ModelBillingContext, 600000, 500001, true, 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewBillingService(&config.Config{}, nil)
			s.globalContextRules = contextRulesStub{rule: &GlobalContextRule{ID: 1, Model: "gpt-6.1-sol", Enabled: true, ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 2, CacheReadMultiplier: 2, CacheWriteMultiplier: 2}}
			cfg := scopedBillingFixture("gpt-6.1-sol", 2, 2)
			cfg.Rules[0].Mode = tc.mode
			cfg.Rules[0].ThresholdTokens = tc.threshold
			cfg.Rules[0].Enabled = tc.enabled
			s.modelBillingSettings = modelBillingSettings(t, cfg)
			p, err := s.GetModelPricing("gpt-6.1-sol")
			require.NoError(t, err)
			resolver := NewModelPricingResolver(nil, s)
			group := &Group{ID: tc.gid, Platform: PlatformOpenAI, Hydrated: true, Status: StatusActive, LongContextPricingEnabled: true}
			tokens := UsageTokens{InputTokens: tc.n - 200, CacheReadTokens: 100, CacheCreationTokens: 100, OutputTokens: 1000000}
			for _, fast := range []string{"", "priority"} {
				base := s.computeTokenBreakdown(p, tokens, .18, fast, false)
				in := CostInput{Ctx: t.Context(), Model: "gpt-6.1-sol", Group: group, Tokens: tokens, RateMultiplier: .18, ServiceTier: fast, Resolver: resolver}
				got, err := s.CalculateCostUnified(in)
				require.NoError(t, err)
				require.InDelta(t, base.ActualCost*tc.want, got.ActualCost, 1e-10)
				in.SkipGlobalContextRules = true
				upstream, err := s.CalculateCostUnified(in)
				require.NoError(t, err)
				wantUpstream := s.computeTokenBreakdown(p, tokens, .18, fast, true)
				require.InDelta(t, wantUpstream.ActualCost, upstream.ActualCost, 1e-10)
			}
			schedule, err := s.ResolveContextPricingSchedule(t.Context(), resolver, ContextPricingScheduleInput{Model: "gpt-6.1-sol", Group: group, Platform: PlatformOpenAI})
			require.NoError(t, err)
			if tc.mode == ModelBillingAlways && tc.enabled {
				require.Len(t, schedule.Tiers, 1)
				require.InDelta(t, p.InputPricePerToken*2, *schedule.Tiers[0].Input, 1e-12)
			} else {
				require.Len(t, schedule.Tiers, 2)
				bound := 500000
				if tc.enabled && tc.gid == 2 {
					bound = tc.threshold
				}
				require.Equal(t, bound, *schedule.Tiers[0].MaxTokens)
				require.InDelta(t, p.InputPricePerToken, *schedule.Tiers[0].Input, 1e-12)
				require.InDelta(t, p.InputPricePerToken*2, *schedule.Tiers[1].Input, 1e-12)
			}
			if tc.enabled && tc.gid == 2 {
				// An irrelevant global store must not block a matched scoped policy.
				s.globalContextRules = contextRulesStub{err: errors.New("unavailable")}
				_, err = s.CalculateCostUnified(CostInput{Model: "gpt-6.1-sol", Group: group, Tokens: tokens, RateMultiplier: .18, Resolver: resolver})
				require.NoError(t, err)
				_, err = s.ResolveContextPricingSchedule(t.Context(), resolver, ContextPricingScheduleInput{Model: "gpt-6.1-sol", Group: group, Platform: PlatformOpenAI})
				require.NoError(t, err)
			}
		})
	}
}
