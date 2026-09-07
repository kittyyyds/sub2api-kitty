package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func cursorTestAccount(key string) *Account {
	return &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"cursor_auth_mode": CursorAuthMode, "api_key": key}}
}

func TestCursorAccountRoutingAndCredentialIsolation(t *testing.T) {
	t.Setenv("CURSOR_BRIDGE_URL", "http://shark-cursor-api:10100")
	t.Setenv("CURSOR_BRIDGE_API_KEY", "bridge-secret-for-test")
	for _, key := range []string{"crsr_test_account_aaaaaaaa", "crsr_test_account_bbbbbbbb"} {
		account := cursorTestAccount(key)
		header := http.Header{"x-kitty-cursor-api-key": []string{"forged-client-key"}, "Authorization": []string{"Bearer customer-key"}}
		require.NoError(t, setAnthropicAPIKeyAuthHeader(header, account, key))
		require.Equal(t, "http://shark-cursor-api:10100", account.GetBaseURL())
		require.Equal(t, key, header.Get(CursorKeyHeader))
		require.Equal(t, "bridge-secret-for-test", header.Get("x-api-key"))
		require.Empty(t, header.Get("Authorization"))
		require.NotContains(t, header, "x-kitty-cursor-api-key")
	}
}

func TestCursorAccountValidationPreventsCredentialRedirect(t *testing.T) {
	account := cursorTestAccount("crsr_test_account_aaaaaaaa")
	require.NoError(t, ValidateCursorAccount(account))
	account.Credentials["base_url"] = "https://other.example"
	require.ErrorContains(t, ValidateCursorAccount(account), "Base URL")
	delete(account.Credentials, "base_url")
	account.Platform = PlatformOpenAI
	require.Error(t, ValidateCursorAccount(account))
	account.Platform = PlatformAnthropic
	account.Credentials["api_key"] = "sk-anthropic-key"
	require.ErrorContains(t, ValidateCursorAccount(account), "crsr_")
}

func TestCursorAccountMissingBridgeFailsWithoutForwardingCredential(t *testing.T) {
	t.Setenv("CURSOR_BRIDGE_URL", "")
	t.Setenv("CURSOR_BRIDGE_API_KEY", "")
	header := http.Header{}
	require.Error(t, setAnthropicAPIKeyAuthHeader(header, cursorTestAccount("crsr_test_account_aaaaaaaa"), "crsr_test_account_aaaaaaaa"))
	require.Empty(t, header)
}

func TestCursorAccountVerifyAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "Bearer bridge-test-secret", req.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path == "/v1/cursor/status" {
			_, _ = w.Write([]byte(`{"multi_account":true}`))
			return
		}
		require.Equal(t, "crsr_test_account_aaaaaaaa", req.Header.Get(CursorKeyHeader))
		_, _ = w.Write([]byte(`{"authenticated":true}`))
	}))
	defer server.Close()
	t.Setenv("CURSOR_BRIDGE_URL", server.URL)
	t.Setenv("CURSOR_BRIDGE_API_KEY", "bridge-test-secret")
	require.True(t, GetCursorBridgeStatus(context.Background()).Available)
	require.NoError(t, VerifyCursorUserKey(context.Background(), "crsr_test_account_aaaaaaaa"))
}

func TestCursorAccountVerifyRejectsRedirectAndDoesNotEchoUpstream(t *testing.T) {
	for _, code := range []int{302, 403} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Location", "https://should-not-be-contacted.invalid")
			w.WriteHeader(code)
			_, _ = w.Write([]byte("crsr_secret_from_upstream"))
		}))
		t.Setenv("CURSOR_BRIDGE_URL", server.URL)
		t.Setenv("CURSOR_BRIDGE_API_KEY", "bridge-test-secret")
		err := VerifyCursorUserKey(context.Background(), "crsr_test_account_aaaaaaaa")
		require.Error(t, err)
		require.False(t, strings.Contains(err.Error(), "crsr_secret"))
		server.Close()
	}
}

func TestCursorAccountEditKeepsSecretAndOrdinaryAnthropicAuth(t *testing.T) {
	account := cursorTestAccount("crsr_test_account_aaaaaaaa")
	account.Credentials = MergePreservingSensitiveCreds(account.Credentials, map[string]any{"cursor_auth_mode": CursorAuthMode})
	require.NoError(t, ValidateCursorAccount(account))
	ordinary := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	header := http.Header{CursorKeyHeader: []string{"forged-key"}}
	require.NoError(t, setAnthropicAPIKeyAuthHeader(header, ordinary, "ordinary-api-key"))
	require.Equal(t, "ordinary-api-key", header.Get("x-api-key"))
	require.Empty(t, header.Get(CursorKeyHeader))
}

func TestCursorPartialEditAndInvalidConfigurationFailClosed(t *testing.T) {
	a := cursorTestAccount("crsr_test_account_aaaaaaaa")
	a.Credentials = MergePreservingSensitiveCreds(a.Credentials, map[string]any{"model_mapping": map[string]any{"claude-test": "claude-test"}})
	require.True(t, a.IsCursor())
	require.NoError(t, ValidateCursorAccount(a))
	require.False(t, isUpstreamBillingProbeAccount(a))
	a.Extra = map[string]any{"proxy_pool": []int64{2}}
	require.Error(t, ValidateCursorAccount(a))
	a.Extra = nil
	delete(a.Credentials, "cursor_auth_mode")
	require.Error(t, ValidateCursorAccount(a))
	h := http.Header{}
	require.Error(t, setAnthropicAPIKeyAuthHeader(h, a, a.GetCredential("api_key")))
	require.Empty(t, h)
}

func TestCursorManagedBridgeWorksWithPublicOnlyURLPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "bridge-test-secret", req.Header.Get("x-api-key"))
		require.Equal(t, "crsr_test_account_aaaaaaaa", req.Header.Get(CursorKeyHeader))
		if req.URL.Path == "/redirect" {
			w.Header().Set("Location", "https://must-not-contact.invalid")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, "event: message_stop\ndata: {}\n\n")
	}))
	defer server.Close()
	t.Setenv("CURSOR_BRIDGE_URL", server.URL)
	t.Setenv("CURSOR_BRIDGE_API_KEY", "bridge-test-secret")
	a := cursorTestAccount("crsr_test_account_aaaaaaaa")
	deny := func(string) (string, error) { return "", errors.New("public HTTPS only") }
	base, err := validateAccountUpstreamBaseURL(a, a.GetBaseURL(), deny)
	require.NoError(t, err)
	require.Equal(t, server.URL, base)
	_, err = validateAccountUpstreamBaseURL(a, "https://another.invalid", deny)
	require.Error(t, err)
	_, err = validateAccountUpstreamBaseURL(&Account{}, server.URL, deny)
	require.ErrorContains(t, err, "public HTTPS only")
	for _, path := range []string{"/v1/messages", "/redirect"} {
		req, _ := http.NewRequest(http.MethodPost, base+path, nil)
		require.NoError(t, setAnthropicAPIKeyAuthHeader(req.Header, a, a.GetCredential("api_key")))
		res, err := DoCursorBridgeRequest(req)
		require.NoError(t, err)
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if path == "/redirect" {
			require.Equal(t, http.StatusFound, res.StatusCode)
		} else {
			require.Contains(t, string(body), "message_stop")
		}
		req.URL.Host = "must-not-contact.invalid"
		_, err = DoCursorBridgeRequest(req)
		require.Error(t, err)
	}
}
