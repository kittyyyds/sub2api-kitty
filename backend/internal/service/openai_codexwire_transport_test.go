package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// codexWireRecordingUpstream records which HTTPUpstream entry point the final
// fallthrough used and with which TLS profile.
type codexWireRecordingUpstream struct {
	doCalls  int
	tlsCalls int
	profile  *tlsfingerprint.Profile
	request  *http.Request
	body     []byte
}

func (u *codexWireRecordingUpstream) record(req *http.Request) (*http.Response, error) {
	u.request = req
	if req.Body != nil {
		u.body, _ = io.ReadAll(req.Body)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
}

func (u *codexWireRecordingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.doCalls++
	return u.record(req)
}

func (u *codexWireRecordingUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.tlsCalls++
	u.profile = profile
	return u.record(req)
}

func codexWireTestConfig(enabled bool) *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ExactFidelityEnabled = enabled
	return cfg
}

func codexWireTestAccount() *Account {
	return &Account{ID: 42, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Concurrency: 1, Extra: map[string]any{}, Credentials: map[string]any{}}
}

func codexWireResponsesRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(body))
	require.NoError(t, err)
	return req
}

func TestCodexWireHTTPEligibleMatrix(t *testing.T) {
	on := codexWireTestConfig(true)
	req := codexWireResponsesRequest(t, `{}`)

	require.True(t, codexWireHTTPEligible(on, req, codexWireTestAccount()))
	setup := codexWireTestAccount()
	setup.Type = AccountTypeSetupToken
	require.True(t, codexWireHTTPEligible(on, req, setup), "setup tokens share the Codex transport")
	legacy := codexWireTestAccount()
	legacy.Platform = ""
	require.True(t, codexWireHTTPEligible(on, req, legacy), "implicit-platform legacy OAuth is OpenAI")

	require.False(t, codexWireHTTPEligible(codexWireTestConfig(false), req, codexWireTestAccount()), "disabled config")
	require.False(t, codexWireHTTPEligible(nil, req, codexWireTestAccount()))
	apiKey := codexWireTestAccount()
	apiKey.Type = AccountTypeAPIKey
	require.False(t, codexWireHTTPEligible(on, req, apiKey), "API-key accounts stay on net/http")
	grok := codexWireTestAccount()
	grok.Platform = PlatformGrok
	require.False(t, codexWireHTTPEligible(on, req, grok), "non-OpenAI OAuth must not get Codex headers")
	vmBound := codexWireTestAccount()
	vmBound.Extra["openai_vm_bound"] = true
	require.False(t, codexWireHTTPEligible(on, req, vmBound))
	vmManaged := codexWireTestAccount()
	vmManaged.Extra[OpenAIVMManagedExtraKey] = true
	require.False(t, codexWireHTTPEligible(on, req, vmManaged))
	excel := codexWireTestAccount()
	excel.Credentials["client_id"] = openai.ExcelClientID
	require.False(t, codexWireHTTPEligible(on, req, excel))
	require.False(t, codexWireHTTPEligible(on, req, nil))
}

func TestDoOpenAIUpstreamFinalFallthroughUsesCodexWire(t *testing.T) {
	upstream := &codexWireRecordingUpstream{}
	svc := &OpenAIGatewayService{cfg: codexWireTestConfig(true), httpUpstream: upstream}
	plain := `{"model":"gpt-5","input":[],"stream":true}`

	resp, err := svc.doOpenAIUpstream(codexWireResponsesRequest(t, plain), "", codexWireTestAccount())
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Zero(t, upstream.doCalls)
	require.Equal(t, 1, upstream.tlsCalls)
	require.NotNil(t, upstream.profile)
	require.True(t, upstream.profile.H1, "default Codex HTTP lane is OpenSSL raw h1")
	require.False(t, upstream.profile.H2)
	require.Equal(t, "zstd", upstream.request.Header.Get("Content-Encoding"))
	require.True(t, CodexBusinessHTTPFromContext(upstream.request.Context()))
	zr, err := zstd.NewReader(bytes.NewReader(upstream.body))
	require.NoError(t, err)
	defer zr.Close()
	decoded, err := io.ReadAll(zr)
	require.NoError(t, err)
	require.Equal(t, plain, string(decoded))
}

func TestDoOpenAIUpstreamKeepsLegacyTransportForIneligibleAccounts(t *testing.T) {
	cases := map[string]func(*Account){
		"api_key":  func(a *Account) { a.Type = AccountTypeAPIKey },
		"vm_bound": func(a *Account) { a.Extra["openai_vm_bound"] = true },
		"excel":    func(a *Account) { a.Credentials["client_id"] = openai.ExcelClientID },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			upstream := &codexWireRecordingUpstream{}
			svc := &OpenAIGatewayService{cfg: codexWireTestConfig(true), httpUpstream: upstream}
			account := codexWireTestAccount()
			mutate(account)
			proxy := ""
			if name == "vm_bound" {
				// A VM-bound account without the VM service keeps its own
				// managed proxy on the legacy transport.
				id := int64(7)
				account.ProxyID = &id
				account.Proxy = &Proxy{ID: id, Protocol: "http", Host: "127.0.0.1", Port: 8080, Username: "42", Password: "p", Status: StatusActive}
				proxy = account.Proxy.URL()
			}
			resp, err := svc.doOpenAIUpstream(codexWireResponsesRequest(t, `{"a":1}`), proxy, account)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, 1, upstream.doCalls)
			require.Zero(t, upstream.tlsCalls)
			require.Empty(t, upstream.request.Header.Get("Content-Encoding"))
			require.Equal(t, `{"a":1}`, string(upstream.body))
		})
	}

	t.Run("exact_fidelity_disabled", func(t *testing.T) {
		upstream := &codexWireRecordingUpstream{}
		svc := &OpenAIGatewayService{cfg: codexWireTestConfig(false), httpUpstream: upstream}
		resp, err := svc.doOpenAIUpstream(codexWireResponsesRequest(t, `{"a":1}`), "", codexWireTestAccount())
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, 1, upstream.doCalls)
		require.Zero(t, upstream.tlsCalls)
	})
}

func TestCompressCodexResponsesRequestBody(t *testing.T) {
	plain := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":true}`)
	req := codexWireResponsesRequest(t, string(plain))
	compressCodexResponsesRequestBody(req)
	require.Equal(t, "zstd", req.Header.Get("Content-Encoding"))
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.EqualValues(t, len(body), req.ContentLength)
	zr, err := zstd.NewReader(bytes.NewReader(body))
	require.NoError(t, err)
	defer zr.Close()
	got, err := io.ReadAll(zr)
	require.NoError(t, err)
	require.Equal(t, plain, got)
	// 重试重放：GetBody 必须返回相同的压缩字节。
	rc, err := req.GetBody()
	require.NoError(t, err)
	replay, err := io.ReadAll(rc)
	_ = rc.Close()
	require.NoError(t, err)
	require.Equal(t, body, replay)

	for _, path := range []string{
		"https://chatgpt.com/backend-api/codex/responses/count_tokens",
		"https://chatgpt.com/backend-api/codex/images/generations",
		"https://chatgpt.com/v1/embeddings",
	} {
		other, err := http.NewRequest(http.MethodPost, path, strings.NewReader(`{"a":1}`))
		require.NoError(t, err)
		compressCodexResponsesRequestBody(other)
		require.Empty(t, other.Header.Get("Content-Encoding"), path)
		raw, _ := io.ReadAll(other.Body)
		require.Equal(t, `{"a":1}`, string(raw), path)
	}

	encoded := codexWireResponsesRequest(t, `{"model":"gpt-5"}`)
	encoded.Header.Set("Content-Encoding", "gzip")
	compressCodexResponsesRequestBody(encoded)
	require.Equal(t, "gzip", encoded.Header.Get("Content-Encoding"))
	raw, _ := io.ReadAll(encoded.Body)
	require.Equal(t, `{"model":"gpt-5"}`, string(raw))
}

func TestResolveCodexHTTPSTLSProfileDefaultsAndOverrides(t *testing.T) {
	// Official Linux Codex CLI HTTP lane: OpenSSL, no ALPN, raw HTTP/1.1.
	if p := resolveCodexHTTPSTLSProfile(nil, codexWireTestAccount()); p == nil || !p.H1 || p.H2 || len(p.ALPNProtocols) != 0 || !strings.Contains(p.Name, "OpenSSL") {
		t.Fatalf("default profile = %+v, want openssl36 h1", p)
	}
	// Custom-CA deployment emulation: rustls + h2.
	h2 := &config.Config{}
	h2.Gateway.OpenAIWS.CodexHTTPH2Enabled = true
	if p := resolveCodexHTTPSTLSProfile(h2, codexWireTestAccount()); p == nil || !p.H2 || !strings.Contains(strings.ToLower(p.Name), "rustls") {
		t.Fatalf("h2 profile = %+v, want rustls-h2", p)
	}
	withProfile := codexWireTestAccount()
	withProfile.Extra["codex_tls_profile"] = "nodejs24"
	if p := resolveCodexHTTPSTLSProfile(nil, withProfile); p == nil || !strings.Contains(strings.ToLower(p.Name), "node") {
		t.Fatalf("override profile = %+v, want nodejs", p)
	}
	withNone := codexWireTestAccount()
	withNone.Extra["codex_tls_profile"] = "none"
	require.Nil(t, resolveCodexHTTPSTLSProfile(nil, withNone))
	_, ok, err := resolveCodexWireHTTPProfile(codexWireTestConfig(true), codexWireResponsesRequest(t, `{}`), withNone)
	require.NoError(t, err)
	require.False(t, ok, "explicit none keeps the legacy transport")
}

func TestOpenAIWSAccountDialerRawAndFallback(t *testing.T) {
	cfg := codexWireTestConfig(true)
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "raw-handshake")
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		kind, frame, err := conn.Read(r.Context())
		if err == nil {
			err = conn.Write(r.Context(), kind, frame)
		}
		serverErr <- err
	}))
	defer server.Close()
	fallback := &openAIWSCaptureDialer{conn: &openAIWSCaptureConn{}}
	req := openAIWSAcquireRequest{Account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, WSURL: "ws" + strings.TrimPrefix(server.URL, "http")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, headers, err := dialOpenAIWSForAccount(ctx, cfg, req, http.Header{"Authorization": {"Bearer fake"}}, fallback)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.IsType(t, &codexWireClientConn{}, conn)
	require.Zero(t, fallback.DialCount())
	require.Equal(t, "raw-handshake", headers.Get("X-Request-ID"))
	frameConn, ok := conn.(openaiwsv2.FrameConn)
	require.True(t, ok)
	require.NoError(t, frameConn.WriteFrame(ctx, coderws.MessageBinary, []byte(`{"type":"response.create"}`)))
	kind, frame, err := frameConn.ReadFrame(ctx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageBinary, kind)
	require.JSONEq(t, `{"type":"response.create"}`, string(frame))
	require.NoError(t, <-serverErr)

	req.Account.Type = AccountTypeAPIKey
	_, _, _, err = dialOpenAIWSForAccount(ctx, cfg, req, nil, fallback)
	require.NoError(t, err)
	require.Equal(t, 1, fallback.DialCount(), "API-key accounts retain their existing dialer")

	req.Account = &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_vm_bound": true}}
	_, _, _, err = dialOpenAIWSForAccount(ctx, cfg, req, nil, fallback)
	require.NoError(t, err)
	require.Equal(t, 2, fallback.DialCount(), "VM-bound accounts retain the VM dialer")

	req.Account = &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	_, _, _, err = dialOpenAIWSForAccount(withOpenAIVMWSHTTPClient(ctx, &http.Client{}), cfg, req, nil, fallback)
	require.NoError(t, err)
	require.Equal(t, 3, fallback.DialCount(), "a VM route in context keeps the VM HTTP client")

	_, _, _, err = dialOpenAIWSForAccount(ctx, codexWireTestConfig(false), req, nil, fallback)
	require.NoError(t, err)
	require.Equal(t, 4, fallback.DialCount(), "exact fidelity disabled keeps coder/websocket")
}

func TestOpenAIWSAccountDialerHandshakeFailureAndProxy(t *testing.T) {
	cfg := codexWireTestConfig(true)
	fallback := &openAIWSCaptureDialer{conn: &openAIWSCaptureConn{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "failed-handshake")
		w.WriteHeader(http.StatusUpgradeRequired)
		_, _ = w.Write([]byte(`{"error":"upgrade"}`))
	}))
	defer server.Close()
	req := openAIWSAcquireRequest{Account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, WSURL: "ws" + strings.TrimPrefix(server.URL, "http")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, status, headers, err := dialOpenAIWSForAccount(ctx, cfg, req, nil, fallback)
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusUpgradeRequired, status)
	require.Equal(t, "failed-handshake", headers.Get("X-Request-ID"))
	var handshakeErr *openAIWSHandshakeError
	require.ErrorAs(t, err, &handshakeErr)
	require.JSONEq(t, `{"error":"upgrade"}`, string(handshakeErr.Body))

	// The account's own HTTP proxy (IPv6 lease proxies included) carries the
	// CONNECT tunnel.
	proxyRequests := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests <- r.Method + " " + r.Host + " " + r.Header.Get("Proxy-Authorization")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer proxy.Close()
	req.WSURL = "wss://codex-proxy-test.invalid/responses"
	req.ProxyURL = strings.Replace(proxy.URL, "http://", "http://v6-lease:secret@", 1)
	_, _, _, err = dialOpenAIWSForAccount(ctx, cfg, req, nil, fallback)
	require.Error(t, err)
	select {
	case request := <-proxyRequests:
		require.Equal(t, "CONNECT codex-proxy-test.invalid:443 Basic djYtbGVhc2U6c2VjcmV0", request)
	case <-ctx.Done():
		t.Fatal("configured proxy was not used")
	}
	require.Zero(t, fallback.DialCount())
	// A plaintext proxy stays on the existing generic interface.
	req.WSURL = "ws://codex-proxy-test.invalid/responses"
	_, _, _, err = dialOpenAIWSForAccount(ctx, cfg, req, nil, fallback)
	require.NoError(t, err)
	require.Equal(t, 1, fallback.DialCount())
}
