package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func accountSyncFixture(t *testing.T) (map[string]VideoSupplierPrice, []VideoAccountModelBinding, VideoRuntimeSettings) {
	t.Helper()
	raw, err := os.ReadFile("testdata/video-account-pricing-20261009.json")
	require.NoError(t, err)
	models, _, err := parseVideoSupplierCatalog(raw)
	require.NoError(t, err)
	b := VideoAccountModelBinding{BaseURL: "https://suanliai.top/v1", Models: map[string]string{}}
	for model := range models {
		b.Models[model] = model
	}
	cfg := DefaultVideoRuntimeSettings()
	cfg.AutoImportAccountModels, cfg.AutoSync = true, true
	cfg.Markup = VideoMarkup{OutputPerSecond: .1, ReferencePerSecond: .1, PerRequest: .5}
	return models, []VideoAccountModelBinding{b}, cfg
}

func TestVideoAccountSyncAllConfiguredModelsAndCapabilities(t *testing.T) {
	models, bindings, cfg := accountSyncFixture(t)
	rows, warning := reconcileVideoAccountPricing(nil, bindings, models, cfg, "test", true)
	require.False(t, warning)
	require.Len(t, rows, 18)
	require.NoError(t, validateVideoGroupPricing(rows))
	group := &Group{Platform: PlatformVideo, ModelPricing: rows}
	require.Len(t, VideoGroupModelIDs(group), 18)
	for _, row := range rows {
		model := row.Models[0]
		cost := models[model]
		require.InDelta(t, cost.UnitPrice+.1, *row.PerRequestPrice, 1e-8, model)
		require.True(t, VideoGroupCapability(group, model).Supported)
		body, err := json.Marshal(map[string]any{"model": model, "prompt": "synthetic acceptance", "seconds": row.Video.Spec.MinSeconds, "resolution": row.Video.Spec.Resolutions[0]})
		require.NoError(t, err)
		_, err = PrepareUnifiedVideoInput(body, "application/json", row.Video.Spec)
		require.NoError(t, err, model)
		if row.Video.Spec.ReferencePricePending {
			require.Zero(t, row.Video.Spec.MaxVideos)
			require.Zero(t, row.Video.ReferencePerSecond)
		}
	}
	again, warning := reconcileVideoAccountPricing(rows, bindings, models, cfg, "test", true)
	require.False(t, warning)
	require.Equal(t, rows, again, "same observation must be idempotent")
}

func TestVideoAccountSyncAliasesScopeManualAndLifecycle(t *testing.T) {
	models, bindings, cfg := accountSyncFixture(t)
	model := "官方直连sd2.0(fast全参不排队480P)"
	bindings[0].Models = map[string]string{"public-video": model, "not-in-group": model, "missing-price": "unpriced", "wild*": model}
	bindings[0].AllowedModels = []string{"public-*", "missing-price"}
	rows, warning := reconcileVideoAccountPricing(nil, bindings, models, cfg, "t1", true)
	require.True(t, warning)
	require.Len(t, rows, 1)
	require.Equal(t, "public-video", rows[0].Models[0])
	require.Equal(t, model, rows[0].Video.Cost.Model)
	// Saving from the group editor must preserve authoritative alias cost/spec.
	saved, err := prepareVideoPricingForSave(rows, rows, VideoSupplierCatalog{Models: models}, cfg)
	require.NoError(t, err)
	require.Equal(t, model, saved[0].Video.Cost.Model)
	require.True(t, saved[0].Video.AccountSynced)
	// An explicit manual zero is still fixed, and survives removal from accounts.
	manual := rows[0].Clone()
	v := *manual.Video
	manual.Video = &v
	v.PriceSource = "manual"
	zero := 0.0
	manual.PerRequestPrice = &zero
	kept, _ := reconcileVideoAccountPricing([]ChannelModelPricing{manual}, nil, models, cfg, "t2", true)
	require.Equal(t, []ChannelModelPricing{manual}, kept)
	removed, _ := reconcileVideoAccountPricing(rows, nil, models, cfg, "t2", true)
	require.Empty(t, removed)
	// Disabling auto-import leaves existing entries and prices alone when sync is off.
	cfg.AutoImportAccountModels = false
	kept, _ = reconcileVideoAccountPricing(rows, nil, models, cfg, "t2", false)
	require.Equal(t, rows, kept)
}

func TestVideoAccountSyncConflictsMissingPricesAndFutureModels(t *testing.T) {
	models, bindings, cfg := accountSyncFixture(t)
	a, b := "官方直连sd2.0(fast全参不排队480P)", "官方直连sd2.0(fast全参不排队720P)"
	bindings[0].Models = map[string]string{"public": a}
	bindings = append(bindings, VideoAccountModelBinding{BaseURL: "https://suanliai.top/v1", Models: map[string]string{"public": b}})
	rows, warning := reconcileVideoAccountPricing(nil, bindings, models, cfg, "t1", true)
	require.True(t, warning)
	require.Empty(t, rows)
	bindings = bindings[:1]
	rows, _ = reconcileVideoAccountPricing(nil, bindings, models, cfg, "t1", true)
	kept, warning := reconcileVideoAccountPricing(rows, bindings, map[string]VideoSupplierPrice{}, cfg, "t2", true)
	require.True(t, warning)
	require.Equal(t, rows, kept)
	bindings[0].Models["public"] = b
	remapped, warning := reconcileVideoAccountPricing(rows, bindings, models, cfg, "t2", true)
	require.False(t, warning)
	require.Equal(t, b, remapped[0].Video.Cost.Model)
	require.InDelta(t, .65, *remapped[0].PerRequestPrice, 1e-8)
	// Unknown supplier IDs require no new hardcoded catalog entry.
	future, _, err := parseVideoSupplierCatalog([]byte(`{"success":true,"group_ratio":{"default":1},"data":[{"model_name":"future-video","billing_mode":"per_second","model_price":0.6},{"model_name":"unselected-image","billing_mode":"per_request","model_price":0.2}]}`))
	require.NoError(t, err)
	bindings[0].Models["future-public"] = "future-video"
	for id, cost := range future {
		models[id] = cost
	}
	updated, warning := reconcileVideoAccountPricing(remapped, bindings, models, cfg, "t3", true)
	require.False(t, warning)
	require.Len(t, updated, 2)
	require.True(t, updated[1].Video.Spec.Generic)
	require.InDelta(t, .7, *updated[1].PerRequestPrice, 1e-8)
	// An unrelated supplier cannot inherit this provider's price by matching ID.
	bindings[0].BaseURL = "https://another-provider.example/v1"
	rows, warning = reconcileVideoAccountPricing(nil, bindings, models, cfg, "t4", true)
	require.True(t, warning)
	require.Empty(t, rows)
}

func TestVideoAccountSyncKeepsConfiguredDirectionAndManualPrices(t *testing.T) {
	models, bindings, cfg := accountSyncFixture(t)
	model := "官方直连sd2.0(fast全参不排队480P)"
	bindings[0].Models = map[string]string{model: model}
	rows, _ := reconcileVideoAccountPricing(nil, bindings, models, cfg, "t1", true)
	cost := models[model]
	cost.UnitPrice = .2
	models[model] = cost
	updated, warning := reconcileVideoAccountPricing(rows, bindings, models, cfg, "t2", true)
	require.False(t, warning)
	require.InDelta(t, .42, *updated[0].PerRequestPrice, 1e-8)
	cost.UnitPrice = .8
	models[model] = cost
	updated, warning = reconcileVideoAccountPricing(updated, bindings, models, cfg, "t3", true)
	require.False(t, warning)
	require.InDelta(t, .9, *updated[0].PerRequestPrice, 1e-8)
	updated[0].Video.PriceSource = "manual"
	fixed, warning := reconcileVideoAccountPricing(updated, bindings, models, cfg, "t4", true)
	require.False(t, warning)
	require.Equal(t, updated, fixed)
}

func TestVideoAccountSyncIncompleteStaticMetadataUsesGenericContract(t *testing.T) {
	incomplete := UnifiedVideoSpec{MinSeconds: 4, MaxSeconds: 30}
	cost, ok := selectedVideoCost(VideoSupplierPrice{Model: "selected-by-admin", BillingMode: BillingModeVideo, UnitPrice: .6, Spec: &incomplete})
	require.True(t, ok)
	require.True(t, cost.Spec.Generic)
	require.NoError(t, cost.Spec.Validate())
	require.True(t, cost.Spec.ReferencePricePending)
	require.Zero(t, cost.Spec.MaxVideos)
}
