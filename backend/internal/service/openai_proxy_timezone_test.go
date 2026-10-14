package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type timezoneProberFunc func(context.Context, string) (*ProxyExitInfo, int64, error)

func (f timezoneProberFunc) ProbeProxy(c context.Context, u string) (*ProxyExitInfo, int64, error) {
	return f(c, u)
}
func timezoneProxyFixture() *Proxy {
	return &Proxy{ID: 7, Protocol: "http", Host: "proxy.example", Port: 8080, Username: "session-a", Password: "test-secret", Status: StatusActive}
}
func timezoneResult(zone string) *ProxyExitInfo {
	return &ProxyExitInfo{IP: "192.0.2.7", City: "test city", Country: "test country", Timezone: zone}
}

func TestProxyTimezoneCacheForceExpiryAndCredentialChange(t *testing.T) {
	calls := 0
	clock := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	resolver := newProxyTimezoneResolver(timezoneProberFunc(func(_ context.Context, _ string) (*ProxyExitInfo, int64, error) {
		calls++
		return timezoneResult("America/Los_Angeles"), 1, nil
	}))
	resolver.now = func() time.Time { return clock }
	p := timezoneProxyFixture()
	first := resolver.resolve(context.Background(), p, false)
	require.True(t, first.Success)
	require.False(t, first.Cached)
	require.Equal(t, "America/Los_Angeles", first.Timezone)
	require.Equal(t, clock.Add(15*time.Minute), first.ExpiresAt)
	require.True(t, resolver.resolve(context.Background(), p, false).Cached)
	require.Equal(t, 1, calls)
	resolver.resolve(context.Background(), p, true)
	require.Equal(t, 2, calls)
	clock = clock.Add(15 * time.Minute)
	require.False(t, resolver.resolve(context.Background(), p, false).Cached)
	require.Equal(t, 3, calls)
	p.Username = "session-b"
	resolver.resolve(context.Background(), p, false)
	p.Password = "changed-secret"
	resolver.resolve(context.Background(), p, false)
	p.Host = "new.example"
	resolver.resolve(context.Background(), p, false)
	require.Equal(t, 6, calls)
	for k, v := range resolver.cache {
		require.Len(t, k, 64)
		raw, _ := json.Marshal(v)
		require.NotContains(t, string(raw), "secret")
		require.NotContains(t, string(raw), "session-")
	}
}

func TestProxyTimezoneFailureDoesNotReuseOldLocation(t *testing.T) {
	calls := 0
	clock := time.Now()
	resolver := newProxyTimezoneResolver(timezoneProberFunc(func(_ context.Context, _ string) (*ProxyExitInfo, int64, error) {
		calls++
		if calls == 1 {
			return timezoneResult("Asia/Tokyo"), 0, nil
		}
		return nil, 0, errors.New("credentials must not escape")
	}))
	resolver.now = func() time.Time { return clock }
	p := timezoneProxyFixture()
	require.True(t, resolver.resolve(context.Background(), p, false).Success)
	v := resolver.resolve(context.Background(), p, true)
	require.False(t, v.Success)
	require.Empty(t, v.Timezone)
	require.Equal(t, "probe_failed", v.Reason)
	require.Equal(t, clock.Add(30*time.Second), v.ExpiresAt)
	require.True(t, resolver.resolve(context.Background(), p, false).Cached)
	require.Equal(t, 2, calls)
	clock = clock.Add(30 * time.Second)
	resolver.resolve(context.Background(), p, false)
	require.Equal(t, 3, calls)
}

func TestProxyTimezoneRejectsUnreliableMetadataAndUnavailableProxy(t *testing.T) {
	for _, info := range []*ProxyExitInfo{nil, timezoneResult(""), timezoneResult("Local"), timezoneResult("US"), {IP: "not an IP", Timezone: "Asia/Tokyo"}} {
		resolver := newProxyTimezoneResolver(timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) { return info, 0, nil }))
		require.False(t, resolver.resolve(context.Background(), timezoneProxyFixture(), false).Success)
	}
	var calls atomic.Int32
	resolver := newProxyTimezoneResolver(timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) {
		calls.Add(1)
		return timezoneResult("Asia/Tokyo"), 0, nil
	}))
	require.Equal(t, "no_proxy", resolver.resolve(context.Background(), nil, false).Reason)
	p := timezoneProxyFixture()
	p.Status = "disabled"
	require.Equal(t, "proxy_unavailable", resolver.resolve(context.Background(), p, false).Reason)
	p.Status = StatusActive
	expired := time.Now().Add(-time.Second)
	p.ExpiresAt = &expired
	require.Equal(t, "proxy_unavailable", resolver.resolve(context.Background(), p, false).Reason)
	require.Zero(t, calls.Load())
	var nilResolver *proxyTimezoneResolver
	require.Equal(t, "probe_unavailable", nilResolver.resolve(context.Background(), timezoneProxyFixture(), false).Reason)
}

func TestProxyTimezoneConcurrentSingleFlightAndCancellation(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	resolver := newProxyTimezoneResolver(timezoneProberFunc(func(ctx context.Context, _ string) (*ProxyExitInfo, int64, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return timezoneResult("Asia/Tokyo"), 0, nil
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}))
	p := timezoneProxyFixture()
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan ProxyTimezoneResult, 1)
	go func() { canceled <- resolver.resolve(ctx, p, false) }()
	<-started
	cancel()
	require.Equal(t, "request_canceled", (<-canceled).Reason)
	results := make(chan ProxyTimezoneResult, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- resolver.resolve(context.Background(), p, false) }()
	}
	close(release)
	wg.Wait()
	close(results)
	for v := range results {
		require.True(t, v.Success)
	}
	require.Equal(t, int32(1), calls.Load())
}

func TestProxyTimezoneProbeDeadline(t *testing.T) {
	resolver := newProxyTimezoneResolver(timezoneProberFunc(func(ctx context.Context, _ string) (*ProxyExitInfo, int64, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
		<-ctx.Done()
		return nil, 0, ctx.Err()
	}))
	start := time.Now()
	v := resolver.resolve(context.Background(), timezoneProxyFixture(), false)
	require.False(t, v.Success)
	require.Less(t, time.Since(start), 7*time.Second)
}

func TestProxyTimezoneNormalizerPreservesBodyAndSharedAccount(t *testing.T) {
	for _, zone := range []string{"America/Los_Angeles", "Asia/Tokyo"} {
		for _, search := range []bool{false, true} {
			t.Run(zone+"/"+map[bool]string{false: "search-off", true: "search-on"}[search], func(t *testing.T) {
				svc := &OpenAIGatewayService{proxyTimezone: newProxyTimezoneResolver(timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) { return timezoneResult(zone), 0, nil }))}
				p := timezoneProxyFixture()
				account := &Account{Platform: PlatformOpenAI, ProxyID: &p.ID, Proxy: p, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneModeExtraKey: "proxy", openAIRequestTimezoneExtraKey: "Asia/Singapore", openAIRequestTimezoneSyncSearchExtraKey: search, "pool": "keep"}}
				originalExtra, _ := json.Marshal(account.Extra)
				text := "<environment_context><timezone>Asia/Shanghai</timezone><current_date>2026-10-14</current_date><cwd>/keep</cwd></environment_context>"
				body := localeTestBody([]string{text, text}, []string{"environments.environment_context", "user.text"})
				original := bytes.Clone(body)
				for _, transport := range []string{"http", "ws"} {
					out := svc.normalizeOpenAIRequestLocale(context.Background(), account, body, transport, time.Now())
					require.Contains(t, gjson.GetBytes(out, "input.0.content.0.text").String(), "<timezone>"+zone+"</timezone>")
					require.Contains(t, gjson.GetBytes(out, "input.0.content.0.text").String(), "<current_date>2026-10-14</current_date>")
					require.Equal(t, text, gjson.GetBytes(out, "input.0.content.1.text").String())
					for _, path := range []string{"model", "reasoning", "prompt_cache_key", "previous_response_id", "tools.0.user_location.city", "tools.0.user_location.country"} {
						require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(out, path).Raw)
					}
					expectedSearch := "America/New_York"
					if search {
						expectedSearch = zone
					}
					require.Equal(t, expectedSearch, gjson.GetBytes(out, "tools.0.user_location.timezone").String())
				}
				extra, _ := json.Marshal(account.Extra)
				require.Equal(t, originalExtra, extra)
				require.Equal(t, original, body)
				account.ProxyID = nil
				require.Equal(t, body, svc.normalizeOpenAIRequestLocale(context.Background(), account, body, "http", time.Now()))
				account.ProxyID = &p.ID
				svc.proxyTimezone = newProxyTimezoneResolver(timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) { return nil, 0, errors.New("unavailable") }))
				require.Equal(t, body, svc.normalizeOpenAIRequestLocale(context.Background(), account, body, "http", time.Now()))
			})
		}
	}
}

func TestProxyTimezoneModeValidationAndPreservation(t *testing.T) {
	for _, mode := range []any{"manual", "proxy"} {
		require.NoError(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, map[string]any{openAIRequestTimezoneModeExtraKey: mode}))
	}
	for _, mode := range []any{"auto", "", true, 42, nil, map[string]any{}, []any{}} {
		require.Error(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, map[string]any{openAIRequestTimezoneModeExtraKey: mode}))
	}
	a := &Account{Extra: map[string]any{openAIRequestTimezoneModeExtraKey: "proxy", "pool": "keep"}}
	require.Equal(t, "proxy", preserveOpenAIRequestTimezoneExtra(a, map[string]any{"pool": "new"})[openAIRequestTimezoneModeExtraKey])
	require.Equal(t, "keep", preserveOpenAIRequestTimezoneExtra(a, map[string]any{openAIRequestTimezoneModeExtraKey: "manual"})["pool"])
}
