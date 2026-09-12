package tlsfingerprint

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

// BrowserDialTLS is an opt-in TLS hello experiment for HTTP/1.1. The templates
// are library browser hellos, adapted to advertise only HTTP/1.1; they are not
// captures of a Codex client or claims of a particular operating system.
// roots is nil in production (system trust). Tests can supply a private CA;
// certificate and hostname verification are never disabled.
func BrowserDialTLS(template string, proxyURL *url.URL, roots *x509.CertPool) (func(context.Context, string, string) (net.Conn, error), error) {
	if _, err := browserHelloSpec(template); err != nil {
		return nil, err
	}
	if proxyURL != nil {
		switch strings.ToLower(proxyURL.Scheme) {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("unsupported TLS experiment proxy scheme")
		}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		conn, err := browserTunnel(ctx, network, addr, proxyURL, roots)
		if err != nil {
			return nil, err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		spec, err := browserHelloSpec(template)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		secured := utls.UClient(conn, &utls.Config{ServerName: host, RootCAs: roots}, utls.HelloCustom)
		if err = secured.ApplyPreset(spec); err == nil {
			err = secured.HandshakeContext(ctx)
		}
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("TLS experiment handshake: %w", err)
		}
		if alpn := secured.ConnectionState().NegotiatedProtocol; alpn != "" && alpn != "http/1.1" {
			_ = secured.Close()
			return nil, fmt.Errorf("TLS experiment negotiated unsupported protocol %q", alpn)
		}
		return secured, nil
	}, nil
}

func browserHelloSpec(template string) (*utls.ClientHelloSpec, error) {
	var id utls.ClientHelloID
	switch template {
	case "chrome":
		id = utls.HelloChrome_120
	case "firefox":
		id = utls.HelloFirefox_120
	case "safari":
		id = utls.HelloSafari_16_0
	default:
		return nil, fmt.Errorf("unknown TLS experiment template %q", template)
	}
	spec, err := utls.UTLSIdToSpec(id)
	if err != nil {
		return nil, err
	}
	extensions := spec.Extensions[:0]
	for _, extension := range spec.Extensions {
		switch ext := extension.(type) {
		case *utls.ALPNExtension:
			ext.AlpnProtocols = []string{"http/1.1"}
		case *utls.ApplicationSettingsExtension, *utls.ApplicationSettingsExtensionNew:
			continue // ALPS belongs to the removed h2 ALPN offer.
		}
		extensions = append(extensions, extension)
	}
	spec.Extensions = extensions
	return &spec, nil
}

func browserTunnel(ctx context.Context, network, addr string, proxyURL *url.URL, roots *x509.CertPool) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if proxyURL == nil {
		return dialer.DialContext(ctx, network, addr)
	}
	scheme := strings.ToLower(proxyURL.Scheme)
	if scheme == "socks5" || scheme == "socks5h" {
		var auth *proxy.Auth
		if proxyURL.User != nil {
			password, _ := proxyURL.User.Password()
			auth = &proxy.Auth{User: proxyURL.User.Username(), Password: password}
		}
		upstream, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, dialer)
		if err != nil {
			return nil, err
		}
		if contextual, ok := upstream.(proxy.ContextDialer); ok {
			return contextual.DialContext(ctx, network, addr)
		}
		return nil, fmt.Errorf("SOCKS proxy lacks context-aware dialing")
	}
	proxyAddr := proxyURL.Host
	if proxyURL.Port() == "" {
		port := "80"
		if scheme == "https" {
			port = "443"
		}
		proxyAddr = net.JoinHostPort(proxyURL.Hostname(), port)
	}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	connectionToCancel := conn
	stopCancellation := context.AfterFunc(ctx, func() { _ = connectionToCancel.Close() })
	defer stopCancellation()
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()
	if deadline, exists := ctx.Deadline(); exists {
		_ = conn.SetDeadline(deadline)
	}
	if scheme == "https" {
		secured := tls.Client(conn, &tls.Config{ServerName: proxyURL.Hostname(), RootCAs: roots, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}})
		if err = secured.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = secured
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: addr}, Host: addr, Header: make(http.Header)}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username()+":"+password)))
	}
	if err = request.Write(conn); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy CONNECT returned HTTP %d", response.StatusCode)
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	ok = true
	if reader.Buffered() > 0 {
		return &browserBufferedConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type browserBufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *browserBufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
