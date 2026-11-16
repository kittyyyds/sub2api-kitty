// Package mediasession implements a private first-party login handoff, not a
// general OAuth/OIDC provider. Public issuance must run in SUB's authenticated
// same-origin UI after its existing login/MFA flow. Backchannel operations are
// served only by mediawalletapi's pinned mutual-TLS listener.
package mediasession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var (
	ErrDenied      = errors.New("media login denied")
	ErrUnavailable = errors.New("media login unavailable")
	opaque         = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	verifier       = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
)

const codeTTL = time.Minute

// A media session never outlives the source login family. Short access tokens
// are rotated independently; every media operation revalidates that family.
const sessionTTL = 30 * 24 * time.Hour

type SourceAuthority interface {
	MediaSourceFromToken(context.Context, string) (*service.MediaSourceSession, error)
	ValidateMediaSource(context.Context, *service.MediaSourceSession) error
}

type Config struct {
	Issuer      string
	Audience    string
	CallbackURL string
	Permissions interface {
		MediaPermissions(context.Context, int64, []service.MediaModelCandidate) ([]service.MediaPermissionGroup, error)
	}
}

type Authority struct {
	cfg        Config
	prefix     string
	rdb        *redis.Client
	sources    SourceAuthority
	identities service.MediaWalletIdentityRepository
	balances   service.MediaWalletBalanceReader
}

type record struct {
	Source    service.MediaSourceSession `json:"source"`
	Subject   string                     `json:"subject"`
	Challenge string                     `json:"challenge,omitempty"`
	StateHash string                     `json:"state_hash,omitempty"`
	ExpiresAt time.Time                  `json:"expires_at"`
}

type Identity struct {
	Issuer    string    `json:"issuer"`
	Audience  string    `json:"audience"`
	Subject   string    `json:"subject"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Grant struct {
	Identity
	// Handle is for the media backend only, never a browser URL, log or local
	// storage value. Browser sessions need their own rotated host-only cookie.
	Handle string `json:"handle"`
}

func New(cfg Config, rdb *redis.Client, sources SourceAuthority, identities service.MediaWalletIdentityRepository, balances service.MediaWalletBalanceReader) (*Authority, error) {
	issuer, err := url.Parse(cfg.Issuer)
	callback, callbackErr := url.Parse(cfg.CallbackURL)
	if err != nil || callbackErr != nil || issuer.Scheme != "https" || issuer.Hostname() == "" || issuer.User != nil ||
		issuer.RawQuery != "" || issuer.ForceQuery || issuer.Fragment != "" || issuer.Opaque != "" || issuer.RawPath != "" || issuer.Path != "" ||
		callback.Scheme != "https" || callback.Hostname() == "" || callback.User != nil || callback.Path == "" ||
		callback.RawQuery != "" || callback.ForceQuery || callback.Fragment != "" || callback.Opaque != "" || callback.RawPath != "" ||
		cfg.Audience == "" || len(cfg.Audience) > 128 || rdb == nil || sources == nil || identities == nil || balances == nil {
		return nil, errors.New("media login requires fixed HTTPS issuer/callback, audience, Redis and authoritative services")
	}
	namespace := digest(cfg.Issuer + "\n" + cfg.Audience + "\n" + cfg.CallbackURL)
	return &Authority{cfg: cfg, prefix: "kitty:media-sso:" + namespace + ":", rdb: rdb, sources: sources, identities: identities, balances: balances}, nil
}

// Issue accepts only the existing fully authenticated SUB access token. It
// never accepts a caller-supplied user ID, email, subject, return URL or price.
func (a *Authority) Issue(ctx context.Context, accessToken, challenge, state string) (string, error) {
	if !opaque.MatchString(challenge) || !opaque.MatchString(state) {
		return "", ErrDenied
	}
	source, err := a.sources.MediaSourceFromToken(ctx, accessToken)
	if err != nil {
		return "", sourceError(err)
	}
	if source == nil || !time.Now().Before(source.ExpiresAt) {
		return "", ErrDenied
	}
	subject, err := a.identities.EnsureSubject(ctx, source.UserID)
	if err != nil || !opaque.MatchString(subject) {
		return "", ErrUnavailable
	}
	code, err := randomSecret()
	if err != nil {
		return "", ErrUnavailable
	}
	r := record{Source: *source, Subject: subject, Challenge: challenge, StateHash: digest(state), ExpiresAt: minTime(source.ExpiresAt, time.Now().Add(codeTTL))}
	if err = a.store(ctx, "code:", code, r); err != nil {
		return "", err
	}
	return code, nil
}

// Exchange is service-only and consumes the code even on invalid PKCE/state.
// Retrying a lost exchange requires a new browser-initiated handoff, not replay.
func (a *Authority) Exchange(ctx context.Context, code, proof, state, audience, callback string) (*Grant, error) {
	if !opaque.MatchString(code) || !verifier.MatchString(proof) || !opaque.MatchString(state) || audience != a.cfg.Audience || callback != a.cfg.CallbackURL {
		return nil, ErrDenied
	}
	key := a.prefix + "code:" + digest(code)
	// GET+DEL in one script works on Redis versions predating GETDEL.
	raw, err := a.rdb.Eval(ctx, `local v=redis.call('GET',KEYS[1]); if v then redis.call('DEL',KEYS[1]) end; return v`, []string{key}).Text()
	if errors.Is(err, redis.Nil) {
		return nil, ErrDenied
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	var r record
	if json.Unmarshal([]byte(raw), &r) != nil || !time.Now().Before(r.ExpiresAt) || !opaque.MatchString(r.Subject) ||
		r.Challenge != challengeFor(proof) || r.StateHash != digest(state) {
		return nil, ErrDenied
	}
	if err := a.sources.ValidateMediaSource(ctx, &r.Source); err != nil {
		return nil, sourceError(err)
	}
	handle, err := randomSecret()
	if err != nil {
		return nil, ErrUnavailable
	}
	r.Challenge, r.StateHash = "", ""
	r.ExpiresAt = minTime(r.Source.ExpiresAt, time.Now().Add(sessionTTL)).UTC()
	if err := a.store(ctx, "session:", handle, r); err != nil {
		return nil, err
	}
	return &Grant{Identity: a.identity(r), Handle: handle}, nil
}

func (a *Authority) Check(ctx context.Context, handle, expectedSubject string) (*Identity, error) {
	r, err := a.load(ctx, handle, expectedSubject)
	if err != nil {
		return nil, err
	}
	identity := a.identity(*r)
	return &identity, nil
}

func (a *Authority) Balance(ctx context.Context, handle string) (*Identity, *service.MediaWalletBalance, error) {
	r, err := a.load(ctx, handle, "")
	if err != nil {
		return nil, nil, err
	}
	balance, err := a.balances.Balance(ctx, r.Source.UserID)
	if err != nil || balance == nil {
		return nil, nil, ErrUnavailable
	}
	id := a.identity(*r)
	return &id, balance, nil
}

func (a *Authority) Revoke(ctx context.Context, handle string) error {
	if !opaque.MatchString(handle) {
		return ErrDenied
	}
	if err := a.rdb.Del(ctx, a.prefix+"session:"+digest(handle)).Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (a *Authority) Permissions(ctx context.Context, handle, subject string, models []service.MediaModelCandidate) (*Identity, []service.MediaPermissionGroup, error) {
	r, err := a.load(ctx, handle, subject)
	if err != nil {
		return nil, nil, err
	}
	if a.cfg.Permissions == nil {
		return nil, nil, ErrUnavailable
	}
	groups, err := a.cfg.Permissions.MediaPermissions(ctx, r.Source.UserID, models)
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	id := a.identity(*r)
	return &id, groups, nil
}

func (a *Authority) load(ctx context.Context, handle, expectedSubject string) (*record, error) {
	if !opaque.MatchString(handle) || expectedSubject != "" && !opaque.MatchString(expectedSubject) {
		return nil, ErrDenied
	}
	raw, err := a.rdb.Get(ctx, a.prefix+"session:"+digest(handle)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrDenied
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	var r record
	if json.Unmarshal(raw, &r) != nil || !opaque.MatchString(r.Subject) || !time.Now().Before(r.ExpiresAt) ||
		expectedSubject != "" && expectedSubject != r.Subject {
		return nil, ErrDenied
	}
	if err := a.sources.ValidateMediaSource(ctx, &r.Source); err != nil {
		return nil, sourceError(err)
	}
	return &r, nil
}

func (a *Authority) store(ctx context.Context, kind, secret string, r record) error {
	raw, err := json.Marshal(r)
	ttl := time.Until(r.ExpiresAt)
	if err != nil || ttl <= 0 {
		return ErrDenied
	}
	ok, err := a.rdb.SetNX(ctx, a.prefix+kind+digest(secret), raw, ttl).Result()
	if err != nil || !ok {
		return ErrUnavailable
	}
	return nil
}

func (a *Authority) identity(r record) Identity {
	return Identity{Issuer: a.cfg.Issuer, Audience: a.cfg.Audience, Subject: r.Subject, ExpiresAt: r.ExpiresAt}
}

func sourceError(err error) error {
	if errors.Is(err, service.ErrServiceUnavailable) {
		return ErrUnavailable
	}
	return ErrDenied
}

func randomSecret() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:]), err
}

func digest(value string) string {
	b := sha256.Sum256([]byte(value))
	return hex.EncodeToString(b[:])
}

func challengeFor(proof string) string {
	b := sha256.Sum256([]byte(proof))
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// AssistantCredential is available only to the authenticated media backend.
// Never trust a user ID or an existing key supplied by the browser.
func (a *Authority) AssistantCredential(ctx context.Context, handle, subject string, groupID int64, model string) (*Identity, *service.MediaAssistantCredential, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := a.load(ctx, handle, subject)
	if err != nil {
		return nil, nil, err
	}
	issuer, ok := a.cfg.Permissions.(interface {
		MediaAssistantKey(context.Context, int64, int64, string, string, time.Time) (*service.MediaAssistantCredential, error)
	})
	if !ok || !opaque.MatchString(subject) || groupID <= 0 || model == "" || len(model) > 512 || model != strings.TrimSpace(model) || strings.ContainsAny(model, "\r\n\x00") {
		return nil, nil, ErrDenied
	}
	cacheKey := a.prefix + "assistant:" + digest(handle) + ":" + fmt.Sprint(groupID)
	lockKey := cacheKey + ":lock"
	proof, err := randomSecret()
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	acquired, err := a.rdb.SetNX(ctx, lockKey, proof, 10*time.Second).Result()
	if err != nil || !acquired {
		return nil, nil, ErrUnavailable
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		// Bound socket I/O even when Redis ContextTimeoutEnabled is false.
		_ = a.rdb.WithTimeout(time.Second).Eval(unlockCtx, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end; return 0`, []string{lockKey}, proof).Err()
	}()
	cached, err := a.rdb.Get(ctx, cacheKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, nil, ErrUnavailable
	}
	credential, err := issuer.MediaAssistantKey(ctx, r.Source.UserID, groupID, model, cached, r.ExpiresAt)
	if err != nil {
		return nil, nil, ErrDenied
	}
	if credential == nil || credential.Key == "" || !time.Now().Before(credential.ExpiresAt) || credential.ExpiresAt.After(r.ExpiresAt) {
		return nil, nil, ErrUnavailable
	}
	// Session revocation/expiry can race credential issuance. Revalidate before
	// publishing the credential and never retain it beyond its own expiry.
	if _, err := a.load(ctx, handle, subject); err != nil {
		return nil, nil, err
	}
	ttl := time.Until(credential.ExpiresAt)
	if ttl <= 0 {
		return nil, nil, ErrDenied
	}
	if err := a.rdb.Set(ctx, cacheKey, credential.Key, ttl).Err(); err != nil {
		return nil, nil, ErrUnavailable
	}
	id := a.identity(*r)
	return &id, credential, nil
}
