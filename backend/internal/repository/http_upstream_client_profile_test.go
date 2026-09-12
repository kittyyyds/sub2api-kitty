package repository

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIClientProfileHTTPPoolsIsolateAccountProxyAndProfile(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.ConnectionPoolIsolation = config.ConnectionPoolIsolationProxy
	s := NewHTTPUpstream(cfg).(*httpUpstreamService)
	p := &service.OpenAIClientProfile{Enabled: true, HTTPVersion: "http1", TLSProfile: "inherit", Preset: "macos", MaxConnsPerHost: 4, MaxIdleConnsPerHost: 2, IdleConnTimeoutSeconds: 25}
	a, err := s.getClientEntryWithClientProfile("", 10, 8, service.HTTPUpstreamProfileOpenAI, p, true, true)
	require.NoError(t, err)
	b, err := s.getClientEntryWithClientProfile("", 10, 8, service.HTTPUpstreamProfileOpenAI, p, false, true)
	require.NoError(t, err)
	require.Same(t, a, b)
	c, err := s.getClientEntryWithClientProfile("", 11, 8, service.HTTPUpstreamProfileOpenAI, p, false, true)
	require.NoError(t, err)
	require.NotSame(t, a, c)
	p2 := *p
	p2.Preset = "linux"
	d, err := s.getClientEntryWithClientProfile("", 10, 8, service.HTTPUpstreamProfileOpenAI, &p2, false, true)
	require.NoError(t, err)
	require.NotSame(t, a, d)
	require.Equal(t, int64(1), atomic.LoadInt64(&a.inFlight), "switching profile must preserve active entry")
	transport := a.client.Transport.(*http.Transport)
	require.Equal(t, 4, transport.MaxConnsPerHost)
	require.Equal(t, 2, transport.MaxIdleConnsPerHost)
	require.Equal(t, 25*time.Second, transport.IdleConnTimeout)
	require.False(t, transport.ForceAttemptHTTP2)
	legacyA, err := s.getClientEntry("", 10, 8, service.HTTPUpstreamProfileOpenAI, false, true)
	require.NoError(t, err)
	legacyB, err := s.getClientEntryWithClientProfile("", 11, 8, service.HTTPUpstreamProfileOpenAI, nil, false, true)
	require.NoError(t, err)
	require.Same(t, legacyA, legacyB, "disabled profile must preserve configured legacy pool sharing")
	require.NotSame(t, a, legacyA)
}

func TestOpenAIClientProfileHTTPFreshAndReuseUseRealConnections(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "reuse", true: "fresh"}[fresh], func(t *testing.T) {
			var connections atomic.Int64
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, r.Header.Get("X-Client-Request-Id"))
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					connections.Add(1)
				}
			}
			server.Start()
			defer server.Close()
			p := &service.OpenAIClientProfile{Enabled: true, HTTPVersion: "http1", TLSProfile: "inherit", ConnectionReuse: map[bool]string{false: "reuse", true: "fresh"}[fresh]}
			s := NewHTTPUpstream(nil)
			for i := 0; i < 2; i++ {
				req, err := http.NewRequestWithContext(service.WithOpenAIClientProfile(context.Background(), p), http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				req.Header.Set("X-Client-Request-Id", "unchanged")
				resp, err := s.Do(req, "", 10, 8)
				require.NoError(t, err)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.Equal(t, "unchanged", string(body))
			}
			if fresh {
				require.Equal(t, int64(2), connections.Load())
			} else {
				require.Equal(t, int64(1), connections.Load())
			}
		})
	}
}

func TestOpenAIClientProfileTLSRejectsCleartextBeforeConnecting(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1) }))
	defer server.Close()
	p := &service.OpenAIClientProfile{Enabled: true, TLSProfile: "chrome"}
	req, err := http.NewRequestWithContext(service.WithOpenAIClientProfile(context.Background(), p), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	_, err = NewHTTPUpstream(nil).Do(req, "", 10, 1)
	require.ErrorContains(t, err, "HTTPS")
	require.Zero(t, requests.Load())
}

func TestOpenAIClientProfileSwitchDoesNotInterruptInFlightResponse(t *testing.T) {
	continueStream := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		<-continueStream
		_, _ = io.WriteString(w, "second")
	}))
	defer server.Close()
	streamReleased := false
	defer func() {
		if !streamReleased {
			close(continueStream)
		}
	}()
	s := NewHTTPUpstream(nil).(*httpUpstreamService)
	profile := &service.OpenAIClientProfile{Enabled: true, Preset: "macos", HTTPVersion: "http1"}
	req, err := http.NewRequestWithContext(service.WithOpenAIClientProfile(context.Background(), profile), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	response, err := s.Do(req, "", 10, 4)
	require.NoError(t, err)
	defer response.Body.Close()
	first := make([]byte, 5)
	_, err = io.ReadFull(response.Body, first)
	require.NoError(t, err)
	require.Equal(t, "first", string(first))
	changed := *profile
	changed.Preset = "windows"
	_, err = s.getClientEntryWithClientProfile("", 10, 4, service.HTTPUpstreamProfileDefault, &changed, false, true)
	close(continueStream)
	streamReleased = true
	require.NoError(t, err)
	rest, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "second", string(rest))
}

func TestOpenAIClientProfileHTTP2UsesSelectedProtocolAndReusePolicy(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "reuse", true: "fresh"}[fresh], func(t *testing.T) {
			var connections atomic.Int64
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "h2") }))
			server.EnableHTTP2 = true
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					connections.Add(1)
				}
			}
			server.StartTLS()
			defer server.Close()
			p := &service.OpenAIClientProfile{Enabled: true, HTTPVersion: "http2", TLSProfile: "inherit", ConnectionReuse: map[bool]string{false: "reuse", true: "fresh"}[fresh]}
			s := NewHTTPUpstream(nil).(*httpUpstreamService)
			entry, err := s.getClientEntryWithClientProfile("", 20, 4, service.HTTPUpstreamProfileOpenAI, p, false, true)
			require.NoError(t, err)
			transport := entry.client.Transport.(*http.Transport)
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			transport.TLSClientConfig = &tls.Config{RootCAs: roots, NextProtos: []string{"h2", "http/1.1"}}
			defer transport.CloseIdleConnections()
			for i := 0; i < 2; i++ {
				ctx := service.WithHTTPUpstreamProfile(service.WithOpenAIClientProfile(context.Background(), p), service.HTTPUpstreamProfileOpenAI)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				resp, err := s.Do(req, "", 20, 4)
				require.NoError(t, err)
				require.Equal(t, 2, resp.ProtoMajor)
				_, err = io.Copy(io.Discard, resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
			}
			if fresh {
				require.Equal(t, int64(2), connections.Load())
			} else {
				require.Equal(t, int64(1), connections.Load())
			}
		})
	}
}

func TestOpenAIClientProfileTLSBlocksCleartextRedirectBeforeDial(t *testing.T) {
	var cleartextHits atomic.Int64
	cleartext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { cleartextHits.Add(1); w.WriteHeader(200) }))
	defer cleartext.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, cleartext.URL, http.StatusFound) }))
	defer secure.Close()
	p := &service.OpenAIClientProfile{Enabled: true, TLSProfile: "chrome"}
	s := NewHTTPUpstream(nil).(*httpUpstreamService)
	entry, err := s.getClientEntryWithClientProfile("", 30, 1, service.HTTPUpstreamProfileDefault, p, false, true)
	require.NoError(t, err)
	guarded, ok := entry.client.Transport.(*openAIClientSecureTransport)
	require.True(t, ok)
	roots := x509.NewCertPool()
	roots.AddCert(secure.Certificate())
	// Inject only the test CA; keep the same production redirect guard and
	// custom TLS dialer path. No insecure certificate option is used.
	dialTLS, err := tlsfingerprint.BrowserDialTLS("chrome", nil, roots)
	require.NoError(t, err)
	guarded.Transport.DialTLSContext = dialTLS
	defer guarded.CloseIdleConnections()
	req, err := http.NewRequestWithContext(service.WithOpenAIClientProfile(context.Background(), p), http.MethodGet, secure.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer synthetic-test-token")
	_, err = s.Do(req, "", 30, 1)
	require.ErrorContains(t, err, "HTTPS")
	require.Zero(t, cleartextHits.Load(), "a redirect must not bypass the TLS/proxy path")
}
