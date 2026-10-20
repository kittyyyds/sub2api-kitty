package mediawalletapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/mediasession"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const testSubject = "opaque_sub_identity_fixture_42"
const testOrder = "media-order-fixture"

var testHash = strings.Repeat("a", 64)

// Transport-only tests stub customer authorization. The PostgreSQL login
// process test exercises the real authority and source-session revocation.
type transportSessions struct{ SessionAuthority }

func (transportSessions) Exchange(context.Context, string, string, string, string, string) (*mediasession.Grant, error) {
	return nil, mediasession.ErrDenied
}
func (transportSessions) Balance(context.Context, string) (*mediasession.Identity, *service.MediaWalletBalance, error) {
	return nil, nil, mediasession.ErrDenied
}
func (transportSessions) Revoke(context.Context, string) error { return mediasession.ErrDenied }

func (transportSessions) Check(_ context.Context, handle, subject string) (*mediasession.Identity, error) {
	if handle != strings.Repeat("h", 43) || subject != testSubject {
		return nil, mediasession.ErrDenied
	}
	return &mediasession.Identity{Subject: subject}, nil
}

type stubWallet struct {
	call func(string, service.MediaWalletKey, any) (*service.MediaWalletOrder, bool, error)
}

func (s stubWallet) Reserve(_ context.Context, v service.MediaWalletReserve) (*service.MediaWalletOrder, bool, error) {
	return s.call("reserve", v.Key, v)
}
func (s stubWallet) Get(_ context.Context, k service.MediaWalletKey) (*service.MediaWalletOrder, error) {
	o, _, e := s.call("get", k, nil)
	return o, e
}
func (s stubWallet) BeginSubmission(_ context.Context, k service.MediaWalletKey) (*service.MediaWalletOrder, bool, error) {
	return s.call("begin_submission", k, nil)
}
func (s stubWallet) RecordSubmission(_ context.Context, k service.MediaWalletKey, state, id, evidence string) (*service.MediaWalletOrder, error) {
	o, _, e := s.call("record_submission", k, Submission{State: state, UpstreamID: id, EvidenceHash: evidence})
	return o, e
}
func (s stubWallet) Capture(_ context.Context, k service.MediaWalletKey, amount, evidence string) (*service.MediaWalletOrder, error) {
	o, _, e := s.call("capture", k, Settlement{ActualUSD: amount, EvidenceHash: evidence})
	return o, e
}
func (s stubWallet) Release(_ context.Context, k service.MediaWalletKey, evidence string) (*service.MediaWalletOrder, error) {
	o, _, e := s.call("release", k, Settlement{EvidenceHash: evidence})
	return o, e
}

type tlsFixture struct {
	ca             *x509.Certificate
	key            *ecdsa.PrivateKey
	pool           *x509.CertPool
	server, client tls.Certificate
}

func newTLSFixture(t *testing.T) *tlsFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated wallet test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	raw, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	f := &tlsFixture{ca: ca, key: key, pool: x509.NewCertPool()}
	f.pool.AddCert(ca)
	f.server = f.issue(t, true, false)
	f.client = f.issue(t, false, false)
	return f
}

func (f *tlsFixture) issue(t *testing.T, server, expired bool) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	if expired {
		template.NotAfter = time.Now().Add(-time.Minute)
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, f.ca, &key.PublicKey, f.key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{raw, f.ca.Raw}, PrivateKey: key, Leaf: leaf}
}

func (f *tlsFixture) config() Config {
	return Config{Addr: "127.0.0.1:0", ServerCertificate: f.server, ClientCAs: f.pool,
		Sessions:          transportSessions{},
		AllowedClientSPKI: [][32]byte{sha256.Sum256(f.client.Leaf.RawSubjectPublicKeyInfo)},
		ResolveSubject: func(_ context.Context, subject string) (int64, error) {
			if subject != testSubject {
				return 0, ErrSubjectNotFound
			}
			return 42, nil
		}}
}

func startWalletServer(t *testing.T, cfg Config, repo stubWallet) (*httptest.Server, *http.Server) {
	t.Helper()
	srv, err := NewServer(cfg, repo)
	if err != nil {
		t.Fatal(err)
	}
	srv.ErrorLog = log.New(io.Discard, "", 0)
	ts := httptest.NewUnstartedServer(srv.Handler)
	ts.Config = srv
	ts.TLS = srv.TLSConfig.Clone()
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts, srv
}

func (f *tlsFixture) httpClient(cert *tls.Certificate) *http.Client {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: f.pool}
	if cert != nil {
		cfg.Certificates = []tls.Certificate{*cert}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 5 * time.Second}
}

func orderFor(k service.MediaWalletKey) *service.MediaWalletOrder {
	return &service.MediaWalletOrder{Key: k, Fingerprint: testHash, Currency: "USD", HoldUSD: "7.60000001", ActualUSD: "0.00000000",
		ReleasedUSD: "0.00000000", ExpiredUSD: "0.00000000", BillingState: "held", SubmissionState: "reserved",
		Quote: json.RawMessage(`{"sell_usd_credit":"7.60000001"}`), PriceContract: json.RawMessage(`{"schema":"fixture"}`)}
}

func perform(t *testing.T, client *http.Client, url, method, body string, headers map[string]string) (int, Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result Response
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Version != Version || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unsafe response contract")
	}
	return resp.StatusCode, result, string(raw)
}

func encode(t *testing.T, cmd Command) string {
	t.Helper()
	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestWalletTransportAuthentication(t *testing.T) {
	f := newTLSFixture(t)
	repo := stubWallet{call: func(_ string, k service.MediaWalletKey, _ any) (*service.MediaWalletOrder, bool, error) {
		return orderFor(k), false, nil
	}}
	server, srv := startWalletServer(t, f.config(), repo)
	body := encode(t, Command{Version: Version, Operation: "get", Subject: testSubject, OrderID: testOrder})
	for _, tc := range []struct {
		name string
		cert *tls.Certificate
	}{
		{"missing certificate", nil}, {"unpinned certificate", ptrCert(f.issue(t, false, false))},
		{"foreign CA", ptrCert(newTLSFixture(t).client)}, {"expired certificate", ptrCert(f.issue(t, false, true))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := f.httpClient(tc.cert)
			defer client.CloseIdleConnections()
			resp, err := client.Post(server.URL+Path, "application/json", strings.NewReader(body))
			if err == nil {
				resp.Body.Close()
				t.Fatal("unauthorized TLS handshake accepted")
			}
		})
	}
	client := f.httpClient(&f.client)
	defer client.CloseIdleConnections()
	status, _, raw := perform(t, client, server.URL+Path, http.MethodPost, body, nil)
	if status != 200 || strings.Contains(raw, "user_id") {
		t.Fatalf("valid opaque identity failed: %d %s", status, raw)
	}
	for _, header := range []string{"Origin", "Cookie", "Authorization"} {
		status, _, _ = perform(t, client, server.URL+Path, http.MethodPost, body, map[string]string{header: "not-a-service-credential"})
		if status != 403 {
			t.Fatalf("accepted browser credential %s", header)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
	req.Header.Set("X-SSL-Client-Verify", "SUCCESS")
	req.Header.Set("X-Forwarded-Proto", "https")
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal("plaintext forwarded certificate assertions accepted")
	}
}

func ptrCert(cert tls.Certificate) *tls.Certificate { return &cert }

func TestWalletTransportCommandsAndValidation(t *testing.T) {
	f := newTLSFixture(t)
	var gotOp string
	var gotKey service.MediaWalletKey
	var got any
	repo := stubWallet{call: func(op string, k service.MediaWalletKey, v any) (*service.MediaWalletOrder, bool, error) {
		gotOp, gotKey, got = op, k, v
		o := orderFor(k)
		if op == "begin_submission" {
			o.SubmissionState = "submitting"
		}
		return o, op == "reserve" || op == "begin_submission", nil
	}}
	server, _ := startWalletServer(t, f.config(), repo)
	client := f.httpClient(&f.client)
	defer client.CloseIdleConnections()
	base := Command{Version: Version, Subject: testSubject, OrderID: testOrder, Session: strings.Repeat("h", 43), ClientIP: "192.0.2.1", UserAgent: "isolated-test"}
	for _, op := range []string{"reserve", "get", "begin_submission", "record_submission", "capture", "release"} {
		t.Run(op, func(t *testing.T) {
			cmd := base
			cmd.Operation = op
			switch op {
			case "reserve":
				cmd.Reserve = &Reserve{AmountUSD: "7.60000001", RequestHash: testHash, Quote: json.RawMessage(`{"exact":"7.60000001"}`), PriceContract: json.RawMessage(`{"immutable":true}`)}
			case "record_submission":
				cmd.Submission = &Submission{State: "unknown", EvidenceHash: testHash}
			case "capture":
				cmd.Settlement = &Settlement{ActualUSD: "7.60000001", EvidenceHash: testHash}
			case "release":
				cmd.Settlement = &Settlement{EvidenceHash: testHash}
			}
			status, result, _ := perform(t, client, server.URL+Path, http.MethodPost, encode(t, cmd), nil)
			if status != 200 || gotOp != op || gotKey.UserID != 42 || gotKey.OrderID != testOrder || result.Created != (op == "reserve") || result.SubmissionGranted != (op == "begin_submission") {
				t.Fatalf("wrong dispatch: %d %#v", status, result)
			}
			if op == "reserve" {
				v := got.(service.MediaWalletReserve)
				if v.AmountUSD != "7.60000001" || string(v.Quote) != string(cmd.Reserve.Quote) {
					t.Fatal("price changed in transport")
				}
			}
			if op == "capture" && got.(Settlement).ActualUSD != "7.60000001" {
				t.Fatal("settlement lost decimal precision")
			}
		})
	}
	base.Operation = "get"
	valid := encode(t, base)
	for _, tc := range []struct{ name, body string }{
		{"unknown raw user ID", strings.TrimSuffix(valid, "}") + `,"user_id":99}`},
		{"unknown nested field", strings.Replace(valid, `"operation":"get"`, `"operation":"reserve","reserve":{"amount_usd":"1","user_id":99}`, 1)},
		{"wrong version", strings.Replace(valid, Version, "unsupported", 1)},
		{"email identity", strings.Replace(valid, testSubject, "someone@example.com", 1)},
		{"raw user ID identity", strings.Replace(valid, testSubject, "42", 1)},
		{"invalid operation", strings.Replace(valid, `"get"`, `"refund_all"`, 1)},
		{"trailing object", valid + `{}`}, {"trailing garbage", valid + `oops`},
		{"mixed operation fields", strings.TrimSuffix(valid, "}") + `,"settlement":{"actual_usd":"1","evidence_hash":"` + testHash + `"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotOp = ""
			status, _, _ := perform(t, client, server.URL+Path, http.MethodPost, tc.body, nil)
			if status != 400 || gotOp != "" {
				t.Fatalf("invalid command reached repository: %d", status)
			}
		})
	}
	for _, amount := range []string{"-1", "1000000", "0.000000001", "1e2"} {
		cmd := base
		cmd.Operation = "capture"
		cmd.Settlement = &Settlement{ActualUSD: amount, EvidenceHash: testHash}
		status, _, _ := perform(t, client, server.URL+Path, http.MethodPost, encode(t, cmd), nil)
		if status != 400 {
			t.Fatalf("accepted invalid amount %s", amount)
		}
	}
	for _, tc := range []struct {
		url, method, body string
		headers           map[string]string
		want              int
	}{
		{server.URL + Path + "?user_id=99", "POST", valid, nil, 404}, {server.URL + "/public", "POST", valid, nil, 404},
		{server.URL + Path, "GET", valid, nil, 405}, {server.URL + Path, "POST", valid, map[string]string{"Content-Type": "text/plain"}, 415},
		{server.URL + Path, "POST", strings.Repeat(" ", maxBody) + valid, nil, 413},
		{server.URL + Path, "POST", valid + strings.Repeat(" ", maxBody), nil, 413},
	} {
		status, _, _ := perform(t, client, tc.url, tc.method, tc.body, tc.headers)
		if status != tc.want {
			t.Fatalf("boundary returned %d, want %d", status, tc.want)
		}
	}
}

func TestWalletTransportWithoutSessionAuthorityCannotSpend(t *testing.T) {
	f := newTLSFixture(t)
	cfg := f.config()
	cfg.Sessions = nil
	called := false
	server, _ := startWalletServer(t, cfg, stubWallet{call: func(_ string, k service.MediaWalletKey, _ any) (*service.MediaWalletOrder, bool, error) {
		called = true
		return orderFor(k), false, nil
	}})
	client := f.httpClient(&f.client)
	defer client.CloseIdleConnections()
	cmd := Command{Version: Version, Subject: testSubject, OrderID: testOrder, Operation: "begin_submission"}
	status, _, _ := perform(t, client, server.URL+Path, http.MethodPost, encode(t, cmd), nil)
	if status != 401 || called {
		t.Fatal("missing authority must disable new spending")
	}
	cmd.Operation = "get"
	status, _, _ = perform(t, client, server.URL+Path, http.MethodPost, encode(t, cmd), nil)
	if status != 200 || !called {
		t.Fatal("recovery must remain available")
	}
}

func TestMediaSessionBackchannelRejectsBrowserAndIdentityInjection(t *testing.T) {
	f := newTLSFixture(t)
	server, _ := startWalletServer(t, f.config(), stubWallet{})
	client := f.httpClient(&f.client)
	defer client.CloseIdleConnections()
	body := `{"version":"` + Version + `","operation":"balance","handle":"` + strings.Repeat("h", 43) + `","client_ip":"192.0.2.1","user_agent":"test"}`
	for _, header := range []string{"Origin", "Cookie", "Authorization"} {
		status, _, _ := perform(t, client, server.URL+SessionPath, http.MethodPost, body, map[string]string{header: "browser-credential"})
		if status != 403 {
			t.Fatal("browser credential accepted by login backchannel")
		}
	}
	for _, invalid := range []string{body + ` {}`, strings.TrimSuffix(body, "}") + `,"user_id":1}`, strings.Replace(body, `"192.0.2.1"`, `"not-an-IP"`, 1), strings.Replace(body, `"balance"`, `"unknown-operation"`, 1)} {
		status, _, _ := perform(t, client, server.URL+SessionPath, http.MethodPost, invalid, nil)
		if status != 400 {
			t.Fatalf("unsafe login request accepted: %d", status)
		}
	}
	status, _, _ := perform(t, client, server.URL+SessionPath, http.MethodPost, body, nil)
	if status != 401 {
		t.Fatal("denied source session accepted")
	}
}

func TestWalletTransportErrorsAndConcurrency(t *testing.T) {
	f := newTLSFixture(t)
	var repoErr error
	repo := stubWallet{call: func(_ string, k service.MediaWalletKey, _ any) (*service.MediaWalletOrder, bool, error) {
		return orderFor(k), false, repoErr
	}}
	server, _ := startWalletServer(t, f.config(), repo)
	client := f.httpClient(&f.client)
	defer client.CloseIdleConnections()
	cmd := Command{Version: Version, Operation: "get", Subject: testSubject, OrderID: testOrder}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{sql.ErrNoRows, 404, "not_found"}, {service.ErrMediaWalletConflict, 409, "order_conflict"}, {service.ErrMediaWalletState, 409, "invalid_order_state"},
		{service.ErrBatchImageInsufficientBalance, 402, "insufficient_balance"}, {service.ErrBatchImageSettlementCostExceedsHold, 422, "actual_exceeds_hold"},
		{errors.New("database password secret must never leak"), 503, "wallet_unavailable"},
	} {
		repoErr = tc.err
		status, result, raw := perform(t, client, server.URL+Path, http.MethodPost, encode(t, cmd), nil)
		if status != tc.status || result.Error != tc.code || result.Order != nil || strings.Contains(raw, "secret") {
			t.Fatalf("unsafe error: %d %s", status, raw)
		}
	}
	cmd.Subject = "opaque_sub_unknown_identity_99"
	status, _, _ := perform(t, client, server.URL+Path, http.MethodPost, encode(t, cmd), nil)
	if status != 404 {
		t.Fatal("unknown mapping accepted")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	cfg := f.config()
	cfg.MaxConcurrent = 1
	blocking := stubWallet{call: func(_ string, k service.MediaWalletKey, _ any) (*service.MediaWalletOrder, bool, error) {
		close(entered)
		<-release
		return orderFor(k), false, nil
	}}
	limited, _ := startWalletServer(t, cfg, blocking)
	cmd.Subject = testSubject
	body := encode(t, cmd)
	done := make(chan error, 1)
	go func() {
		resp, err := client.Post(limited.URL+Path, "application/json", strings.NewReader(body))
		if err == nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first request did not start")
	}
	status, _, _ = perform(t, client, limited.URL+Path, http.MethodPost, body, nil)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if status != 429 {
		t.Fatal("concurrent requests not bounded")
	}
}

func TestWalletTransportRejectsIncompleteConfiguration(t *testing.T) {
	f := newTLSFixture(t)
	repo := stubWallet{}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.ResolveSubject = nil }, func(c *Config) { c.ClientCAs = nil }, func(c *Config) { c.AllowedClientSPKI = nil },
		func(c *Config) { c.ServerCertificate = tls.Certificate{} }, func(c *Config) { c.Addr = "" }, func(c *Config) { c.MaxConcurrent = 65 },
	} {
		cfg := f.config()
		mutate(&cfg)
		if _, err := NewServer(cfg, repo); err == nil {
			t.Fatal("incomplete configuration accepted")
		}
	}
}

func TestWalletTransportNewAPIClient(t *testing.T) {
	probe := os.Getenv("MEDIA_WALLET_PROBE_EXE")
	if probe == "" {
		t.Skip("set MEDIA_WALLET_PROBE_EXE for cross-process New API client verification")
	}
	f := newTLSFixture(t)
	repo := stubWallet{call: func(op string, k service.MediaWalletKey, _ any) (*service.MediaWalletOrder, bool, error) {
		o := orderFor(k)
		if op == "begin_submission" {
			o.SubmissionState = "submitting"
		}
		if op == "capture" {
			o.SubmissionState, o.BillingState, o.ActualUSD = "accepted", "captured", o.HoldUSD
		}
		if op == "release" {
			o.BillingState, o.ReleasedUSD = "released", o.HoldUSD
		}
		if op == "record_submission" {
			o.SubmissionState, o.UpstreamID, o.EvidenceHash = "accepted", "provider-task", testHash
		}
		return o, op == "reserve" || op == "begin_submission", nil
	}}
	server, _ := startWalletServer(t, f.config(), repo)
	dir := t.TempDir()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.ca.Raw})
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.client.Certificate[0]})
	key, err := x509.MarshalPKCS8PrivateKey(f.client.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"ca.pem": ca, "client.pem": cert, "client-key.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})} {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, op := range []string{"get", "begin_submission", "capture", "record_submission", "reserve", "release"} {
		command := Command{Version: Version, Operation: op, Subject: testSubject, OrderID: testOrder, Session: strings.Repeat("h", 43), ClientIP: "192.0.2.1", UserAgent: "isolated-test"}
		switch op {
		case "reserve":
			command.Reserve = &Reserve{AmountUSD: "7.60000001", RequestHash: testHash, Quote: json.RawMessage(`{"test":true}`), PriceContract: json.RawMessage(`{"test":true}`)}
		case "record_submission":
			command.Submission = &Submission{State: "accepted", UpstreamID: "provider-task", EvidenceHash: testHash}
		case "capture":
			command.Settlement = &Settlement{ActualUSD: "7.60000001", EvidenceHash: testHash}
		case "release":
			command.Settlement = &Settlement{EvidenceHash: testHash}
		}
		process := exec.CommandContext(ctx, probe, "-base-url", server.URL, "-ca", filepath.Join(dir, "ca.pem"), "-cert", filepath.Join(dir, "client.pem"), "-key", filepath.Join(dir, "client-key.pem"))
		process.Stdin = bytes.NewBufferString(encode(t, command))
		out, err := process.CombinedOutput()
		if err != nil {
			t.Fatalf("real New API client %s: %v %s", op, err, out)
		}
		var result Response
		if json.Unmarshal(out, &result) != nil || result.Order == nil || result.Order.Subject != testSubject || result.Order.HoldUSD != "7.60000001" {
			t.Fatalf("invalid cross-process receipt: %s", out)
		}
	}
}
