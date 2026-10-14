package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayRefreshesMissingImageModelPriceFromPartialSnapshot(t *testing.T) {
	groupID := int64(140)
	flatPrice := 0.5
	finalPrice := 0.01
	repo := &openAIMediaPriceGroupRepoStub{group: &Group{
		ID:           groupID,
		Platform:     PlatformOpenAI,
		ImagePrice4K: &flatPrice,
		ImageModelPrices: map[string]map[string]float64{
			"gpt-image-2": {ImageBillingSize4K: finalPrice},
		},
	}}
	svc := &OpenAIGatewayService{
		billingService: &BillingService{},
		channelService: &ChannelService{groupRepo: repo},
	}
	apiKey := &APIKey{
		GroupID: i64p(groupID),
		Group:   &Group{ID: groupID, Platform: PlatformOpenAI, ImagePrice4K: &flatPrice, Hydrated: true},
	}

	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{Model: "gpt-image-2", BillingModel: "gpt-image-2", ImageCount: 1, ImageSize: ImageBillingSize4K},
		apiKey,
		[]string{"gpt-image-2"},
		"gpt-image-2",
		1,
		1,
		1,
		1,
		UsageTokens{},
		"",
		boolPtr(true),
		time.Time{},
	)

	require.NoError(t, err)
	require.True(t, cost.FinalPriceApplied)
	require.InDelta(t, finalPrice, cost.ActualCost, 1e-12)
}

func TestOpenAIGatewayRefreshesModelOnlyImagePriceFromPartialSnapshot(t *testing.T) {
	groupID := int64(143)
	finalPrice := 0.013
	repo := &openAIMediaPriceGroupRepoStub{group: &Group{
		ID:       groupID,
		Platform: PlatformOpenAI,
		ImageModelPrices: map[string]map[string]float64{
			"gpt-image-2": {ImageBillingSize2K: finalPrice},
		},
	}}
	svc := &OpenAIGatewayService{
		billingService: &BillingService{},
		channelService: &ChannelService{groupRepo: repo},
	}

	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{Model: "gpt-image-2", BillingModel: "gpt-image-2", ImageCount: 1, ImageSize: ImageBillingSize2K},
		&APIKey{GroupID: i64p(groupID), Group: &Group{ID: groupID, Platform: PlatformOpenAI, Hydrated: true}},
		[]string{"gpt-image-2"},
		"gpt-image-2",
		1,
		1,
		1,
		1,
		UsageTokens{},
		"",
		boolPtr(true),
		time.Time{},
	)

	require.NoError(t, err)
	require.True(t, cost.FinalPriceApplied)
	require.InDelta(t, finalPrice, cost.ActualCost, 1e-12)
}

func TestOpenAIGatewayRefreshesMissingVideoModelPriceFromPartialSnapshot(t *testing.T) {
	groupID := int64(141)
	flatPrice := 0.5
	finalPrice := 0.01
	repo := &openAIMediaPriceGroupRepoStub{group: &Group{
		ID:             groupID,
		Platform:       PlatformGrok,
		VideoPrice720P: &flatPrice,
		VideoModelPrices: map[string]map[string]float64{
			VideoPriceFamilyGrokImagineVideo15: {VideoBillingResolution720P: finalPrice},
		},
	}}
	svc := &OpenAIGatewayService{
		billingService: &BillingService{},
		channelService: &ChannelService{groupRepo: repo},
	}
	apiKey := &APIKey{
		GroupID: i64p(groupID),
		Group:   &Group{ID: groupID, Platform: PlatformGrok, VideoPrice720P: &flatPrice, Hydrated: true},
	}

	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{
			Model: "grok-imagine-video-1.5-preview", BillingModel: "grok-imagine-video-1.5-preview",
			VideoCount: 1, VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: 15,
		},
		apiKey,
		[]string{"grok-imagine-video-1.5-preview"},
		"grok-imagine-video-1.5-preview",
		1,
		1,
		1,
		1,
		UsageTokens{},
		"",
		boolPtr(true),
		time.Time{},
	)

	require.NoError(t, err)
	require.True(t, cost.FinalPriceApplied)
	require.InDelta(t, finalPrice*15, cost.ActualCost, 1e-12)
}

func TestOpenAIGatewayRefreshesModelOnlyVideoPriceFromPartialSnapshot(t *testing.T) {
	groupID := int64(144)
	finalPrice := 0.017
	repo := &openAIMediaPriceGroupRepoStub{group: &Group{
		ID:       groupID,
		Platform: PlatformGrok,
		VideoModelPrices: map[string]map[string]float64{
			VideoPriceFamilyGrokImagineVideo15: {VideoBillingResolution720P: finalPrice},
		},
	}}
	svc := &OpenAIGatewayService{
		billingService: &BillingService{},
		channelService: &ChannelService{groupRepo: repo},
	}

	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{
			Model: "grok-imagine-video-1.5-preview", BillingModel: "grok-imagine-video-1.5-preview",
			VideoCount: 1, VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: 15,
		},
		&APIKey{GroupID: i64p(groupID), Group: &Group{ID: groupID, Platform: PlatformGrok, Hydrated: true}},
		[]string{"grok-imagine-video-1.5-preview"},
		"grok-imagine-video-1.5-preview",
		1,
		1,
		1,
		1,
		UsageTokens{},
		"",
		boolPtr(true),
		time.Time{},
	)

	require.NoError(t, err)
	require.True(t, cost.FinalPriceApplied)
	require.InDelta(t, finalPrice*15, cost.ActualCost, 1e-12)
}

func TestGatewayRefreshesMissingImageModelPriceFromPartialSnapshot(t *testing.T) {
	groupID := int64(142)
	flatPrice := 0.5
	finalPrice := 0.01
	repo := &openAIMediaPriceGroupRepoStub{group: &Group{
		ID:           groupID,
		Platform:     PlatformGrok,
		ImagePrice2K: &flatPrice,
		ImageModelPrices: map[string]map[string]float64{
			"grok-imagine-image-quality": {ImageBillingSize2K: finalPrice},
		},
	}}
	svc := &GatewayService{billingService: &BillingService{}, groupRepo: repo}

	cost := svc.calculateRecordUsageCost(
		context.Background(),
		&ForwardResult{Model: "grok-imagine-image-quality", ImageCount: 1, ImageSize: ImageBillingSize2K},
		&APIKey{GroupID: i64p(groupID), Group: &Group{ID: groupID, Platform: PlatformGrok, ImagePrice2K: &flatPrice, Hydrated: true}},
		"grok-imagine-image-quality",
		"grok-imagine-image-quality",
		1,
		1,
		time.Time{},
	)

	require.True(t, cost.FinalPriceApplied)
	require.InDelta(t, finalPrice, cost.ActualCost, 1e-12)
}

func TestImageModelPriceMayBeMissingSkipsRefreshForConfiguredCell(t *testing.T) {
	group := &Group{ImageModelPrices: map[string]map[string]float64{
		"gpt-image-2": {ImageBillingSize4K: 0.01},
	}}
	apiKey := &APIKey{Group: group}

	require.False(t, imageModelPriceMayBeMissing(apiKey, ImageBillingSize4K, "gpt-image-2"))
}

func TestVideoModelPriceMayBeMissingSkipsRefreshForConfiguredCell(t *testing.T) {
	group := &Group{VideoModelPrices: map[string]map[string]float64{
		VideoPriceFamilyGrokImagineVideo15: {VideoBillingResolution720P: 0.01},
	}}
	apiKey := &APIKey{Group: group}

	require.False(t, videoModelPriceMayBeMissing(apiKey, VideoBillingResolution720P, "grok-imagine-video-1.5-preview"))
}
