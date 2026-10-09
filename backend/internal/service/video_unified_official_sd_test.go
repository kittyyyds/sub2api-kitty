package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const officialSD4KModel = "官方直连sd2.0(满血全参不排队4K)"

// Published /api/docs/content and /api/pricing snapshot, 2026-10-09.
// These are adapter contracts, not evidence of real generation acceptance.
var officialSDCases = []struct {
	model, resolution                           string
	maxSeconds, maxImages, maxVideos, maxAudios int
	outputPrice                                 float64
	resolutionPrice                             bool
}{
	{"官方直连sd2.0(满血全参不排队480P)", "480p", 15, 9, 3, 3, .46, true},
	{"官方直连sd2.0(满血全参不排队720P)", "720p", 15, 9, 3, 3, .68, true},
	{"官方直连sd2.0(满血全参不排队1080P)", "1080p", 15, 9, 3, 3, 1.65, true},
	{officialSD4KModel, "4k", 15, 9, 0, 3, 5.45, false},
	{"官方直连sd2.0(mini全参不排队480P)", "480p", 15, 9, 3, 3, .093, true},
	{"官方直连sd2.0(mini全参不排队720P)", "720p", 15, 9, 3, 3, .22, true},
	{"官方直连sd2.0(fast全参不排队480P)", "480p", 15, 9, 3, 3, .32, true},
	{"官方直连sd2.0(fast全参不排队720P)", "720p", 15, 9, 3, 3, .55, true},
	{"官方直连sd2.5(满血全参不排队480P)", "480p", 30, 30, 10, 10, .82, false},
	{"官方直连sd2.5(满血全参不排队720P)", "720p", 30, 30, 10, 10, 1.1, false},
	{"官方直连sd2.5(满血全参不排队1080P)", "1080p", 30, 30, 10, 10, 2.4, true},
}

func officialSDURLs(kind string, count int) []string {
	urls := make([]string, count)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/%s/%d", kind, i)
	}
	return urls
}

func TestVideoUnifiedOfficialSDPublishedLimits(t *testing.T) {
	for _, tc := range officialSDCases {
		t.Run(tc.model, func(t *testing.T) {
			spec, ok := UnifiedVideoSpecification(tc.model)
			require.True(t, ok)
			require.NoError(t, spec.Validate())
			reason := "按上游文档适配，尚未完成真实生成验收"
			if tc.model == officialSD4KModel {
				reason = "支持文生/参考图及音频；参考视频计费未明确，暂不接受视频参考；未真实生成验收"
				require.Zero(t, spec.MaxReferenceSeconds)
			} else {
				require.Equal(t, float64(15), spec.MaxReferenceSeconds)
			}
			require.Equal(t, reason, spec.Reason)
			require.Equal(t, []string{tc.resolution}, spec.Resolutions)
			require.False(t, spec.ReferenceBilled, "never infer same-rate fallback from current video_multi prices")
			require.False(t, spec.AllowAudioOnly)
			for _, seconds := range []int{4, 6, tc.maxSeconds} {
				body := map[string]any{"model": tc.model, "prompt": "A landscape", "seconds": seconds}
				raw, err := json.Marshal(body)
				require.NoError(t, err)
				input, err := PrepareUnifiedVideoInput(raw, "application/json", spec)
				require.NoError(t, err, "text-only seconds=%d", seconds)
				require.Equal(t, tc.resolution, input.Resolution)
				require.Equal(t, tc.model, input.Model)
			}
			body := map[string]any{
				"model": tc.model, "prompt": "A landscape", "seconds": tc.maxSeconds, "aspect_ratio": "9:16",
				"images": officialSDURLs("image", tc.maxImages), "videos": officialSDURLs("video", tc.maxVideos),
				"audios": officialSDURLs("audio", tc.maxAudios),
			}
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			_, err = PrepareUnifiedVideoInput(raw, "application/json", spec)
			require.NoError(t, err, "documented maximum material counts")
			for _, patch := range []map[string]any{
				{"seconds": 3}, {"seconds": tc.maxSeconds + 1},
				{"images": officialSDURLs("image", tc.maxImages+1)},
				{"videos": officialSDURLs("video", tc.maxVideos+1)},
				{"audios": officialSDURLs("audio", tc.maxAudios+1)},
				{"images": []string{}, "videos": []string{}}, // Audio needs a visual reference.
			} {
				changed := make(map[string]any, len(body))
				for key, value := range body {
					changed[key] = value
				}
				for key, value := range patch {
					changed[key] = value
				}
				raw, err = json.Marshal(changed)
				require.NoError(t, err)
				_, err = PrepareUnifiedVideoInput(raw, "application/json", spec)
				require.Error(t, err, "patch=%v", patch)
			}
		})
	}
}

func TestVideoUnifiedOfficialSD4KRejectsVideoBeforeTaskPreparation(t *testing.T) {
	spec, ok := UnifiedVideoSpecification(officialSD4KModel)
	require.True(t, ok)
	require.NoError(t, spec.Validate())
	price := &VideoGroupPrice{Model: officialSD4KModel, BillingMode: BillingModeVideo, UnitPrice: 5.45, Video: &VideoModelConfig{Spec: spec}}
	account := &Account{Platform: PlatformVideo, Type: AccountTypeAPIKey, Credentials: map[string]any{"grok_media_provider": SuanliSpecialVideoProvider}}
	jsonBody, err := json.Marshal(map[string]any{
		"model": officialSD4KModel, "prompt": "A landscape", "seconds": 5, "resolution": "4K",
		"images": []string{"https://example.com/image.png"}, "videos": []string{"https://example.com/ref.mp4"},
	})
	require.NoError(t, err)
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	for key, value := range map[string]string{
		"model": officialSD4KModel, "prompt": "A landscape", "seconds": "5", "resolution": "4K",
		"images[]": "https://example.com/image.png", "videos[]": "https://example.com/ref.mp4",
	} {
		require.NoError(t, w.WriteField(key, value))
	}
	require.NoError(t, w.Close())
	for _, tc := range []struct {
		body []byte
		ct   string
	}{{jsonBody, "application/json"}, {form.Bytes(), w.FormDataContentType()}} {
		// No HTTP client, media store, API key, settings or billing repository:
		// the real submission path must reject before reaching any of them.
		svc := &OpenAIGatewayService{}
		task, err := svc.beginUnifiedVideoTask(t.Context(), nil, account, nil, officialSD4KModel, tc.body, tc.ct, price)
		require.EqualError(t, err, "videos素材数量超出限制")
		require.Nil(t, task)
	}
	capability := price.Video.Capability()
	require.True(t, capability.Supported)
	require.Zero(t, capability.ReferenceVideos.Max)
	require.Zero(t, capability.MaxReferenceSeconds)
}

func officialSDParsePriceRows(t *testing.T, rows []map[string]any) map[string]VideoSupplierPrice {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"success": true, "group_ratio": map[string]float64{"default": 1}, "data": rows})
	require.NoError(t, err)
	catalog, _, err := parseVideoSupplierCatalog(raw)
	require.NoError(t, err)
	return catalog
}

func TestVideoUnifiedOfficialSDPublishedPricesCanReprice(t *testing.T) {
	for _, tc := range officialSDCases {
		t.Run(tc.model, func(t *testing.T) {
			row := map[string]any{"model_name": tc.model, "model_price": tc.outputPrice, "billing_mode": "per_second"}
			if tc.model != officialSD4KModel {
				row["billing_mode"] = "video_multi"
				video := map[string]any{"output_per_second": tc.outputPrice, "ref_video_per_second": tc.outputPrice}
				if tc.resolutionPrice {
					video["resolution_per_second"] = map[string]float64{tc.resolution: tc.outputPrice}
				}
				row["video_pricing"] = video
			}
			cost := officialSDParsePriceRows(t, []map[string]any{row})[tc.model]
			require.Empty(t, cost.UnavailableReason)
			require.NoError(t, cost.Spec.Validate())
			cfg := DefaultVideoRuntimeSettings()
			cfg.Markup = VideoMarkup{OutputPerSecond: .1, ReferencePerSecond: .03}
			entry, err := repriceVideoEntry(ChannelModelPricing{Models: []string{tc.model}, Video: &VideoModelConfig{Spec: *cost.Spec}}, cost, cfg, "test", true)
			require.NoError(t, err)
			require.NoError(t, validateVideoGroupPricing([]ChannelModelPricing{entry}))
			require.InDelta(t, tc.outputPrice+.1, *entry.PerRequestPrice, 1e-8)
			if tc.model == officialSD4KModel {
				require.False(t, cost.ReferencePriceKnown)
				require.Zero(t, cost.ReferencePerSecond)
				require.Zero(t, entry.Video.ReferencePerSecond, "no reference markup for unsupported video inputs")
			} else {
				require.True(t, cost.ReferencePriceKnown)
				require.InDelta(t, tc.outputPrice+.03, entry.Video.ReferencePerSecond, 1e-8)
			}
		})
	}
}

func TestVideoUnifiedOfficialSDVideoMultiKeepsIndependentReferencePrice(t *testing.T) {
	for _, tc := range officialSDCases {
		if tc.model == officialSD4KModel {
			continue
		}
		t.Run(tc.model, func(t *testing.T) {
			for _, reference := range []any{.11, 1.7, 0.0, nil} {
				video := map[string]any{"output_per_second": .5, "resolution_per_second": map[string]float64{" " + strings.ToUpper(tc.resolution) + " ": .7}}
				if reference != nil {
					video["ref_video_per_second"] = reference
				}
				row := map[string]any{"model_name": tc.model, "billing_mode": "video_multi", "model_price": 9.0, "video_pricing": video}
				cost := officialSDParsePriceRows(t, []map[string]any{row})[tc.model]
				if reference == nil {
					require.Empty(t, cost.UnavailableReason, "a missing reference price must not hide a valid output price")
					require.False(t, cost.ReferencePriceKnown)
					require.True(t, cost.Spec.ReferencePricePending)
					require.Zero(t, cost.Spec.MaxVideos)
					entry, err := repriceVideoEntry(ChannelModelPricing{Models: []string{tc.model}, Video: &VideoModelConfig{Spec: *cost.Spec}}, cost, DefaultVideoRuntimeSettings(), "test", true)
					require.NoError(t, err)
					require.Zero(t, entry.Video.ReferencePerSecond)
					_, err = PrepareUnifiedVideoInput([]byte(`{"model":"fixture","prompt":"x","seconds":5,"videos":["https://example.com/ref.mp4"]}`), "application/json", entry.Video.Spec)
					require.Error(t, err, "unpriced reference video must fail before billing or submission")
					continue
				}
				require.Empty(t, cost.UnavailableReason)
				require.True(t, cost.ReferencePriceKnown, "explicit zero is a known reference price")
				// Persisting the supplier catalog must preserve the zero/missing distinction.
				raw, err := json.Marshal(cost)
				require.NoError(t, err)
				var stored VideoSupplierPrice
				require.NoError(t, json.Unmarshal(raw, &stored))
				require.True(t, stored.ReferencePriceKnown)
				require.Equal(t, .5, cost.UnitPrice)
				require.Len(t, cost.ResolutionPrices, 1)
				require.Equal(t, .7, cost.ResolutionPrices[tc.resolution])
				require.Equal(t, reference.(float64), cost.ReferencePerSecond)
				cfg := DefaultVideoRuntimeSettings()
				cfg.Markup = VideoMarkup{OutputPerSecond: .1, ReferencePerSecond: .03}
				entry, err := repriceVideoEntry(ChannelModelPricing{Models: []string{tc.model}, Video: &VideoModelConfig{Spec: *stored.Spec}}, stored, cfg, "test", true)
				require.NoError(t, err)
				require.InDelta(t, reference.(float64)+.03, entry.Video.ReferencePerSecond, 1e-8)
				require.InDelta(t, .8, entry.Video.ResolutionPrices[tc.resolution], 1e-8)
				snapshot := &VideoPriceSnapshot{Model: tc.model, BillingMode: BillingModeVideo, UnitPrice: entry.Video.ResolutionPrices[tc.resolution], DurationSeconds: 5, ReferenceSeconds: 2, ReferencePerSecond: entry.Video.ReferencePerSecond, FinalUnitPrice: true}
				charged, err := calculateVideoPriceSnapshotCost(snapshot, 1)
				require.NoError(t, err)
				require.InDelta(t, 5*.8+2*(reference.(float64)+.03), charged.ActualCost, 1e-8)
			}
		})
	}
}
