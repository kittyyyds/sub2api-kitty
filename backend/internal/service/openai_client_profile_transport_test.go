package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func clientProfileTransportAccount(id int64, preset, reuse string) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 8, Extra: map[string]any{
		OpenAIClientProfileExtraKey: map[string]any{"enabled": true, "preset": preset, "connection_reuse": reuse},
	}}
}

func TestOpenAIClientProfileContextClearsOnDisabledAccountSwitch(t *testing.T) {
	ctx := context.Background()
	require.True(t, ctx == WithOpenAIClientProfile(ctx, nil))
	profile := &OpenAIClientProfile{Enabled: true, Preset: "macos"}
	ctx = WithOpenAIClientProfile(ctx, profile)
	require.NotNil(t, OpenAIClientProfileFromContext(ctx))
	ctx = WithOpenAIClientProfile(ctx, nil)
	require.Nil(t, OpenAIClientProfileFromContext(ctx))
}

func TestOpenAIClientProfileWSSwitchUsesNewSocketsAndPreservesContinuation(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 4
	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{}
	pool.setClientDialerForTest(dialer)
	account := clientProfileTransportAccount(1401, "macos", "reuse")
	request := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/responses", Headers: http.Header{}}
	first, err := pool.Acquire(context.Background(), request)
	require.NoError(t, err)
	oldID := first.ConnID()
	require.True(t, pool.PinConn(account.ID, oldID))
	first.Release()
	changed := clientProfileTransportAccount(account.ID, "windows", "reuse")
	request.Account = changed
	request.PreferredConnID = oldID
	request.ForcePreferredConn = true
	continuation, err := pool.Acquire(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, oldID, continuation.ConnID())
	require.True(t, continuation.Reused())
	continuation.Release()
	request.PreferredConnID = ""
	request.ForcePreferredConn = false
	next, err := pool.Acquire(context.Background(), request)
	require.NoError(t, err)
	require.NotEqual(t, oldID, next.ConnID())
	require.False(t, next.Reused())
	next.Release()
	pool.UnpinConn(account.ID, oldID)
	require.Equal(t, 2, dialer.DialCount())
}

func TestOpenAIClientProfileWSFreshPreservesKnownContinuation(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 4
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 2
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCountingDialer{})
	account := clientProfileTransportAccount(1402, "linux", "fresh")
	request := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/responses", Headers: http.Header{}}
	first, err := pool.Acquire(context.Background(), request)
	require.NoError(t, err)
	firstID := first.ConnID()
	first.Release()
	ap, ok := pool.getAccountPool(account.ID)
	require.True(t, ok)
	ap.mu.Lock()
	prewarming := ap.prewarmActive
	ap.mu.Unlock()
	require.False(t, prewarming, "fresh sockets must not trigger unused background prewarming")
	second, err := pool.Acquire(context.Background(), request)
	require.NoError(t, err)
	require.NotEqual(t, firstID, second.ConnID())
	secondID := second.ConnID()
	second.Release()
	request.PreferredConnID = secondID
	continued, err := pool.Acquire(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, secondID, continued.ConnID())
	continued.Release()
}

func TestOpenAIClientProfileWSContinuationStillChecksExistingIdentity(t *testing.T) {
	key := openAIWSHandshakeCompatibilityKey{clientProfile: "old", sessionIDHyphen: "tenant-a"}
	conn := &openAIWSConn{handshakeCompatibility: key}
	key.clientProfile = "new"
	require.True(t, conn.matchesContinuationCompatibility(key))
	require.False(t, conn.matchesHandshakeCompatibility(key))
	key.sessionIDHyphen = "tenant-b"
	require.False(t, conn.matchesContinuationCompatibility(key))
}

func TestOpenAIClientProfileWSPoolLimitsAndIdleTimeout(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 8
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 4
	pool := newOpenAIWSConnPool(cfg)
	account := clientProfileTransportAccount(1403, "linux", "reuse")
	raw := account.Extra[OpenAIClientProfileExtraKey].(map[string]any)
	raw["max_conns_per_host"] = 2
	raw["max_idle_conns_per_host"] = 1
	require.Equal(t, 2, pool.effectiveMaxConnsByAccount(account))
	ap := pool.getOrCreateAccountPool(account.ID)
	ap.lastAcquire = &openAIWSAcquireRequest{Account: account}
	old := newOpenAIWSConn("old", account.ID, &openAIWSIdlePingUnsupportedConn{}, nil)
	old.profileIdleTimeout = 5 * time.Second
	old.lastUsedNano.Store(time.Now().Add(-10 * time.Second).UnixNano())
	ap.conns[old.id] = old
	evicted := pool.cleanupAccountLocked(ap, time.Now(), 2)
	require.Len(t, evicted, 1)
	closeOpenAIWSConns(evicted)
}

func TestOpenAIClientProfileExplicitlyUsesCoreTransport(t *testing.T) {
	manager := &PluginManager{}
	manager.route.Store(&pluginRoute{rolloutPercent: 100})
	account := clientProfileTransportAccount(1404, "macos", "inherit")
	require.False(t, manager.ShouldRouteOpenAIOAuth(account))
	account.Extra[OpenAIClientProfileExtraKey].(map[string]any)["enabled"] = false
	require.True(t, manager.ShouldRouteOpenAIOAuth(account))
	delete(account.Extra, OpenAIClientProfileExtraKey)
	require.True(t, manager.ShouldRouteOpenAIOAuth(account))
}

func TestOpenAIClientProfileRealWebSocketPreservesToolTurns(t *testing.T) {
	for index, preset := range []string{"macos", "linux", "windows"} {
		t.Run(preset, func(t *testing.T) {
			headerReceived := make(chan http.Header, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headerReceived <- r.Header.Clone()
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for turn := 0; turn < 2; turn++ {
					messageType, payload, err := conn.Read(r.Context())
					if err != nil {
						return
					}
					if conn.Write(r.Context(), messageType, payload) != nil {
						return
					}
				}
			}))
			defer server.Close()
			account := clientProfileTransportAccount(int64(1500+index), preset, "reuse")
			headers := make(http.Header)
			headers.Set("Session-Id", "stable-session")
			ensureCodexIdentityHeaders(headers)
			ApplyOpenAIClientProfileHeaders(headers, account)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			dialer := newDefaultOpenAIWSClientDialer()
			conn, _, _, err := dialer.Dial(withOpenAIAccountClientProfile(ctx, account), strings.Replace(server.URL, "http://", "ws://", 1), headers, "")
			require.NoError(t, err)
			defer conn.Close()
			seen := <-headerReceived
			require.Equal(t, headers.Get("User-Agent"), seen.Get("User-Agent"))
			require.Equal(t, "stable-session", seen.Get("Session-Id"))
			for _, payload := range []map[string]any{
				{"type": "response.create", "model": "gpt-5.6-sol", "reasoning": map[string]any{"effort": "xhigh"}, "input": []any{map[string]any{"type": "function_call", "call_id": "call-kept", "name": "read_file", "arguments": "{}"}}},
				{"type": "response.create", "previous_response_id": "response-kept", "input": []any{map[string]any{"type": "function_call_output", "call_id": "call-kept", "output": "file-content"}}},
			} {
				require.NoError(t, conn.WriteJSON(ctx, payload))
				response, err := conn.ReadMessage(ctx)
				require.NoError(t, err)
				encoded, err := json.Marshal(payload)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), string(response))
			}
		})
	}
}
