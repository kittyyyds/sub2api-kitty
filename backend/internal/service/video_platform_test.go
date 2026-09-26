//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func videoTestGroup(mode BillingMode, price float64) *Group {
	return &Group{ID: 25, Platform: PlatformVideo, RateMultiplier: 1, ModelPricing: []ChannelModelPricing{{
		Platform: PlatformVideo, Models: []string{"exact-video"}, BillingMode: mode, PerRequestPrice: &price,
	}}}
}

func TestVideoPlatformAccountIsolation(t *testing.T) {
	account := &Account{Platform: PlatformVideo, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "test-token", "base_url": "https://video.example/v1",
	}}
	require.False(t, account.IsGrok())
	require.False(t, account.IsGrokOAuth())
	require.True(t, account.IsGrokMediaAccount())
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGrokMediaGeneration))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.Equal(t, PlatformVideo, NormalizeOpenAICompatiblePlatform(PlatformVideo))
	require.Contains(t, schedulerSnapshotPlatforms(), PlatformVideo)
	require.Equal(t, "https://video.example/v1", account.GetGrokMediaBaseURL())
	validator, err := grokBaseURLValidator(account, &config.Config{})
	require.NoError(t, err)
	_, err = validator(account.GetGrokMediaBaseURL())
	require.NoError(t, err)
	service := &OpenAIGatewayService{}
	token, auth, err := service.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "test-token", token)
	require.Equal(t, "apikey", auth)
	account.Credentials["grok_media_provider"] = SuanliSpecialVideoProvider
	require.True(t, account.IsSuanliSpecialVideo())
	account.Type = AccountTypeOAuth
	require.False(t, account.IsGrokMediaAccount())
	require.False(t, account.IsSuanliSpecialVideo())
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGrokMediaGeneration))
	_, _, err = service.GetAccessToken(context.Background(), account)
	require.Error(t, err)
}

func TestVideoAccountValidationRejectsTypeAndProvider(t *testing.T) {
	require.NoError(t, ValidateVideoAccount(PlatformGrok, AccountTypeOAuth, nil))
	require.NoError(t, ValidateVideoAccount(PlatformVideo, AccountTypeAPIKey, nil))
	require.NoError(t, ValidateVideoAccount(PlatformVideo, AccountTypeAPIKey, map[string]any{"grok_media_provider": SuanliSpecialVideoProvider}))
	require.Error(t, ValidateVideoAccount(PlatformVideo, AccountTypeOAuth, nil))
	require.Error(t, ValidateVideoAccount(PlatformVideo, AccountTypeAPIKey, map[string]any{"grok_media_provider": "typo"}))
	require.Error(t, ValidateVideoAccount(PlatformVideo, AccountTypeAPIKey, map[string]any{"grok_media_provider": 1}))
	_, err := buildAccountForCreate(&CreateAccountInput{Platform: PlatformVideo, Type: AccountTypeOAuth}, map[string]any{})
	require.Error(t, err)
}

func TestVideoGroupExplicitPriceValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ChannelModelPricing)
	}{
		{"missing price", func(p *ChannelModelPricing) { p.PerRequestPrice = nil }},
		{"token mode", func(p *ChannelModelPricing) { p.BillingMode = BillingModeToken }},
		{"negative", func(p *ChannelModelPricing) { v := -1.0; p.PerRequestPrice = &v }},
		{"infinity", func(p *ChannelModelPricing) { v := math.Inf(1); p.PerRequestPrice = &v }},
		{"nan", func(p *ChannelModelPricing) { v := math.NaN(); p.PerRequestPrice = &v }},
		{"numeric column limit", func(p *ChannelModelPricing) { v := 1e12; p.PerRequestPrice = &v }},
		{"wildcard", func(p *ChannelModelPricing) { p.Models = []string{"video*"} }},
		{"empty model", func(p *ChannelModelPricing) { p.Models = []string{" "} }},
		{"no model", func(p *ChannelModelPricing) { p.Models = nil }},
		{"foreign platform", func(p *ChannelModelPricing) { p.Platform = PlatformGrok }},
		{"tiers", func(p *ChannelModelPricing) { p.Intervals = []PricingInterval{{}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := videoTestGroup(BillingModeVideo, .25)
			tt.mutate(&group.ModelPricing[0])
			_, err := normalizeGroupModelPricing(PlatformVideo, group.ModelPricing)
			require.Error(t, err)
		})
	}
	group := videoTestGroup(BillingModePerRequest, 0)
	_, err := normalizeGroupModelPricing(PlatformVideo, group.ModelPricing)
	require.NoError(t, err)
	price, err := ResolveVideoGroupPrice(group, "exact-video")
	require.NoError(t, err)
	require.Zero(t, price.UnitPrice)
	_, err = ResolveVideoGroupPrice(group, "EXACT-video")
	require.Error(t, err)
	group.ModelPricing = append(group.ModelPricing, group.ModelPricing[0])
	_, err = ResolveVideoGroupPrice(group, "exact-video")
	require.Error(t, err)
}

func TestVideoGroupDistinctExactIDsRemainDistinct(t *testing.T) {
	group := videoTestGroup(BillingModeVideo, 1)
	other := group.ModelPricing[0].Clone()
	other.Models = []string{"Exact-video"}
	group.ModelPricing = append(group.ModelPricing, other)
	_, err := normalizeGroupModelPricing(PlatformVideo, group.ModelPricing)
	require.NoError(t, err)
	require.Equal(t, []string{"exact-video", "Exact-video"}, plazaConfiguredModelNames(group))
	group.VideoModelPrices = map[string]map[string]float64{"exact-video": {"720p": 999}}
	display := plazaMediaDisplayPricing(nil, group, "Exact-video")
	require.Equal(t, BillingModeVideo, display.BillingMode)
	require.Equal(t, 1.0, *display.PerRequestPrice)
}

func TestVideoGroupPriceIgnoresGlobalAndLegacyMatrices(t *testing.T) {
	group := videoTestGroup(BillingModePerRequest, 4)
	group.VideoModelPrices = map[string]map[string]float64{"exact-video": {"720p": 900}}
	global := NewGlobalModelPricingService(&stubGlobalPricingRepo{rows: []GlobalModelPrice{{
		ID: 1, ModelPattern: "exact-video", BillingMode: BillingModeVideo, PerRequestPrice: fptr(800), Enabled: true,
	}}})
	resolver := NewModelPricingResolverWithGlobal(nil, &BillingService{}, global)
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "exact-video", Group: group})
	require.Equal(t, PricingSourceGroup, resolved.Source)
	require.Equal(t, BillingModePerRequest, resolved.Mode)
	require.Equal(t, 4.0, resolved.DefaultPerRequestPrice)
	require.Nil(t, resolver.Resolve(context.Background(), PricingInput{Model: "missing-video", Group: group}))
	group.Platform = PlatformGrok
	resolved = resolver.Resolve(context.Background(), PricingInput{Model: "exact-video", Group: group})
	require.Equal(t, PricingSourceGlobal, resolved.Source)
}

func TestVideoGroupSubmissionReloadsCurrentPrice(t *testing.T) {
	oldGroup := videoTestGroup(BillingModeVideo, .1)
	newGroup := videoTestGroup(BillingModePerRequest, 3)
	repo := &groupRepoStubForAdmin{getByID: newGroup}
	service := &OpenAIGatewayService{channelService: &ChannelService{groupRepo: repo}}
	price, err := service.ResolveVideoGroupPricing(context.Background(), &APIKey{GroupID: &oldGroup.ID, Group: oldGroup}, "exact-video")
	require.NoError(t, err)
	require.Equal(t, BillingModePerRequest, price.BillingMode)
	require.Equal(t, 3.0, price.UnitPrice)
}

func TestVideoSnapshotCostRemainsFixedAfterGroupEdits(t *testing.T) {
	group := videoTestGroup(BillingModeVideo, 999)
	group.VideoModelPrices = map[string]map[string]float64{"exact-video": {"720p": 800}}
	result := &OpenAIForwardResult{Model: "exact-video", VideoCount: 1, VideoDurationSeconds: 120,
		VideoPriceSnapshot: &VideoPriceSnapshot{Model: "exact-video", BillingMode: BillingModeVideo,
			UnitPrice: .25, RateMultiplier: 2, DurationSeconds: 30}}
	service := &OpenAIGatewayService{}
	cost, err := service.calculateOpenAIRecordUsageCost(context.Background(), result, &APIKey{Group: group}, []string{"exact-video"}, "", 999, 999, 999, 999, UsageTokens{}, "", nil, time.Now())
	require.NoError(t, err)
	require.Equal(t, 7.5, cost.TotalCost)
	require.Equal(t, 15.0, cost.ActualCost)
	require.Equal(t, string(BillingModeVideo), cost.BillingMode)
	result.VideoPriceSnapshot.BillingMode = BillingModePerRequest
	cost = service.calculateOpenAIVideoCost(context.Background(), "exact-video", "exact-video", &APIKey{Group: group}, result, 999)
	require.Equal(t, .25, cost.TotalCost)
	require.Equal(t, .5, cost.ActualCost)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
	result.VideoPriceSnapshot = nil
	_, err = service.calculateOpenAIRecordUsageCost(context.Background(), result, &APIKey{Group: group}, []string{"exact-video"}, "", 1, 1, 1, 1, UsageTokens{}, "", nil, time.Now())
	require.Error(t, err)
}

func TestVideoLegacyPendingUsesExplicitGroupPrice(t *testing.T) {
	group := videoTestGroup(BillingModePerRequest, 4)
	result := &OpenAIForwardResult{Model: "exact-video", VideoCount: 1, VideoLegacyPricing: true, VideoDurationSeconds: 30}
	service := &OpenAIGatewayService{}
	cost, err := service.calculateOpenAIRecordUsageCost(context.Background(), result, &APIKey{Group: group}, []string{"exact-video"}, "", 1, 1, 2, 1, UsageTokens{}, "", nil, time.Now())
	require.NoError(t, err)
	require.Equal(t, 4.0, cost.TotalCost)
	require.Equal(t, 8.0, cost.ActualCost)
	result.Model = "missing"
	_, err = service.calculateOpenAIRecordUsageCost(context.Background(), result, &APIKey{Group: group}, nil, "", 1, 1, 1, 1, UsageTokens{}, "", nil, time.Now())
	require.Error(t, err)
}

func TestVideoSnapshotInvalidPriceRejected(t *testing.T) {
	for _, price := range []float64{-1, math.NaN(), math.Inf(1), 1e12} {
		_, err := calculateVideoPriceSnapshotCost(&VideoPriceSnapshot{Model: "a", BillingMode: BillingModePerRequest, UnitPrice: price, RateMultiplier: 1}, 1)
		require.Error(t, err)
	}
	_, err := calculateVideoPriceSnapshotCost(&VideoPriceSnapshot{Model: "a", BillingMode: BillingModeVideo, UnitPrice: 1, RateMultiplier: 1}, 1)
	require.Error(t, err)
}

func TestVideoSnapshotRejectsCostOutsideDatabaseRange(t *testing.T) {
	for _, snapshot := range []VideoPriceSnapshot{
		{Model: "a", BillingMode: BillingModeVideo, UnitPrice: 1e11, DurationSeconds: 10, RateMultiplier: 0},
		{Model: "a", BillingMode: BillingModePerRequest, UnitPrice: 1e11, RateMultiplier: 10},
		{Model: "a", BillingMode: BillingModeVideo, UnitPrice: math.MaxFloat64, DurationSeconds: 2, RateMultiplier: 1},
	} {
		_, err := calculateVideoPriceSnapshotCost(&snapshot, 1)
		require.Error(t, err)
	}
	cost, err := calculateVideoPriceSnapshotCost(&VideoPriceSnapshot{
		Model: "a", BillingMode: BillingModePerRequest, UnitPrice: 999999999999, RateMultiplier: 1,
	}, 1)
	require.NoError(t, err)
	require.Equal(t, 999999999999.0, cost.ActualCost)
}

func TestVideoModelPlazaUsesOnlyAuthoritativePrices(t *testing.T) {
	group := videoTestGroup(BillingModeVideo, .25)
	group.RateMultiplier = 9
	group.VideoRateIndependent = true
	group.VideoRateMultiplier = 11
	channel := plazaPricedChannel(1, "old-channel", []int64{group.ID}, PlatformVideo, "unpriced-old-model", "exact-video")
	group.VideoModelPrices = map[string]map[string]float64{"exact-video": {"720p": 900}}
	svc := newPlazaService([]Channel{channel}, []Group{*group}, nil)
	groups, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Models, 1)
	model := groups[0].Models[0]
	require.Equal(t, "exact-video", model.Name)
	require.Equal(t, PlatformVideo, model.Platform)
	require.Equal(t, BillingModeVideo, model.Pricing.BillingMode)
	require.Equal(t, .25, *model.Pricing.PerRequestPrice)
	group.ModelPricing[0].BillingMode = BillingModePerRequest
	display := plazaMediaDisplayPricing(model.Pricing, group, "exact-video")
	require.Equal(t, BillingModePerRequest, display.BillingMode)
	require.Equal(t, .25, *display.PerRequestPrice, "per-request prices are also final customer unit prices")
}
