//go:build media_postgres

package repository

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/mediasession"
	"github.com/Wei-Shaw/sub2api/internal/mediawalletapi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type mediaSSOUserReader struct {
	service.UserRepository
	db *sql.DB
}

// Used only by the original transport/accounting process test. The source-login
// process test below instead uses AuthService and the real Redis authority.
type walletTransportSessions struct {
	mediawalletapi.SessionAuthority
}

func (walletTransportSessions) Check(_ context.Context, handle, subject string) (*mediasession.Identity, error) {
	if handle != strings.Repeat("h", 43) || len(subject) != 43 {
		return nil, mediasession.ErrDenied
	}
	return &mediasession.Identity{Subject: subject}, nil
}

func (r mediaSSOUserReader) GetByID(ctx context.Context, id int64) (*service.User, error) {
	u := &service.User{}
	err := r.db.QueryRowContext(ctx, `SELECT id,email,password_hash,status FROM users WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Status)
	if err == sql.ErrNoRows {
		return nil, service.ErrUserNotFound
	}
	return u, err
}

func TestMediaWalletPostgresSourceLoginNewAPIProcessAndExactBalance(t *testing.T) {
	probe := os.Getenv("MEDIA_WALLET_PROBE_EXE")
	require.NotEmpty(t, probe, "login verification requires a real New API process")
	addr := os.Getenv("MEDIA_SSO_TEST_REDIS_ADDR")
	host, _, err := net.SplitHostPort(addr)
	require.NoError(t, err, "use only the isolated test Redis listener")
	require.Contains(t, []string{"127.0.0.1", "::1"}, host)
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15, MaxRetries: -1})
	defer rdb.Close()
	require.NoError(t, rdb.Ping(context.Background()).Err())
	db, wallet := newMediaWalletFixture(t)
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT 'media-test@example.invalid'; ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT 'original-hash';
	 INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at) VALUES(1,1,2,2,NOW()-INTERVAL '1 minute'),(1,2,3,3,NULL)`)
	require.NoError(t, err)
	reader := mediaSSOUserReader{db: db}
	user, err := reader.GetByID(context.Background(), 1)
	require.NoError(t, err)
	cfg := &config.Config{}
	cfg.JWT.Secret = "isolated-sso-process-test-secret-not-used-in-production"
	cfg.JWT.AccessTokenExpireMinutes = 5
	cfg.JWT.RefreshTokenExpireDays = 1
	auth := service.NewAuthService(nil, reader, nil, NewRefreshTokenCache(rdb), cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	ctx := service.WithSessionBinding(context.Background(), &service.SessionBinding{IP: "192.0.2.1", UserAgent: "isolated-browser-test"})
	pair, err := auth.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	ids := NewMediaWalletIdentityRepository(db)
	settings := mediasession.Config{Issuer: "https://sub.example", Audience: "kitty-media", CallbackURL: "https://media.example/media-login/callback"}
	authority, err := mediasession.New(settings, rdb, auth, ids, NewMediaWalletBalanceReader(db))
	require.NoError(t, err)
	proof, state := strings.Repeat("p", 43), strings.Repeat("t", 43)
	challenge := sha256.Sum256([]byte(proof))
	code, err := authority.Issue(ctx, pair.AccessToken, base64.RawURLEncoding.EncodeToString(challenge[:]), state)
	require.NoError(t, err)
	subject, err := ids.EnsureSubject(ctx, 1)
	require.NoError(t, err)
	before, err := NewMediaWalletBalanceReader(db).Balance(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "8.00000000", before.AvailableUSD)
	require.Equal(t, "3.00000000", before.BonusAvailableUSD)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(raw)
	require.NoError(t, err)
	cert := tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key, Leaf: leaf}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	srv, err := mediawalletapi.NewServer(mediawalletapi.Config{Addr: "127.0.0.1:0", ServerCertificate: cert, ClientCAs: pool, AllowedClientSPKI: [][32]byte{sha256.Sum256(leaf.RawSubjectPublicKeyInfo)}, ResolveSubject: ids.ResolveSubject, Sessions: authority}, wallet)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(srv.Handler)
	server.Config = srv
	server.TLS = srv.TLSConfig.Clone()
	server.StartTLS()
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{cert}}}}
	defer client.CloseIdleConnections()
	post := func(path string, input any, output any) int {
		t.Helper()
		body, err := json.Marshal(input)
		require.NoError(t, err)
		response, err := client.Post(server.URL+path, "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer response.Body.Close()
		payload, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(payload, output))
		return response.StatusCode
	}
	request := mediaReserveFixture(t, "sso-process-image-fixture", "0.10")
	reserve := mediawalletapi.Command{Version: mediawalletapi.Version, Subject: subject, Operation: "reserve", OrderID: request.Key.OrderID,
		Reserve: &mediawalletapi.Reserve{AmountUSD: request.AmountUSD, RequestHash: request.RequestHash, Quote: request.Quote, PriceContract: request.PriceContract}}
	var denied mediawalletapi.Response
	require.Equal(t, 401, post(mediawalletapi.Path, reserve, &denied), "mTLS alone cannot authorize new spending")
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
	dir := t.TempDir()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
	private, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600))
	input, err := json.Marshal(map[string]any{"issuer": settings.Issuer, "audience": settings.Audience, "callback_url": settings.CallbackURL, "code": code, "verifier": proof, "state": state,
		"binding": map[string]string{"client_ip": "192.0.2.1", "user_agent": "isolated-browser-test"}, "order": reserve})
	require.NoError(t, err)
	process := exec.CommandContext(ctx, probe, "-mode", "login", "-base-url", server.URL, "-ca", filepath.Join(dir, "cert.pem"), "-cert", filepath.Join(dir, "cert.pem"), "-key", filepath.Join(dir, "key.pem"))
	process.Stdin = bytes.NewReader(input)
	out, err := process.CombinedOutput()
	require.NoError(t, err, "login probe: %s", out)
	var result struct {
		Identity mediasession.Identity      `json:"identity"`
		Balance  service.MediaWalletBalance `json:"balance"`
		Order    mediawalletapi.Response    `json:"order"`
	}
	require.NoError(t, json.Unmarshal(out, &result))
	require.Equal(t, subject, result.Identity.Subject)
	require.Equal(t, "7.90000000", result.Balance.AvailableUSD)
	require.Equal(t, "0.10000000", result.Balance.FrozenUSD)
	require.Equal(t, "2.90000000", result.Balance.BonusAvailableUSD)
	require.True(t, result.Order.Created)
	assertMediaBalance(t, db, "7.90000000", "0.10000000")
	_, err = authority.Exchange(ctx, code, proof, state, settings.Audience, settings.CallbackURL)
	require.ErrorIs(t, err, mediasession.ErrDenied)
	code, err = authority.Issue(ctx, pair.AccessToken, base64.RawURLEncoding.EncodeToString(challenge[:]), state)
	require.NoError(t, err)
	g, err := authority.Exchange(ctx, code, proof, state, settings.Audience, settings.CallbackURL)
	require.NoError(t, err)
	reserve.Session = g.Handle
	reserve.ClientIP = "192.0.2.1"
	reserve.UserAgent = "isolated-browser-test"
	reserve.OrderID = "cross-subject-order"
	reserve.Subject, err = ids.EnsureSubject(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, 401, post(mediawalletapi.Path, reserve, &denied), "cannot spend from another subject")
	_, err = db.Exec(`UPDATE users SET password_hash='changed-hash' WHERE id=1`)
	require.NoError(t, err)
	check := mediawalletapi.SessionCommand{Version: mediawalletapi.Version, Operation: "balance", Handle: g.Handle, ClientIP: "192.0.2.1", UserAgent: "isolated-browser-test"}
	var sessionResponse mediawalletapi.SessionResponse
	require.Equal(t, 401, post(mediawalletapi.SessionPath, check, &sessionResponse))
	// Recovery must not strand an old reservation when its source login expires.
	release := mediawalletapi.Command{Version: mediawalletapi.Version, Subject: subject, Operation: "release", OrderID: request.Key.OrderID, Settlement: &mediawalletapi.Settlement{EvidenceHash: mediaEvidence("source revoked before submission")}}
	var refund mediawalletapi.Response
	require.Equal(t, 200, post(mediawalletapi.Path, release, &refund))
	require.Equal(t, "released", refund.Order.BillingState)
	assertMediaBalance(t, db, "8.00000000", "0.00000000")
	_, err = db.Exec(`UPDATE users SET password_hash='original-hash' WHERE id=1`)
	require.NoError(t, err)
	require.NoError(t, auth.RevokeRefreshToken(ctx, pair.RefreshToken))
	require.Equal(t, 401, post(mediawalletapi.SessionPath, check, &sessionResponse), "stale family index after logout must not keep media login alive")
}

// Refuse production database names and non-loopback endpoints. The runner
// supplies a disposable PostgreSQL container, never the live database.
func newMediaWalletFixture(t *testing.T) (*sql.DB, service.MediaWalletRepository) {
	t.Helper()
	parsed, err := url.Parse(os.Getenv("MEDIA_WALLET_TEST_DATABASE_URL"))
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname())
	require.Equal(t, "/kitty_media_wallet_test", parsed.Path)
	admin, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	require.NoError(t, admin.Ping())
	schema := fmt.Sprintf("media_wallet_test_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema+",pg_catalog")
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(24)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		_, e := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE")
		require.NoError(t, e)
		require.NoError(t, admin.Close())
	})
	_, err = db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY,balance NUMERIC(20,8) NOT NULL,frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0,status TEXT NOT NULL DEFAULT 'active',deleted_at TIMESTAMPTZ,updated_at TIMESTAMPTZ);
	 INSERT INTO users(id,balance) VALUES(1,10),(2,10);
	 CREATE TABLE payment_orders(id BIGINT PRIMARY KEY); INSERT INTO payment_orders VALUES(1),(2);`)
	require.NoError(t, err)
	for _, name := range []string{"182_recharge_bonus_credits.sql", "254_recharge_bonus_permanent.sql", "286_kitty_media_wallet.sql", "287_kitty_media_identities.sql"} {
		raw, e := os.ReadFile("../../migrations/" + name)
		require.NoError(t, e)
		_, e = db.Exec(string(raw))
		require.NoError(t, e, name)
	}
	return db, NewMediaWalletRepository(db)
}

func TestMediaWalletPostgresPersistentOpaqueIdentities(t *testing.T) {
	db, wallet := newMediaWalletFixture(t)
	ctx := context.Background()
	identities := NewMediaWalletIdentityRepository(db)
	var wg sync.WaitGroup
	results := make(chan string, 20)
	for range 20 {
		wg.Go(func() {
			subject, err := identities.EnsureSubject(ctx, 1)
			if err != nil {
				results <- "ERROR:" + err.Error()
				return
			}
			results <- subject
		})
	}
	wg.Wait()
	close(results)
	var first string
	for subject := range results {
		require.Len(t, subject, 43)
		if first == "" {
			first = subject
		}
		require.Equal(t, first, subject)
	}
	second, err := identities.EnsureSubject(ctx, 2)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	for range 2 {
		raw, err := os.ReadFile("../../migrations/287_kitty_media_identities.sql")
		require.NoError(t, err)
		_, err = db.Exec(string(raw))
		require.NoError(t, err)
	}
	identities = NewMediaWalletIdentityRepository(db)
	stable, err := identities.EnsureSubject(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, first, stable)
	id, err := identities.ResolveSubject(ctx, first)
	require.NoError(t, err)
	require.EqualValues(t, 1, id)
	for _, invalid := range []string{"1", "user@example.com", first[:42], second + "x", string(make([]byte, 43))} {
		_, err := identities.ResolveSubject(ctx, invalid)
		require.ErrorIs(t, err, service.ErrMediaWalletSubjectNotFound)
	}
	_, err = identities.EnsureSubject(ctx, 99)
	require.ErrorIs(t, err, service.ErrMediaWalletSubjectNotFound)
	cmd := mediaReserveFixture(t, "identity-refund", "1")
	_, _, err = wallet.Reserve(ctx, cmd)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE users SET status='disabled',deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
	_, err = identities.EnsureSubject(ctx, 1)
	require.ErrorIs(t, err, service.ErrMediaWalletSubjectNotFound)
	id, err = identities.ResolveSubject(ctx, first)
	require.NoError(t, err)
	require.EqualValues(t, 1, id)
	_, err = wallet.Release(ctx, service.MediaWalletKey{UserID: id, OrderID: cmd.Key.OrderID}, mediaEvidence("disabled refund"))
	require.NoError(t, err)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
	cmd.Key.OrderID = "disabled-new-order"
	_, _, err = wallet.Reserve(ctx, cmd)
	require.Error(t, err)
	_, err = db.Exec(`DELETE FROM users WHERE id=2`)
	require.Error(t, err, "identity must not silently disappear or rebind")
}

func TestMediaWalletPostgresAuthenticatedNewAPIProcess(t *testing.T) {
	probe := os.Getenv("MEDIA_WALLET_PROBE_EXE")
	if probe == "" {
		t.Skip("MEDIA_WALLET_PROBE_EXE is required for cross-process wallet verification")
	}
	db, wallet := newMediaWalletFixture(t)
	identities := NewMediaWalletIdentityRepository(db)
	ctx := context.Background()
	subject, err := identities.EnsureSubject(ctx, 1)
	require.NoError(t, err)
	// Ephemeral test PKI only; production requires separate service/server keys.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(raw)
	require.NoError(t, err)
	cert := tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key, Leaf: leaf}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	srv, err := mediawalletapi.NewServer(mediawalletapi.Config{Addr: "127.0.0.1:0", ServerCertificate: cert, ClientCAs: pool,
		AllowedClientSPKI: [][32]byte{sha256.Sum256(leaf.RawSubjectPublicKeyInfo)}, ResolveSubject: identities.ResolveSubject, Sessions: walletTransportSessions{}}, wallet)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(srv.Handler)
	server.Config = srv
	server.TLS = srv.TLSConfig.Clone()
	server.StartTLS()
	defer server.Close()
	dir := t.TempDir()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
	private, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600))
	call := func(command mediawalletapi.Command) mediawalletapi.Response {
		t.Helper()
		command.Version = mediawalletapi.Version
		command.Subject = subject
		command.Session = strings.Repeat("h", 43)
		command.ClientIP = "192.0.2.1"
		command.UserAgent = "isolated-test"
		body, err := json.Marshal(command)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		process := exec.CommandContext(ctx, probe, "-base-url", server.URL, "-ca", filepath.Join(dir, "cert.pem"), "-cert", filepath.Join(dir, "cert.pem"), "-key", filepath.Join(dir, "key.pem"))
		process.Stdin = bytes.NewReader(body)
		out, err := process.CombinedOutput()
		require.NoError(t, err, "New API process: %s", out)
		var response mediawalletapi.Response
		require.NoError(t, json.Unmarshal(out, &response))
		require.NotNil(t, response.Order)
		require.Equal(t, subject, response.Order.Subject)
		require.Equal(t, command.OrderID, response.Order.OrderID)
		return response
	}
	request := mediaReserveFixture(t, "authenticated-process", "0.10")
	if path := os.Getenv("MEDIA_WALLET_TEST_QUOTE_FIXTURE"); path != "" {
		fixture, err := os.ReadFile(path)
		require.NoError(t, err)
		var price struct {
			Quote    json.RawMessage `json:"quote"`
			Contract json.RawMessage `json:"price_contract"`
		}
		require.NoError(t, json.Unmarshal(fixture, &price))
		request.Quote = price.Quote
		request.PriceContract = price.Contract
	}
	reserve := mediawalletapi.Command{Operation: "reserve", OrderID: request.Key.OrderID, Reserve: &mediawalletapi.Reserve{AmountUSD: request.AmountUSD, RequestHash: request.RequestHash, Quote: request.Quote, PriceContract: request.PriceContract}}
	require.True(t, call(reserve).Created)
	require.False(t, call(reserve).Created)
	assertMediaBalance(t, db, "9.90000000", "0.10000000")
	begin := mediawalletapi.Command{Operation: "begin_submission", OrderID: request.Key.OrderID}
	require.True(t, call(begin).SubmissionGranted)
	require.False(t, call(begin).SubmissionGranted)
	accepted := mediawalletapi.Command{Operation: "record_submission", OrderID: request.Key.OrderID, Submission: &mediawalletapi.Submission{State: "accepted", UpstreamID: "isolated-provider-fixture", EvidenceHash: mediaEvidence("authenticated accept")}}
	require.Equal(t, "accepted", call(accepted).Order.SubmissionState)
	capture := mediawalletapi.Command{Operation: "capture", OrderID: request.Key.OrderID, Settlement: &mediawalletapi.Settlement{ActualUSD: "0.10", EvidenceHash: mediaEvidence("authenticated capture")}}
	for range 2 {
		result := call(capture)
		require.Equal(t, "captured", result.Order.BillingState)
		require.Equal(t, "0.10000000", result.Order.ActualUSD)
	}
	assertMediaBalance(t, db, "9.90000000", "0.00000000")
	reserve.OrderID = "authenticated-refund"
	require.True(t, call(reserve).Created)
	_, err = db.Exec(`UPDATE users SET status='disabled' WHERE id=1`)
	require.NoError(t, err)
	release := mediawalletapi.Command{Operation: "release", OrderID: reserve.OrderID, Settlement: &mediawalletapi.Settlement{EvidenceHash: mediaEvidence("authenticated disabled-user refund")}}
	for range 2 {
		require.Equal(t, "released", call(release).Order.BillingState)
	}
	assertMediaBalance(t, db, "9.90000000", "0.00000000")
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM kitty_media_wallet_ledger`).Scan(&count))
	require.Equal(t, 4, count)
}

func mediaEvidence(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func TestMediaWalletPostgresTextAdmissionIgnoresStaleRedis(t *testing.T) {
	db, wallet := newMediaWalletFixture(t)
	ctx := context.Background()
	addr := os.Getenv("MEDIA_SSO_TEST_REDIS_ADDR")
	host, _, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "::1"}, host)
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15, MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	require.NoError(t, rdb.Ping(ctx).Err())
	cache := NewBillingCache(rdb)
	t.Cleanup(func() { require.NoError(t, cache.InvalidateUserBalance(ctx, 1)) })
	newBilling := func() *service.BillingCacheService {
		s := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
		require.NoError(t, s.RequireMediaWalletBalanceAuthority(NewMediaWalletBalanceReader(db)))
		t.Cleanup(s.Stop)
		return s
	}
	billing := newBilling()
	// Existing entries and late async fills deliberately disagree with SQL.
	require.NoError(t, cache.SetUserBalance(ctx, 1, 100))
	balance, err := billing.GetUserBalance(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, float64(10), balance)
	cmd := mediaReserveFixture(t, "text-admission-hold", "10")
	_, _, err = wallet.Reserve(ctx, cmd)
	require.NoError(t, err)
	require.NoError(t, cache.SetUserBalance(ctx, 1, 100))
	billing.Stop()
	billing = newBilling() // A process restart cannot restore stale admission.
	require.ErrorIs(t, billing.CheckBillingEligibility(ctx, &service.User{ID: 1}, nil, nil, nil, ""), service.ErrInsufficientBalance)
	_, err = wallet.Release(ctx, cmd.Key, mediaEvidence("cancel before submission"))
	require.NoError(t, err)
	require.NoError(t, cache.SetUserBalance(ctx, 1, 0))
	require.NoError(t, billing.CheckBillingEligibility(ctx, &service.User{ID: 1}, nil, nil, nil, ""))
	_, err = db.Exec(`INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at) VALUES(1,1,10,10,NOW()-INTERVAL '1 second')`)
	require.NoError(t, err)
	require.ErrorIs(t, billing.CheckBillingEligibility(ctx, &service.User{ID: 1}, nil, nil, nil, ""), service.ErrInsufficientBalance)
	// Eligibility reads never expire/reissue grants or mutate stored balances.
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
	_, err = db.Exec(`UPDATE users SET status='disabled' WHERE id=1`)
	require.NoError(t, err)
	require.ErrorIs(t, billing.CheckBillingEligibility(ctx, &service.User{ID: 1}, nil, nil, nil, ""), service.ErrBillingServiceUnavailable)
}

func TestMediaWalletPostgresConcurrentTextAndMediaConserveBalance(t *testing.T) {
	db, wallet := newMediaWalletFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := mediaReserveFixture(t, "concurrent-text-media", "6")
	start := make(chan struct{})
	errors := make(chan error, 2)
	go func() {
		<-start
		_, _, err := wallet.Reserve(ctx, cmd)
		errors <- err
	}()
	go func() {
		<-start
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			errors <- err
			return
		}
		defer tx.Rollback()
		text := &usageBillingRepository{db: db, bonus: newRechargeBonusRepository(db)}
		_, _, _, err = text.deductUsageBillingBalance(ctx, tx, 1, "completed-text-request", 4)
		if err == nil {
			err = tx.Commit()
		}
		errors <- err
	}()
	close(start)
	for range 2 {
		require.NoError(t, <-errors)
	}
	assertMediaBalance(t, db, "0.00000000", "6.00000000")
	acceptMedia(t, wallet, cmd.Key)
	_, err := wallet.Capture(ctx, cmd.Key, "5", mediaEvidence("completed-media-output"))
	require.NoError(t, err)
	assertMediaBalance(t, db, "1.00000000", "0.00000000")
	// Native text settlement still charges already-delivered usage even when
	// concurrent spending has exhausted admission; do not silently drop usage.
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	text := &usageBillingRepository{db: db, bonus: newRechargeBonusRepository(db)}
	remaining, sufficient, _, err := text.deductUsageBillingBalance(ctx, tx, 1, "inflight-text-request", 2)
	require.NoError(t, err)
	require.False(t, sufficient)
	require.Equal(t, float64(-1), remaining)
	require.NoError(t, tx.Commit())
	assertMediaBalance(t, db, "-1.00000000", "0.00000000")
	_, _, err = wallet.Reserve(ctx, mediaReserveFixture(t, "after-text-overdraft", "0.01"))
	require.ErrorIs(t, err, service.ErrBatchImageInsufficientBalance)
}

func mediaReserveFixture(t *testing.T, id, amount string) service.MediaWalletReserve {
	t.Helper()
	quote, err := json.Marshal(map[string]string{"quote_id": mediaEvidence("quote:" + amount), "snapshot_id": mediaEvidence("snapshot"),
		"currency": "USD", "denomination": "cny_pegged_site_credit", "cny_per_usd_credit": "1", "sell_usd_credit": amount})
	require.NoError(t, err)
	contract, err := json.Marshal(map[string]string{"schema": "kitty-media-wallet-quote-v1", "snapshot_id": mediaEvidence("snapshot")})
	require.NoError(t, err)
	return service.MediaWalletReserve{Key: service.MediaWalletKey{UserID: 1, OrderID: id}, AmountUSD: amount, RequestHash: mediaEvidence("request:" + id), Quote: quote, PriceContract: contract}
}

func assertMediaBalance(t *testing.T, db *sql.DB, balance, frozen string) {
	t.Helper()
	var b, f string
	require.NoError(t, db.QueryRow(`SELECT balance::text,frozen_balance::text FROM users WHERE id=1`).Scan(&b, &f))
	require.Equal(t, balance, b)
	require.Equal(t, frozen, f)
}

func acceptMedia(t *testing.T, r service.MediaWalletRepository, key service.MediaWalletKey) {
	t.Helper()
	_, won, err := r.BeginSubmission(context.Background(), key)
	require.NoError(t, err)
	require.True(t, won)
	_, err = r.RecordSubmission(context.Background(), key, "accepted", "provider-"+key.OrderID, mediaEvidence("accepted:"+key.OrderID))
	require.NoError(t, err)
}

func TestMediaWalletPostgresReplayRestartAndPartialCapture(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	cmd := mediaReserveFixture(t, "partial", "7.60")
	o, created, err := r.Reserve(ctx, cmd)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "7.60000000", o.HoldUSD)
	assertMediaBalance(t, db, "2.40000000", "7.60000000")
	for range 2 {
		raw, e := os.ReadFile("../../migrations/286_kitty_media_wallet.sql")
		require.NoError(t, e)
		_, e = db.Exec(string(raw))
		require.NoError(t, e)
	}
	r = NewMediaWalletRepository(db)
	// Replay the actual JSONB documents after a repository/process restart.
	stored, err := r.Get(ctx, cmd.Key)
	require.NoError(t, err)
	cmd.Quote, cmd.PriceContract = stored.Quote, stored.PriceContract
	_, created, err = r.Reserve(ctx, cmd)
	require.NoError(t, err)
	require.False(t, created)
	changed := cmd
	changed.RequestHash = mediaEvidence("different input")
	_, _, err = r.Reserve(ctx, changed)
	require.ErrorIs(t, err, service.ErrMediaWalletConflict)
	acceptMedia(t, r, cmd.Key)
	for range 2 {
		o, err = r.Capture(ctx, cmd.Key, "5.70", mediaEvidence("successful completion"))
		require.NoError(t, err)
		require.Equal(t, "1.90000000", o.ReleasedUSD)
	}
	assertMediaBalance(t, db, "4.30000000", "0.00000000")
	_, err = r.Release(ctx, cmd.Key, mediaEvidence("late failure"))
	require.ErrorIs(t, err, service.ErrMediaWalletConflict)
	_, err = r.Capture(ctx, cmd.Key, "5.71", mediaEvidence("successful completion"))
	require.ErrorIs(t, err, service.ErrMediaWalletConflict)
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM kitty_media_wallet_ledger`).Scan(&count))
	require.Equal(t, 2, count)
}

func TestMediaWalletPostgresConcurrentAdmissionDoesNotOverdraw(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	commands := make([]service.MediaWalletReserve, 24)
	for i := range commands {
		commands[i] = mediaReserveFixture(t, fmt.Sprint("order-", i), "4")
	}
	var wg sync.WaitGroup
	results := make(chan error, len(commands))
	for _, cmd := range commands {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, err := r.Reserve(context.Background(), cmd); results <- err }()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			require.ErrorIs(t, err, service.ErrBatchImageInsufficientBalance)
		}
	}
	require.Equal(t, 2, accepted)
	assertMediaBalance(t, db, "2.00000000", "8.00000000")
}

func TestMediaWalletPostgresOneSubmissionAndUnknownCannotRefund(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	cmd := mediaReserveFixture(t, "unknown", "7.60")
	_, _, err := r.Reserve(ctx, cmd)
	require.NoError(t, err)
	var wg sync.WaitGroup
	winners := make(chan bool, 16)
	errors := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); _, won, e := r.BeginSubmission(ctx, cmd.Key); winners <- won; errors <- e }()
	}
	wg.Wait()
	close(winners)
	close(errors)
	for e := range errors {
		require.NoError(t, e)
	}
	n := 0
	for won := range winners {
		if won {
			n++
		}
	}
	require.Equal(t, 1, n)
	_, err = r.Release(ctx, cmd.Key, mediaEvidence("client disconnected"))
	require.ErrorIs(t, err, service.ErrMediaWalletState)
	_, err = r.RecordSubmission(ctx, cmd.Key, "unknown", "", mediaEvidence("submit timeout"))
	require.NoError(t, err)
	r = NewMediaWalletRepository(db)
	_, won, err := r.BeginSubmission(ctx, cmd.Key)
	require.NoError(t, err)
	require.False(t, won)
	_, err = r.Release(ctx, cmd.Key, mediaEvidence("age timeout"))
	require.ErrorIs(t, err, service.ErrMediaWalletState)
	assertMediaBalance(t, db, "2.40000000", "7.60000000")
	_, err = r.RecordSubmission(ctx, cmd.Key, "rejected", "", mediaEvidence("provider confirmed not created"))
	require.NoError(t, err)
	for range 2 {
		_, err = r.Release(ctx, cmd.Key, mediaEvidence("confirmed refund"))
		require.NoError(t, err)
	}
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
}

func TestMediaWalletPostgresCaptureVersusConfirmedFailure(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	cmd := mediaReserveFixture(t, "race", "4")
	_, _, err := r.Reserve(ctx, cmd)
	require.NoError(t, err)
	acceptMedia(t, r, cmd.Key)
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; _, e := r.Capture(ctx, cmd.Key, "4", mediaEvidence("completed")); results <- e }()
	go func() {
		<-start
		_, e := r.RecordSubmission(ctx, cmd.Key, "rejected", "provider-race", mediaEvidence("failed"))
		if e == nil {
			_, e = r.Release(ctx, cmd.Key, mediaEvidence("failure refund"))
		}
		results <- e
	}()
	close(start)
	a, b := <-results, <-results
	require.NotEqual(t, a == nil, b == nil, "exactly one terminal outcome wins")
	o, err := r.Get(ctx, cmd.Key)
	require.NoError(t, err)
	if o.BillingState == "captured" {
		assertMediaBalance(t, db, "6.00000000", "0.00000000")
	} else {
		require.Equal(t, "released", o.BillingState)
		assertMediaBalance(t, db, "10.00000000", "0.00000000")
	}
}

func TestMediaWalletPostgresBonusExpiryAndPermanentCredits(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint("expired-", expired), func(t *testing.T) {
			db, r := newMediaWalletFixture(t)
			ctx := context.Background()
			_, err := db.Exec(`INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at) VALUES(1,1,2,2,NULL)`)
			require.NoError(t, err)
			cmd := mediaReserveFixture(t, "bonus", "4")
			_, _, err = r.Reserve(ctx, cmd)
			require.NoError(t, err)
			if expired {
				_, err = db.Exec(`UPDATE recharge_bonus_grants SET expires_at=NOW()-INTERVAL '1 minute'`)
				require.NoError(t, err)
			}
			o, err := r.Release(ctx, cmd.Key, mediaEvidence("not submitted"))
			require.NoError(t, err)
			if expired {
				assertMediaBalance(t, db, "8.00000000", "0.00000000")
				require.Equal(t, "2.00000000", o.ExpiredUSD)
			} else {
				assertMediaBalance(t, db, "10.00000000", "0.00000000")
				var remaining string
				require.NoError(t, db.QueryRow(`SELECT remaining_amount::text FROM recharge_bonus_grants`).Scan(&remaining))
				require.Equal(t, "2.00000000", remaining)
			}
		})
	}
}

func TestMediaWalletPostgresValidationIsolationAndRollback(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	for _, invalid := range []string{"-1", "NaN", "Inf", "1e1000000000", "1.000000001", "1000000"} {
		cmd := mediaReserveFixture(t, "invalid", invalid)
		_, _, err := r.Reserve(ctx, cmd)
		require.Error(t, err)
	}
	cmd := mediaReserveFixture(t, "rollback", "4")
	_, err := db.Exec(`ALTER TABLE kitty_media_wallet_ledger ADD CONSTRAINT reject_hold CHECK(entry_type <> 'hold')`)
	require.NoError(t, err)
	_, _, err = r.Reserve(ctx, cmd)
	require.Error(t, err)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
	_, err = db.Exec(`ALTER TABLE kitty_media_wallet_ledger DROP CONSTRAINT reject_hold`)
	require.NoError(t, err)
	_, _, err = r.Reserve(ctx, cmd)
	require.NoError(t, err)
	wrong := cmd.Key
	wrong.UserID = 2
	_, err = r.Release(ctx, wrong, mediaEvidence("wrong user"))
	require.ErrorIs(t, err, sql.ErrNoRows)
	acceptMedia(t, r, cmd.Key)
	_, err = r.Capture(ctx, cmd.Key, "4.00000001", mediaEvidence("excess"))
	require.ErrorIs(t, err, service.ErrBatchImageSettlementCostExceedsHold)
	_, err = db.Exec(`ALTER TABLE kitty_media_wallet_ledger ADD CONSTRAINT reject_capture CHECK(entry_type <> 'capture')`)
	require.NoError(t, err)
	_, err = r.Capture(ctx, cmd.Key, "4", mediaEvidence("complete"))
	require.Error(t, err)
	assertMediaBalance(t, db, "6.00000000", "4.00000000")
	o, err := r.Get(ctx, cmd.Key)
	require.NoError(t, err)
	require.Equal(t, "held", o.BillingState)
	_, err = db.Exec(`ALTER TABLE kitty_media_wallet_ledger DROP CONSTRAINT reject_capture`)
	require.NoError(t, err)
	_, err = r.Capture(ctx, cmd.Key, "4", mediaEvidence("complete"))
	require.NoError(t, err)
	assertMediaBalance(t, db, "6.00000000", "0.00000000")
}

func TestMediaWalletPostgresZeroPriceStillHasReceipt(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	cmd := mediaReserveFixture(t, "free", "0")
	_, _, err := r.Reserve(ctx, cmd)
	require.NoError(t, err)
	acceptMedia(t, r, cmd.Key)
	o, err := r.Capture(ctx, cmd.Key, "0", mediaEvidence("free success"))
	require.NoError(t, err)
	require.Equal(t, "captured", o.BillingState)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
}

func TestMediaWalletPostgresActualNewAPIPriceContract(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	raw, err := os.ReadFile(os.Getenv("MEDIA_WALLET_TEST_QUOTE_FIXTURE"))
	require.NoError(t, err)
	var fixture struct {
		Quote    json.RawMessage `json:"quote"`
		Contract json.RawMessage `json:"price_contract"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	cmd := service.MediaWalletReserve{Key: service.MediaWalletKey{UserID: 1, OrderID: "newapi-image"}, AmountUSD: "0.10",
		RequestHash: mediaEvidence("actual newapi image quote"), Quote: fixture.Quote, PriceContract: fixture.Contract}
	_, created, err := r.Reserve(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, created)
	acceptMedia(t, r, cmd.Key)
	_, err = r.Capture(context.Background(), cmd.Key, "0.10", mediaEvidence("fixture success without generating media"))
	require.NoError(t, err)
	assertMediaBalance(t, db, "9.90000000", "0.00000000")
}

func TestMediaWalletPostgresConcurrentSameOrderAndSettlement(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	cmd := mediaReserveFixture(t, "same-order", "0.10")
	var wg sync.WaitGroup
	created := make(chan bool, 20)
	errors := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); _, won, err := r.Reserve(ctx, cmd); created <- won; errors <- err }()
	}
	wg.Wait()
	close(created)
	close(errors)
	winners := 0
	for won := range created {
		if won {
			winners++
		}
	}
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, 1, winners)
	acceptMedia(t, r, cmd.Key)
	errors = make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Capture(ctx, cmd.Key, "0.10", mediaEvidence("same completion"))
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	assertMediaBalance(t, db, "9.90000000", "0.00000000")
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM kitty_media_wallet_ledger`).Scan(&count))
	require.Equal(t, 2, count)
}

func TestMediaWalletPostgresConfirmedAsyncFailureRefunds(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	cmd := mediaReserveFixture(t, "failed-async", "4")
	_, _, err := r.Reserve(ctx, cmd)
	require.NoError(t, err)
	acceptMedia(t, r, cmd.Key)
	_, err = r.RecordSubmission(ctx, cmd.Key, "rejected", "other-provider-task", mediaEvidence("failed"))
	require.ErrorIs(t, err, service.ErrMediaWalletState)
	_, err = r.RecordSubmission(ctx, cmd.Key, "rejected", "provider-failed-async", mediaEvidence("provider confirmed failed"))
	require.NoError(t, err)
	_, err = r.Release(ctx, cmd.Key, mediaEvidence("confirmed failed refund"))
	require.NoError(t, err)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
}

func TestMediaWalletPostgresPartialCaptureDoesNotReviveExpiredBonus(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO recharge_bonus_grants(user_id,payment_order_id,granted_amount,remaining_amount,expires_at) VALUES(1,1,2,2,NULL)`)
	require.NoError(t, err)
	cmd := mediaReserveFixture(t, "partial-bonus", "4")
	_, _, err = r.Reserve(ctx, cmd)
	require.NoError(t, err)
	acceptMedia(t, r, cmd.Key)
	_, err = db.Exec(`UPDATE recharge_bonus_grants SET expires_at=NOW()-INTERVAL '1 minute'`)
	require.NoError(t, err)
	o, err := r.Capture(ctx, cmd.Key, "1", mediaEvidence("partial success"))
	require.NoError(t, err)
	require.Equal(t, "1.00000000", o.ActualUSD)
	require.Equal(t, "1.00000000", o.ExpiredUSD)
	require.Equal(t, "2.00000000", o.ReleasedUSD)
	assertMediaBalance(t, db, "8.00000000", "0.00000000")
}

func TestMediaWalletPostgresDisabledIdentityCannotStrandExistingHold(t *testing.T) {
	db, r := newMediaWalletFixture(t)
	ctx := context.Background()
	cmd := mediaReserveFixture(t, "disabled", "4")
	_, err := db.Exec(`UPDATE users SET status='disabled' WHERE id=1`)
	require.NoError(t, err)
	_, _, err = r.Reserve(ctx, cmd)
	require.ErrorIs(t, err, sql.ErrNoRows)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
	_, err = db.Exec(`UPDATE users SET status='active' WHERE id=1`)
	require.NoError(t, err)
	_, _, err = r.Reserve(ctx, cmd)
	require.NoError(t, err)
	_, won, err := r.BeginSubmission(ctx, cmd.Key)
	require.NoError(t, err)
	require.True(t, won)
	_, err = db.Exec(`UPDATE users SET status='disabled',deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
	_, err = r.RecordSubmission(ctx, cmd.Key, "accepted", "existing-job", mediaEvidence("accepted before identity was disabled"))
	require.NoError(t, err)
	_, err = r.Capture(ctx, cmd.Key, "4", mediaEvidence("existing task completion"))
	require.NoError(t, err)
	assertMediaBalance(t, db, "6.00000000", "0.00000000")
	cmd.Key.OrderID = "new-disabled-order"
	_, _, err = r.Reserve(ctx, cmd)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
