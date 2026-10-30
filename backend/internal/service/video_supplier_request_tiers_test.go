package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func parseRequestTierFixture(t *testing.T, video map[string]any) (map[string]VideoSupplierPrice, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"success": true, "pricing_version": "request-tiers", "group_ratio": map[string]float64{"default": 1},
		"data": []map[string]any{{"model_name": "request-tier-fixture", "billing_mode": "video_multi", "model_price": 5.85, "video_pricing": video}},
	})
	require.NoError(t, err)
	catalog, _, err := parseVideoSupplierCatalog(raw)
	return catalog, err
}

func TestVideoSupplierPerRequestResolutionTiers(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "tier-only", true: "explicit-request-price"}[explicit], func(t *testing.T) {
			video := map[string]any{"output_per_second": 0, "resolution_per_request": map[string]float64{"480p": 5.85, " 720P ": 7.85, "1080p": 9.85}}
			wantUnit := 5.85
			if explicit {
				video["per_request"], wantUnit = 9.85, 9.85
			}
			catalog, err := parseRequestTierFixture(t, video)
			require.NoError(t, err)
			cost := catalog["request-tier-fixture"]
			require.Empty(t, cost.UnavailableReason)
			require.Equal(t, BillingModePerRequest, cost.BillingMode)
			require.Equal(t, wantUnit, cost.UnitPrice)
			require.Equal(t, map[string]float64{"480p": 5.85, "720p": 7.85, "1080p": 9.85}, cost.ResolutionPrices)
			require.True(t, cost.Spec.ReferencePricePending)
			require.Zero(t, cost.Spec.MaxVideos, "absent reference pricing must not become free reference input")
		})
	}
}

func TestVideoSupplierPerRequestRejectsMixedUnitsAndInvalidTiers(t *testing.T) {
	for name, patch := range map[string]map[string]any{
		"output-seconds":     {"output_per_second": .2},
		"reference-seconds":  {"ref_video_per_second": .1},
		"resolution-seconds": {"resolution_per_second": map[string]float64{"720p": .3}},
		"empty-tier-only":    {"per_request": nil, "resolution_per_request": map[string]float64{}},
	} {
		t.Run(name, func(t *testing.T) {
			video := map[string]any{"output_per_second": 0, "per_request": 5.85, "resolution_per_request": map[string]float64{"720p": 7.85}}
			for k, v := range patch {
				video[k] = v
			}
			catalog, err := parseRequestTierFixture(t, video)
			require.NoError(t, err)
			require.NotEmpty(t, catalog["request-tier-fixture"].UnavailableReason)
		})
	}
	for name, tiers := range map[string]map[string]float64{
		"negative":  {"720p": -1},
		"empty":     {" ": 1},
		"duplicate": {"720p": 1, " 720P ": 2},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseRequestTierFixture(t, map[string]any{"output_per_second": 0, "resolution_per_request": tiers})
			require.Error(t, err)
		})
	}
	_, err := parseRequestTierFixture(t, map[string]any{"output_per_second": 0, "per_request": -1})
	require.Error(t, err)
}

func TestVideoPerRequestTiersFollowRequestMarkupAndLockedCharge(t *testing.T) {
	catalog, err := parseRequestTierFixture(t, map[string]any{"output_per_second": 0, "resolution_per_request": map[string]float64{"480p": 5.85, "720p": 7.85}})
	require.NoError(t, err)
	cost := catalog["request-tier-fixture"]
	cfg := DefaultVideoRuntimeSettings()
	cfg.Markup = VideoMarkup{OutputPerSecond: .1, PerRequest: .5}
	entry := ChannelModelPricing{Models: []string{cost.Model}, Video: &VideoModelConfig{AutoPrice: true, Spec: *cost.Spec}}
	first, err := repriceVideoEntry(entry, cost, cfg, "initial", true)
	require.NoError(t, err)
	require.InDelta(t, 6.35, *first.PerRequestPrice, 1e-8)
	require.InDelta(t, 8.35, first.Video.ResolutionPrices["720p"], 1e-8)
	require.NoError(t, validateVideoGroupPricing([]ChannelModelPricing{first}))
	snapshot := &VideoPriceSnapshot{Model: cost.Model, BillingMode: BillingModePerRequest, UnitPrice: first.Video.ResolutionPrices["720p"], DurationSeconds: 30, FinalUnitPrice: true, RateMultiplier: 1}

	cost.UnitPrice = 5
	cost.ResolutionPrices = map[string]float64{"480p": 5, "720p": 7}
	cfg.Markup = VideoMarkup{OutputPerSecond: .01, PerRequest: .25}
	kept, err := repriceVideoEntry(first, cost, cfg, "decrease-disabled", false)
	require.NoError(t, err)
	require.InDelta(t, 6.1, *kept.PerRequestPrice, 1e-8)
	require.InDelta(t, 8.1, kept.Video.ResolutionPrices["720p"], 1e-8)
	cfg.FollowDecrease = true
	next, err := repriceVideoEntry(kept, cost, cfg, "decrease-enabled", false)
	require.NoError(t, err)
	require.InDelta(t, 7.25, next.Video.ResolutionPrices["720p"], 1e-8)
	cost.ResolutionPrices["720p"] = 8
	next, err = repriceVideoEntry(next, cost, cfg, "increase-enabled", false)
	require.NoError(t, err)
	require.InDelta(t, 8.25, next.Video.ResolutionPrices["720p"], 1e-8)
	charged, err := calculateVideoPriceSnapshotCost(snapshot, 1)
	require.NoError(t, err)
	require.InDelta(t, 8.35, charged.ActualCost, 1e-8, "30 seconds and subsequent price changes must not multiply or change a per-request locked charge")

	changedMode := cost
	changedMode.BillingMode = BillingModeVideo
	_, err = repriceVideoEntry(first, changedMode, cfg, "changed-mode", false)
	require.Error(t, err)
	cost.ResolutionPrices["1080p"] = 10
	_, err = repriceVideoEntry(first, cost, cfg, "new-tier", false)
	require.Error(t, err)
}
