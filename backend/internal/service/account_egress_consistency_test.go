package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAccountEgressIQUsesGlobalTimezoneAndPinnedProxy(t *testing.T) {
	proxy := &Proxy{ID: 71, Protocol: "http", Host: "proxy.fixture", Port: 8080, Status: StatusActive}
	account := &Account{ID: 19, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxy.ID, Proxy: proxy, Concurrency: 1}
	gateway := timezoneAutoService(true, true, &egressTimezoneProber{probe: func(ctx context.Context, exit string) (*ProxyExitInfo, int64, error) {
		require.Equal(t, proxy.URL(), exit)
		return &ProxyExitInfo{IP: "192.0.2.50", Timezone: "Asia/Tokyo"}, 0, nil
	}})
	calls := 0
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, exit string, _ int64, _ int) (*http.Response, error) {
		calls++
		require.Equal(t, proxy.URL(), exit)
		require.Equal(t, "Asia/Tokyo", req.Header.Get("X-Timezone"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}}
	svc := &AccountTestService{openaiGatewayService: gateway, httpUpstream: upstream}
	request, _ := http.NewRequestWithContext(context.Background(), "POST", "https://api.openai.com/v1/responses", strings.NewReader(`{"model":"fixture"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Timezone", "America/New_York")
	response, err := svc.doOpenAIAccountTestUpstream(request, proxy.URL(), account, false, nil)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 1, calls)
	_, err = svc.doOpenAIAccountTestUpstream(request, "http://different.fixture:8080", account, false, nil)
	require.ErrorContains(t, err, "不一致")
	require.Equal(t, 1, calls)
	_, err = svc.doOpenAIAccountTestUpstream(request, "", account, false, nil)
	require.Error(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "America/New_York", request.Header.Get("X-Timezone"), "normalization cannot mutate the source request")
}

func TestAccountEgressProbeNeverTreatsCacheAsLive(t *testing.T) {
	_, calls := newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"fixture-model"}]}`)
	gateway := &OpenAIGatewayService{}
	account := newCodexModelsTestAccount()
	_, err := gateway.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)
	_, err = gateway.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	for i := 0; i < 2; i++ {
		_, err = gateway.ProbeOpenAIModels(context.Background(), account)
		require.NoError(t, err)
	}
	require.EqualValues(t, 3, calls.Load(), "each liveness check must send a new authenticated upstream request")
}

func TestAccountEgressProbe403IsNotCredentialRevocation(t *testing.T) {
	for _, tc := range []struct {
		code  int
		state string
	}{{401, AccountTokenGuardProbeAuth}, {403, AccountTokenGuardProbeTransient}, {429, AccountTokenGuardProbeTransient}, {503, AccountTokenGuardProbeTransient}} {
		state, _ := classifyAccountProbeError(&codexModelsManifestUpstreamError{statusCode: tc.code})
		require.Equal(t, tc.state, state)
	}
}
