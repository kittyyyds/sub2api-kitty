package service

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"golang.org/x/net/http2"
)

// GetOpenAIClientProfileHTTPClient retains the existing probe client pool when
// disabled. Active profiles isolate transport instances by account+profile.
func GetOpenAIClientProfileHTTPClient(account *Account, opts httppool.Options) (*http.Client, error) {
	if account == nil {
		return httppool.GetClient(opts)
	}
	return getOpenAIProfileHTTPClient(account.ID, account.GetOpenAIClientProfile(), opts, false)
}

func getOpenAIProfileHTTPClient(accountID int64, profile *OpenAIClientProfile, opts httppool.Options, websocket bool) (*http.Client, error) {
	if profile == nil {
		return httppool.GetClient(opts)
	}
	_, proxyURL, err := proxyurl.Parse(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	copyProfile := *profile
	profile = &copyProfile
	if profile.MaxConnsPerHost > 0 {
		opts.MaxConnsPerHost = profile.MaxConnsPerHost
	}
	if profile.MaxIdleConnsPerHost > 0 {
		opts.MaxIdleConnsPerHost = profile.MaxIdleConnsPerHost
	}
	opts.TransportConfigKey = fmt.Sprintf("openai:%d:%s:ws=%t", accountID, OpenAIClientProfileKey(profile), websocket)
	opts.ConfigureTransport = func(transport *http.Transport) error {
		if profile.IdleConnTimeoutSeconds > 0 {
			transport.IdleConnTimeout = time.Duration(profile.IdleConnTimeoutSeconds) * time.Second
		}
		transport.DisableKeepAlives = profile.ConnectionReuse == "fresh"
		customTLS := profile.TLSProfile != "" && profile.TLSProfile != "inherit"
		// Native WebSocket uses HTTP/1.1 Upgrade; the HTTP selector controls
		// HTTP/SSE requests, not an RFC8441 WebSocket implementation.
		if websocket || customTLS || profile.HTTPVersion == "http1" {
			transport.ForceAttemptHTTP2 = false
			transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
		} else if profile.HTTPVersion == "http2" {
			transport.ForceAttemptHTTP2 = true
			if _, err := http2.ConfigureTransports(transport); err != nil {
				return err
			}
		}
		if customTLS {
			dialer, err := tlsfingerprint.BrowserDialTLS(profile.TLSProfile, proxyURL, nil)
			if err != nil {
				return err
			}
			transport.Proxy = nil
			transport.DialTLSContext = dialer
		}
		return nil
	}
	client, err := httppool.GetClient(opts)
	if err != nil || profile.TLSProfile == "" || profile.TLSProfile == "inherit" {
		return client, err
	}
	copyClient := *client
	copyClient.Transport = &openAIProfileSecureTransport{base: client.Transport}
	return &copyClient, nil
}

type openAIProfileSecureTransport struct{ base http.RoundTripper }

func (t *openAIProfileSecureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil || req.URL.Scheme != "https" {
		return nil, fmt.Errorf("TLS experiment requires an HTTPS upstream URL")
	}
	return t.base.RoundTrip(req)
}
