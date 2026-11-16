package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const videoProbeFixtureModel = "全能sd2.0(满血原生过脸-933-480p)"
const videoProbeFixtureBody = `{"success":true,"pricing_version":"v-test","group_ratio":{"default":1},"data":[{"model_name":"全能sd2.0(满血原生过脸-933-480p)","billing_mode":"video_multi","model_price":0.2,"video_pricing":{"output_per_second":0.2,"ref_video_per_second":0.1,"resolution_per_second":{"480p":0.2,"720p":0.3}}}]}`

func videoProbeFixtureAccount() *Account {
	rate := 0.7
	return &Account{
		ID: 81, Platform: PlatformVideo, Type: AccountTypeAPIKey, Status: StatusActive,
		RateMultiplier: &rate, Concurrency: 1,
		Credentials: map[string]any{
			"base_url": "https://suanliai.top/v1", "api_key": "fixture-account-key",
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides:       map[string]any{"Authorization": "fixture-override", "X-Api-Key": "fixture-header-key", "Cookie": "fixture-cookie"},
		},
		Extra: map[string]any{
			UpstreamBillingProbeEnabledExtraKey:    true,
			UpstreamBillingRateSyncEnabledExtraKey: true, // Stale legacy flag must have no effect.
			AccountCostMultiplierExtraKey:          0.4,
		},
	}
}

func videoProbeFixtureService(account *Account, upstream HTTPUpstream) (*UpstreamBillingProbeService, *upstreamBillingProbeAccountRepo) {
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	svc.now = func() time.Time { return time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC) }
	return svc, repo
}

func videoProbeHTTPResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestUpstreamBillingProbeVideoPublicPricesSnapshotContract(t *testing.T) {
	account := videoProbeFixtureAccount()
	proxyID := int64(7)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "proxy.example", Port: 3128, Status: StatusActive}
	upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
	svc, _ := videoProbeFixtureService(account, upstream)

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeKindVideoModelPrices, snapshot.Kind)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.NotNil(t, snapshot.VideoPricing)
	require.Equal(t, "v-test", snapshot.VideoPricing.Version)
	price := snapshot.VideoPricing.Models[videoProbeFixtureModel]
	require.Equal(t, BillingModeVideo, price.BillingMode)
	require.Equal(t, 0.2, price.UnitPrice)
	require.Equal(t, 0.1, price.ReferencePerSecond)
	require.Equal(t, 0.3, price.ResolutionPrices["720p"])
	require.Empty(t, snapshot.VideoPricing.Changes)
	require.Equal(t, svc.currentTime().Format(time.RFC3339), snapshot.VideoPricing.LastSuccess)
	require.Equal(t, svc.currentTime().Add(time.Hour), *snapshot.FreshUntil)
	require.Nil(t, snapshot.Data)
	require.Nil(t, snapshot.Balance)
	require.Nil(t, snapshot.SyncedRateMultiplier)
	require.Equal(t, 0.7, *account.RateMultiplier)
	require.Equal(t, 0.4, account.Extra[AccountCostMultiplierExtraKey])
	require.False(t, upstreamBillingRateSyncEnabled(account))
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://suanliai.top/api/pricing", req.URL.String())
	require.Equal(t, http.MethodGet, req.Method)
	require.Equal(t, http.Header{"Accept": []string{"application/json"}}, req.Header)
	require.Equal(t, account.Proxy.URL(), upstream.lastProxyURL)
	require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
	require.Equal(t, HTTPUpstreamProfileDefault, HTTPUpstreamProfileFromContext(req.Context()))
	_, deadlineSet := req.Context().Deadline()
	require.True(t, deadlineSet)

	// Exercise the actual JSON shape consumed by manual and background refresh.
	raw, err := json.Marshal(BuildUpstreamBillingRateSnapshotItems([]Account{*account}))
	require.NoError(t, err)
	var items []map[string]any
	require.NoError(t, json.Unmarshal(raw, &items))
	wire := items[0]["snapshot"].(map[string]any)
	require.Equal(t, "video_model_prices", wire["kind"])
	require.Contains(t, wire["video_pricing"].(map[string]any)["models"], videoProbeFixtureModel)
	for _, key := range []string{"data", "balance", "synced_rate_multiplier"} {
		require.NotContains(t, wire, key)
	}
	require.NotContains(t, string(raw), "fixture-account-key")
	require.NotContains(t, string(raw), "fixture-override")
	require.NotContains(t, string(raw), "fixture-header-key")
}

func TestUpstreamBillingProbeVideoAcceptsOnlyConfirmedOrigin(t *testing.T) {
	for _, base := range []string{
		"", "https://other.example/v1", "https://suanliai.top.other.example", "https://api.suanliai.top",
		"https://suanliai.top:8443", "http://suanliai.top", "https://fixture-user@suanliai.top",
		"https://127.0.0.1", "https://suanliai.top%2f.other.example", "https://suanliai.top:bad",
	} {
		t.Run(base, func(t *testing.T) {
			account := videoProbeFixtureAccount()
			account.Credentials["base_url"] = base
			upstream := &httpUpstreamRecorder{}
			svc, _ := videoProbeFixtureService(account, upstream)
			snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
			require.NoError(t, err)
			require.Equal(t, UpstreamBillingProbeStatusUnsupported, snapshot.Status)
			require.Equal(t, UpstreamBillingProbeKindVideoModelPrices, snapshot.Kind)
			require.Empty(t, upstream.requests)
		})
	}
	for _, base := range []string{"https://SUANLIAI.TOP:443/v1", " https://suanliai.top/custom/path?ignored=value "} {
		t.Run(base, func(t *testing.T) {
			account := videoProbeFixtureAccount()
			account.Credentials["base_url"] = base
			delete(account.Credentials, "api_key") // Public prices do not require a key.
			upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
			svc, _ := videoProbeFixtureService(account, upstream)
			snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
			require.NoError(t, err)
			require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
			require.Equal(t, videoModelPriceProbeURL, upstream.lastReq.URL.String())
		})
	}
}

func TestUpstreamBillingProbeVideoPreservesSecurityAndProxyChecks(t *testing.T) {
	for _, scenario := range []string{"allowlist", "missing proxy", "changed proxy"} {
		t.Run(scenario, func(t *testing.T) {
			account := videoProbeFixtureAccount()
			upstream := &httpUpstreamRecorder{}
			svc, _ := videoProbeFixtureService(account, upstream)
			want := "invalid_base_url"
			if scenario == "allowlist" {
				svc.accountTestService.cfg.Security.URLAllowlist = config.URLAllowlistConfig{Enabled: true, UpstreamHosts: []string{"other.example"}}
			} else {
				proxyID := int64(7)
				account.ProxyID = &proxyID
				want = "proxy_unavailable"
				if scenario == "changed proxy" {
					account.Proxy = &Proxy{ID: 8}
				}
			}
			snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
			if scenario == "changed proxy" {
				require.ErrorIs(t, err, ErrUpstreamBillingProbeIdentityChanged)
			} else {
				require.NoError(t, err)
				require.Equal(t, UpstreamBillingProbeStatusFailed, snapshot.Status)
				require.Equal(t, want, snapshot.LastError)
			}
			require.Empty(t, upstream.requests)
		})
	}
}

func TestUpstreamBillingProbeVideoFailurePreservesLastValidPrices(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
		want   string
	}{
		{name: "unsupported", status: 404, want: "unsupported"},
		{name: "redirect", status: 302, want: "http_error"},
		{name: "server failure", status: 503, body: "sensitive-response", want: "http_error"},
		{name: "timeout", err: context.DeadlineExceeded, want: "request_failed"},
		{name: "malformed catalog", status: 200, body: `{"secret":"sensitive-response"}`, want: "invalid_response"},
		{name: "unknown supplier ratio", status: 200, body: strings.Replace(videoProbeFixtureBody, `"default":1`, `"default":2`, 1), want: "invalid_response"},
		{name: "too large", status: 200, body: strings.Repeat("x", videoModelPriceProbeMaxBodyBytes+1), want: "response_too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := videoProbeFixtureAccount()
			upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
			svc, _ := videoProbeFixtureService(account, upstream)
			first, err := svc.ProbeAccount(context.Background(), account.ID)
			require.NoError(t, err)
			later := svc.currentTime().Add(2 * time.Hour)
			svc.now = func() time.Time { return later }
			upstream.resp = videoProbeHTTPResponse(tc.status, tc.body)
			upstream.err = tc.err
			snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, snapshot.LastError)
			require.Equal(t, UpstreamBillingProbeKindVideoModelPrices, snapshot.Kind)
			if tc.want == "unsupported" {
				require.Equal(t, UpstreamBillingProbeStatusUnsupported, snapshot.Status)
			} else {
				require.Equal(t, UpstreamBillingProbeStatusFailed, snapshot.Status)
			}
			beforePrices, err := json.Marshal(first.VideoPricing.Models)
			require.NoError(t, err)
			afterPrices, err := json.Marshal(snapshot.VideoPricing.Models)
			require.NoError(t, err)
			require.JSONEq(t, string(beforePrices), string(afterPrices))
			require.Equal(t, first.VideoPricing.LastSuccess, snapshot.VideoPricing.LastSuccess)
			require.Equal(t, later.Format(time.RFC3339), snapshot.VideoPricing.LastAttempt)
			require.Equal(t, tc.want, snapshot.VideoPricing.LastError)
			require.Equal(t, first.ReceivedAt, snapshot.ReceivedAt)
			require.Equal(t, first.FreshUntil, snapshot.FreshUntil)
			require.True(t, snapshot.NextProbeAt.After(later))
			require.Equal(t, 1, snapshot.FailureCount)
			require.Empty(t, first.VideoPricing.LastError, "failure must not mutate the previous CAS snapshot")
			require.Equal(t, 0.7, *account.RateMultiplier)
			require.Equal(t, 0.4, account.Extra[AccountCostMultiplierExtraKey])
			raw, err := json.Marshal(snapshot)
			require.NoError(t, err)
			require.NotContains(t, string(raw), "sensitive-response")
		})
	}
}

func TestUpstreamBillingProbeVideoScheduledAndManualModes(t *testing.T) {
	account := videoProbeFixtureAccount()
	upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
	svc, _ := videoProbeFixtureService(account, upstream)
	require.NoError(t, svc.RunDue(context.Background()))
	require.Len(t, upstream.requests, 1)
	require.Equal(t, UpstreamBillingProbeKindVideoModelPrices, decodeUpstreamBillingProbeSnapshot(account.Extra).Kind)
	// An account toggle can clear legacy sync; a subsequent periodic invocation
	// must re-read the flag even if a previous due-list contained this account.
	require.NoError(t, svc.SetAccountEnabled(context.Background(), account.ID, false))
	snapshot, err := svc.probeScheduledAccount(context.Background(), account.ID, 30)
	require.NoError(t, err)
	require.Nil(t, snapshot)
	require.Len(t, upstream.requests, 1)
	upstream.resp = videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)
	_, err = svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2, "manual probing remains available with automatic probing disabled")
}

func TestUpstreamBillingProbeVideoDoesNotChangeGrokSupplierProbes(t *testing.T) {
	account := videoProbeFixtureAccount()
	account.Platform = PlatformGrok
	account.Credentials["grok_media_provider"] = SuanliSpecialVideoProvider
	upstream := &httpUpstreamRecorder{err: errors.New("fixture transport failure")}
	svc, _ := videoProbeFixtureService(account, upstream)
	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotEmpty(t, upstream.requests)
	require.Equal(t, "/v1/sub2api/billing", upstream.requests[0].URL.Path)
	require.Empty(t, snapshot.Kind)
	require.Nil(t, snapshot.VideoPricing)
	for _, platform := range []string{PlatformOpenAI, PlatformGrok, PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformTypeSafe} {
		require.True(t, IsUpstreamBillingProbeIdentity(platform, AccountTypeAPIKey))
		require.True(t, IsAccountUpstreamProbeIdentity(platform, AccountTypeAPIKey))
	}
	require.False(t, IsUpstreamBillingProbeIdentity(PlatformVideo, AccountTypeAPIKey))
	require.True(t, IsAccountUpstreamProbeIdentity(PlatformVideo, AccountTypeAPIKey))
	require.False(t, IsAccountUpstreamProbeIdentity(PlatformVideo, AccountTypeOAuth))
	require.False(t, IsAccountUpstreamProbeIdentity("future-platform", AccountTypeAPIKey))
}
