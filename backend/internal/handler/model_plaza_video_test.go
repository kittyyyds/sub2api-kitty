//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type plazaVideoChannelRepo struct{ service.ChannelRepository }

func (plazaVideoChannelRepo) ListAll(context.Context) ([]service.Channel, error) { return nil, nil }

type plazaVideoGroupRepo struct {
	service.GroupRepository
	group service.Group
}

func (r plazaVideoGroupRepo) ListActive(context.Context) ([]service.Group, error) {
	return []service.Group{r.group}, nil
}

func plazaVideoFixture(t *testing.T, mode service.BillingMode, referencePrice float64) (*service.Group, service.PlazaGroup) {
	t.Helper()
	model := "官方直连sd2.0(满血全参不排队720P)"
	spec, ok := service.UnifiedVideoSpecification(model)
	require.True(t, ok)
	spec.Resolutions = []string{"720p", "1080p", "4k"}
	spec.Reason = "https://private-upstream.example/private 秒出稳定保真 PRIVATE_REASON"
	price := .78
	group := &service.Group{ID: 25, Name: "官方视频", Platform: service.PlatformVideo,
		RateMultiplier: 9, VideoRateIndependent: true, VideoRateMultiplier: 11, PeakRateEnabled: true, PeakRateMultiplier: 20,
		ModelPricing: []service.ChannelModelPricing{{Models: []string{model}, BillingMode: mode, PerRequestPrice: &price,
			Video: &service.VideoModelConfig{Spec: spec, ReferencePerSecond: referencePrice, ResolutionPrices: map[string]float64{"720p": .78, "4k": 5.55},
				Cost: &service.VideoSupplierPrice{Model: "PRIVATE_SUPPLIER", UnitPrice: .68}, Markup: &service.VideoMarkup{OutputPerSecond: .1},
				AppliedMarkup: service.VideoMarkup{ReferencePerSecond: .1}, AutoPrice: true, PriceSource: "upstream", SyncedAt: "PRIVATE_TIMESTAMP"}}}}
	svc := service.NewModelPlazaService(plazaVideoChannelRepo{}, plazaVideoGroupRepo{group: *group}, nil, nil, nil)
	groups, err := svc.ListGroups(t.Context())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	return group, groups[0]
}

func TestModelPlazaVideoPublicPricesMatchQuoteWithoutMultipliers(t *testing.T) {
	for _, mode := range []service.BillingMode{service.BillingModeVideo, service.BillingModePerRequest} {
		for _, referencePrice := range []float64{0, .13} {
			t.Run(fmt.Sprintf("%s/reference-%g", mode, referencePrice), func(t *testing.T) {
				group, plaza := plazaVideoFixture(t, mode, referencePrice)
				dto := toModelPlazaGroupDTO(&plaza, map[int64]float64{25: 5}, nil)
				require.Len(t, dto.Models, 1, "dedicated video is not filtered as Grok")
				model := dto.Models[0]
				require.True(t, model.Pricing.FinalUnitPrice)
				require.Equal(t, string(mode), model.Pricing.BillingMode)
				require.Equal(t, .78, *model.Pricing.PerRequestPrice)
				require.Equal(t, referencePrice, *model.Pricing.ReferencePerSecond)
				require.Equal(t, map[string]float64{"720p": .78, "4k": 5.55}, model.Pricing.ResolutionPrices)
				require.Nil(t, model.OfficialPricing)
				require.Nil(t, model.TimePricing)
				require.Contains(t, model.Description, "时长 4–15 秒")
				require.Contains(t, model.Description, "参考音频须同时提供图片或视频")
				require.Equal(t, []string{"720p", "1080p", "4k"}, model.Capabilities.Resolutions)
				require.Equal(t, 3, model.Capabilities.ReferenceVideos.Max)
				require.False(t, model.Capabilities.AllowAudioOnly)
				for resolution, unit := range model.Pricing.ResolutionPrices {
					quote, err := quoteVideoInput(t.Context(), &service.APIKey{Group: group}, videoQuoteInput{
						UnifiedVideoInput: service.UnifiedVideoInput{Model: model.Name, Seconds: 5, Resolution: resolution},
					})
					require.NoError(t, err)
					require.Equal(t, unit, quote.UnitPrice)
					require.Equal(t, *model.Pricing.ReferencePerSecond, quote.ReferencePerSecond)
					want := unit
					if mode == service.BillingModeVideo {
						want *= 5
					}
					require.InDelta(t, want, quote.EstimatedCost, 1e-8)
				}
				_, err := quoteVideoInput(t.Context(), &service.APIKey{Group: group}, videoQuoteInput{
					UnifiedVideoInput: service.UnifiedVideoInput{Model: model.Name, Seconds: 5, Resolution: "1080p"},
				})
				require.Error(t, err, "an absent tier is not free and cannot inherit the default when a tier map exists")
				raw, err := json.Marshal(dto)
				require.NoError(t, err)
				for _, forbidden := range []string{"cost", "markup", "applied_markup", "price_source", "auto_price", "synced_at", "follow_increase", "PRIVATE_", "https://private-upstream", "秒出稳定保真"} {
					require.NotContains(t, string(raw), forbidden)
				}
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(raw, &decoded))
				serializedPrice := decoded["models"].([]any)[0].(map[string]any)["pricing"].(map[string]any)
				require.Equal(t, referencePrice, serializedPrice["reference_per_second"], "explicit zero is present")
				require.Equal(t, true, serializedPrice["final_unit_price"])
			})
		}
	}
}

func TestModelPlazaVideoVisibilityStillHonorsGroupAndModelPermissions(t *testing.T) {
	_, plaza := plazaVideoFixture(t, service.BillingModeVideo, .13)
	plaza.IsExclusive = true
	require.Empty(t, filterPlazaVisibleGroups([]service.PlazaGroup{plaza}, nil, false))
	require.Empty(t, filterPlazaVisibleGroups([]service.PlazaGroup{plaza}, map[int64]struct{}{}, false))
	require.Len(t, filterPlazaVisibleGroups([]service.PlazaGroup{plaza}, map[int64]struct{}{25: {}}, false), 1)
	plaza.IsExclusive = false
	require.Empty(t, filterPlazaVisibleGroups([]service.PlazaGroup{plaza}, map[int64]struct{}{}, true))
	dto := toModelPlazaGroupDTO(&plaza, nil, []string{plaza.Models[0].Name})
	require.Empty(t, dto.Models)
}

func TestModelPlazaVideoDescriptionUsesOnlyContract(t *testing.T) {
	spec, ok := service.UnifiedVideoSpecification("官方直连sd2.0(满血全参不排队4K)")
	require.True(t, ok)
	description := modelPlazaVideoDescription(spec)
	require.Contains(t, description, "分辨率 4k")
	require.Contains(t, description, "不接受参考视频")
	require.Contains(t, description, "参考音频须同时提供图片")
	require.NotContains(t, description, "验收")
	require.NotContains(t, description, "适配")
	spec.Seconds = []int{5, 10, 15}
	spec.MaxVideos, spec.MaxReferenceSeconds, spec.MaxCombinedSeconds = 3, 15, 30
	spec.RequiresVisualOrAudio = true
	description = modelPlazaVideoDescription(spec)
	require.Contains(t, description, "可选时长 5/10/15 秒")
	require.Contains(t, description, "按测得时长和参考秒价计费")
	require.Contains(t, description, "合计不超过 30 秒")
	require.Contains(t, description, "需提供参考素材")
}

func TestModelPlazaPricingExtensionDoesNotTurnChannelOrGrokPriceIntoFinalVideoPrice(t *testing.T) {
	require.Nil(t, toModelPlazaPricing(nil))
	amount := .25
	for _, platform := range []string{"", service.PlatformGrok} {
		p := toModelPlazaPricing(&service.ChannelModelPricing{Platform: platform, BillingMode: service.BillingModeVideo, PerRequestPrice: &amount})
		require.False(t, p.FinalUnitPrice)
		require.Nil(t, p.ReferencePerSecond)
	}
	p := toModelPlazaPricing(&service.ChannelModelPricing{Platform: service.PlatformVideo, BillingMode: service.BillingModePerRequest, PerRequestPrice: &amount})
	require.True(t, p.FinalUnitPrice, "legacy dedicated-video per-request price is still final")
	require.Nil(t, p.ReferencePerSecond, "no invented reference price for a legacy contract")
	p = toModelPlazaPricing(&service.ChannelModelPricing{Platform: service.PlatformVideo, BillingMode: service.BillingModeVideo})
	require.False(t, p.FinalUnitPrice, "missing price is not free")
}
