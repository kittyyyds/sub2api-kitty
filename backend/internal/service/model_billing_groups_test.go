//go:build unit

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestModelBillingGroupOpenAIRecordUsage(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		for _, groupID := range []int64{5, 6, 0} {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			users := &openAIRecordUsageUserRepoStub{}
			subs := &modelBillingSubscriptionRepo{}
			svc := newOpenAIRecordUsageServiceForTest(logs, users, subs, nil)
			svc.StopOpenAICodexTicketHarvester()
			svc.settingService = modelBillingSettings(t, ModelBillingConfig{SchemaVersion: 2, Enabled: true, Rules: []ModelBillingRule{{Model: "gpt-6-luna*", Multiplier: 10, GroupID: 5, Enabled: true, Mode: ModelBillingAlways}}})
			accountRate, accountGroupRate := .3, 2.0
			group := &Group{ID: groupID, RateMultiplier: .2}
			key := &APIKey{ID: 1, GroupID: &group.ID, Group: group}
			if groupID == 0 {
				key.GroupID, key.Group = nil, nil
			}
			input := &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{Model: "gpt-6-luna", Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 200}, SearchCount: 2, Duration: time.Second},
				APIKey: key, User: &User{ID: 3},
				Account: &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{5, 6}, RateMultiplier: &accountRate, GroupRateMultiplier: &accountGroupRate},
			}
			if subscription {
				group.SubscriptionType = SubscriptionTypeSubscription
				input.Subscription = &UserSubscription{ID: 8}
			}
			rate := .2 * accountGroupRate
			if groupID == 0 {
				rate = svc.cfg.Default.RateMultiplier * accountGroupRate
			}
			base := expectedOpenAICost(t, svc, "gpt-6-luna", input.Result.Usage, rate)
			search := svc.billingService.CalculateSearchCost(2, nil, rate)
			surcharge := 1.0
			if groupID == 5 {
				surcharge = 10
			}
			require.NoError(t, svc.RecordUsage(t.Context(), input))
			want := base.ActualCost*surcharge + search.ActualCost
			require.InDelta(t, want, logs.lastLog.ActualCost, 1e-12)
			require.InDelta(t, base.TotalCost+search.TotalCost, logs.lastLog.TotalCost, 1e-12)
			require.Equal(t, accountRate, *logs.lastLog.AccountRateMultiplier)
			require.Equal(t, key.GroupID, logs.lastLog.GroupID)
			require.InDelta(t, rate*surcharge, logs.lastLog.RateMultiplier, 1e-12)
			if subscription && groupID != 0 {
				require.InDelta(t, want, subs.lastAmount, 1e-12)
			} else {
				require.InDelta(t, want, users.lastAmount, 1e-12)
			}
		}
	}
}

func TestModelBillingGroupGenericSearchAndPerRequest(t *testing.T) {
	svc := &GatewayService{billingService: NewBillingService(nil, nil)}
	result := &ForwardResult{Model: "claude-sonnet-4-5", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 100}, SearchCount: 2}
	for _, groupID := range []int64{5, 6, 0} {
		key := &APIKey{ID: 1}
		if groupID != 0 {
			key.GroupID = &groupID
			key.Group = &Group{ID: groupID}
		}
		svc.settingService = nil
		base := svc.calculateRecordUsageCost(t.Context(), result, key, result.Model, result.Model, .2, .2, time.Time{})
		svc.settingService = modelBillingSettings(t, ModelBillingConfig{SchemaVersion: 2, Enabled: true, Rules: []ModelBillingRule{{Model: "claude-sonnet-4-5*", Multiplier: 10, GroupID: 5, Enabled: true, Mode: ModelBillingAlways}}})
		cost := svc.calculateRecordUsageCost(t.Context(), result, key, result.Model, result.Model, .2, .2, time.Time{})
		search := svc.billingService.CalculateSearchCost(2, nil, .2)
		multiplier := 1.0
		if groupID == 5 {
			multiplier = 10
		}
		require.InDelta(t, (base.ActualCost-search.ActualCost)*multiplier+search.ActualCost, cost.ActualCost, 1e-12)
		require.Equal(t, base.TotalCost, cost.TotalCost)
		perRequest := &CostBreakdown{ActualCost: 1, TotalCost: 1, BillingMode: string(BillingModePerRequest)}
		applyModelBillingMultiplier(perRequest, svc.settingService.modelBillingConfigForUsage(t.Context()), groupID, result.Model, UsageTokens{})
		require.Equal(t, 1.0, perRequest.ActualCost)
	}
}

func TestModelBillingGroupGenericRecordUsageCostsAndIdempotency(t *testing.T) {
	for _, groupID := range []int64{5, 6} {
		logs := &openAIRecordUsageLogRepoStub{inserted: true}
		billing := &simpleModeUsageBillingRepoStub{}
		cfg := &config.Config{}
		svc := &GatewayService{cfg: cfg, billingService: NewBillingService(cfg, nil), usageLogRepo: logs, usageBillingRepo: billing, deferredService: &DeferredService{}, billingCacheService: &BillingCacheService{}}
		svc.settingService = modelBillingSettings(t, ModelBillingConfig{SchemaVersion: 2, Enabled: true, Rules: []ModelBillingRule{{Model: "claude-sonnet-4-5*", Multiplier: 10, GroupID: 5, Enabled: true, Mode: ModelBillingAlways}}})
		group := newPeakGroup(true, "14:00", "18:00", 3)
		group.ID, group.RateMultiplier = groupID, .2
		accountRate, accountGroupRate := .3, 2.0
		key := &APIKey{ID: 1, GroupID: &group.ID, Group: group, Quota: 100}
		input := &RecordUsageInput{
			Result: &ForwardResult{RequestID: "scoped-generic-request", Model: "claude-sonnet-4-5", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 100}, SearchCount: 2},
			APIKey: key, User: &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, GroupIDs: []int64{5, 6}, RateMultiplier: &accountRate, GroupRateMultiplier: &accountGroupRate, Extra: map[string]any{"quota_limit": 100.0}},
			Subscription: &UserSubscription{ID: 8}, APIKeyService: &openAIRecordUsageAPIKeyQuotaStub{}, PricingAt: at(15, 30),
		}
		rate := .2 * accountGroupRate * 3
		base, err := svc.billingService.CalculateCost(input.Result.Model, UsageTokens{InputTokens: 1000, OutputTokens: 100}, rate)
		require.NoError(t, err)
		search := svc.billingService.CalculateSearchCost(2, nil, rate)
		modelRate := 1.0
		if groupID == 5 {
			modelRate = 10
		}
		want := QuantizeUsageBillingAmount(base.ActualCost*modelRate + search.ActualCost)
		for range 2 {
			require.NoError(t, svc.RecordUsage(t.Context(), input))
		}
		require.Len(t, billing.seen, 1, "replayed request keeps the existing billing identity")
		require.Len(t, billing.cmds, 2)
		for _, cmd := range billing.cmds {
			require.InDelta(t, want, cmd.SubscriptionCost, 1e-12)
			require.InDelta(t, want, cmd.APIKeyQuotaCost, 1e-12)
			require.InDelta(t, (base.TotalCost+search.TotalCost)*accountRate, cmd.AccountQuotaCost, 1e-8)
			require.Zero(t, cmd.BalanceCost)
		}
		require.InDelta(t, want, logs.lastLog.ActualCost, 1e-12)
		require.InDelta(t, base.TotalCost+search.TotalCost, logs.lastLog.TotalCost, 1e-12)
		require.InDelta(t, rate*modelRate, logs.lastLog.RateMultiplier, 1e-12)
		require.Equal(t, &group.ID, logs.lastLog.GroupID)
	}
}

func TestModelBillingGroupOpenAIPricedFallbackAndPerRequest(t *testing.T) {
	svc := newOpenAIRecordUsageServiceForTest(nil, nil, nil, nil)
	svc.StopOpenAICodexTicketHarvester()
	svc.settingService = modelBillingSettings(t, ModelBillingConfig{SchemaVersion: 2, Enabled: true, Rules: []ModelBillingRule{{Model: "gpt-6-luna*", Multiplier: 10, GroupID: 5, Enabled: true, Mode: ModelBillingAlways}}})
	for _, groupID := range []int64{5, 6} {
		key := &APIKey{ID: 1, GroupID: &groupID, Group: &Group{ID: groupID, Hydrated: true}}
		tokens := UsageTokens{InputTokens: 1000, OutputTokens: 100}
		result := &OpenAIForwardResult{Model: "alias", SearchCount: 2}
		cost, err := svc.calculateOpenAIRecordUsageCost(t.Context(), result, key, []string{"no-price-test-model", "gpt-6-luna"}, result.Model, .2, .2, .2, .2, tokens, "", nil, false, time.Time{})
		require.NoError(t, err)
		base, err := svc.billingService.CalculateCost("gpt-6-luna", tokens, .2)
		require.NoError(t, err)
		search := svc.billingService.CalculateSearchCost(2, nil, .2)
		modelRate := 1.0
		if groupID == 5 {
			modelRate = 10
		}
		require.InDelta(t, base.ActualCost*modelRate+search.ActualCost, cost.ActualCost, 1e-12)
		require.InDelta(t, base.TotalCost+search.TotalCost, cost.TotalCost, 1e-12)
		web := &OpenAIForwardResult{Model: "gpt-6-luna", WebSearchCalls: 2}
		webCost, err := svc.calculateOpenAIRecordUsageCost(t.Context(), web, key, []string{"gpt-6-luna"}, "gpt-6-luna", .2, .2, .2, .2, tokens, "", nil, false, time.Time{})
		require.NoError(t, err)
		require.Equal(t, svc.billingService.CalculateWebSearchCost(2, nil, .2), webCost)
		price := .5
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"gpt-6-luna"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price}}
		svc.resolver = NewModelPricingResolver(nil, svc.billingService)
		perRequest, err := svc.calculateOpenAIRecordUsageCost(t.Context(), &OpenAIForwardResult{Model: "gpt-6-luna"}, key, []string{"gpt-6-luna"}, "gpt-6-luna", .2, .2, .2, .2, tokens, "", nil, false, time.Time{})
		require.NoError(t, err)
		require.Equal(t, string(BillingModePerRequest), perRequest.BillingMode)
		require.InDelta(t, price*.2, perRequest.ActualCost, 1e-12)
		require.InDelta(t, price, perRequest.TotalCost, 1e-12)
		require.Equal(t, 1.0, costModelBillingMultiplier(perRequest))
		svc.resolver = nil
	}
}
