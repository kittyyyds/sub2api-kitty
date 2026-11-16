package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type probeDiagnosticsHTTP struct {
	HTTPUpstream
	respond func(*http.Request) *http.Response
}

func (h *probeDiagnosticsHTTP) DoWithTLS(r *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return h.respond(r), nil
}

func TestProbeDiagnosticsDistinguishCFChallengeFromApplicationErrors(t *testing.T) {
	for _, tt := range []struct {
		status       int
		header       http.Header
		body, reason string
	}{
		{403, http.Header{"Server": {"cloudflare"}, "Cf-Ray": {"ray"}}, `<html>cf-chl-test</html>`, "cloudflare_blocked"},
		{503, http.Header{"Cf-Mitigated": {"challenge"}}, "", "cloudflare_blocked"},
		{403, http.Header{"Server": {"cloudflare"}}, `{"error":"permission denied"}`, "forbidden"},
		{401, http.Header{}, `{"error":"invalid token"}`, "unauthorized"},
		{429, http.Header{}, "", "rate_limited"},
		{502, http.Header{}, "", "upstream_unavailable"},
	} {
		t.Run(tt.reason, func(t *testing.T) {
			snap, _ := upstreamBillingRateSnapshotFromResult(&Account{}, 30, time.Now(), upstreamBillingProbeHTTPResult{statusCode: tt.status, header: tt.header, body: []byte(tt.body)}, 1)
			require.Equal(t, tt.reason, snap.LastError)
			require.Equal(t, tt.status, snap.HTTPStatus)
			require.Equal(t, "billing", snap.ProbeStage)
			require.Equal(t, "sub2api_billing", snap.RateSource)
		})
	}
}

func TestProbeDiagnosticsNewAPIKeepsVersionAndFailureStage(t *testing.T) {
	svc, repo, _, accounts := newAPIConfigFixture()
	_, err := svc.SaveNewAPIConfig(context.Background(), 1, NewAPIConfigRequest{UserID: 7, AccessToken: "private-management-secret", AccountIDs: []int64{1}})
	require.NoError(t, err)
	svc.accountTestService.httpUpstream = &probeDiagnosticsHTTP{respond: func(r *http.Request) *http.Response {
		if r.URL.Path == "/api/status" {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-New-Api-Version": {"v0.10.8"}}, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"quota_per_unit":1}}`))}
		}
		return &http.Response{StatusCode: 403, Header: http.Header{"Server": {"cloudflare"}, "Cf-Ray": {"test"}}, Body: io.NopCloser(strings.NewReader(`<html>cf-chl-private-response-never-store</html>`))}
	}}
	snap, err := svc.probeNewAPIAccount(context.Background(), accounts.accounts[1], repo.bindings[1], 30)
	require.NoError(t, err)
	require.Equal(t, "newapi", snap.Protocol)
	require.Equal(t, "v0.10.8", snap.UpstreamVersion)
	require.Equal(t, 403, snap.HTTPStatus)
	require.Equal(t, "user_wallet", snap.ProbeStage)
	require.Equal(t, "cloudflare_blocked", snap.LastError)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-management-secret")
	require.NotContains(t, string(raw), "private-response-never-store")
}

func TestProbeDiagnosticsVersionHeaderDoesNotTriggerFallback(t *testing.T) {
	snap, rate := upstreamBillingRateSnapshotFromResult(&Account{}, 30, time.Now(), upstreamBillingProbeHTTPResult{statusCode: 404, header: http.Header{"X-New-Api-Version": {"v0.10.8"}}, body: []byte(`{}`)}, 1)
	require.Equal(t, UpstreamBillingProbeStatusUnsupported, snap.Status)
	require.Equal(t, "newapi", snap.Protocol)
	require.Equal(t, "v0.10.8", snap.UpstreamVersion)
	require.Nil(t, rate)
	require.Equal(t, "vtest", sanitizeUpstreamProbeVersion("\r\nv\x00test\n"))
	require.Len(t, sanitizeUpstreamProbeVersion(strings.Repeat("a", 1000)), 96)
}
