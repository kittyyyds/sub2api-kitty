package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func scopedBillingFixture(model string, m float64, gid int64) ModelBillingConfig {
	return ModelBillingConfig{SchemaVersion: 2, Enabled: true, Rules: []ModelBillingRule{{Model: model, Multiplier: m, GroupID: gid, Enabled: true, Mode: ModelBillingAlways}}}
}
func modelBillingSettings(t *testing.T, cfg ModelBillingConfig) *SettingService {
	t.Helper()
	c := DefaultOAuthAutoConfig()
	c.ModelBilling = cfg
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	return NewSettingService(&accountOpsSettingsStub{raw: string(raw)}, nil)
}
func TestModelBillingLegacyRetiredAndExplicitScope(t *testing.T) {
	var c ModelBillingConfig
	require.NoError(t, json.Unmarshal([]byte(`{"enabled":true,"rules":[{"model":"*","multiplier":10}]}`), &c))
	require.Equal(t, DefaultModelBillingConfig(), c)
	require.Equal(t, 1.0, c.multiplier(2, "any", UsageTokens{}))
	c = scopedBillingFixture("gpt-*", 2, 2)
	require.NoError(t, validateModelBillingConfig(c))
	for _, gid := range []int64{0, 1, 3} {
		require.Equal(t, 1.0, c.multiplier(gid, "gpt-6", UsageTokens{}))
	}
	require.Equal(t, 2.0, c.multiplier(2, "gpt-6", UsageTokens{}))
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	var round ModelBillingConfig
	require.NoError(t, json.Unmarshal(raw, &round))
	require.Equal(t, c, round)
	c.Rules[0].GroupID = 0
	require.Error(t, validateModelBillingConfig(c))
	c = scopedBillingFixture("gpt-*", 2, 2)
	c.Rules = append(c.Rules, c.Rules[0])
	require.Error(t, validateModelBillingConfig(c))
	c.Rules[1].GroupID = 3
	require.NoError(t, validateModelBillingConfig(c))
	c.Rules[0].Mode = ModelBillingContext
	c.Rules[0].ThresholdTokens = 0
	require.Error(t, validateModelBillingConfig(c))
	c.Rules[0].ThresholdTokens = 50000
	require.NoError(t, validateModelBillingConfig(c))
}
func TestModelBillingMatchingThresholdAndCache(t *testing.T) {
	c := scopedBillingFixture("gpt-*", 10, 2)
	c.Rules = append(c.Rules, ModelBillingRule{Model: "gpt-test", Multiplier: 2, GroupID: 2, Enabled: true, Mode: ModelBillingContext, ThresholdTokens: 50000})
	for _, tc := range []struct {
		tokens UsageTokens
		want   float64
	}{
		{UsageTokens{InputTokens: 49999}, 1}, {UsageTokens{InputTokens: 50000, OutputTokens: 100000}, 1},
		{UsageTokens{InputTokens: 50001}, 2}, {UsageTokens{InputTokens: 1, CacheReadTokens: 50000}, 2},
		{UsageTokens{InputTokens: 1, CacheCreationTokens: 50000}, 2},
		{UsageTokens{InputTokens: 1, CacheCreation5mTokens: 25000, CacheCreation1hTokens: 25000}, 2},
		{UsageTokens{CacheCreationTokens: 50000, CacheCreation5mTokens: 50000}, 1},
	} {
		require.Equal(t, tc.want, c.multiplier(2, "gpt-test", tc.tokens))
	}
	require.Equal(t, 1.0, c.multiplier(3, "gpt-test", UsageTokens{InputTokens: 99999}))
	c.Rules[1].Enabled = false
	require.Equal(t, 10.0, c.multiplier(2, "gpt-test", UsageTokens{}))
	c.Enabled = false
	require.Equal(t, 1.0, c.multiplier(2, "gpt-test", UsageTokens{InputTokens: 99999}))
}

type modelBillingRepoStub struct {
	SettingRepository
	raw   string
	err   error
	calls int
}

func (s *modelBillingRepoStub) GetValue(context.Context, string) (string, error) {
	s.calls++
	return s.raw, s.err
}
func TestModelBillingCacheRefreshFailuresAndRequestSnapshot(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"model_billing": scopedBillingFixture("gpt-*", 2, 2)})
	repo := &modelBillingRepoStub{raw: string(data)}
	s := NewSettingService(repo, nil)
	ctx := withModelBillingConfig(t.Context(), s)
	require.Equal(t, 2.0, s.modelBillingConfigForUsage(ctx).multiplier(2, "gpt-test", UsageTokens{}))
	repo.err = errors.New("offline")
	s.modelBillingCache.expires = time.Time{}
	require.Equal(t, 2.0, s.modelBillingConfigForUsage(t.Context()).multiplier(2, "gpt-test", UsageTokens{}))
	repo.err = nil
	repo.raw = "{}"
	s.modelBillingCache.expires = time.Time{}
	require.Equal(t, 1.0, s.modelBillingConfigForUsage(t.Context()).multiplier(2, "gpt-test", UsageTokens{}))
	require.Equal(t, 2.0, s.modelBillingConfigForUsage(withModelBillingConfig(ctx, s)).multiplier(2, "gpt-test", UsageTokens{}))
}
func TestModelBillingCustomerCostOnlyAndIdempotent(t *testing.T) {
	cfg := scopedBillingFixture("gpt-test", 2, 2)
	cost := &CostBreakdown{InputCost: .4, OutputCost: .6, TotalCost: 1, ActualCost: .2, BillingMode: string(BillingModeToken)}
	for range 2 {
		applyModelBillingMultiplier(cost, cfg, 2, "gpt-test", UsageTokens{})
	}
	require.Equal(t, .4, cost.ActualCost)
	require.Equal(t, 1.0, cost.TotalCost)
	require.Equal(t, .4, cost.InputCost)
	for _, mode := range []BillingMode{BillingModeImage, BillingModeVideo, BillingModePerRequest} {
		v := &CostBreakdown{ActualCost: 1, BillingMode: string(mode)}
		applyModelBillingMultiplier(v, cfg, 2, "gpt-test", UsageTokens{})
		require.Equal(t, 1.0, v.ActualCost)
	}
}
func TestGroupModelBillingUnifiedAndPriceSchedule(t *testing.T) {
	s := NewBillingService(&config.Config{}, nil)
	cfg := scopedBillingFixture("gpt-6-luna", 2, 2)
	cfg.Rules[0].Mode = ModelBillingContext
	cfg.Rules[0].ThresholdTokens = 50000
	s.modelBillingSettings = modelBillingSettings(t, cfg)
	resolver := NewModelPricingResolver(nil, s)
	pIn, pOut, pRead, pWrite := .001, .002, .0001, .00125
	group := &Group{ID: 2, Platform: PlatformOpenAI, Hydrated: true, Status: StatusActive, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{{Models: []string{"gpt-6-luna"}, BillingMode: BillingModeToken, InputPrice: &pIn, OutputPrice: &pOut, CacheReadPrice: &pRead, CacheWritePrice: &pWrite}}}
	for _, n := range []int{49999, 50000, 50001} {
		in := CostInput{Ctx: t.Context(), Model: "gpt-6-luna", Group: group, Tokens: UsageTokens{InputTokens: n, OutputTokens: 100}, RateMultiplier: .2, Resolver: resolver}
		actual, err := s.CalculateCostUnified(in)
		require.NoError(t, err)
		want := 1.0
		if n > 50000 {
			want = 2
		}
		require.InDelta(t, (float64(n)*pIn+100*pOut)*.2*want, actual.ActualCost, 1e-10)
		in.SkipGlobalContextRules = true
		base, err := s.CalculateCostUnified(in)
		require.NoError(t, err)
		require.InDelta(t, base.TotalCost, actual.accountStatsBaseTotal(), 1e-10)
		other := *group
		other.ID = 3
		in.Group = &other
		in.SkipGlobalContextRules = false
		miss, err := s.CalculateCostUnified(in)
		require.NoError(t, err)
		require.Equal(t, 1.0, costModelBillingMultiplier(miss))
	}
	schedule, err := s.ResolveContextPricingSchedule(t.Context(), resolver, ContextPricingScheduleInput{Model: "gpt-6-luna", Group: group, Platform: PlatformOpenAI})
	require.NoError(t, err)
	require.Len(t, schedule.Tiers, 2)
	require.Equal(t, 50000, *schedule.Tiers[0].MaxTokens)
	require.InDelta(t, pIn, *schedule.Tiers[0].Input, 1e-12)
	require.InDelta(t, pIn*2, *schedule.Tiers[1].Input, 1e-12)
	require.InDelta(t, pOut*2, *schedule.Tiers[1].Output, 1e-12)
	require.InDelta(t, pRead*2, *schedule.Tiers[1].CacheRead, 1e-12)
}

type modelBillingSubscriptionRepo struct {
	openAIRecordUsageSubRepoStub
	lastAmount float64
}

func (s *modelBillingSubscriptionRepo) IncrementUsage(ctx context.Context, id int64, amount float64) error {
	s.lastAmount = amount
	return s.openAIRecordUsageSubRepoStub.IncrementUsage(ctx, id, amount)
}

func TestModelBillingOpenAIRecordUsageBalanceAndSubscription(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		for _, multiplier := range []float64{0, .2} {
			logRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			subRepo := &modelBillingSubscriptionRepo{}
			svc := newOpenAIRecordUsageServiceForTest(logRepo, userRepo, subRepo, nil)
			cfg := scopedBillingFixture("gpt-6-luna*", 10, 2)
			cfg.Enabled = true
			svc.settingService = modelBillingSettings(t, cfg)
			accountRate, accountGroupRate := .3, 2.0
			group := &Group{ID: 2, RateMultiplier: multiplier}
			input := &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{Model: "gpt-6-luna", Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 200, CacheReadInputTokens: 100}, Duration: time.Second},
				APIKey: &APIKey{ID: 1, GroupID: &group.ID, Group: group}, User: &User{ID: 3},
				Account: &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, RateMultiplier: &accountRate, GroupRateMultiplier: &accountGroupRate},
			}
			if subscription {
				group.SubscriptionType = SubscriptionTypeSubscription
				input.Subscription = &UserSubscription{ID: 8}
			}
			base := expectedOpenAICost(t, svc, "gpt-6-luna", input.Result.Usage, multiplier*accountGroupRate)
			require.NoError(t, svc.RecordUsage(t.Context(), input))
			require.NotNil(t, logRepo.lastLog)
			require.InDelta(t, base.ActualCost*10, logRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, base.TotalCost, logRepo.lastLog.TotalCost, 1e-12)
			require.InDelta(t, multiplier*accountGroupRate*10, logRepo.lastLog.RateMultiplier, 1e-12)
			require.Equal(t, accountRate, *logRepo.lastLog.AccountRateMultiplier)
			if subscription {
				require.InDelta(t, base.ActualCost*10, subRepo.lastAmount, 1e-12)
			} else {
				require.InDelta(t, base.ActualCost*10, userRepo.lastAmount, 1e-12)
			}
		}
	}
}

func TestModelBillingGenericTokenAndSearchCharges(t *testing.T) {
	cfg := scopedBillingFixture("claude-sonnet-4-5*", 10, 2)
	svc := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	result := &ForwardResult{Model: "claude-sonnet-4-5", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 100}, SearchCount: 2}
	gid := int64(2)
	key := &APIKey{ID: 1, GroupID: &gid, Group: &Group{ID: gid}}
	base := svc.calculateRecordUsageCost(t.Context(), result, key, result.Model, result.Model, .2, .2, time.Time{})
	svc.settingService = modelBillingSettings(t, cfg)
	cost := svc.calculateRecordUsageCost(t.Context(), result, key, result.Model, result.Model, .2, .2, time.Time{})
	search := svc.billingService.CalculateSearchCost(2, nil, .2)
	require.InDelta(t, (base.ActualCost-search.ActualCost)*10+search.ActualCost, cost.ActualCost, 1e-12)
	require.InDelta(t, base.TotalCost, cost.TotalCost, 1e-12)
	require.Equal(t, 10.0, costModelBillingMultiplier(cost))
}

func TestModelBillingUsesPricedFallbackAndPreservesSearchSurcharge(t *testing.T) {
	svc := newOpenAIRecordUsageServiceForTest(nil, nil, nil, nil)
	cfg := scopedBillingFixture("gpt-6-luna*", 10, 2)
	cfg.Enabled = true
	svc.settingService = modelBillingSettings(t, cfg)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 100}
	result := &OpenAIForwardResult{Model: "custom-alias", SearchCount: 2}
	gid := int64(2)
	key := &APIKey{ID: 1, GroupID: &gid, Group: &Group{ID: gid}}
	base, err := svc.billingService.CalculateCost("gpt-6-luna", tokens, .2)
	require.NoError(t, err)
	search := svc.billingService.CalculateSearchCost(2, nil, .2)
	cost, err := svc.calculateOpenAIRecordUsageCost(t.Context(), result, key,
		[]string{"no-price-test-model", "gpt-6-luna"}, result.Model, .2, .2, .2, .2, tokens, "", nil, false, time.Time{})
	require.NoError(t, err)
	require.InDelta(t, base.ActualCost*10+search.ActualCost, cost.ActualCost, 1e-12)
	require.InDelta(t, base.TotalCost+search.TotalCost, cost.TotalCost, 1e-12)
	require.Equal(t, 10.0, costModelBillingMultiplier(cost))
}

func TestModelBillingFreeFastChargesOnceAndBillsAtomicCommand(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		logs := &openAIRecordUsageLogRepoStub{inserted: true}
		billing := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: false}}
		svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, billing, nil, nil, nil)
		cfg := scopedBillingFixture("gpt-6-luna*", 10, 77)
		cfg.Enabled = true
		svc.settingService = modelBillingSettings(t, cfg)
		svc.resolver = NewModelPricingResolver(nil, svc.billingService)
		inputPrice, outputPrice, fastMultiplier := .001, .002, 3.0
		groupID := int64(77)
		key := &APIKey{ID: 1, GroupID: &groupID, Quota: 100, Group: &Group{
			ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true,
			RateMultiplier: .2, FreeOpenAIFast: true,
			ModelPricing: []ChannelModelPricing{{Models: []string{"gpt-6-luna"}, BillingMode: BillingModeToken, InputPrice: &inputPrice, OutputPrice: &outputPrice, FastMultiplier: &fastMultiplier}},
		}}
		tier := "priority"
		input := &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{RequestID: "model-billing-free-fast", Model: "gpt-6-luna", ServiceTier: &tier, Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 50}, Duration: time.Second},
			APIKey: key, User: &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			APIKeyService: &openAIRecordUsageAPIKeyQuotaStub{},
		}
		if subscription {
			key.Group.SubscriptionType = SubscriptionTypeSubscription
			input.Subscription = &UserSubscription{ID: 9}
		}
		require.NoError(t, svc.RecordUsage(t.Context(), input))
		standard := 100*inputPrice + 50*outputPrice
		require.InDelta(t, standard*fastMultiplier, logs.lastLog.TotalCost, 1e-12)
		require.InDelta(t, standard*.2*10, logs.lastLog.ActualCost, 1e-12)
		require.Equal(t, 2.0, logs.lastLog.RateMultiplier)
		require.NotNil(t, billing.lastCmd)
		require.InDelta(t, standard*.2*10, billing.lastCmd.APIKeyQuotaCost, 1e-12)
		if subscription {
			require.InDelta(t, standard*.2*10, billing.lastCmd.SubscriptionCost, 1e-12)
			require.Zero(t, billing.lastCmd.BalanceCost)
		} else {
			require.InDelta(t, standard*.2*10, billing.lastCmd.BalanceCost, 1e-12)
			require.Zero(t, billing.lastCmd.SubscriptionCost)
		}
	}
}

func TestGroupBillingSaveRequiresExistingGroup(t *testing.T) {
	repo := &accountOpsSettingsStub{raw: "{}"}
	s := NewAccountOpsService(repo, nil, nil)
	cfg := DefaultOAuthAutoConfig()
	cfg.ModelBilling = scopedBillingFixture("gpt-test", 2, 2)
	s.autoGroups = autoConfigGroups{}
	_, err := s.SaveOAuthAutoConfig(t.Context(), cfg)
	require.Error(t, err)
	s.autoGroups = autoConfigGroups{group: &Group{ID: 2, Status: StatusActive}}
	_, err = s.SaveOAuthAutoConfig(t.Context(), cfg)
	require.NoError(t, err)
	loaded, err := GetOAuthAutoConfig(t.Context(), repo)
	require.NoError(t, err)
	require.Equal(t, cfg.ModelBilling, loaded.ModelBilling)
}
func TestGroupBillingProviderRequiresContextRuleDatabase(t *testing.T) {
	s, err := ProvideBillingService(&config.Config{}, nil, modelBillingSettings(t, DefaultModelBillingConfig()), nil)
	require.Error(t, err)
	require.Nil(t, s)
}
