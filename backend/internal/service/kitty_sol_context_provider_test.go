//go:build unit

package service

import (
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestKittySolProductionProviderRestoresOnlyApprovedRule(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	data := sqlmock.NewRows(strings.Split(globalContextColumns, ","))
	data.AddRow(1, "gpt-6.1-sol", "openai", 500000, 2., 2., 2., 2., true, 1, time.Now())
	data.AddRow(2, "gpt-5.6-sol", "openai", 1000, 9., 9., 9., 9., true, 1, time.Now())
	mock.ExpectQuery("SELECT .* FROM global_context_pricing_rules ORDER BY platform,model").WillReturnRows(data)
	cfg := scopedBillingFixture("gpt-6.1-sol", 2, 2)
	cfg.Rules[0].Mode = ModelBillingContext
	cfg.Rules[0].ThresholdTokens = 500000
	second := cfg.Rules[0]
	second.GroupID = 4
	cfg.Rules = append(cfg.Rules, second)
	svc, err := ProvideBillingService(&config.Config{}, nil, modelBillingSettings(t, cfg), db)
	require.NoError(t, err)
	require.NotNil(t, svc.globalContextRules)
	// The retired administration interface and unrelated rules stay retired.
	require.Nil(t, svc.GlobalContextRules())
	other, err := svc.globalContextRules.Find(t.Context(), "gpt-5.6-sol")
	require.NoError(t, err)
	require.Nil(t, other)
	p, err := svc.GetModelPricing("gpt-6.1-sol")
	require.NoError(t, err)
	resolver := NewModelPricingResolver(nil, svc)
	for _, id := range []int64{2, 4, 44} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("group=%d/native=%t", id, enabled), func(t *testing.T) {
				g := &Group{ID: id, Platform: PlatformOpenAI, Hydrated: true, Status: StatusActive, LongContextPricingEnabled: enabled}
				schedule, e := svc.ResolveContextPricingSchedule(t.Context(), resolver, ContextPricingScheduleInput{Model: "gpt-6.1-sol", Group: g, Platform: PlatformOpenAI})
				require.NoError(t, e)
				require.Len(t, schedule.Tiers, 2)
				require.Equal(t, 500000, *schedule.Tiers[0].MaxTokens)
				require.Equal(t, "≤500K", schedule.Tiers[0].Label)
				require.Equal(t, ">500K", schedule.Tiers[1].Label)
				require.InDelta(t, *schedule.Tiers[0].Input*2, *schedule.Tiers[1].Input, 1e-12)
				require.InDelta(t, *schedule.Tiers[0].Output*2, *schedule.Tiers[1].Output, 1e-12)
				for _, n := range []int{272001, 499999, 500000, 500001} {
					tokens := UsageTokens{InputTokens: n - 200, CacheReadTokens: 100, CacheCreationTokens: 100, OutputTokens: 1000000}
					base := svc.computeTokenBreakdown(p, tokens, .18, "", false)
					got, e := svc.CalculateTokenCostForRequest(TokenCostRequest{Ctx: t.Context(), Model: "gpt-6.1-sol", Group: g, Tokens: tokens, RateMultiplier: .18, Resolver: resolver})
					require.NoError(t, e)
					multiplier := 1.
					if n > 500000 {
						multiplier = 2
					}
					require.InDelta(t, base.ActualCost*multiplier, got.ActualCost, 1e-10)
					if id == 44 {
						require.Equal(t, int64(1), got.ContextRuleID)
					} else {
						require.Zero(t, got.ContextRuleID)
					}
				}
			})
		}
	}
	// Customer 500K policy must not rewrite the independent catalog reference.
	official, err := svc.ResolveContextPricingSchedule(t.Context(), resolver, ContextPricingScheduleInput{Model: "gpt-6.1-sol"})
	require.NoError(t, err)
	require.Len(t, official.Tiers, 2)
	require.Equal(t, 272000, *official.Tiers[0].MaxTokens)
	require.InDelta(t, *official.Tiers[0].Output*1.5, *official.Tiers[1].Output, 1e-12)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestKittySolProductionProviderRefusesUnverifiedRuleSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT .* FROM global_context_pricing_rules ORDER BY platform,model").WillReturnError(errors.New("unavailable"))
	svc, err := ProvideBillingService(&config.Config{}, nil, nil, db)
	require.Error(t, err)
	require.Nil(t, svc)
	require.NoError(t, mock.ExpectationsWereMet())
}
