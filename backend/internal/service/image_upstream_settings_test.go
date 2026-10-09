package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageAccountSyncManualDirectionAliasesAndRemoval(t *testing.T) {
	cfg := DefaultImageRuntimeSettings()
	cfg.AutoImportAccountModels = true
	cfg.AutoSync = true
	cfg.Markup = .1
	bindings := []VideoAccountModelBinding{{BaseURL: "https://suanliai.top/", Models: map[string]string{"我的图片": "Banana-2-香蕉2", "别名4K": "新型号4K"}}}
	costs := map[string]ImageSupplierPrice{"Banana-2-香蕉2": {Model: "Banana-2-香蕉2", UnitPrice: .2}, "新型号4K": {Model: "新型号4K", UnitPrice: .4}}
	old := ImagePricingGroup{ID: 1, Pricing: []ChannelModelPricing{}, Prices: map[string]map[string]float64{}}
	next, warn := reconcileImageAccountPricing(old, bindings, costs, cfg, true)
	require.False(t, warn)
	require.Len(t, next.Pricing, 2)
	require.InDelta(t, .3, next.Prices["我的图片"]["2K"], 1e-8)
	require.Empty(t, old.Prices)
	costs["Banana-2-香蕉2"] = ImageSupplierPrice{Model: "Banana-2-香蕉2", UnitPrice: .1}
	cheaper, _ := reconcileImageAccountPricing(next, bindings, costs, cfg, true)
	require.InDelta(t, .3, cheaper.Prices["我的图片"]["2K"], 1e-8)
	cfg.FollowDecrease = true
	cheaper, _ = reconcileImageAccountPricing(cheaper, bindings, costs, cfg, true)
	require.InDelta(t, .2, cheaper.Prices["我的图片"]["2K"], 1e-8)
	cheaper.Prices["我的图片"]["2K"] = .75
	manual, _ := reconcileImageAccountPricing(cheaper, bindings, costs, cfg, true)
	require.Equal(t, .75, manual.Prices["我的图片"]["2K"])
	removed, _ := reconcileImageAccountPricing(manual, nil, costs, cfg, true)
	require.Len(t, removed.Pricing, 1)
	require.Equal(t, .75, removed.Prices["我的图片"]["2K"])
}
func TestImageAccountSyncMissingCostAndProviderConflict(t *testing.T) {
	cfg := DefaultImageRuntimeSettings()
	cfg.AutoImportAccountModels = true
	old := ImagePricingGroup{ID: 1, Prices: map[string]map[string]float64{}}
	bindings := []VideoAccountModelBinding{{BaseURL: "https://suanliai.top", Models: map[string]string{"any-name": "unpriced"}}}
	next, warn := reconcileImageAccountPricing(old, bindings, nil, cfg, true)
	require.True(t, warn)
	require.Empty(t, next.Pricing)
	costs := map[string]ImageSupplierPrice{"unpriced": {UnitPrice: .1}}
	bindings = append(bindings, VideoAccountModelBinding{BaseURL: "https://unrelated.example", Models: map[string]string{"any-name": "different"}})
	next, warn = reconcileImageAccountPricing(old, bindings, costs, cfg, true)
	require.True(t, warn)
	require.Empty(t, next.Pricing)
}

func TestImageAccountClearedRowsRecoverMarkupTiersAndFinalPrice(t *testing.T) {
	cfg := DefaultImageRuntimeSettings()
	cfg.AutoImportAccountModels, cfg.AutoSync = true, true
	bindings := []VideoAccountModelBinding{{BaseURL: "https://suanliai.top", Models: map[string]string{"exact-model": "upstream"}}}
	costs := map[string]ImageSupplierPrice{"upstream": {UnitPrice: .095}}
	initial, _ := reconcileImageAccountPricing(ImagePricingGroup{ID: 1}, bindings, costs, cfg, true)
	cfg.Markup = .02
	marked, _ := reconcileImageAccountPricing(initial, bindings, costs, cfg, true)
	require.InDelta(t, .115, *marked.Pricing[0].PerRequestPrice, 1e-8)
	for _, legacyManual := range []bool{false, true} {
		t.Run(fmt.Sprint(legacyManual), func(t *testing.T) {
			old := ImagePricingGroup{ID: 1, Pricing: append([]ChannelModelPricing(nil), marked.Pricing...)}
			im := *old.Pricing[0].Image
			im.AutoPrice = !legacyManual
			old.Pricing[0].Image = &im
			recovered, warn := reconcileImageAccountPricing(old, bindings, costs, cfg, true)
			require.False(t, warn)
			require.True(t, recovered.Pricing[0].Image.AutoPrice)
			require.Equal(t, map[string]float64{"1K": .115, "2K": .115, "4K": .115}, recovered.Prices["exact-model"])
			require.InDelta(t, .115, *recovered.Pricing[0].PerRequestPrice, 1e-8)
			require.Empty(t, old.Prices)
			g := &Group{Platform: PlatformOpenAI, AllowImageGeneration: true, ModelPricing: recovered.Pricing, ImageModelPrices: recovered.Prices}
			display := plazaImageDisplayPricing(&g.ModelPricing[0], g, "exact-model")
			require.Len(t, display.Intervals, 3)
			require.Len(t, plazaFinalImagePriceTiers(g, "exact-model"), 3)
			for _, tier := range display.Intervals {
				require.Equal(t, LookupImageModelPrice(g.ImageModelPrices, "exact-model", tier.TierLabel), tier.PerRequestPrice)
			}
			again, _ := reconcileImageAccountPricing(recovered, bindings, costs, cfg, true)
			require.Equal(t, recovered, again)
		})
	}
	// Explicit zero remains a manual override, even when other tiers are blank.
	marked.Prices["exact-model"] = map[string]float64{"2K": 0}
	manual, _ := reconcileImageAccountPricing(marked, bindings, costs, cfg, true)
	require.False(t, manual.Pricing[0].Image.AutoPrice)
	require.Equal(t, map[string]float64{"2K": 0}, manual.Prices["exact-model"])
	// Missing costs cannot create free tiers or show a stale billable fallback.
	missing := ImagePricingGroup{ID: 1, Pricing: marked.Pricing}
	unchanged, warning := reconcileImageAccountPricing(missing, bindings, nil, cfg, true)
	require.True(t, warning)
	require.Empty(t, unchanged.Prices)
	g := &Group{Platform: PlatformOpenAI, AllowImageGeneration: true, ModelPricing: unchanged.Pricing}
	display := plazaImageDisplayPricing(&g.ModelPricing[0], g, "exact-model")
	require.Nil(t, display.PerRequestPrice)
	require.Empty(t, display.Intervals)
}
func TestImageSupplierPricingFailsClosedAndSupportsFutureNames(t *testing.T) {
	body := []byte(`{"success":true,"pricing_version":"v1","group_ratio":{"default":1},"data":[{"model_name":"任意新型号","model_price":0.123,"billing_mode":"per_request"},{"model_name":"video","model_price":1,"billing_mode":"per_second"}]}`)
	rows, v, err := parseImageSupplierCatalog(body)
	require.NoError(t, err)
	require.Equal(t, "v1", v)
	require.Len(t, rows, 1)
	require.Equal(t, .123, rows["任意新型号"].UnitPrice)
	for _, bad := range []string{`{}`, `{"success":true,"group_ratio":{"default":2},"data":[{}]}`, `{"success":true,"group_ratio":{"default":1},"data":[{"model_name":"x","billing_mode":"per_request"}]}`} {
		_, _, err = parseImageSupplierCatalog([]byte(bad))
		require.Error(t, err)
	}
}
func TestSuanliImagesProtocolsAndResponseShapes(t *testing.T) {
	parsed := &OpenAIImagesRequest{N: 1, Model: "nano-banana-pro-稳定出图", Prompt: "test", Size: "2048x1152", InputImageURLs: []string{"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX1sAAAAASUVORK5CYII="}}
	path, body, err := suanliImageRequest(parsed, parsed.Model)
	require.NoError(t, err)
	require.Equal(t, "/v1/chat/completions", path)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, parsed.Model, payload["model"])
	require.Contains(t, string(body), "image_url")
	path, _, err = suanliImageRequest(parsed, "优质渠道-gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "/v1/images/generations", path)
	parsed.N = 2
	_, _, err = suanliImageRequest(parsed, parsed.Model)
	require.Error(t, err)
	for _, raw := range []string{`{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX1sAAAAASUVORK5CYII="}]}`, `{"data":[{"url":"https://example.com/image.png"}]}`, `{"choices":[{"message":{"content":"![image](https://example.com/result.png)"}}]}`, `{"code":0,"data":{"status":"succeeded","results":[{"url":"https://example.com/result.png"}]}}`} {
		body, task, err := suanliImageResult([]byte(raw), "application/json")
		require.NoError(t, err)
		require.Empty(t, task)
		require.Equal(t, 1, extractOpenAIImageCountFromJSONBytes(body))
	}
	_, task, err := suanliImageResult([]byte(`{"code":0,"data":{"id":"task-1"}}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "task-1", task)
	for _, raw := range []string{`{"data":[]}`, `{"code":0,"data":{"status":"failed"}}`, `{"code":500,"data":{"id":"x"}}`, `{"data":[{"url":"http://127.0.0.1/private"}]}`} {
		_, _, err := suanliImageResult([]byte(raw), "application/json")
		require.Error(t, err)
	}
}

func TestSuanliImageInvalidBytesAndPreviewAreNotBillable(t *testing.T) {
	for _, body := range []string{`{"data":[{"b64_json":"aGVsbG8="}]}`, `{"data":[{"url":"https://example.com/1.png"},{"url":"https://example.com/2.png"}]}`} {
		_, _, err := suanliImageResult([]byte(body), "application/json")
		require.Error(t, err)
	}
	_, _, err := suanliImageResult([]byte("not an image"), "image/png")
	require.Error(t, err)
	raw, pending, err := suanliImageResult([]byte(`{"code":0,"data":{"status":"running","results":[{"url":"https://example.com/preview.png"}]}}`), "application/json")
	require.NoError(t, err)
	require.Empty(t, raw)
	require.NotEmpty(t, pending)
}
