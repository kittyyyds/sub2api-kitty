//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// newPlazaChannelService 构造 ListPlazaGroups 测试用的 ChannelService。
func newPlazaChannelService(channels []Channel, groups []Group, pricing *PricingService) *ChannelService {
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return channels, nil },
	}
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{activeGroups: groups}, nil, nil)
	svc.pricingService = pricing
	return svc
}

func plazaPricedChannel(id int64, name string, groupIDs []int64, platform string, models ...string) Channel {
	return Channel{
		ID:       id,
		Name:     name,
		Status:   StatusActive,
		GroupIDs: groupIDs,
		ModelPricing: []ChannelModelPricing{{
			Platform:    platform,
			Models:      models,
			BillingMode: BillingModeToken,
			InputPrice:  testPtrFloat64(3e-6),
			OutputPrice: testPtrFloat64(1.5e-5),
		}},
	}
}

func TestListPlazaGroups_GroupCentricAggregation(t *testing.T) {
	// 两个渠道挂同一分组:模型并入同一 PlazaGroup;无模型的分组不返回。
	channels := []Channel{
		plazaPricedChannel(1, "chA", []int64{10}, "anthropic", "claude-sonnet"),
		plazaPricedChannel(2, "chB", []int64{10}, "anthropic", "claude-opus"),
	}
	groups := []Group{
		{
			ID: 10, Name: "g-main", Description: "desc", Platform: "anthropic", RateMultiplier: 1,
			ImageRateIndependent: true, ImageRateMultiplier: 0.15,
			VideoRateIndependent: true, VideoRateMultiplier: 0.25,
		},
		{ID: 20, Name: "g-empty", Platform: "anthropic", RateMultiplier: 0.5},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1, "无模型的分组不应返回")
	require.Equal(t, int64(10), out[0].ID)
	require.Equal(t, "desc", out[0].Description)
	require.True(t, out[0].ImageRateIndependent)
	require.InDelta(t, 0.15, out[0].ImageRateMultiplier, 1e-12)
	require.True(t, out[0].VideoRateIndependent)
	require.InDelta(t, 0.25, out[0].VideoRateMultiplier, 1e-12)
	require.Len(t, out[0].Models, 2)
	// 组内模型按名称排序
	require.Equal(t, "claude-opus", out[0].Models[0].Name)
	require.Equal(t, "claude-sonnet", out[0].Models[1].Name)
}

func TestListPlazaGroups_UsesConfiguredGroupModelsWithoutChannels(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"claude-sonnet": {
			InputCostPerToken:  3e-6,
			OutputCostPerToken: 15e-6,
		},
	})
	groups := []Group{{
		ID: 10, Name: "configured", Platform: PlatformAnthropic, RateMultiplier: 0.7,
		ModelsListConfig: GroupModelsListConfig{
			Enabled: false,
			Models:  []string{" claude-sonnet ", "Claude-Sonnet", "", "claude-opus"},
		},
	}}

	out, err := newPlazaChannelService(nil, groups, pricingSvc).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2, "configured models are case-insensitively deduplicated")
	byName := map[string]PlazaModel{}
	for _, model := range out[0].Models {
		byName[model.Name] = model
		require.Equal(t, PlatformAnthropic, model.Platform)
	}
	require.Contains(t, byName, "claude-sonnet")
	require.NotNil(t, byName["claude-sonnet"].OfficialPricing)
	require.InDelta(t, 3e-6, *byName["claude-sonnet"].OfficialPricing.InputPrice, 1e-12)
	require.Nil(t, byName["claude-sonnet"].Pricing)
}

func TestListPlazaGroups_IncludesModelsConfiguredOnlyInMediaPriceMaps(t *testing.T) {
	groups := []Group{{
		ID: 10, Name: "grok-media", Platform: PlatformGrok, RateMultiplier: 0.15,
		ModelsListConfig: GroupModelsListConfig{Models: []string{"grok-imagine-image-2.0"}},
		ImageModelPrices: map[string]map[string]float64{
			"grok-imagine-image-2.0": {
				ImageBillingSize1K: 0.01,
				ImageBillingSize2K: 0.01,
			},
		},
		VideoModelPrices: map[string]map[string]float64{
			VideoPriceFamilyGrokImagineVideo15: {
				VideoBillingResolution480P:  0.13,
				VideoBillingResolution720P:  0.24,
				VideoBillingResolution1080P: 0.35,
			},
		},
	}}

	out, err := newPlazaChannelService(nil, groups, nil).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2, "price-map models join the catalog and duplicates remain collapsed")
	byName := make(map[string]PlazaModel, len(out[0].Models))
	for _, model := range out[0].Models {
		byName[model.Name] = model
	}

	image := byName["grok-imagine-image-2.0"]
	require.NotNil(t, image.Pricing)
	require.Equal(t, BillingModeImage, image.Pricing.BillingMode)
	require.ElementsMatch(t, []string{ImageBillingSize1K, ImageBillingSize2K}, image.FinalImagePriceTiers)

	video := byName[VideoPriceFamilyGrokImagineVideo15]
	require.NotNil(t, video.Pricing)
	require.Equal(t, BillingModeVideo, video.Pricing.BillingMode)
	require.Equal(t, []string{
		VideoBillingResolution480P,
		VideoBillingResolution720P,
		VideoBillingResolution1080P,
	}, video.FinalVideoPriceTiers)
}

func TestListPlazaGroups_ConfiguredCompositeModelsUseDetectedPlatforms(t *testing.T) {
	groups := []Group{{
		ID: 10, Name: "composite", Platform: PlatformComposite, RateMultiplier: 1,
		ModelsListConfig: GroupModelsListConfig{
			Models: []string{"claude-sonnet-4", "openai/gpt-5", "OPENAI/GPT-5", "ambiguous-model"},
		},
	}}

	out, err := newPlazaChannelService(nil, groups, nil).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2, "ambiguous and duplicate composite models are excluded")
	platforms := map[string]string{}
	for _, model := range out[0].Models {
		platforms[model.Name] = model.Platform
	}
	require.Equal(t, PlatformAnthropic, platforms["claude-sonnet-4"])
	require.Equal(t, PlatformOpenAI, platforms["openai/gpt-5"])
}

func TestListPlazaGroups_GroupPricingOverridesChannelPricing(t *testing.T) {
	groupInput := 1e-6
	channelInput := 2e-6
	groups := []Group{{
		ID: 10, Name: "priority", Platform: PlatformAnthropic, RateMultiplier: 1,
		ModelsListConfig: GroupModelsListConfig{Models: []string{"claude-sonnet"}},
		ModelPricing: []ChannelModelPricing{{
			Platform: PlatformAnthropic, Models: []string{"claude-*"}, BillingMode: BillingModeToken,
			InputPrice: &groupInput,
		}},
	}}
	channels := []Channel{{
		ID: 1, Name: "channel", Status: StatusActive, GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{{
			Platform: PlatformAnthropic, Models: []string{"claude-sonnet"}, BillingMode: BillingModeToken,
			InputPrice: &channelInput,
		}},
	}}

	out, err := newPlazaChannelService(channels, groups, nil).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.NotNil(t, out[0].Models[0].Pricing)
	require.InDelta(t, groupInput, *out[0].Models[0].Pricing.InputPrice, 1e-12)
}

func TestListPlazaGroups_DedupFirstWinsWithPricingUpgrade(t *testing.T) {
	// 同名模型:先见者胜;仅当已存条目无定价而新条目有定价时升级替换。
	unpriced := Channel{
		ID: 1, Name: "alpha", Status: StatusActive, GroupIDs: []int64{10},
		// mapping-only → SupportedModels 产出无定价条目
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet": "claude-sonnet"},
		},
	}
	priced := plazaPricedChannel(2, "beta", []int64{10}, "anthropic", "claude-sonnet")
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}

	// alpha(无价)按名称序先于 beta(有价):先见者无价,应被有价条目升级。
	svc := newPlazaChannelService([]Channel{priced, unpriced}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.NotNil(t, out[0].Models[0].Pricing, "无价条目应被有价条目升级")
	require.NotNil(t, out[0].Models[0].Pricing.InputPrice)
}

func TestListPlazaGroups_PlatformIsolation(t *testing.T) {
	// 渠道同时有 anthropic/openai 定价,anthropic 分组只应看到 anthropic 模型。
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet"}, InputPrice: testPtrFloat64(3e-6)},
			{Platform: "openai", Models: []string{"gpt-5"}, InputPrice: testPtrFloat64(2e-6)},
		},
	}
	groups := []Group{
		{ID: 10, Name: "g-claude", Platform: "anthropic", RateMultiplier: 1},
		{ID: 20, Name: "g-gpt", Platform: "openai", RateMultiplier: 1},
	}
	svc := newPlazaChannelService([]Channel{ch}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	byName := map[string][]PlazaModel{}
	for _, g := range out {
		byName[g.Name] = g.Models
	}
	require.Len(t, byName["g-claude"], 1)
	require.Equal(t, "claude-sonnet", byName["g-claude"][0].Name)
	require.Len(t, byName["g-gpt"], 1)
	require.Equal(t, "gpt-5", byName["g-gpt"][0].Name)
}

func TestListPlazaGroups_CompositeIncludesConfiguredConcretePlatforms(t *testing.T) {
	anthropicPrice := 3e-6
	openAIPrice := 2e-6
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformAnthropic, Models: []string{"shared-model"}, InputPrice: &anthropicPrice},
			{Platform: PlatformOpenAI, Models: []string{"shared-model"}, InputPrice: &openAIPrice},
			{Platform: "", Models: []string{"empty-platform"}},
			{Platform: PlatformComposite, Models: []string{"nested-composite"}},
			{Platform: "unknown-platform", Models: []string{"unknown-platform"}},
		},
	}
	groups := []Group{{ID: 10, Name: "composite", Platform: PlatformComposite, RateMultiplier: 1}}

	out, err := newPlazaChannelService([]Channel{ch}, groups, nil).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2, "only concrete platforms are included and same-named models remain distinct")
	require.Equal(t, PlatformAnthropic, out[0].Models[0].Platform)
	require.Equal(t, PlatformOpenAI, out[0].Models[1].Platform)
	require.InDelta(t, anthropicPrice, *out[0].Models[0].Pricing.InputPrice, 1e-12)
	require.InDelta(t, openAIPrice, *out[0].Models[1].Pricing.InputPrice, 1e-12)
}

func TestListPlazaGroups_CompositeAndOrdinaryGroupsDoNotLeakPlatforms(t *testing.T) {
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformAnthropic, Models: []string{"claude-sonnet"}, InputPrice: testPtrFloat64(3e-6)},
			{Platform: PlatformOpenAI, Models: []string{"gpt-5"}, InputPrice: testPtrFloat64(2e-6)},
		},
	}
	groups := []Group{
		{ID: 10, Name: "anthropic-only", Platform: PlatformAnthropic, RateMultiplier: 1},
		{ID: 20, Name: "composite", Platform: PlatformComposite, RateMultiplier: 1},
	}

	out, err := newPlazaChannelService([]Channel{ch}, groups, nil).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 2)
	byName := map[string]PlazaGroup{}
	for _, group := range out {
		byName[group.Name] = group
	}
	require.Len(t, byName["anthropic-only"].Models, 1)
	require.Equal(t, []PlazaModel{{
		Name: "claude-sonnet", Platform: PlatformAnthropic, Pricing: byName["anthropic-only"].Models[0].Pricing,
	}}, byName["anthropic-only"].Models)
	require.Len(t, byName["composite"].Models, 2)
	require.Equal(t, []string{"claude-sonnet", "gpt-5"}, []string{
		byName["composite"].Models[0].Name,
		byName["composite"].Models[1].Name,
	})
	require.Equal(t, []string{PlatformAnthropic, PlatformOpenAI}, []string{
		byName["composite"].Models[0].Platform,
		byName["composite"].Models[1].Platform,
	})
}

func TestListPlazaGroups_InactiveChannelSkipped(t *testing.T) {
	inactive := plazaPricedChannel(1, "off", []int64{10}, "anthropic", "claude-sonnet")
	inactive.Status = "inactive"
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}
	svc := newPlazaChannelService([]Channel{inactive}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestListPlazaGroups_SortedByRateMultiplierAsc(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10, 20, 30}, "anthropic", "claude-sonnet"),
	}
	groups := []Group{
		{ID: 10, Name: "b-standard", Platform: "anthropic", RateMultiplier: 1},
		{ID: 20, Name: "a-standard", Platform: "anthropic", RateMultiplier: 1},
		{ID: 30, Name: "cheap", Platform: "anthropic", RateMultiplier: 0.5},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "cheap", out[0].Name, "倍率低者在前")
	require.Equal(t, "a-standard", out[1].Name, "同倍率按名称")
	require.Equal(t, "b-standard", out[2].Name)
}

func TestListPlazaGroups_OfficialPricingFill(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"claude-sonnet": {
			Mode:                                "chat",
			InputCostPerToken:                   3e-6,
			OutputCostPerToken:                  1.5e-5,
			CacheCreationInputTokenCost:         3.75e-6,
			CacheCreationInputTokenCostAbove1hr: 6e-6,
			CacheReadInputTokenCost:             3e-7,
		},
		"token-absent": {Mode: "image_generation", TokenPricingAbsent: true, OutputCostPerImage: 0.04},
	})
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10}, "anthropic", "claude-sonnet", "unknown-model", "token-absent"),
	}
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}
	svc := newPlazaChannelService(channels, groups, pricingSvc)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 3)

	byName := map[string]PlazaModel{}
	for _, m := range out[0].Models {
		byName[m.Name] = m
	}
	// 命中:填充完整官方价(含 1h 缓存写入)
	official := byName["claude-sonnet"].OfficialPricing
	require.NotNil(t, official)
	require.InDelta(t, 3e-6, *official.InputPrice, 1e-12)
	require.InDelta(t, 6e-6, *official.CacheWrite1hPrice, 1e-12)
	require.InDelta(t, 3e-7, *official.CacheReadPrice, 1e-12)
	// 未命中:nil(GetModelPricing 的 claude 系列模糊匹配对非 claude 名不生效)
	require.Nil(t, byName["unknown-model"].OfficialPricing)
	// TokenPricingAbsent 条目不作为官方 token 价展示
	require.Nil(t, byName["token-absent"].OfficialPricing)
}

func TestListPlazaGroups_GroupImagePriceOverridesChannelPricing(t *testing.T) {
	// 图片计费模型:档位价按实收口径合成(分组图片价 > 渠道档位价 > 渠道默认按次价),
	// 分组独立倍率字段透传;未配图片价的分组保持渠道定价原样。
	perReq := 0.2
	tier4K := 0.3
	imgPrice := 0.02
	channels := []Channel{{
		ID: 1, Name: "img-ch", Status: StatusActive, GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{{
			Platform:        "openai",
			Models:          []string{"gpt-image-2"},
			BillingMode:     BillingModeImage,
			PerRequestPrice: &perReq,
			Intervals:       []PricingInterval{{TierLabel: "4K", PerRequestPrice: &tier4K}},
		}},
	}}
	groups := []Group{
		{ID: 10, Name: "g-media", Platform: "openai", RateMultiplier: 1,
			ImagePrice1K: &imgPrice, ImageRateIndependent: true, ImageRateMultiplier: 1},
		{ID: 20, Name: "g-plain", Platform: "openai", RateMultiplier: 0.1},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	byName := map[string]PlazaGroup{}
	for _, g := range out {
		byName[g.Name] = g
	}

	media := byName["g-media"]
	require.True(t, media.ImageRateIndependent)
	require.InDelta(t, 1.0, media.ImageRateMultiplier, 1e-9)
	require.Len(t, media.Models, 1)
	p := media.Models[0].Pricing
	require.NotNil(t, p)
	require.Len(t, p.Intervals, 3)
	tierPrices := map[string]float64{}
	for _, iv := range p.Intervals {
		require.NotNil(t, iv.PerRequestPrice)
		tierPrices[iv.TierLabel] = *iv.PerRequestPrice
	}
	require.InDelta(t, 0.02, tierPrices["1K"], 1e-9, "1K 用分组图片价")
	require.InDelta(t, 0.2, tierPrices["2K"], 1e-9, "2K 分组未配,回落渠道默认按次价")
	require.InDelta(t, 0.3, tierPrices["4K"], 1e-9, "4K 分组未配,回落渠道档位价")

	plain := byName["g-plain"]
	require.False(t, plain.ImageRateIndependent)
	require.Len(t, plain.Models, 1)
	pp := plain.Models[0].Pricing
	require.NotNil(t, pp)
	require.Len(t, pp.Intervals, 1, "未配分组图片价:渠道定价原样")
	require.InDelta(t, 0.2, *pp.PerRequestPrice, 1e-9)

	// 合成为克隆,渠道原始定价不被修改
	require.Len(t, channels[0].ModelPricing[0].Intervals, 1)
}

func TestListPlazaGroups_GroupImagePriceIgnoredForNonImageModes(t *testing.T) {
	// token 模式定价不受分组图片价影响。
	imgPrice := 0.02
	channels := []Channel{plazaPricedChannel(1, "ch", []int64{10}, "openai", "gpt-5")}
	groups := []Group{{ID: 10, Name: "g", Platform: "openai", RateMultiplier: 1, ImagePrice1K: &imgPrice}}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	p := out[0].Models[0].Pricing
	require.NotNil(t, p)
	require.Empty(t, p.Intervals)
	require.NotNil(t, p.InputPrice)
	require.Nil(t, p.PerRequestPrice)
}

func TestListPlazaGroups_ModelImagePricesAreFinalAndOverrideTokenPricing(t *testing.T) {
	tokenPrice := 99.0
	channels := []Channel{{
		ID: 1, Name: "grok", Status: StatusActive, GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{{
			Platform: "grok", Models: []string{"grok-imagine-image-2.0"},
			BillingMode: BillingModeToken, InputPrice: &tokenPrice, OutputPrice: &tokenPrice,
		}},
	}}
	groups := []Group{{
		ID: 10, Name: "grok-final", Platform: PlatformGrok, RateMultiplier: 0.15,
		ImageModelPrices: map[string]map[string]float64{
			"grok-imagine-image-2.0": {
				ImageBillingSize1K: 0.01,
				ImageBillingSize2K: 0.02,
			},
		},
	}}

	out, err := newPlazaChannelService(channels, groups, nil).ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)

	model := out[0].Models[0]
	require.Equal(t, []string{ImageBillingSize1K, ImageBillingSize2K}, model.FinalImagePriceTiers)
	require.NotNil(t, model.Pricing)
	require.Equal(t, BillingModeImage, model.Pricing.BillingMode)
	require.Len(t, model.Pricing.Intervals, 3)
	tierPrices := map[string]float64{}
	for _, interval := range model.Pricing.Intervals {
		require.NotNil(t, interval.PerRequestPrice)
		tierPrices[interval.TierLabel] = *interval.PerRequestPrice
	}
	require.InDelta(t, 0.01, tierPrices[ImageBillingSize1K], 1e-12)
	require.InDelta(t, 0.02, tierPrices[ImageBillingSize2K], 1e-12)
	require.InDelta(t, 0.08, tierPrices[ImageBillingSize4K], 1e-12, "empty model tier uses model default")
}

func TestListPlazaGroups_ModelVideoPerSecondPricesAreFinalAndOverrideTokenPricing(t *testing.T) {
	tokenPrice := 99.0
	channels := []Channel{{
		ID: 1, Name: "grok", Status: StatusActive, GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{{
			Platform: PlatformGrok, Models: []string{"grok-imagine-video-1.5-preview"},
			BillingMode: BillingModeToken, InputPrice: &tokenPrice, OutputPrice: &tokenPrice,
		}},
	}}
	groups := []Group{{
		ID: 10, Name: "grok-video-final-per-second", Platform: PlatformGrok, RateMultiplier: 0.15,
		VideoModelPrices: map[string]map[string]float64{
			VideoPriceFamilyGrokImagineVideo15: {
				VideoBillingResolution480P: 0.01,
				VideoBillingResolution720P: 0.02,
			},
		},
	}}

	out, err := newPlazaChannelService(channels, groups, nil).ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)

	model := out[0].Models[0]
	require.Equal(t, []string{
		VideoBillingResolution480P,
		VideoBillingResolution720P,
	}, model.FinalVideoPriceTiers)
	require.NotNil(t, model.Pricing)
	require.Equal(t, BillingModeVideo, model.Pricing.BillingMode)
	require.Len(t, model.Pricing.Intervals, 3)
	tierPrices := map[string]float64{}
	for _, interval := range model.Pricing.Intervals {
		require.NotNil(t, interval.PerRequestPrice)
		tierPrices[interval.TierLabel] = *interval.PerRequestPrice
	}
	require.InDelta(t, 0.01, tierPrices[VideoBillingResolution480P], 1e-12)
	require.InDelta(t, 0.02, tierPrices[VideoBillingResolution720P], 1e-12)
	require.InDelta(t, 0.25, tierPrices[VideoBillingResolution1080P], 1e-12, "empty model tier uses per-second model default")
}

func TestListPlazaGroups_CanonicalizesGrokVideo15AliasesForDisplay(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "grok-preview", []int64{10}, PlatformGrok, "grok-imagine-video-1.5-preview"),
		plazaPricedChannel(2, "grok-dated", []int64{10}, PlatformGrok, "grok-imagine-video-1.5-2026-05-30"),
	}
	groups := []Group{{
		ID: 10, Name: "grok-video-catalog", Platform: PlatformGrok, RateMultiplier: 1,
		ModelsListConfig: GroupModelsListConfig{
			Enabled: true,
			Models: []string{
				"grok-imagine-video-1.5-preview",
				"grok-imagine-video-1.5-2026-05-30",
				VideoPriceFamilyGrokImagineVideo15,
			},
		},
		VideoModelPrices: map[string]map[string]float64{
			VideoPriceFamilyGrokImagineVideo15: {
				VideoBillingResolution480P:  0.13,
				VideoBillingResolution720P:  0.24,
				VideoBillingResolution1080P: 0.35,
			},
		},
	}}

	out, err := newPlazaChannelService(channels, groups, nil).ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1, "canonical and compatibility aliases must collapse into one catalog row")

	model := out[0].Models[0]
	require.Equal(t, VideoPriceFamilyGrokImagineVideo15, model.Name)
	require.Equal(t, []string{
		VideoBillingResolution480P,
		VideoBillingResolution720P,
		VideoBillingResolution1080P,
	}, model.FinalVideoPriceTiers)
	require.NotNil(t, model.Pricing)
	require.Equal(t, BillingModeVideo, model.Pricing.BillingMode)
	require.Len(t, model.Pricing.Intervals, 3)

	want := map[string]float64{
		VideoBillingResolution480P:  0.13,
		VideoBillingResolution720P:  0.24,
		VideoBillingResolution1080P: 0.35,
	}
	for _, interval := range model.Pricing.Intervals {
		require.NotNil(t, interval.PerRequestPrice)
		require.InDelta(t, want[interval.TierLabel], *interval.PerRequestPrice, 1e-12)
	}
}

func TestListPlazaGroups_RepoErrorsPropagate(t *testing.T) {
	sentinel := errors.New("boom")
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, sentinel },
	}
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{}, nil, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)

	svc2 := NewChannelService(
		&mockChannelRepository{listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, nil }},
		&stubGroupRepoForAvailable{listActiveErr: sentinel},
		nil, nil,
	)
	out2, err2 := svc2.ListPlazaGroups(context.Background())
	require.Nil(t, out2)
	require.ErrorIs(t, err2, sentinel)
}
