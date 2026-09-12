package repository

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Enforce TLS on every redirect hop as well as the original request. A custom
// TLS dialer owns proxy tunneling; a cleartext redirect must never use the
// ordinary direct DialContext left on the underlying transport.
type openAIClientSecureTransport struct{ *http.Transport }

func (t *openAIClientSecureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil || req.URL.Scheme != "https" {
		return nil, fmt.Errorf("TLS experiment requires an HTTPS upstream URL")
	}
	return t.Transport.RoundTrip(req)
}

func openAIClientTransportKey(p *service.OpenAIClientProfile) string {
	return service.OpenAIClientProfileKey(p)
}

func applyOpenAIClientTransportSettings(settings poolSettings, protocolMode string, p *service.OpenAIClientProfile) (poolSettings, string) {
	if p.MaxConnsPerHost > 0 {
		settings.maxConnsPerHost = p.MaxConnsPerHost
	}
	if p.MaxIdleConnsPerHost > 0 {
		settings.maxIdleConnsPerHost = p.MaxIdleConnsPerHost
	}
	if p.IdleConnTimeoutSeconds > 0 {
		settings.idleConnTimeout = time.Duration(p.IdleConnTimeoutSeconds) * time.Second
	}
	switch p.HTTPVersion {
	case "http1":
		protocolMode = upstreamProtocolModeOpenAIH1
	case "http2":
		protocolMode = upstreamProtocolModeOpenAIH2
	}
	if p.TLSProfile != "" && p.TLSProfile != "inherit" {
		protocolMode = upstreamProtocolModeOpenAIH1
	}
	return settings, protocolMode
}

func applyOpenAIClientTransport(transport *http.Transport, proxyURL *url.URL, p *service.OpenAIClientProfile) error {
	if p.ConnectionReuse == "fresh" {
		transport.DisableKeepAlives = true
	}
	if p.ConnectionReuse == "reuse" {
		transport.DisableKeepAlives = false
	}
	if p.TLSProfile == "" || p.TLSProfile == "inherit" {
		return nil
	}
	dialTLS, err := tlsfingerprint.BrowserDialTLS(p.TLSProfile, proxyURL, nil)
	if err != nil {
		return err
	}
	// The dialer establishes CONNECT/SOCKS tunnels itself and verifies the
	// proxy and target certificates. Leaving Proxy set would bypass DialTLS.
	transport.Proxy = nil
	transport.DialTLSContext = dialTLS
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	return nil
}
