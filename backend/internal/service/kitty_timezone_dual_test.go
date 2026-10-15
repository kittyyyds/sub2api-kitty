package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTimezoneDualExplicitAccountPolicyNeverUsesGlobalEgress(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, search := range []bool{false, true} {
			probe := &egressTimezoneProber{probe: func(context.Context, string) (*ProxyExitInfo, int64, error) {
				t.Fatal("explicit account policy must not trigger global probing")
				return nil, 0, nil
			}}
			svc := timezoneAutoService(true, true, probe)
			account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				openAIRequestTimezoneEnabledExtraKey:    enabled,
				openAIRequestTimezoneSyncSearchExtraKey: search,
				openAIRequestTimezoneExtraKey:           "America/Los_Angeles",
			}}
			ctx := svc.withRequestTimezoneEgress(context.Background(), account)
			original := localeTestBody([]string{"<environment_context><timezone>Asia/Shanghai</timezone><cwd>/keep</cwd></environment_context>"}, []string{"environments.environment_context"})
			body := svc.normalizeRequestTimezone(ctx, account, original, "http")
			req, err := http.NewRequestWithContext(ctx, "POST", "https://example.com", strings.NewReader(string(body)))
			require.NoError(t, err)
			req.Header.Set("X-Timezone", "Asia/Shanghai")
			svc.requestTimezoneHeaders(ctx, account, nil, req.Header)
			bound, err := BindOpenAIRequestTimezone(req, "http://tokyo.example:8080")
			require.NoError(t, err)
			require.Same(t, req, bound)
			got, err := io.ReadAll(bound.Body)
			require.NoError(t, err)
			want := "Asia/Shanghai"
			if enabled {
				want = "America/Los_Angeles"
			}
			require.Contains(t, gjson.GetBytes(got, "input.0.content.0.text").String(), want)
			wantSearch := "America/New_York"
			if enabled && search {
				wantSearch = "America/Los_Angeles"
			}
			require.Equal(t, wantSearch, gjson.GetBytes(got, "tools.0.user_location.timezone").String())
			require.Empty(t, probe.calls)
			require.Equal(t, "Asia/Shanghai", bound.Header.Get("X-Timezone"))
		}
	}
}

func TestTimezoneDualWSCompatibilitySeparatesGlobalMode(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI}
	a := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com", timezoneMode: "auto"}
	b := a
	b.timezoneMode = ""
	require.NotEqual(t, openAIWSAcquireCompatibility(a), openAIWSAcquireCompatibility(b))
	require.False(t, sameOpenAIWSPrewarmTarget(a, b))
	b = a
	b.Headers = http.Header{"X-Timezone": []string{"Asia/Tokyo"}}
	require.NotEqual(t, openAIWSAcquireCompatibility(a), openAIWSAcquireCompatibility(b))
}

func TestTimezoneDualGlobalOffAndOnPreservesSourceRequest(t *testing.T) {
	probe := &egressTimezoneProber{probe: func(_ context.Context, proxy string) (*ProxyExitInfo, int64, error) {
		zone := "Asia/Tokyo"
		if proxy == "" {
			zone = "America/Los_Angeles"
		}
		return timezoneResult(zone), 0, nil
	}}
	svc := timezoneAutoService(true, true, probe)
	account := &Account{Platform: PlatformOpenAI}
	ctx := svc.withRequestTimezoneEgress(context.Background(), account)
	body := localeTestBody([]string{"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}, nil)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://example.com", strings.NewReader(string(body)))
	require.NoError(t, err)
	for _, proxy := range []string{"", "http://tokyo.example:8080"} {
		got, err := BindOpenAIRequestTimezone(req, proxy)
		require.NoError(t, err)
		payload, err := io.ReadAll(got.Body)
		require.NoError(t, err)
		require.Contains(t, string(payload), map[bool]string{true: "America/Los_Angeles", false: "Asia/Tokyo"}[proxy == ""])
	}
	svc.settingService = timezoneAutoService(false, true, nil).settingService
	got, err := BindOpenAIRequestTimezone(req, "http://tokyo.example:8080")
	require.NoError(t, err)
	require.Same(t, req, got)
	reader, err := req.GetBody()
	require.NoError(t, err)
	original, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, body, original)
	require.Len(t, probe.calls, 2)
}
