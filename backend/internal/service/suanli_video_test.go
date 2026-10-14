//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func suanliTestAccount() *Account {
	a := grokMediaContentTestAccount()
	a.Credentials["base_url"] = "https://suanliai.top/v1"
	a.Credentials["grok_media_provider"] = SuanliSpecialVideoProvider
	return a
}

func TestSuanliProviderOptInAndURLs(t *testing.T) {
	a := suanliTestAccount()
	require.True(t, a.IsSuanliSpecialVideo())
	u, err := buildGrokMediaURL(a, &config.Config{}, GrokMediaEndpointVideosGenerations, "")
	require.NoError(t, err)
	require.Equal(t, "https://suanliai.top/v1/videos", u)
	u, err = buildGrokMediaURL(a, &config.Config{}, GrokMediaEndpointVideoContent, "task/1")
	require.NoError(t, err)
	require.Equal(t, "https://suanliai.top/v1/videos/task%2F1/content", u)
	_, err = buildGrokMediaURL(a, &config.Config{}, GrokMediaEndpointImagesGenerations, "")
	require.Error(t, err)
	a.Type = AccountTypeOAuth
	require.False(t, a.IsSuanliSpecialVideo())
	a = grokMediaContentTestAccount()
	u, err = buildGrokMediaURL(a, &config.Config{}, GrokMediaEndpointVideosGenerations, "")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example/v1/videos/generations", u)
}

func TestSuanliFixedThirtySecondsAndExactPayload(t *testing.T) {
	input := []byte(`{"model":"特价/sd-2.5","prompt":"slow camera movement","duration":30,"resolution":"720p","aspect_ratio":"16:9","images":["https://example.com/image.png"]}`)
	body, _, err := normalizeSuanliVideoRequest(GrokMediaEndpointVideosGenerations, input, "application/json")
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "duration").Exists())
	require.False(t, gjson.GetBytes(body, "seconds").Exists())
	info := ParseGrokMediaRequest("application/json", body)
	suanliRequestBillingInfo(body, &info)
	require.Equal(t, 30, info.DurationSeconds)
	require.Equal(t, "720p", info.Resolution)
	_, _, err = normalizeSuanliVideoRequest(GrokMediaEndpointVideosGenerations, []byte(strings.Replace(string(input), `"duration":30`, `"duration":15`, 1)), "application/json")
	require.ErrorContains(t, err, "30 seconds")
}

func TestSuanliSeedanceAndH3Contracts(t *testing.T) {
	for _, tc := range []struct{ model, size, resolution string }{
		{"官转稳定sd2.0(933原生过真人480P)", "864x480", "480p"},
		{"官转稳定sd2.0(933原生过真人720P)", "720x1280", "720p"},
		{"官转稳定sd2.5(满血30-10-10不卡脸1080P)", "1920x1080", "1080p"},
		{"sd2.0(满血9-3-3不卡脸720P)", "1280x720", "720p"},
		{"sd2.5(满血30-10-10不卡脸720P)", "1280x720", "720p"},
		{"特价/官方h3-2k", "2560x1440", "2k"},
		{"特价/官方h3-720p", "1280x720", "720p"},
		{"特价/官方h3-1080p", "1920x1080", "1080p"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			input := []byte(`{"model":"` + tc.model + `","prompt":"gentle motion","seconds":8,"size":"` + tc.size + `","reference_images":[{"url":"https://example.com/a.png","role":"reference_image"}]}`)
			body, _, err := normalizeSuanliVideoRequest(GrokMediaEndpointVideosGenerations, input, "application/json")
			require.NoError(t, err)
			require.Equal(t, "8", gjson.GetBytes(body, "seconds").String())
			require.Equal(t, "reference_image", gjson.GetBytes(body, "reference_images.0.role").String())
			info := GrokMediaRequestInfo{}
			suanliRequestBillingInfo(body, &info)
			require.Equal(t, tc.resolution, info.Resolution)
			require.Equal(t, 8, info.DurationSeconds)
		})
	}
	_, _, err := normalizeSuanliVideoRequest(GrokMediaEndpointVideosGenerations, []byte(`{"model":"特价/官方h3-720p","prompt":"a","seconds":8,"size":"1280x720","reference_images":[{"url":"https://example.com/a.png"}],"reference_videos":[]}`), "application/json")
	require.ErrorContains(t, err, "unsupported parameter")
}

func TestSuanliStatusBillingAndOutputValidation(t *testing.T) {
	completed := []byte(`{"id":"task-1","request_id":"internal-ignore","status":"completed","metadata":{"url":"https://cdn.example.com/video.mp4"},"seconds":"30"}`)
	body, err := normalizeSuanliVideoResponse(GrokMediaEndpointVideoStatus, completed, "task-1")
	require.NoError(t, err)
	require.Equal(t, "task-1", gjson.GetBytes(body, "request_id").String())
	stable, err := normalizeSuanliVideoResponse(GrokMediaEndpointVideoStatus, completed, "gateway-owned-task")
	require.NoError(t, err)
	require.Equal(t, "gateway-owned-task", gjson.GetBytes(stable, "request_id").String())
	require.True(t, IsGrokVideoStatusBillable(body))
	created, err := normalizeSuanliVideoResponse(GrokMediaEndpointVideosGenerations, completed, "")
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(created, "video").Exists(), "creation must not expose output before binding and billing observation")
	require.Equal(t, "pending", gjson.GetBytes(created, "status").String())
	billed := ExtractGrokVideoBillingFromStatusBody(body, nil, "task-1", SuanliSpecialVideoProvider)
	require.Equal(t, 30, billed.VideoDurationSeconds)
	require.Empty(t, billed.Model, "pending snapshot supplies the model rather than defaulting to Grok")
	for _, state := range []string{"queued", "not_start", "in_progress", "failed", "expired"} {
		out, err := normalizeSuanliVideoResponse(GrokMediaEndpointVideoStatus, []byte(`{"id":"task-1","status":"`+state+`","progress":100,"metadata":{"url":"https://cdn.example.com/v.mp4"}}`), "task-1")
		require.NoError(t, err)
		require.False(t, IsGrokVideoStatusBillable(out), state)
	}
	for _, bad := range []string{"http://cdn.example.com/a.mp4", "https://127.0.0.1/a", "https://user:secret@example.com/a"} {
		_, err := normalizeSuanliVideoResponse(GrokMediaEndpointVideoStatus, []byte(`{"id":"task-1","status":"completed","metadata":{"url":"`+bad+`"}}`), "task-1")
		require.Error(t, err)
	}
	_, err = normalizeSuanliVideoResponse(GrokMediaEndpointVideosGenerations, []byte(`{"request_id":"internal-only"}`), "")
	require.ErrorContains(t, err, "no task id")
}

func TestSuanliProviderAwarePricingPreservesLegacyGrok(t *testing.T) {
	svc := &BillingService{}
	prices := &VideoPriceConfig{ModelPrices: map[string]map[string]float64{"特价/sd-2.5": {"720p": 0.01}, "特价/官方h3-2k": {"2k": 0.05}}}
	require.InDelta(t, .30, svc.CalculateVideoCost("特价/sd-2.5", "720p", 1, 30, prices, 1, SuanliSpecialVideoProvider).ActualCost, 1e-9)
	require.InDelta(t, .15, svc.CalculateVideoCost("特价/sd-2.5", "720p", 1, 30, prices, 1).ActualCost, 1e-9)
	require.InDelta(t, .5, svc.CalculateVideoCost("特价/官方h3-2k", "2k", 1, 10, prices, 1, SuanliSpecialVideoProvider).ActualCost, 1e-9)
	require.Equal(t, 15, NormalizeVideoBillingDurationSecondsOrDefault(30))
	require.Equal(t, "2k", NormalizeVideoBillingResolutionOrDefault("2K"))
}

func TestSuanliPerRequestPricingChargesOnceForThirtySeconds(t *testing.T) {
	model := "特价/sd-2.5"
	price := 0.20
	group := &Group{ID: 31, Platform: PlatformGrok, ModelPricing: []ChannelModelPricing{{
		Platform: PlatformGrok, Models: []string{model}, BillingMode: BillingModePerRequest, PerRequestPrice: &price,
	}}}
	group.VideoModelPrices = map[string]map[string]float64{model: {"720p": 99}}
	key := &APIKey{Group: group, GroupID: &group.ID}
	billing := &BillingService{}
	svc := &OpenAIGatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	require.NoError(t, svc.ValidateSuanliVideoPricing(context.Background(), key, model))
	result := &OpenAIForwardResult{Model: model, BillingModel: model, VideoProvider: SuanliSpecialVideoProvider, VideoCount: 1, VideoDurationSeconds: 30, VideoResolution: "720p"}
	cost := svc.calculateOpenAIVideoCost(context.Background(), model, model, key, result, 1)
	require.InDelta(t, price, cost.ActualCost, 1e-9, "per-request pricing must not multiply by 30 seconds")
	result.VideoDurationSeconds = 15
	cost = svc.calculateOpenAIVideoCost(context.Background(), model, model, key, result, 1)
	require.InDelta(t, price, cost.ActualCost, 1e-9)
	group.ModelPricing = nil
	group.VideoModelPrices = nil
	svc.resolver = nil
	require.Error(t, svc.ValidateSuanliVideoPricing(context.Background(), key, model))
}

func TestSuanliForwardCreateUsesRealContract(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task-30","status":"queued"}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, suanliTestAccount(), GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"特价/sd-2.5","prompt":"motion","images":["https://example.com/ref.png"],"resolution":"720p"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "https://suanliai.top/v1/videos", upstream.request.URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.request.Header.Get("Authorization"))
	body, err := io.ReadAll(upstream.request.Body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "seconds").Exists())
	require.Equal(t, "task-30", result.ResponseID)
	require.Equal(t, 30, result.VideoDurationSeconds)
	require.Zero(t, result.VideoCount, "creation never charges")
	require.Equal(t, SuanliSpecialVideoProvider, result.VideoProvider)
}

type suanliFailingTransport struct{}

func (suanliFailingTransport) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, errors.New("timeout after submission")
}
func (s suanliFailingTransport) DoWithTLS(r *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(r, p, id, n)
}

func TestSuanliUnknownSubmissionDoesNotFailOver(t *testing.T) {
	input := []byte(`{"model":"特价/sd-2.5","prompt":"motion","images":["https://example.com/ref.png"]}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: suanliFailingTransport{}}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
	_, err := svc.ForwardGrokMedia(context.Background(), c, suanliTestAccount(), GrokMediaEndpointVideosGenerations, "", input, "application/json")
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), "submission_unknown")
	svc.httpUpstream = &grokMediaContentUpstreamStub{response: &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"busy"}`))}}
	c, _ = grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
	_, err = svc.ForwardGrokMedia(context.Background(), c, suanliTestAccount(), GrokMediaEndpointVideosGenerations, "", input, "application/json")
	require.Error(t, err)
	require.False(t, errors.As(err, &failover))
}

func TestSuanliContentUsesSupplierProxyWithoutCDNCredentials(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"task-1","status":"completed","seconds":"30","metadata":{"url":"https://cdn.example.com/video.mp4"}}`),
		{StatusCode: 200, ContentLength: 5, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader("video"))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, rec := grokMediaContentTestContext(http.MethodGet, "https://site.example/v1/videos/task-1/content", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, suanliTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.NoError(t, err)
	require.Equal(t, "video", rec.Body.String())
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://suanliai.top/v1/videos/task-1/content", upstream.requests[1].URL.String())
	require.Equal(t, 30, result.VideoDurationSeconds)
	require.Equal(t, 1, result.VideoCount)
}
