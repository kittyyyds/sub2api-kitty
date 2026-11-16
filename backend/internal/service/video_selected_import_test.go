//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// Public pricing snapshot and account 205's selected IDs, 2026-10-09.
// These are offline contract fixtures, not real generation acceptance.
var selectedVideoImportCases = []struct {
	model, upstreamMode string
	cost, selling       float64
}{
	{"特价/sd-2.5", "per_request", 1.95, 2.45},
	{"特价/官方h3-2k", "per_second", .25, .35},
	{"特价/官方h3-720p", "per_second", .18, .28},
	{"特价/官方h3-1080p", "per_second", .23, .33},
	{"sd2.0(满血9-3-3不卡脸720P)", "per_request", 2.55, 3.05},
	{"sd2.5(满血30-10-10不卡脸720P)", "per_request", 4, 4.5},
	{"官转稳定sd2.0(933原生过真人480P)", "per_second", .42, .52},
	{"官转稳定sd2.0(933原生过真人720P)", "per_second", .75, .85},
	{"官转稳定sd2.5(满血30-10-10不卡脸1080P)", "per_second", .88, .98},
}

type selectedVideoImportRepo struct {
	UsageLogRepository
	VideoPricingRepository
	group  *Group
	writes int
}

func (r *selectedVideoImportRepo) ListVideoPriceChanges(context.Context) ([]VideoPriceChange, error) {
	return nil, nil
}

func (r *selectedVideoImportRepo) CompareAndSwapVideoPricing(_ context.Context, id int64, previous, next []ChannelModelPricing) (bool, error) {
	if id != r.group.ID || !reflect.DeepEqual(previous, r.group.ModelPricing) {
		return false, nil
	}
	r.writes++
	r.group.ModelPricing = next
	return true, nil
}

func TestVideoSelectedNineImportUsesDocumentedContractsAndFinalPrices(t *testing.T) {
	var rows []map[string]any
	var models []string
	for _, tc := range selectedVideoImportCases {
		rows = append(rows, map[string]any{"model_name": tc.model, "billing_mode": tc.upstreamMode, "model_price": tc.cost})
		models = append(models, tc.model)
	}
	costs := officialSDParsePriceRows(t, rows)
	for _, tc := range selectedVideoImportCases {
		cost := costs[tc.model]
		require.Empty(t, cost.UnavailableReason, tc.model)
		require.NotNil(t, cost.Spec, tc.model)
		require.True(t, cost.Spec.Supported, tc.model)
		require.NoError(t, cost.Spec.Validate(), tc.model)
		require.False(t, cost.ReferencePriceKnown, "no fabricated independent reference price")
	}
	// A larger supplier catalog must not automatically enable extra models.
	extra := costs[models[0]]
	extra.Model = "unselected-catalog-model"
	costs[extra.Model] = extra
	catalogJSON, err := json.Marshal(VideoSupplierCatalog{Models: costs})
	require.NoError(t, err)
	cfg := DefaultVideoRuntimeSettings()
	cfg.AutoSync = true
	cfg.Markup = VideoMarkup{OutputPerSecond: .1, ReferencePerSecond: .1, PerRequest: .5}
	runtimeJSON, err := json.Marshal(cfg)
	require.NoError(t, err)
	settings := &upstreamBillingProbeSettingRepo{values: map[string]string{
		videoCatalogSettingKey: string(catalogJSON), videoRuntimeSettingKey: string(runtimeJSON),
	}}
	group := &Group{ID: 25, Platform: PlatformVideo, RateMultiplier: 9, VideoRateMultiplier: 11}
	repo := &selectedVideoImportRepo{group: group}
	svc := &OpenAIGatewayService{
		settingService: NewSettingService(settings, nil), usageLogRepo: repo,
		channelService: &ChannelService{groupRepo: videoReceiptGroupRepo{group: group}},
	}
	require.NoError(t, svc.ImportVideoPricing(t.Context(), group.ID, models))
	require.Equal(t, 1, repo.writes)
	require.Len(t, group.ModelPricing, 9)
	require.ElementsMatch(t, models, VideoGroupModelIDs(group))
	for _, tc := range selectedVideoImportCases {
		price, err := ResolveVideoGroupPrice(group, tc.model)
		require.NoError(t, err)
		require.Equal(t, tc.selling, price.UnitPrice, tc.model)
		require.Zero(t, price.Video.Spec.MaxVideos, "all nine selected contracts reject video references")
		require.Zero(t, price.Video.Spec.MaxReferenceSeconds)
		require.Zero(t, price.Video.ReferencePerSecond, "video-disabled contracts do not invent a reference price")
		require.True(t, price.Video.AutoPrice)
		require.Equal(t, "upstream", price.Video.PriceSource)
	}
	plaza, err := newPlazaService(nil, []Group{*group}, nil).ListGroups(t.Context())
	require.NoError(t, err)
	require.Len(t, plaza, 1)
	require.Len(t, plaza[0].Models, 9)
	require.ErrorContains(t, svc.ImportVideoPricing(t.Context(), group.ID, models), "已有价格")
	require.Equal(t, 1, repo.writes, "retry must not overwrite selling prices")
}

var selectedVideoStopBeforeBilling = errors.New("fixture stops before billing")

type selectedVideoPrepareRepo struct {
	UsageLogRepository
	PrepaidVideoRepository
	task *VideoTask
}

func (r *selectedVideoPrepareRepo) CreatePrepaidVideoTask(_ context.Context, task *VideoTask, _ int) (*VideoTask, bool, error) {
	r.task = task
	return nil, false, selectedVideoStopBeforeBilling
}

func TestVideoSelectedSDTextImageAudioPrepareWithoutSupplierCalls(t *testing.T) {
	for _, tc := range selectedVideoImportCases[4:] {
		t.Run(tc.model, func(t *testing.T) {
			spec, ok := UnifiedVideoSpecification(tc.model)
			require.True(t, ok)
			price := &VideoGroupPrice{Model: tc.model, BillingMode: BillingModeVideo, UnitPrice: tc.selling, Video: &VideoModelConfig{Spec: spec}}
			if tc.upstreamMode == "per_request" {
				price.BillingMode = BillingModePerRequest
			}
			account := &Account{Platform: PlatformVideo, Type: AccountTypeAPIKey, Credentials: map[string]any{"grok_media_provider": SuanliSpecialVideoProvider}}
			key := &APIKey{Group: &Group{ID: 25}}
			for _, seconds := range []int{spec.MinSeconds, spec.MaxSeconds} {
				for _, material := range []string{"text", "images", "images-audio"} {
					input := UnifiedVideoInput{Model: tc.model, Prompt: "A landscape", Seconds: seconds}
					if material != "text" {
						input.Images = officialSDURLs("image", spec.MaxImages)
					}
					if material == "images-audio" {
						input.Audios = officialSDURLs("audio", spec.MaxAudios)
					}
					raw, err := json.Marshal(input)
					require.NoError(t, err)
					repo := &selectedVideoPrepareRepo{}
					svc := &OpenAIGatewayService{usageLogRepo: repo}
					_, err = svc.beginUnifiedVideoTask(t.Context(), key, account, nil, tc.model, raw, "application/json", price)
					require.ErrorIs(t, err, selectedVideoStopBeforeBilling)
					require.NotNil(t, repo.task)
					var prepared UnifiedVideoInput
					require.NoError(t, json.Unmarshal(repo.task.PreparedBody, &prepared))
					input.Resolution, input.AspectRatio = spec.Resolutions[0], "16:9"
					require.Equal(t, input, prepared, "canonical supplier payload keeps text/image/audio fields")
					want := tc.selling
					if price.BillingMode == BillingModeVideo {
						want *= float64(seconds)
					}
					require.InDelta(t, want, repo.task.Pending.PrepaidAmount, 1e-8)
				}
			}
			for _, invalid := range []UnifiedVideoInput{
				{Model: tc.model, Prompt: "demo", Seconds: spec.MinSeconds, Audios: []string{"https://example.com/audio.mp3"}},
				{Model: tc.model, Prompt: "demo", Seconds: spec.MaxSeconds + 1},
			} {
				raw, err := json.Marshal(invalid)
				require.NoError(t, err)
				_, err = PrepareUnifiedVideoInput(raw, "application/json", spec)
				require.Error(t, err)
			}
		})
	}
}

func TestVideoSelectedFiveSDRejectsReferencesBeforeTaskOrBilling(t *testing.T) {
	for _, tc := range selectedVideoImportCases[4:] {
		t.Run(tc.model, func(t *testing.T) {
			spec, _ := UnifiedVideoSpecification(tc.model)
			require.Zero(t, spec.MaxVideos)
			require.Zero(t, spec.MaxReferenceSeconds)
			require.False(t, spec.ReferenceBilled)
			price := &VideoGroupPrice{Model: tc.model, BillingMode: BillingModeVideo, UnitPrice: tc.selling, Video: &VideoModelConfig{Spec: spec}}
			if tc.upstreamMode == "per_request" {
				price.BillingMode = BillingModePerRequest
			}
			account := &Account{Platform: PlatformVideo, Type: AccountTypeAPIKey, Credentials: map[string]any{"grok_media_provider": SuanliSpecialVideoProvider}}
			raw, err := json.Marshal(UnifiedVideoInput{Model: tc.model, Prompt: "demo", Seconds: 5, Videos: []string{"https://example.com/ref.mp4"}})
			require.NoError(t, err)
			var form bytes.Buffer
			writer := multipart.NewWriter(&form)
			for key, value := range map[string]string{"model": tc.model, "prompt": "demo", "seconds": "5", "videos[]": "https://example.com/ref.mp4"} {
				require.NoError(t, writer.WriteField(key, value))
			}
			require.NoError(t, writer.Close())
			for contentType, body := range map[string][]byte{"application/json": raw, writer.FormDataContentType(): form.Bytes()} {
				svc := &OpenAIGatewayService{}
				task, err := svc.beginUnifiedVideoTask(t.Context(), nil, account, nil, tc.model, body, contentType, price)
				require.EqualError(t, err, "videos素材数量超出限制")
				require.Nil(t, task)
			}
		})
	}
}
