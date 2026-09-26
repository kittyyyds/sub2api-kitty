package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func imageModelPricingTestGroup(groupID int64) *Group {
	tokenPrice := 99.0
	return &Group{
		ID:             groupID,
		Platform:       PlatformGrok,
		Status:         StatusActive,
		RateMultiplier: 1,
		ImageModelPrices: map[string]map[string]float64{
			"grok-imagine":               {ImageBillingSize2K: 0.031},
			"grok-imagine-image":         {ImageBillingSize2K: 0.041},
			"grok-imagine-image-quality": {ImageBillingSize2K: 0.071},
		},
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{"grok-imagine"},
			BillingMode: BillingModeToken,
			InputPrice:  &tokenPrice,
			OutputPrice: &tokenPrice,
		}},
	}
}

func TestGatewayImageModelPriceOverridesTokenModeRouting(t *testing.T) {
	t.Parallel()
	groupID := int64(301)
	group := imageModelPricingTestGroup(groupID)
	billingService := &BillingService{}
	service := &GatewayService{
		billingService: billingService,
		resolver:       NewModelPricingResolver(nil, billingService),
	}

	cost := service.calculateRecordUsageCost(
		context.Background(),
		&ForwardResult{Model: "grok-imagine-image-quality", ImageCount: 2, ImageSize: ImageBillingSize2K},
		&APIKey{GroupID: &groupID, Group: group},
		"grok-imagine-image-quality",
		"grok-imagine",
		1,
		1.5,
		time.Time{},
	)

	require.Equal(t, string(BillingModeImage), cost.BillingMode)
	require.InDelta(t, 0.062, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.062, cost.ActualCost, 1e-12)
}

func TestOpenAIGatewayImageModelPriceOverridesTokenModeRouting(t *testing.T) {
	t.Parallel()
	groupID := int64(302)
	group := imageModelPricingTestGroup(groupID)
	billingService := &BillingService{}
	service := &OpenAIGatewayService{
		billingService: billingService,
		resolver:       NewModelPricingResolver(nil, billingService),
	}

	cost, err := service.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{
			Model:        "grok-imagine-image-quality",
			BillingModel: "grok-imagine-image-quality",
			ImageCount:   3,
			ImageSize:    ImageBillingSize2K,
		},
		&APIKey{GroupID: &groupID, Group: group},
		[]string{"grok-imagine-image-quality", "grok-imagine"},
		"grok-imagine",
		1,
		2,
		1,
		1,
		UsageTokens{},
		"",
		boolPtr(true),
		time.Time{},
	)

	require.NoError(t, err)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
	require.InDelta(t, 0.093, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.093, cost.ActualCost, 1e-12)
}

func TestOpenAIGatewayGPTImageModelPriceIsFinalUSDCustomerPrice(t *testing.T) {
	t.Parallel()
	groupID := int64(304)
	group := &Group{
		ID:             groupID,
		Platform:       PlatformOpenAI,
		Status:         StatusActive,
		RateMultiplier: 0.15,
		ImageModelPrices: map[string]map[string]float64{
			"gpt-image-2": {ImageBillingSize4K: 0.01},
		},
	}
	billingService := &BillingService{}
	service := &OpenAIGatewayService{
		billingService: billingService,
		resolver:       NewModelPricingResolver(nil, billingService),
	}

	cost, err := service.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{
			Model:        "gpt-image-2",
			BillingModel: "gpt-image-2",
			ImageCount:   1,
			ImageSize:    ImageBillingSize4K,
		},
		&APIKey{GroupID: &groupID, Group: group},
		[]string{"gpt-image-2"},
		"gpt-image-2",
		1,
		0.15,
		1,
		1,
		UsageTokens{},
		"",
		boolPtr(true),
		time.Time{},
	)

	require.NoError(t, err)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
	require.InDelta(t, 0.01, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.01, cost.ActualCost, 1e-12)
	require.True(t, cost.FinalPriceApplied)
}

func TestOpenAIGatewayVideoModelPriceOverridesTokenModeAsFinalPerSecondPrice(t *testing.T) {
	t.Parallel()
	groupID := int64(305)
	tokenPrice := 99.0
	group := &Group{
		ID:             groupID,
		Platform:       PlatformGrok,
		Status:         StatusActive,
		RateMultiplier: 0.15,
		VideoModelPrices: map[string]map[string]float64{
			VideoPriceFamilyGrokImagineVideo15: {VideoBillingResolution720P: 0.01},
		},
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{"grok-imagine-video-1.5"},
			BillingMode: BillingModeToken,
			InputPrice:  &tokenPrice,
			OutputPrice: &tokenPrice,
		}},
	}
	billingService := &BillingService{}
	service := &OpenAIGatewayService{
		billingService: billingService,
		resolver:       NewModelPricingResolver(nil, billingService),
	}

	cost, err := service.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{
			Model:                "customer-video-alias",
			BillingModel:         "grok-imagine-video-1.5",
			UpstreamModel:        "grok-imagine-video-1.5-preview",
			VideoCount:           2,
			VideoResolution:      VideoBillingResolution720P,
			VideoDurationSeconds: 15,
		},
		&APIKey{GroupID: &groupID, Group: group},
		[]string{"grok-imagine-video-1.5"},
		"customer-video-alias",
		0.15,
		0.15,
		0.15,
		0.15,
		UsageTokens{InputTokens: 100, OutputTokens: 200},
		"",
		boolPtr(true),
		time.Time{},
	)

	require.NoError(t, err)
	require.Equal(t, string(BillingModeVideo), cost.BillingMode)
	require.InDelta(t, 0.30, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.30, cost.ActualCost, 1e-12)
	require.True(t, cost.FinalPriceApplied)
}

func TestGatewayUsageLogShowsOneXForFinalImageModelPrice(t *testing.T) {
	t.Parallel()
	groupID := int64(303)
	cost := &CostBreakdown{
		TotalCost:         0.01,
		ActualCost:        0.01,
		BillingMode:       string(BillingModeImage),
		FinalPriceApplied: true,
	}

	usageLog := (&GatewayService{}).buildRecordUsageLog(
		context.Background(),
		&recordUsageCoreInput{},
		&ForwardResult{RequestID: "req-final-image", Model: "grok-imagine-image-2.0", ImageCount: 1, ImageSize: ImageBillingSize1K},
		&APIKey{ID: 1, GroupID: &groupID},
		&User{ID: 2},
		&Account{ID: 3},
		nil,
		"grok-imagine-image-2.0",
		0.15,
		0.15,
		1,
		0,
		false,
		cost,
	)

	require.InDelta(t, 1.0, usageLog.RateMultiplier, 1e-12)
	require.InDelta(t, 0.01, usageLog.ActualCost, 1e-12)
}
