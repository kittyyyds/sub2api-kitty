package kittyrisk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type gateSettings struct {
	SettingRepository
	mu    sync.Mutex
	raw   string
	reads atomic.Int64
}

func (r *gateSettings) GetValue(context.Context, string) (string, error) {
	r.reads.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.raw == "" {
		return "", ErrSettingNotFound
	}
	return r.raw, nil
}
func (r *gateSettings) Set(_ context.Context, _ string, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.raw = value
	return nil
}
func (r *gateSettings) GetMultiple(ctx context.Context, _ []string) (map[string]string, error) {
	raw, err := r.GetValue(ctx, SettingKeyContentModerationConfig)
	if err == ErrSettingNotFound {
		return map[string]string{}, nil
	}
	return map[string]string{SettingKeyContentModerationConfig: raw}, err
}
func gateService(t *testing.T) (*ContentModerationService, *gateSettings) {
	t.Helper()
	settings := &gateSettings{}
	svc := NewContentModerationService(settings, nil, nil, nil, nil, nil, nil, nil)
	svc.releaseID = "test-release-1"
	return svc, settings
}
func gateConfig(t *testing.T, r *gateSettings, cfg *ContentModerationConfig) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, r.Set(context.Background(), SettingKeyContentModerationConfig, string(raw)))
}

type countingTransport struct{ calls atomic.Int64 }

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
}

func TestKittyReleaseGatesMissingConfigNeverRuns(t *testing.T) {
	svc, _ := gateService(t)
	ctx := context.Background()
	require.False(t, svc.ReviewEnabled(ctx))
	require.False(t, svc.EvidenceEnabled(ctx))
	view, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.False(t, view.EvidenceEnabled)
	decision, err := svc.Check(ctx, ContentModerationCheckInput{Body: []byte(`{"input":"test"}`)})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, svc.launchContentModerationShadowReview(func() { t.Error("disabled shadow executed") }))
	// A marked upstream response cannot bypass the evidence switch.
	svc.RecordCyberPolicyEvent(ctx, CyberPolicyRecordInput{UserID: 1, APIKeyID: 2, UpstreamMessage: "cyber_policy"})
	require.False(t, svc.persistContentModerationLogWithInput(ctx, nil, &ContentModerationLog{InputExcerpt: "private-body"}, "hash", true, true, nil))
}

func TestKittyReleaseGatesIndependentAndUpgradeReset(t *testing.T) {
	svc, r := gateService(t)
	ctx := context.Background()
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.EvidenceEnabled = true
	cfg.ReviewRelease = "test-release-0"
	cfg.EvidenceRelease = "test-release-0"
	cfg.UserEmailWhitelist = []string{"owner@example.invalid"}
	gateConfig(t, r, cfg)
	require.False(t, svc.ReviewEnabled(ctx))
	require.False(t, svc.EvidenceEnabled(ctx))
	yes := true
	no := false
	view, err := svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{Enabled: &yes})
	require.NoError(t, err)
	require.True(t, view.Enabled)
	require.False(t, view.EvidenceEnabled)
	require.Equal(t, []string{"owner@example.invalid"}, view.UserEmailWhitelist)
	view, err = svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{Enabled: &no, EvidenceEnabled: &yes})
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.True(t, view.EvidenceEnabled)
	svc.releaseID = "test-release-2"
	require.False(t, svc.ReviewEnabled(ctx))
	require.False(t, svc.EvidenceEnabled(ctx))
}

func TestKittyReviewerNetworkBoundaryRechecksSwitch(t *testing.T) {
	svc, r := gateService(t)
	ctx := context.Background()
	base := &countingTransport{}
	client := &http.Client{Transport: gatedReviewTransport{service: svc, base: base}}
	_, err := client.Get("https://reviewer.example.invalid/v1")
	require.Error(t, err)
	require.EqualValues(t, 0, base.calls.Load())
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.ReviewRelease = svc.releaseID
	gateConfig(t, r, cfg)
	svc.gates.Store(nil) // Simulate expiration after an out-of-band configuration edit.
	res, err := client.Get("https://reviewer.example.invalid/v1")
	require.NoError(t, err)
	res.Body.Close()
	require.EqualValues(t, 1, base.calls.Load())
	no := false
	_, err = svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{Enabled: &no})
	require.NoError(t, err)
	_, err = client.Get("https://reviewer.example.invalid/v1")
	require.Error(t, err)
	require.EqualValues(t, 1, base.calls.Load())
}

func TestKittyDisabledGateCachesAndSaveIsImmediate(t *testing.T) {
	svc, settings := gateService(t)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		require.False(t, svc.ReviewEnabled(ctx))
		require.False(t, svc.EvidenceEnabled(ctx))
	}
	require.EqualValues(t, 1, settings.reads.Load())
	yes, no := true, false
	_, err := svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{Enabled: &yes, EvidenceEnabled: &yes})
	require.NoError(t, err)
	require.True(t, svc.ReviewEnabled(ctx))
	require.True(t, svc.EvidenceEnabled(ctx))
	_, err = svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{Enabled: &no, EvidenceEnabled: &no})
	require.NoError(t, err)
	require.False(t, svc.ReviewEnabled(ctx))
	require.False(t, svc.EvidenceEnabled(ctx))
}
