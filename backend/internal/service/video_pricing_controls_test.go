package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoPricingControlsEmptyGroupAndUnpricedRequest(t *testing.T) {
	s := &adminServiceImpl{}
	prices, err := s.normalizeSavedGroupModelPricing(context.Background(), PlatformVideo, nil, nil)
	require.NoError(t, err)
	require.Empty(t, prices)
	_, err = ResolveVideoGroupPrice(&Group{Platform: PlatformVideo, ModelPricing: prices}, "fixture")
	require.Error(t, err) // Saving an empty group does not create a free model.
}

func TestVideoPricingControlsServerCostAndManualZero(t *testing.T) {
	spec := unifiedFixtureSpec()
	spec.ReferenceBilled = true
	cost := VideoSupplierPrice{Model: "fixture", BillingMode: BillingModeVideo, UnitPrice: .2, ReferencePerSecond: .1, Spec: &spec}
	cat := VideoSupplierCatalog{Models: map[string]VideoSupplierPrice{"fixture": cost}}
	cfg := DefaultVideoRuntimeSettings()
	cfg.Markup.OutputPerSecond, cfg.Markup.ReferencePerSecond = .1, .03
	maliciousPrice := 900.0
	p := ChannelModelPricing{Models: []string{"fixture"}, BillingMode: BillingModeVideo, PerRequestPrice: &maliciousPrice, Video: &VideoModelConfig{PriceSource: "upstream", AutoPrice: true, Cost: &VideoSupplierPrice{UnitPrice: 900}}}
	rows, err := prepareVideoPricingForSave([]ChannelModelPricing{p}, nil, cat, cfg)
	require.NoError(t, err)
	require.NoError(t, validateVideoGroupPricing(rows))
	require.InDelta(t, .3, *rows[0].PerRequestPrice, 1e-8)
	require.InDelta(t, .13, rows[0].Video.ReferencePerSecond, 1e-8)
	require.Equal(t, spec, rows[0].Video.Spec)

	manual := rows[0].Clone()
	v := *manual.Video
	v.PriceSource, v.AutoPrice = "manual", true // Fixed price defeats a stale true toggle.
	manual.Video = &v
	zero := 0.0
	manual.PerRequestPrice = &zero
	rows, err = prepareVideoPricingForSave([]ChannelModelPricing{manual}, rows, cat, cfg)
	require.NoError(t, err)
	require.False(t, rows[0].Video.AutoPrice)
	cost.UnitPrice = 20
	kept, err := repriceVideoEntry(rows[0], cost, cfg, "later", false)
	require.NoError(t, err)
	require.Zero(t, *kept.PerRequestPrice)

	// Clearing fixed zero is an explicit switch back to current supplier + markup.
	inherit := kept.Clone()
	copyVideo := *inherit.Video
	copyVideo.PriceSource = "upstream"
	inherit.Video = &copyVideo
	rows, err = prepareVideoPricingForSave([]ChannelModelPricing{inherit}, []ChannelModelPricing{kept}, cat, cfg)
	require.NoError(t, err)
	require.InDelta(t, .3, *rows[0].PerRequestPrice, 1e-8)
}

func TestVideoPricingControlsUsesAuthoritativePreviousAndDirections(t *testing.T) {
	spec := unifiedFixtureSpec()
	cost := VideoSupplierPrice{Model: "fixture", BillingMode: BillingModeVideo, UnitPrice: .2, Spec: &spec}
	cat := VideoSupplierCatalog{Models: map[string]VideoSupplierPrice{"fixture": cost}}
	cfg := DefaultVideoRuntimeSettings()
	p := ChannelModelPricing{Models: []string{"fixture"}, Video: &VideoModelConfig{PriceSource: "upstream", AutoPrice: true}}
	old, err := prepareVideoPricingForSave([]ChannelModelPricing{p}, nil, cat, cfg)
	require.NoError(t, err)
	stale := old[0].Clone()
	bogus := 0.0
	stale.PerRequestPrice = &bogus
	cost.UnitPrice = .1
	cat.Models["fixture"] = cost
	rows, err := prepareVideoPricingForSave([]ChannelModelPricing{stale}, old, cat, cfg)
	require.NoError(t, err)
	require.InDelta(t, .2, *rows[0].PerRequestPrice, 1e-8) // Follow-decrease disabled.

	cfg.FollowDecrease = true
	rows, err = prepareVideoPricingForSave([]ChannelModelPricing{stale}, old, cat, cfg)
	require.NoError(t, err)
	require.InDelta(t, .1, *rows[0].PerRequestPrice, 1e-8)

	// A missing catalog retains server-persisted costs, not submitted fake costs.
	rows, err = prepareVideoPricingForSave([]ChannelModelPricing{stale}, old, VideoSupplierCatalog{}, cfg)
	require.NoError(t, err)
	require.InDelta(t, .2, *rows[0].PerRequestPrice, 1e-8)
	_, err = prepareVideoPricingForSave([]ChannelModelPricing{stale}, nil, VideoSupplierCatalog{}, cfg)
	require.Error(t, err)

	// Old custom prices are never implicitly converted or enabled for syncing.
	legacy := ChannelModelPricing{Models: []string{"legacy"}, BillingMode: BillingModePerRequest, PerRequestPrice: &bogus}
	rows, err = prepareVideoPricingForSave([]ChannelModelPricing{legacy}, nil, cat, cfg)
	require.NoError(t, err)
	require.Equal(t, legacy, rows[0])
}

func TestVideoPricingControlsManualCannotSeedUntrustedFallback(t *testing.T) {
	spec := unifiedFixtureSpec()
	price := .5
	p := ChannelModelPricing{Models: []string{"fixture"}, BillingMode: BillingModeVideo, PerRequestPrice: &price,
		Video: &VideoModelConfig{PriceSource: "manual", Spec: spec, Cost: &VideoSupplierPrice{Model: "fixture", UnitPrice: .01, Spec: &spec}}}
	rows, err := prepareVideoPricingForSave([]ChannelModelPricing{p}, nil, VideoSupplierCatalog{}, DefaultVideoRuntimeSettings())
	require.NoError(t, err)
	require.Nil(t, rows[0].Video.Cost)
	inherit := rows[0].Clone()
	video := *inherit.Video
	video.PriceSource = "upstream"
	inherit.Video = &video
	_, err = prepareVideoPricingForSave([]ChannelModelPricing{inherit}, rows, VideoSupplierCatalog{}, DefaultVideoRuntimeSettings())
	require.Error(t, err)
}
