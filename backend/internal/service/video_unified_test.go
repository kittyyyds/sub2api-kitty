package service

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func unifiedFixtureSpec() UnifiedVideoSpec {
	return UnifiedVideoSpec{Supported: true, MinSeconds: 4, MaxSeconds: 15, Seconds: []int{5, 10, 15}, Resolutions: []string{"720p", "1080p"}, Ratios: []string{"16:9", "9:16"}, MaxImages: 9, MaxVideos: 3, MaxAudios: 3, MaxReferenceSeconds: 15, GenerateAudio: true}
}
func TestVideoUnifiedInputRejectsUnsupportedAndDuplicateParameters(t *testing.T) {
	base := map[string]any{"model": "fixture", "prompt": "demo", "seconds": 10, "aspect_ratio": "16:9", "resolution": "720p"}
	body, _ := json.Marshal(base)
	input, err := PrepareUnifiedVideoInput(body, "application/json", unifiedFixtureSpec())
	require.NoError(t, err)
	require.Equal(t, 10, input.Seconds)
	for _, patch := range []map[string]any{{"seconds": 6}, {"seconds": 10.5}, {"resolution": "4k"}, {"n": 2}, {"metadata": map[string]any{}}, {"face": true}, {"images": []string{"https://example.com/a", "https://example.com/a"}}, {"videos": []string{"http://127.0.0.1/private"}}} {
		copy := map[string]any{}
		for k, v := range base {
			copy[k] = v
		}
		for k, v := range patch {
			copy[k] = v
		}
		raw, _ := json.Marshal(copy)
		_, err := PrepareUnifiedVideoInput(raw, "application/json", unifiedFixtureSpec())
		require.Error(t, err, "patch=%v", patch)
	}
}
func TestVideoUnifiedMarkupAndCostDirections(t *testing.T) {
	spec := unifiedFixtureSpec()
	spec.ReferenceBilled = true
	cost := VideoSupplierPrice{Model: "fixture", BillingMode: BillingModeVideo, UnitPrice: .2, ReferencePerSecond: .1, Spec: &spec}
	cfg := DefaultVideoRuntimeSettings()
	cfg.Markup = VideoMarkup{OutputPerSecond: .1, ReferencePerSecond: .03, PerRequest: .3}
	entry := ChannelModelPricing{Models: []string{"fixture"}, Video: &VideoModelConfig{AutoPrice: true, Spec: spec}}
	first, err := repriceVideoEntry(entry, cost, cfg, "now", true)
	require.NoError(t, err)
	require.InDelta(t, .3, *first.PerRequestPrice, 1e-8)
	require.InDelta(t, .13, first.Video.ReferencePerSecond, 1e-8)
	cost.UnitPrice = .15
	cost.ReferencePerSecond = .05
	next, err := repriceVideoEntry(first, cost, cfg, "later", false)
	require.NoError(t, err)
	require.InDelta(t, .3, *next.PerRequestPrice, 1e-8)
	require.InDelta(t, .13, next.Video.ReferencePerSecond, 1e-8)
	cost.ReferencePerSecond = 0
	spec.ReferenceBilled = false
	next, err = repriceVideoEntry(next, cost, cfg, "zero-reference", false)
	require.NoError(t, err)
	require.InDelta(t, .13, next.Video.ReferencePerSecond, 1e-8)
	// Turning down one's markup is an explicit operator change, independent of following a supplier decrease.
	cfg.Markup.OutputPerSecond = .05
	next, err = repriceVideoEntry(next, cost, cfg, "later2", false)
	require.NoError(t, err)
	require.InDelta(t, .25, *next.PerRequestPrice, 1e-8)
	cfg.FollowDecrease = true
	next, err = repriceVideoEntry(next, cost, cfg, "later3", false)
	require.NoError(t, err)
	require.InDelta(t, .2, *next.PerRequestPrice, 1e-8)
	require.InDelta(t, .03, next.Video.ReferencePerSecond, 1e-8)
	cost.BillingMode = BillingModePerRequest
	cost.ReferencePerSecond = 0
	spec.ReferenceBilled = false
	cost.Spec = &spec
	each, err := repriceVideoEntry(entry, cost, cfg, "initial", true)
	require.NoError(t, err)
	require.InDelta(t, .45, *each.PerRequestPrice, 1e-8)
	require.Zero(t, each.Video.ReferencePerSecond)
	// Supplier changes cannot retroactively modify a task's immutable price.
	snapshot := &VideoPriceSnapshot{Model: "fixture", BillingMode: BillingModeVideo, UnitPrice: .3, ReferencePerSecond: .13, ReferenceSeconds: 5, DurationSeconds: 10, FinalUnitPrice: true, RateMultiplier: 1}
	charged, err := calculateVideoPriceSnapshotCost(snapshot, 1)
	require.NoError(t, err)
	require.InDelta(t, 3.65, charged.ActualCost, 1e-8)
}
func TestVideoUnifiedCatalogSupportedSpecsAreValid(t *testing.T) {
	for name, spec := range unifiedVideoSpecs {
		if spec.Supported {
			require.NoError(t, spec.Validate(), name)
		}
	}
}
