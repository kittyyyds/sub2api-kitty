//go:build unit

package handler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// This transport is only a tripwire. Responses HTTP fallback must not silently
// hide a repeated inference after the real upstream WS produced partial output.
type wsPartialSettlementHTTPTripwire struct{ calls atomic.Int32 }

func (t *wsPartialSettlementHTTPTripwire) Do(*http.Request, string, int64, int) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("unexpected HTTP fallback in partial-settlement fixture")
}
func (t *wsPartialSettlementHTTPTripwire) DoWithTLS(r *http.Request, p string, a int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return t.Do(r, p, a, n)
}

func wsPartialSettlementTLS(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic settlement loopback"}, DNSNames: []string{"chatgpt.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cert, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	require.NoError(t, err)
	caPath := filepath.Join(t.TempDir(), "settlement-ca.pem")
	require.NoError(t, os.WriteFile(caPath, certPEM, 0600))
	t.Setenv("CODEX_CA_CERTIFICATE", caPath)
	t.Setenv("SSL_CERT_FILE", "")
	return cert
}

// The production OAuth URL is unchanged. The explicit account proxy accepts
// only that CONNECT target and routes it to the test's loopback TLS server.
func wsPartialSettlementProxy(t *testing.T, target string) (*service.Proxy, *atomic.Int32) {
	t.Helper()
	connects := &atomic.Int32{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "chatgpt.com:443" {
			http.Error(w, "unexpected synthetic proxy destination", http.StatusForbidden)
			return
		}
		connects.Add(1)
		remote, err := net.DialTimeout("tcp", target, 2*time.Second)
		if err != nil {
			http.Error(w, "loopback unavailable", 502)
			return
		}
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = remote.Close()
			return
		}
		defer client.Close()
		defer remote.Close()
		if _, err = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(remote, buffer); _ = remote.Close(); close(done) }()
		_, _ = io.Copy(client, remote)
		_ = client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	u, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return &service.Proxy{ID: 991, Protocol: "http", Host: u.Hostname(), Port: port, Status: service.StatusActive}, connects
}
