package service

import (
	"bytes"
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOfficialAccountRestoreRetiredIdentityFieldsCannotOverrideRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := newTestOAuthAccount(101, map[string]any{
		"codex_fingerprint_mode": "device",
		"openai_device_id":       "official-device",
		"enable_tls_fingerprint": false,
	})
	legacy := *base
	legacy.Extra = maps.Clone(base.Extra)
	for key, value := range map[string]any{
		"anti_degradation":              true,
		"anti_degrade":                  map[string]any{"enabled": true, "mode": "mode1", "policy_version": 3},
		"client_installation_id":        "retired-device",
		"client_identity_compatibility": "identity_v2",
		"fingerprint_v2_seed":           "22222222-2222-4222-8222-222222222222",
		"client_identity":               map[string]any{"enabled": true, "schema_version": 2, "preset": "windows", "client_version": "9.9.9", "ws_mode": "off"},
		"fingerprint_v2":                map[string]any{"enabled": true, "schema_version": 2, "preset": "windows"},
		"tls_fingerprint_builtin":       "nodejs22",
		"enable_tls_fingerprint":        true,
	} {
		legacy.Extra[key] = value
	}
	build := func(account *Account) *http.Request {
		body := []byte(`{"model":"gpt-6-astra","input":"hello","stream":true}`)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		c.Request.Header.Set("session-id", "fixed-client-session")
		stageCodexFingerprintIDs(c, resolveCodexFingerprintIDsFromRequest(account, c.Request.Header))
		request, err := (&OpenAIGatewayService{}).buildUpstreamRequest(c.Request.Context(), c, account, body, "test-token", true, "fixed-cache", true)
		require.NoError(t, err)
		return request
	}
	want, got := build(base), build(&legacy)
	for _, header := range []string{"User-Agent", "Originator", "Version", "Session_Id", "Session-Id", "X-Codex-Installation-Id", "Accept-Language"} {
		require.Equal(t, want.Header.Get(header), got.Header.Get(header), header)
	}
	wantBody, err := io.ReadAll(want.Body)
	require.NoError(t, err)
	gotBody, err := io.ReadAll(got.Body)
	require.NoError(t, err)
	require.Equal(t, wantBody, gotBody)
	require.Equal(t, "official-device", resolveCodexFingerprintIDsFromRequest(&legacy, nil).installationID)
	require.Nil(t, (&TLSFingerprintProfileService{}).ResolveTLSProfile(&legacy), "retired strategy flags must not enable TLS")
}

func TestOfficialAccountRestoreMissingProxyNeverReachesTransport(t *testing.T) {
	proxyID := int64(91)
	account := &Account{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID}
	request := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	// Nil transports deliberately make any attempted fallback fail the test.
	tests := map[string]func() error{
		"inference": func() error {
			_, err := (&OpenAIGatewayService{}).doOpenAIUpstream(request, "", account)
			return err
		},
		"account test": func() error {
			_, err := (&AccountTestService{}).doOpenAIAccountTestUpstream(request, "", account, false, nil)
			return err
		},
		"websocket acquire": func() error {
			_, err := (&openAIWSConnPool{}).Acquire(context.Background(), openAIWSAcquireRequest{Account: account})
			return err
		},
		"websocket prewarm dial": func() error {
			_, err := (&openAIWSConnPool{}).dialConn(context.Background(), openAIWSAcquireRequest{Account: account})
			return err
		},
		"model catalog": func() error {
			_, err := (&OpenAIGatewayService{}).fetchOpenAIModelsUpstream(context.Background(), openAIModelsRequest{credentialAccount: account}, "")
			return err
		},
	}
	for name, invoke := range tests {
		t.Run(name, func(t *testing.T) { require.ErrorContains(t, invoke(), "configured proxy is unavailable") })
	}
	require.NoError(t, ValidateConfiguredAccountProxy(account, "http://127.0.0.1:8080"))
	require.NoError(t, ValidateConfiguredAccountProxy(&Account{}, ""), "accounts explicitly configured for direct access remain supported")
}
