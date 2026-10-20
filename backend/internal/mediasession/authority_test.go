package mediasession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type sources struct {
	mu  sync.Mutex
	err error
}

func (s *sources) MediaSourceFromToken(ctx context.Context, token string) (*service.MediaSourceSession, error) {
	if token != "completed-sub-login" {
		return nil, service.ErrInvalidToken
	}
	source := &service.MediaSourceSession{UserID: 42, FamilyID: "source-session", TokenVersion: 7, ExpiresAt: time.Now().Add(time.Hour)}
	return source, s.ValidateMediaSource(ctx, source)
}

func (s *sources) ValidateMediaSource(context.Context, *service.MediaSourceSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

type identities struct {
	service.MediaWalletIdentityRepository
}

func (identities) EnsureSubject(_ context.Context, id int64) (string, error) {
	if id != 42 {
		return "", errors.New("wrong verified user")
	}
	return strings.Repeat("s", 43), nil
}

type balances struct{ calls atomic.Int32 }

func (b *balances) Balance(_ context.Context, id int64) (*service.MediaWalletBalance, error) {
	if id != 42 {
		return nil, errors.New("wrong verified user")
	}
	b.calls.Add(1)
	return &service.MediaWalletBalance{Currency: "USD", Denomination: "cny_pegged_site_credit", AvailableUSD: "9.90000000", FrozenUSD: "0.10000000", BonusAvailableUSD: "0.00000000", AsOf: time.Now()}, nil
}

func fixture(t *testing.T) (*Authority, *miniredis.Miniredis, *sources, *balances) {
	t.Helper()
	m := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: m.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	s := &sources{}
	b := &balances{}
	a, err := New(Config{Issuer: "https://sub.example", Audience: "kitty-media", CallbackURL: "https://media.example/media-login/callback"}, rdb, s, identities{}, b)
	require.NoError(t, err)
	return a, m, s, b
}

func grantFixture(t *testing.T, a *Authority) *Grant {
	t.Helper()
	proof, err := randomSecret()
	require.NoError(t, err)
	state, err := randomSecret()
	require.NoError(t, err)
	code, err := a.Issue(context.Background(), "completed-sub-login", challengeFor(proof), state)
	require.NoError(t, err)
	grant, err := a.Exchange(context.Background(), code, proof, state, a.cfg.Audience, a.cfg.CallbackURL)
	require.NoError(t, err)
	return grant
}

func TestMediaLoginSingleUseConcurrentExchangeAndBalance(t *testing.T) {
	a, m, _, b := fixture(t)
	ctx := context.Background()
	proof, _ := randomSecret()
	state, _ := randomSecret()
	code, err := a.Issue(ctx, "completed-sub-login", challengeFor(proof), state)
	require.NoError(t, err)
	var won atomic.Int32
	var wg sync.WaitGroup
	results := make(chan *Grant, 20)
	for range 20 {
		wg.Go(func() {
			g, err := a.Exchange(ctx, code, proof, state, a.cfg.Audience, a.cfg.CallbackURL)
			if err == nil {
				won.Add(1)
				results <- g
			} else {
				require.ErrorIs(t, err, ErrDenied)
			}
		})
	}
	wg.Wait()
	close(results)
	require.EqualValues(t, 1, won.Load())
	g := <-results
	require.Equal(t, strings.Repeat("s", 43), g.Subject)
	require.Equal(t, a.cfg.Issuer, g.Issuer)
	require.LessOrEqual(t, time.Until(g.ExpiresAt), time.Hour)
	require.Greater(t, time.Until(g.ExpiresAt), 15*time.Minute)
	for _, key := range m.Keys() {
		require.NotContains(t, key, code)
		require.NotContains(t, key, g.Handle)
		raw, _ := m.Get(key)
		require.NotContains(t, raw, code)
		require.NotContains(t, raw, g.Handle)
		require.NotContains(t, raw, "completed-sub-login")
	}
	for range 2 {
		id, balance, err := a.Balance(ctx, g.Handle)
		require.NoError(t, err)
		require.Equal(t, g.Identity, *id)
		require.Equal(t, "9.90000000", balance.AvailableUSD)
	}
	require.EqualValues(t, 2, b.calls.Load(), "no copied/cached media balance")
	_, err = a.Check(ctx, g.Handle, strings.Repeat("x", 43))
	require.ErrorIs(t, err, ErrDenied)
	require.NoError(t, a.Revoke(ctx, g.Handle))
	require.NoError(t, a.Revoke(ctx, g.Handle))
	_, err = a.Check(ctx, g.Handle, g.Subject)
	require.ErrorIs(t, err, ErrDenied)
}

func TestMediaLoginRejectsReplayProofStateAudienceRedirectAndExpiry(t *testing.T) {
	for _, kind := range []string{"pkce", "state", "audience", "callback", "code expiry", "invalid token", "short challenge", "revoked after issuance"} {
		t.Run(kind, func(t *testing.T) {
			a, m, s, _ := fixture(t)
			ctx := context.Background()
			proof, _ := randomSecret()
			state, _ := randomSecret()
			token, challenge := "completed-sub-login", challengeFor(proof)
			if kind == "invalid token" {
				token = "pre-mfa-challenge"
			}
			if kind == "short challenge" {
				challenge = "plain"
			}
			code, err := a.Issue(ctx, token, challenge, state)
			if kind == "invalid token" || kind == "short challenge" {
				require.ErrorIs(t, err, ErrDenied)
				return
			}
			require.NoError(t, err)
			audience, callback := a.cfg.Audience, a.cfg.CallbackURL
			switch kind {
			case "pkce":
				proof, _ = randomSecret()
			case "state":
				state, _ = randomSecret()
			case "audience":
				audience = "another-service"
			case "callback":
				callback += "?redirect=https://evil.example"
			case "code expiry":
				m.FastForward(61 * time.Second)
			case "revoked after issuance":
				s.err = service.ErrTokenRevoked
			}
			_, err = a.Exchange(ctx, code, proof, state, audience, callback)
			require.ErrorIs(t, err, ErrDenied)
		})
	}
}

func TestMediaLoginSourceRevocationAndUnavailableFailClosed(t *testing.T) {
	a, m, s, b := fixture(t)
	g := grantFixture(t, a)
	ctx := context.Background()
	for _, reason := range []error{service.ErrTokenRevoked, service.ErrSessionBindingMismatch, service.ErrServiceUnavailable} {
		s.err = reason
		_, _, err := a.Balance(ctx, g.Handle)
		want := ErrDenied
		if errors.Is(reason, service.ErrServiceUnavailable) {
			want = ErrUnavailable
		}
		require.ErrorIs(t, err, want)
	}
	require.Zero(t, b.calls.Load())
	s.err = nil
	m.FastForward(61 * time.Minute)
	_, err := a.Check(ctx, g.Handle, g.Subject)
	require.ErrorIs(t, err, ErrDenied)
	g = grantFixture(t, a)
	m.Close()
	_, err = a.Check(ctx, g.Handle, g.Subject)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestMediaLoginIssuanceRejectsCrossOriginAndUnknownIdentityFields(t *testing.T) {
	a, _, _, _ := fixture(t)
	proof, _ := randomSecret()
	state, _ := randomSecret()
	body := `{"challenge":"` + challengeFor(proof) + `","state":"` + state + `"}`
	for _, tc := range []struct {
		name, origin, token, body string
		status                    int
	}{
		{"same origin post-MFA", a.cfg.Issuer, "completed-sub-login", body, 200},
		{"sibling domain", "https://media.example", "completed-sub-login", body, 403},
		{"no origin", "", "completed-sub-login", body, 403},
		{"pre-MFA challenge", a.cfg.Issuer, "pre-mfa-challenge", body, 401},
		{"raw user id", a.cfg.Issuer, "completed-sub-login", strings.TrimSuffix(body, "}") + `,"user_id":1}`, 400},
		{"trailing JSON", a.cfg.Issuer, "completed-sub-login", body + ` {}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "https://sub.example/api/v1/media/login/authorize", bytes.NewBufferString(tc.body))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			a.IssueHandler().ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.NotContains(t, w.Body.String(), "completed-sub-login")
			if tc.status == http.StatusOK {
				var envelope struct {
					Code int `json:"code"`
					Data struct {
						Code        string `json:"code"`
						CallbackURL string `json:"callback_url"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
				require.Zero(t, envelope.Code)
				require.Len(t, envelope.Data.Code, 43)
				require.Equal(t, a.cfg.CallbackURL, envelope.Data.CallbackURL)
			}
		})
	}
}
