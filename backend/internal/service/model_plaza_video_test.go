//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelPlazaVideoPreservesFinalPricingAndDropsPrivateControls(t *testing.T) {
	group := videoTestGroup(BillingModeVideo, .78)
	group.RateMultiplier, group.VideoRateMultiplier = 9, 11
	group.VideoRateIndependent, group.PeakRateEnabled, group.PeakRateMultiplier = true, true, 20
	spec := unifiedFixtureSpec()
	spec.Resolutions = []string{"480p", "720p", "1080p", "4k"}
	group.ModelPricing[0].Video = &VideoModelConfig{
		Spec: spec, ReferencePerSecond: .13, ResolutionPrices: map[string]float64{"720p": .78, "4k": 5.55},
		Cost:   &VideoSupplierPrice{Model: "supplier-private", UnitPrice: .68, ReferencePerSecond: .03},
		Markup: &VideoMarkup{OutputPerSecond: .1}, AppliedMarkup: VideoMarkup{ReferencePerSecond: .1},
		PriceSource: "upstream", AutoPrice: true, FollowIncrease: testPtrBool(true), SyncedAt: "private-timestamp",
	}
	channel := plazaPricedChannel(1, "probe-only", []int64{group.ID}, PlatformVideo, "unselected", "exact-video")
	catalog := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{"exact-video": {InputCostPerToken: 99, OutputCostPerToken: 99}})
	groups, err := newPlazaService([]Channel{channel}, []Group{*group}, catalog).ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Models, 1, "channel/probe models must not become selling models")
	model := groups[0].Models[0]
	require.Equal(t, "exact-video", model.Name)
	require.Nil(t, model.OfficialPricing, "a generic token reference price must not accompany a dedicated video price")
	price, err := ResolveVideoGroupPrice(group, model.Name)
	require.NoError(t, err)
	require.Equal(t, price.UnitPrice, *model.Pricing.PerRequestPrice)
	require.Equal(t, price.Video.ReferencePerSecond, model.Pricing.Video.ReferencePerSecond)
	require.Equal(t, price.Video.ResolutionPrices, model.Pricing.Video.ResolutionPrices)
	require.Equal(t, price.Video.Spec, model.Pricing.Video.Spec)
	require.Nil(t, model.Pricing.Video.Cost)
	require.Nil(t, model.Pricing.Video.Markup)
	require.Nil(t, model.Pricing.Video.FollowIncrease)
	require.Zero(t, model.Pricing.Video.AppliedMarkup)
	require.False(t, model.Pricing.Video.AutoPrice)
	require.Empty(t, model.Pricing.Video.PriceSource)
	require.Empty(t, model.Pricing.Video.SyncedAt)
	require.NotContains(t, model.Pricing.Video.ResolutionPrices, "1080p", "missing output tiers must not be invented")
	model.Pricing.Video.ResolutionPrices["4k"] = 0
	require.Equal(t, 5.55, group.ModelPricing[0].Video.ResolutionPrices["4k"], "public projection must not alias saved price maps")
}

func TestModelPlazaVideoDoesNotPublishUnpricedOrProbeOnlyGroups(t *testing.T) {
	for _, mode := range []string{"empty", "probe-only", "missing-selling-price"} {
		t.Run(mode, func(t *testing.T) {
			group := &Group{ID: 25, Platform: PlatformVideo, Name: "官方视频", ModelAllowlist: GroupModelAllowlist{Models: []string{"exact-video"}}}
			var channels []Channel
			if mode == "probe-only" {
				channels = []Channel{plazaPricedChannel(1, "probe", []int64{25}, PlatformVideo, "exact-video")}
			}
			if mode == "missing-selling-price" {
				group.ModelPricing = []ChannelModelPricing{{Models: []string{"exact-video"}, BillingMode: BillingModeVideo,
					Video: &VideoModelConfig{Spec: unifiedFixtureSpec(), Cost: &VideoSupplierPrice{UnitPrice: .68}}}}
			}
			groups, err := newPlazaService(channels, []Group{*group}, nil).ListGroups(context.Background())
			require.NoError(t, err)
			require.Empty(t, groups)
		})
	}
}

func TestModelPlazaVideoPreservesExplicitFreeAndPerRequestPrices(t *testing.T) {
	for _, mode := range []BillingMode{BillingModeVideo, BillingModePerRequest} {
		for _, amount := range []float64{0, 1.25} {
			group := videoTestGroup(mode, amount)
			groups, err := newPlazaService(nil, []Group{*group}, nil).ListGroups(context.Background())
			require.NoError(t, err)
			require.Len(t, groups, 1)
			require.Len(t, groups[0].Models, 1)
			require.Equal(t, mode, groups[0].Models[0].Pricing.BillingMode)
			require.NotNil(t, groups[0].Models[0].Pricing.PerRequestPrice)
			require.Equal(t, amount, *groups[0].Models[0].Pricing.PerRequestPrice)
		}
	}
}
