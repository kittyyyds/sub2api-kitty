package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIClientProfilePresetUA(t *testing.T) {
	for _, tc := range []struct{ preset, prefix string }{{"macos", "codex-tui/"}, {"windows", "codex-tui/"}, {"linux", "codex_cli_rs/"}} {
		ua := OpenAIClientProfilePresetUA(tc.preset, "0.146.0")
		if len(ua) == 0 || ua[:len(tc.prefix)] != tc.prefix {
			t.Fatalf("preset %s UA=%q", tc.preset, ua)
		}
	}
}

func TestValidateOpenAIClientProfileExtra(t *testing.T) {
	valid := map[string]any{OpenAIClientProfileExtraKey: map[string]any{"enabled": true, "preset": "macos", "http_version": "http1", "tls_profile": "safari"}}
	if err := ValidateOpenAIClientProfileExtra(PlatformOpenAI, AccountTypeOAuth, valid); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOpenAIClientProfileExtra(PlatformOpenAI, AccountTypeAPIKey, valid); err == nil {
		t.Fatal("expected OAuth-only rejection")
	}
	bad := map[string]any{OpenAIClientProfileExtraKey: map[string]any{"enabled": true, "preset": "linux", "http_version": "http2", "tls_profile": "chrome"}}
	if err := ValidateOpenAIClientProfileExtra(PlatformOpenAI, AccountTypeOAuth, bad); err == nil {
		t.Fatal("expected h2/custom TLS rejection")
	}
}

func TestAccountClientProfileDisabledPreservesUA(t *testing.T) {
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"user_agent": "codex-tui/0.146.0 (Linux; x86_64) xterm"}, Extra: map[string]any{OpenAIClientProfileExtraKey: map[string]any{"enabled": false, "preset": "macos"}}}
	if got := a.GetOpenAIUserAgent(); got != a.GetCredential("user_agent") {
		t.Fatalf("disabled profile changed UA: %q", got)
	}
}

func TestOpenAIClientProfileRejectsUnsafeSettings(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"injected language", "language", "en-US\r\nAuthorization: fake"},
		{"leading controls", "language", "\r\nen-US"},
		{"language syntax", "language", "en-US;q=3"},
		{"language non ASCII", "language", "中文"},
		{"unsupported field", "rpm", 10},
		{"negative integer", "max_conns_per_host", -1},
		{"fraction", "max_conns_per_host", 1.5},
		{"nan", "max_conns_per_host", math.NaN()},
		{"inf", "max_conns_per_host", math.Inf(1)},
		{"overflow", "max_conns_per_host", 1e50},
		{"connection budget", "max_conns_per_host", 257},
		{"idle duration", "idle_conn_timeout_seconds", 601},
		{"retry budget", "retry_total_budget_ms", 120001},
		{"stale version", "client_version", "0.100.0"},
		{"version controls", "client_version", "0.150.0\n"},
		{"bool type", "enabled", "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOpenAIClientProfile(map[string]any{"enabled": true, tc.key: tc.value})
			require.Error(t, err)
		})
	}
	_, err := parseOpenAIClientProfile(map[string]any{"max_conns_per_host": json.Number("32"), "language": "zh-CN,zh;q=0.9,en-US;q=0.8"})
	require.NoError(t, err)
}

func TestOpenAIClientProfileHeadersPreserveRelationships(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		OpenAIClientProfileExtraKey: map[string]any{"enabled": true, "preset": "windows", "client_version": "0.160.0", "language": "en-US,en;q=0.9"},
	}}
	headers := http.Header{
		"Originator": {"codex-tui"}, "User-Agent": {"codex-tui/0.146.0 (Linux; x86_64) xterm"},
		"Authorization": {"Bearer test-secret"}, "Session-Id": {"thread-original"}, "X-Codex-Turn-State": {"state-original"},
		"X-Client-Request-Id": {"request-original"}, "X-Codex-Turn-Metadata": {`{"turn_id":"turn-original"}`},
	}
	before := headers.Clone()
	ApplyOpenAIClientProfileHeaders(headers, account)
	require.Contains(t, headers.Get("User-Agent"), "codex-tui/0.160.0 (Windows 11; x86_64)")
	require.Equal(t, "0.160.0", headers.Get("Version"))
	require.Equal(t, "codex-tui", headers.Get("Originator"))
	require.Equal(t, "en-US,en;q=0.9", headers.Get("Accept-Language"))
	for _, key := range []string{"Authorization", "Session-Id", "X-Codex-Turn-State", "X-Client-Request-Id", "X-Codex-Turn-Metadata"} {
		require.Equal(t, before.Values(key), headers.Values(key), key)
	}
	account.Extra[OpenAIClientProfileExtraKey].(map[string]any)["enabled"] = false
	unchanged := before.Clone()
	ApplyOpenAIClientProfileHeaders(unchanged, account)
	require.Equal(t, before, unchanged)
	account.Extra[OpenAIClientProfileExtraKey].(map[string]any)["enabled"] = true
	compat := before.Clone()
	compat.Del("Originator")
	compatBefore := compat.Clone()
	ApplyOpenAIClientProfileHeaders(compat, account)
	require.Equal(t, compatBefore, compat)
}

func TestOpenAIClientProfileDedicatedSavePreservesAccountData(t *testing.T) {
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token-preserved"}, Concurrency: 5,
		Extra: map[string]any{"quota_used": 42.0, "model_rate_limits": map[string]any{"test": true}, codexFingerprintModeExtraKey: "session", codexFingerprintSeedExtraKey: testCodexFingerprintSeed},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{12: account}}
	svc := &adminServiceImpl{accountRepo: repo}
	profile := map[string]any{"enabled": true, "preset": "macos"}
	updated, err := svc.UpdateOpenAIClientProfile(context.Background(), 12, profile, nil)
	require.NoError(t, err)
	require.Equal(t, profile, updated.Extra[OpenAIClientProfileExtraKey])
	require.Equal(t, 42.0, updated.Extra["quota_used"])
	require.Equal(t, map[string]any{"test": true}, updated.Extra["model_rate_limits"])
	require.Equal(t, "session", updated.Extra[codexFingerprintModeExtraKey])
	require.Equal(t, testCodexFingerprintSeed, updated.Extra[codexFingerprintSeedExtraKey])
	require.Equal(t, "token-preserved", updated.Credentials["access_token"])
	require.Equal(t, 5, updated.Concurrency)
	require.Len(t, repo.updates[12], 1)
	require.Len(t, repo.updates[12][0], 1, "only profile may be written when convergence was not edited")
	mode := "device"
	_, err = svc.UpdateOpenAIClientProfile(context.Background(), 12, profile, &mode)
	require.NoError(t, err)
	require.Len(t, repo.updates[12][1], 2)
	require.NotContains(t, repo.updates[12][1], codexFingerprintSeedExtraKey)
	_, err = svc.UpdateOpenAIClientProfile(context.Background(), 12, map[string]any{"language": "en\nInjected: 1"}, nil)
	require.Error(t, err)
	require.Len(t, repo.updates[12], 2, "invalid settings must not write")
}

func TestOpenAIClientProfileBulkValidatesEveryTargetBeforeWrite(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		2: {ID: 2, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
	}}
	_, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2}, Extra: map[string]any{OpenAIClientProfileExtraKey: map[string]any{"enabled": true, "preset": "macos"}},
	})
	require.Error(t, err)
	require.Empty(t, repo.bulkUpdates)
}

func TestOpenAIClientProfileWSModeAndRetryOverrides(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		"openai_oauth_responses_websockets_v2_mode": "ctx_pool",
		OpenAIClientProfileExtraKey:                 map[string]any{"enabled": true, "ws_mode": "off", "retry_backoff_initial_ms": 300, "retry_backoff_max_ms": 700, "retry_total_budget_ms": 1500},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	require.Equal(t, "off", account.ResolveOpenAIResponsesWebSocketV2Mode("passthrough"))
	require.Equal(t, 300*time.Millisecond, svc.openAIWSRetryBackoffForAccount(1, account))
	require.Equal(t, 700*time.Millisecond, svc.openAIWSRetryBackoffForAccount(4, account))
	require.Equal(t, 1500*time.Millisecond, svc.openAIWSRetryTotalBudgetForAccount(account))
	account.Extra[OpenAIClientProfileExtraKey].(map[string]any)["enabled"] = false
	require.Equal(t, "ctx_pool", account.ResolveOpenAIResponsesWebSocketV2Mode("passthrough"))
	require.Equal(t, svc.openAIWSRetryBackoff(1), svc.openAIWSRetryBackoffForAccount(1, account))
	require.Equal(t, svc.openAIWSRetryTotalBudget(), svc.openAIWSRetryTotalBudgetForAccount(account))
}

func TestOpenAIClientProfileHTTPAndWSFinalIdentityAndPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"stream":true,"previous_response_id":"resp_original","tools":[{"type":"function","name":"run","parameters":{"type":"object"}}],"input":[{"type":"function_call_output","call_id":"call_original","output":"done"}]}`)
	account := &Account{ID: 87, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "same-account"}}
	svc := &OpenAIGatewayService{}
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		c.Request.Header.Set("User-Agent", "codex-tui/0.146.0 (Linux; x86_64) xterm")
		c.Request.Header.Set("Accept-Language", "zh-CN")
		c.Request.Header.Set("Session-Id", "session-original")
		c.Request.Header.Set("X-Codex-Installation-Id", "installation-original")
		c.Request.Header.Set("X-Codex-Turn-State", "state-original")
		c.Request.Header.Set("X-Client-Request-Id", "request-original")
		c.Set("api_key", &APIKey{ID: 41})
		return c
	}
	buildHTTP := func(passthrough bool) *http.Request {
		c := newContext()
		var req *http.Request
		var err error
		if passthrough {
			req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "same-token")
		} else {
			req, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "same-token", true, "same-cache-key", true)
		}
		require.NoError(t, err)
		wireBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, body, wireBody, "profiles must not edit model/effort/tools/continuation/payload")
		return req
	}
	buildWS := func() http.Header {
		h, _, err := svc.buildOpenAIWSHeaders(context.Background(), newContext(), account, "same-token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "state-original", "", "same-cache-key", "gpt-5.6-sol", "")
		require.NoError(t, err)
		return h
	}
	baselineHTTP := buildHTTP(false).Header
	baselinePass := buildHTTP(true).Header
	baselineWS := buildWS()
	for _, preset := range []string{"macos", "linux", "windows"} {
		account.Extra = map[string]any{OpenAIClientProfileExtraKey: map[string]any{"enabled": true, "preset": preset, "client_version": "0.160.0", "language": "en-US"}}
		for _, pair := range []struct {
			name             string
			actual, baseline http.Header
		}{
			{"HTTP", buildHTTP(false).Header, baselineHTTP}, {"passthrough", buildHTTP(true).Header, baselinePass}, {"WS", buildWS(), baselineWS},
		} {
			require.Equal(t, OpenAIClientProfilePresetUA(preset, "0.160.0"), pair.actual.Get("User-Agent"), pair.name)
			require.Equal(t, "0.160.0", pair.actual.Get("Version"), pair.name)
			require.Equal(t, "en-US", pair.actual.Get("Accept-Language"), pair.name)
			for _, key := range []string{"Authorization", "session_id", "Session-Id", "Conversation-Id", "Thread-Id", "X-Codex-Installation-Id", "X-Codex-Turn-State", "X-Client-Request-Id"} {
				require.Equal(t, pair.baseline.Get(key), pair.actual.Get(key), pair.name+":"+key)
			}
		}
	}
	account.Extra[OpenAIClientProfileExtraKey].(map[string]any)["enabled"] = false
	require.Equal(t, baselineHTTP, buildHTTP(false).Header)
	require.Equal(t, baselinePass, buildHTTP(true).Header)
	require.Equal(t, baselineWS, buildWS())
}

func TestOpenAIClientProfileRejectsCredentialShadows(t *testing.T) {
	parent := int64(13)
	account := &Account{ID: 14, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parent}
	profile := map[string]any{"enabled": true, "preset": "linux"}
	account.Extra = map[string]any{OpenAIClientProfileExtraKey: profile}
	require.Nil(t, account.GetOpenAIClientProfile())
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{14: account}}
	_, err := (&adminServiceImpl{accountRepo: repo}).UpdateOpenAIClientProfile(context.Background(), 14, profile, nil)
	require.Error(t, err)
	require.Empty(t, repo.updates)
}

func TestOpenAIClientProfileUpdateValidatesEffectiveAccountType(t *testing.T) {
	profile := map[string]any{"enabled": true, "preset": "macos"}
	apiAccount := &Account{ID: 83, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	input := &UpdateAccountInput{Type: AccountTypeOAuth, Extra: map[string]any{OpenAIClientProfileExtraKey: profile}}
	result, err := normalizeOpenAIClientProfileUpdateExtra(apiAccount, input, input.Extra)
	require.NoError(t, err)
	require.Equal(t, profile, result[OpenAIClientProfileExtraKey])
	require.Equal(t, AccountTypeAPIKey, apiAccount.Type, "validation must not mutate the loaded account")

	oauthAccount := &Account{ID: 84, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	input.Type = AccountTypeAPIKey
	_, err = normalizeOpenAIClientProfileUpdateExtra(oauthAccount, input, input.Extra)
	require.Error(t, err)
	parent := int64(85)
	oauthAccount.ParentAccountID = &parent
	input.Type = AccountTypeOAuth
	_, err = normalizeOpenAIClientProfileUpdateExtra(oauthAccount, input, input.Extra)
	require.Error(t, err, "effective OAuth type must not bypass shadow guard")
}
