package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBrowserTLSHelloTemplatesVerifyCertificateAndAdvertiseHTTP1(t *testing.T) {
	for _, template := range []string{"chrome", "firefox", "safari"} {
		t.Run(template, func(t *testing.T) {
			hello := make(chan *tls.ClientHelloInfo, 2)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "HTTP/1.1", r.Proto)
				_, _ = io.WriteString(w, "ok")
			}))
			server.TLS = &tls.Config{GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) { hello <- info; return nil, nil }}
			server.StartTLS()
			defer server.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			dial, err := BrowserDialTLS(template, nil, roots)
			require.NoError(t, err)
			transport := &http.Transport{DialTLSContext: dial, ForceAttemptHTTP2: false}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			response, err := client.Get(server.URL)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, "ok", string(body))
			captured := <-hello
			require.Equal(t, []string{"http/1.1"}, captured.SupportedProtos)
			require.Greater(t, len(captured.CipherSuites), 5)
			require.Contains(t, captured.SupportedVersions, uint16(tls.VersionTLS13))

			untrustedDial, err := BrowserDialTLS(template, nil, nil)
			require.NoError(t, err)
			_, err = untrustedDial(context.Background(), "tcp", server.Listener.Addr().String())
			require.Error(t, err, "an untrusted upstream certificate must fail")
		})
	}
}

func TestBrowserTLSUsesHTTPAndHTTPSProxyWithoutDirectFallback(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "tunneled") }))
	defer target.Close()
	for _, secureProxy := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "https"}[secureProxy], func(t *testing.T) {
			connects := make(chan string, 1)
			proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodConnect {
					http.Error(w, "CONNECT required", 405)
					return
				}
				connects <- r.Header.Get("Proxy-Authorization")
				upstream, err := net.DialTimeout("tcp", r.Host, time.Second)
				if err != nil {
					http.Error(w, "dial failed", 502)
					return
				}
				client, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					_ = upstream.Close()
					return
				}
				_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
				go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
				go func() { _, _ = io.Copy(client, upstream); _ = client.Close() }()
			})
			var proxyServer *httptest.Server
			if secureProxy {
				proxyServer = httptest.NewTLSServer(proxyHandler)
			} else {
				proxyServer = httptest.NewServer(proxyHandler)
			}
			defer proxyServer.Close()
			roots := x509.NewCertPool()
			roots.AddCert(target.Certificate())
			if secureProxy {
				roots.AddCert(proxyServer.Certificate())
			}
			proxyURL, err := url.Parse(proxyServer.URL)
			require.NoError(t, err)
			proxyURL.User = url.UserPassword("test-user", "test-password")
			dial, err := BrowserDialTLS("chrome", proxyURL, roots)
			require.NoError(t, err)
			transport := &http.Transport{DialTLSContext: dial}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			response, err := client.Get(target.URL)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, "tunneled", string(body))
			require.Equal(t, "Basic dGVzdC11c2VyOnRlc3QtcGFzc3dvcmQ=", <-connects)
		})
	}
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "denied", http.StatusProxyAuthRequired) }))
	defer refusing.Close()
	proxyURL, _ := url.Parse(refusing.URL)
	dial, err := BrowserDialTLS("safari", proxyURL, nil)
	require.NoError(t, err)
	_, err = dial(context.Background(), "tcp", target.Listener.Addr().String())
	require.ErrorContains(t, err, "HTTP 407")
}

func TestBrowserTLSRejectsUnsupportedProfileAndProxy(t *testing.T) {
	_, err := BrowserDialTLS("invented", nil, nil)
	require.Error(t, err)
	_, err = BrowserDialTLS("chrome", &url.URL{Scheme: "file", Host: "localhost"}, nil)
	require.Error(t, err)
}

func TestBrowserTLSUsesSOCKSTunnel(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "socks") }))
	defer target.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	finished := make(chan error, 1)
	go func() {
		client, err := listener.Accept()
		if err != nil {
			finished <- err
			return
		}
		defer client.Close()
		_ = client.SetDeadline(time.Now().Add(4 * time.Second))
		header := make([]byte, 2)
		if _, err = io.ReadFull(client, header); err != nil {
			finished <- err
			return
		}
		methods := make([]byte, int(header[1]))
		if _, err = io.ReadFull(client, methods); err != nil {
			finished <- err
			return
		}
		_, _ = client.Write([]byte{5, 0})
		header = make([]byte, 4)
		if _, err = io.ReadFull(client, header); err != nil {
			finished <- err
			return
		}
		var host string
		switch header[3] {
		case 1:
			b := make([]byte, 4)
			_, err = io.ReadFull(client, b)
			host = net.IP(b).String()
		case 3:
			b := make([]byte, 1)
			_, err = io.ReadFull(client, b)
			if err == nil {
				name := make([]byte, int(b[0]))
				_, err = io.ReadFull(client, name)
				host = string(name)
			}
		}
		if err != nil {
			finished <- err
			return
		}
		port := make([]byte, 2)
		if _, err = io.ReadFull(client, port); err != nil {
			finished <- err
			return
		}
		upstream, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port)))), time.Second)
		if err != nil {
			finished <- err
			return
		}
		defer upstream.Close()
		_, _ = client.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
		_, _ = io.Copy(client, upstream)
		finished <- nil
	}()
	roots := x509.NewCertPool()
	roots.AddCert(target.Certificate())
	proxyURL := &url.URL{Scheme: "socks5h", Host: listener.Addr().String()}
	dial, err := BrowserDialTLS("firefox", proxyURL, roots)
	require.NoError(t, err)
	transport := &http.Transport{DialTLSContext: dial, DisableKeepAlives: true}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get(target.URL)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "socks", string(body))
	require.NoError(t, <-finished)
}

func TestBrowserTLSCancellationInterruptsPendingCONNECT(t *testing.T) {
	connected := make(chan struct{})
	stop := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { close(connected); <-stop }))
	defer func() { close(stop); server.Close() }()
	proxyURL, _ := url.Parse(server.URL)
	dial, err := BrowserDialTLS("chrome", proxyURL, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := dial(ctx, "tcp", "example.com:443"); finished <- err }()
	<-connected
	cancel()
	select {
	case err := <-finished:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt CONNECT")
	}
}
