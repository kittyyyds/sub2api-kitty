//go:build unit

package service

import (
	"errors"
	"io"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestKitty29DefaultCompressionPreservesTLSAndReplay(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		name := "h1"
		if h2 {
			name = "h2"
		}
		t.Run(name, func(t *testing.T) {
			cfg := codexWireTestConfig(true)
			cfg.Gateway.OpenAIWS.CodexHTTPH2Enabled = h2
			upstream := &codexWireRecordingUpstream{}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			plain := `{ "model":"gpt-6-astra", "input":"中文 <tag> & unchanged", "stream":true }`
			resp, err := svc.doOpenAIUpstream(codexWireResponsesRequest(t, plain), "", codexWireTestAccount())
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Zero(t, upstream.doCalls)
			require.Equal(t, 1, upstream.tlsCalls)
			require.NotNil(t, upstream.profile)
			require.Equal(t, h2, upstream.profile.H2)
			require.Empty(t, upstream.request.Header.Get("Content-Encoding"))
			require.Equal(t, "disabled", openAIRequestCompressionPolicy(upstream.request))
			require.Equal(t, plain, string(upstream.body))
			require.EqualValues(t, len(plain), upstream.request.ContentLength)
			replay, err := upstream.request.GetBody()
			require.NoError(t, err)
			body, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.NoError(t, replay.Close())
			require.Equal(t, plain, string(body))
		})
	}
}

func TestKitty29CompressionOptInDoesNotCrossAccountLanes(t *testing.T) {
	for name, mutate := range map[string]func(*Account){
		"api_key":  func(a *Account) { a.Type = AccountTypeAPIKey },
		"grok":     func(a *Account) { a.Platform = PlatformGrok },
		"excel":    func(a *Account) { a.Credentials["client_id"] = openai.ExcelClientID },
		"no_tls":   func(a *Account) { a.Extra["codex_tls_profile"] = "none" },
		"vm_bound": func(a *Account) { a.Extra["openai_vm_bound"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := codexWireTestConfig(true)
			cfg.Gateway.OpenAIRequestCompressionEnabled = true
			upstream := &codexWireRecordingUpstream{}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := codexWireTestAccount()
			mutate(account)
			proxy := ""
			if name == "vm_bound" {
				id := int64(7)
				account.ProxyID = &id
				account.Proxy = &Proxy{ID: id, Protocol: "http", Host: "127.0.0.1", Port: 8080, Username: "42", Password: "fixture", Status: StatusActive}
				proxy = account.Proxy.URL()
			}
			plain := `{"model":"fixture","input":"unchanged"}`
			resp, err := svc.doOpenAIUpstream(codexWireResponsesRequest(t, plain), proxy, account)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, 1, upstream.doCalls)
			require.Zero(t, upstream.tlsCalls)
			require.Empty(t, upstream.request.Header.Get("Content-Encoding"))
			require.Equal(t, plain, string(upstream.body))
		})
	}
}

type kitty29ReadFailure struct{ closed bool }

func (b *kitty29ReadFailure) Read(p []byte) (int, error) {
	n := copy(p, `{"partial":`)
	return n, errors.New("synthetic body read failure")
}
func (b *kitty29ReadFailure) Close() error { b.closed = true; return nil }

func TestKitty29CompressionReadFailureNeverDispatches(t *testing.T) {
	cfg := codexWireTestConfig(true)
	cfg.Gateway.OpenAIRequestCompressionEnabled = true
	upstream := &codexWireRecordingUpstream{}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	req := codexWireResponsesRequest(t, `{}`)
	broken := &kitty29ReadFailure{}
	req.Body = broken
	resp, err := svc.doOpenAIUpstream(req, "", codexWireTestAccount())
	require.ErrorContains(t, err, "read Codex request body for compression")
	require.Nil(t, resp)
	require.True(t, broken.closed)
	// A partial read must never become a zero-length upstream request.
	require.Zero(t, upstream.doCalls)
	require.Zero(t, upstream.tlsCalls)
}

func TestKitty29DiagnosticProbeUsesSameCompressionPolicy(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := codexWireTestConfig(true)
		cfg.Gateway.OpenAIRequestCompressionEnabled = enabled
		upstream := &codexWireRecordingUpstream{}
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		resp, err := svc.doOpenAICodexProbe(codexWireResponsesRequest(t, `{"input":"fixture"}`), "", codexWireTestAccount())
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, 1, upstream.tlsCalls)
		want := ""
		if enabled {
			want = "zstd"
		}
		require.Equal(t, want, upstream.request.Header.Get("Content-Encoding"))
	}
}
