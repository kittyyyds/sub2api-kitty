//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func plazaGroups() []service.PlazaGroup {
	return []service.PlazaGroup{
		{ID: 1, Name: "public-standard", Platform: "anthropic", SubscriptionType: "standard", RateMultiplier: 1},
		{ID: 2, Name: "exclusive-a", Platform: "anthropic", IsExclusive: true, RateMultiplier: 0.5},
		{ID: 3, Name: "public-subscription", Platform: "openai", SubscriptionType: "subscription", RateMultiplier: 1},
		{ID: 4, Name: "exclusive-b", Platform: "openai", IsExclusive: true, RateMultiplier: 0.8},
	}
}

func TestFilterPlazaVisibleGroups_AnonymousSeesOnlyNonExclusive(t *testing.T) {
	// 匿名(allowedExclusive == nil):仅非专属分组;订阅型公开分组照常可见(橱窗语义)。
	visible := filterPlazaVisibleGroups(plazaGroups(), nil)
	require.Len(t, visible, 2)
	ids := []int64{visible[0].ID, visible[1].ID}
	require.ElementsMatch(t, []int64{1, 3}, ids)
}

func TestFilterPlazaVisibleGroups_AuthedSeesGrantedExclusive(t *testing.T) {
	// 登录:非专属 + 授权的专属;未授权的专属仍不可见。
	allowed := map[int64]struct{}{2: {}}
	visible := filterPlazaVisibleGroups(plazaGroups(), allowed)
	require.Len(t, visible, 3)
	ids := make([]int64, 0, len(visible))
	for _, g := range visible {
		ids = append(ids, g.ID)
	}
	require.ElementsMatch(t, []int64{1, 2, 3}, ids)
}

func TestFilterPlazaVisibleGroups_AuthedEmptySetSeesNoExclusive(t *testing.T) {
	// 登录但无任何专属授权(空集合,非 nil):与匿名同样只见非专属,
	// 但语义区分要保持——空集合不能被当作 nil 匿名分支。
	visible := filterPlazaVisibleGroups(plazaGroups(), map[int64]struct{}{})
	require.Len(t, visible, 2)
}

func TestModelPlazaHandler_NilSettingServiceFailsClosed404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &ModelPlazaHandler{} // settingService == nil → fail-closed
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/model-plaza", nil)

	h.Get(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestToModelPlazaGroupDTO_UserRateAndFieldWhitelist(t *testing.T) {
	g := service.PlazaGroup{
		ID: 2, Name: "vip", Description: "d", Platform: "anthropic",
		SubscriptionType: "standard", RateMultiplier: 1, IsExclusive: true,
		ImageRateIndependent: true, ImageRateMultiplier: 0.15,
		VideoRateIndependent: true, VideoRateMultiplier: 0.25,
		Models: []service.PlazaModel{{
			Name:                 "claude-sonnet",
			Platform:             "anthropic",
			FinalImagePriceTiers: []string{"1K"},
			FinalVideoPriceTiers: []string{"720p"},
			Pricing: &service.ChannelModelPricing{
				BillingMode: service.BillingModeToken,
				InputPrice:  testPtr(3e-6),
			},
			OfficialPricing: &service.PlazaOfficialPricing{
				InputPrice:     testPtr(3e-6),
				CacheReadPrice: testPtr(3e-7),
			},
		}},
	}

	// 有专属倍率:user_rate_multiplier 序列化输出
	dto := toModelPlazaGroupDTO(&g, map[int64]float64{2: 0.5})
	raw, err := json.Marshal(dto)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{
		"id", "name", "description", "platform", "subscription_type",
		"rate_multiplier", "user_rate_multiplier", "is_exclusive", "models",
		"image_rate_independent", "image_rate_multiplier",
		"video_rate_independent", "video_rate_multiplier",
		"peak_rate_enabled", "peak_start", "peak_end", "peak_rate_multiplier",
		"image_rate_independent", "image_rate_multiplier",
	} {
		_, exists := decoded[key]
		require.Truef(t, exists, "plaza group DTO must expose %q", key)
	}
	require.InDelta(t, 0.5, decoded["user_rate_multiplier"].(float64), 1e-9)
	require.Equal(t, true, decoded["image_rate_independent"])
	require.InDelta(t, 0.15, decoded["image_rate_multiplier"].(float64), 1e-9)
	require.Equal(t, true, decoded["video_rate_independent"])
	require.InDelta(t, 0.25, decoded["video_rate_multiplier"].(float64), 1e-9)

	// 模型条目:pricing + official_pricing 并存;official 缺失字段输出 null 而非省略
	models := decoded["models"].([]any)
	require.Len(t, models, 1)
	model := models[0].(map[string]any)
	require.Contains(t, model, "pricing")
	require.Contains(t, model, "official_pricing")
	require.Equal(t, []any{"1K"}, model["final_image_price_tiers"])
	require.Equal(t, []any{"720p"}, model["final_video_price_tiers"])
	official := model["official_pricing"].(map[string]any)
	require.Contains(t, official, "input_price")
	require.Contains(t, official, "cache_read_price")
	_, has1h := official["cache_write_1h_price"]
	require.False(t, has1h, "1h 缓存写价为 nil 时应 omitempty")

	// 无专属倍率:user_rate_multiplier 整个字段省略
	dtoNoRate := toModelPlazaGroupDTO(&g, nil)
	rawNoRate, err := json.Marshal(dtoNoRate)
	require.NoError(t, err)
	var decodedNoRate map[string]any
	require.NoError(t, json.Unmarshal(rawNoRate, &decodedNoRate))
	_, hasRate := decodedNoRate["user_rate_multiplier"]
	require.False(t, hasRate, "无专属倍率时 user_rate_multiplier 应 omitempty")
}

func TestToModelPlazaOfficialPricing_NilPassthrough(t *testing.T) {
	require.Nil(t, toModelPlazaOfficialPricing(nil))
}

func TestSelectModelPlazaModels_OpenAIImageGroupsKeepOnlyConfiguredGPTImageTiers(t *testing.T) {
	image := plazaMediaCatalogModel(
		"gpt-image-2",
		service.PlatformOpenAI,
		service.BillingModeImage,
		[]string{service.ImageBillingSize1K, service.ImageBillingSize2K, service.ImageBillingSize4K},
	)
	tests := []struct {
		name      string
		groupName string
		wantTiers []string
	}{
		{name: "1K group", groupName: "【生图专属】image 1K", wantTiers: []string{service.ImageBillingSize1K}},
		{name: "2K and 4K group", groupName: " image生图 2k/4K", wantTiers: []string{service.ImageBillingSize2K, service.ImageBillingSize4K}},
		{name: "2 slash 4K group", groupName: "【生图专属】image原生 2/4K", wantTiers: []string{service.ImageBillingSize2K, service.ImageBillingSize4K}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := service.PlazaGroup{
				Name: tt.groupName, Platform: service.PlatformOpenAI,
				Models: []service.PlazaModel{
					image,
					plazaMediaCatalogModel("gpt-image-legacy", service.PlatformOpenAI, service.BillingModeImage, []string{service.ImageBillingSize1K}),
					{Name: "gpt-5.6", Platform: service.PlatformOpenAI},
				},
			}

			selected := selectModelPlazaModels(&group)

			require.Len(t, selected, 1)
			require.Equal(t, "gpt-image-2", selected[0].Name)
			require.NotNil(t, selected[0].Pricing)
			require.Len(t, selected[0].Pricing.Intervals, len(tt.wantTiers))
			require.Equal(t, tt.wantTiers, selected[0].FinalImagePriceTiers)
			for i, tier := range tt.wantTiers {
				require.Equal(t, tier, selected[0].Pricing.Intervals[i].TierLabel)
			}
			require.Len(t, image.Pricing.Intervals, 3, "catalog filtering must not mutate service data")
		})
	}
}

func TestSelectModelPlazaModels_OtherOpenAIGroupsKeepOnlyText(t *testing.T) {
	group := service.PlazaGroup{
		Name: "Codex", Platform: service.PlatformOpenAI,
		Models: []service.PlazaModel{
			{Name: "gpt-5.6", Platform: service.PlatformOpenAI},
			{Name: "gpt-image-without-pricing", Platform: service.PlatformOpenAI},
			plazaMediaCatalogModel("gpt-image-2", service.PlatformOpenAI, service.BillingModeImage, []string{service.ImageBillingSize1K}),
			plazaMediaCatalogModel("sora-video", service.PlatformOpenAI, service.BillingModeVideo, []string{service.VideoBillingResolution720P}),
			{Name: "search-tool", Platform: service.PlatformOpenAI, Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModePerRequest}},
		},
	}

	selected := selectModelPlazaModels(&group)

	require.Len(t, selected, 1)
	require.Equal(t, "gpt-5.6", selected[0].Name)
}

func TestSelectModelPlazaModels_GrokUsesStrictCatalogAndFinalTiers(t *testing.T) {
	group := service.PlazaGroup{
		Name: "Grok", Platform: service.PlatformGrok,
		Models: []service.PlazaModel{
			{Name: "grok-4.5", Platform: service.PlatformGrok},
			{Name: "grok-4.6", Platform: service.PlatformGrok},
			{Name: "grok-4.4", Platform: service.PlatformGrok},
			plazaMediaCatalogModel("grok-imagine-image", service.PlatformGrok, service.BillingModeImage, []string{service.ImageBillingSize1K, service.ImageBillingSize2K, service.ImageBillingSize4K}),
			plazaMediaCatalogModel("grok-imagine-image-2.0", service.PlatformGrok, service.BillingModeImage, []string{service.ImageBillingSize1K, service.ImageBillingSize2K, service.ImageBillingSize4K}),
			plazaMediaCatalogModel("grok-imagine-image-quality", service.PlatformGrok, service.BillingModeImage, []string{service.ImageBillingSize1K, service.ImageBillingSize2K, service.ImageBillingSize4K}),
			plazaMediaCatalogModel("grok-imagine", service.PlatformGrok, service.BillingModeImage, []string{service.ImageBillingSize1K}),
			plazaMediaCatalogModel(service.VideoPriceFamilyGrokImagineVideo, service.PlatformGrok, service.BillingModeVideo, []string{service.VideoBillingResolution480P, service.VideoBillingResolution720P, service.VideoBillingResolution1080P}),
			plazaMediaCatalogModel(service.VideoPriceFamilyGrokImagineVideo15, service.PlatformGrok, service.BillingModeVideo, []string{service.VideoBillingResolution480P, service.VideoBillingResolution720P, service.VideoBillingResolution1080P}),
			plazaMediaCatalogModel("grok-imagine-video-1.5-preview", service.PlatformGrok, service.BillingModeVideo, []string{service.VideoBillingResolution720P}),
		},
	}

	selected := selectModelPlazaModels(&group)

	names := make([]string, 0, len(selected))
	for _, model := range selected {
		names = append(names, model.Name)
		if model.Pricing == nil {
			continue
		}
		switch model.Pricing.BillingMode {
		case service.BillingModeImage:
			require.Equal(t, []string{service.ImageBillingSize1K, service.ImageBillingSize2K}, model.FinalImagePriceTiers)
			require.Len(t, model.Pricing.Intervals, 2)
		case service.BillingModeVideo:
			require.Equal(t, []string{
				service.VideoBillingResolution480P,
				service.VideoBillingResolution720P,
				service.VideoBillingResolution1080P,
			}, model.FinalVideoPriceTiers)
			require.Len(t, model.Pricing.Intervals, 3)
		}
	}
	require.ElementsMatch(t, []string{
		"grok-4.5",
		"grok-4.6",
		"grok-imagine-image",
		"grok-imagine-image-2.0",
		"grok-imagine-image-quality",
		service.VideoPriceFamilyGrokImagineVideo,
		service.VideoPriceFamilyGrokImagineVideo15,
	}, names)
}

func TestSelectModelPlazaModels_ClaudeCatalogIsUnchanged(t *testing.T) {
	group := service.PlazaGroup{
		Name: "Claude", Platform: service.PlatformAnthropic,
		Models: []service.PlazaModel{
			{Name: "claude-opus", Platform: service.PlatformAnthropic},
			{Name: "claude-sonnet", Platform: service.PlatformAnthropic},
		},
	}

	selected := selectModelPlazaModels(&group)

	require.Equal(t, group.Models, selected)
}

func plazaMediaCatalogModel(
	name string,
	platform string,
	mode service.BillingMode,
	tiers []string,
) service.PlazaModel {
	intervals := make([]service.PricingInterval, 0, len(tiers))
	for i, tier := range tiers {
		price := float64(i+1) / 100
		intervals = append(intervals, service.PricingInterval{
			TierLabel: tier, PerRequestPrice: &price, SortOrder: i,
		})
	}
	model := service.PlazaModel{
		Name: name, Platform: platform,
		Pricing: &service.ChannelModelPricing{BillingMode: mode, Intervals: intervals},
	}
	if mode == service.BillingModeVideo {
		model.FinalVideoPriceTiers = append([]string(nil), tiers...)
	} else {
		model.FinalImagePriceTiers = append([]string(nil), tiers...)
	}
	return model
}

func testPtr(v float64) *float64 { return &v }
