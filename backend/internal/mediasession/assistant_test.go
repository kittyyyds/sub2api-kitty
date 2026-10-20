package mediasession

import (
	"context"
	"fmt"

	"strings"

	"github.com/alicebob/miniredis/v2/server"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type assistantPermissions struct {
	calls  int
	cached []string
}

func (*assistantPermissions) MediaPermissions(context.Context, int64, []service.MediaModelCandidate) ([]service.MediaPermissionGroup, error) {
	return nil, nil
}
func (p *assistantPermissions) MediaAssistantKey(_ context.Context, userID, groupID int64, model, cached string, expiry time.Time) (*service.MediaAssistantCredential, error) {
	if userID != 42 || groupID != 7 || model != "text-model" {
		return nil, service.ErrGroupNotAllowed
	}
	p.calls++
	p.cached = append(p.cached, cached)
	return &service.MediaAssistantCredential{Key: "test-private-relay-credential", ExpiresAt: expiry}, nil
}
func TestMediaAssistantCredentialReusesOnlyValidatedSessionAndGroup(t *testing.T) {
	a, _, sources, _ := fixture(t)
	permissions := &assistantPermissions{}
	a.cfg.Permissions = permissions
	grant := grantFixture(t, a)
	ctx := context.Background()
	_, key, err := a.AssistantCredential(ctx, grant.Handle, grant.Subject, 7, "text-model")
	require.NoError(t, err)
	require.Equal(t, grant.ExpiresAt, key.ExpiresAt)
	_, _, err = a.AssistantCredential(ctx, grant.Handle, grant.Subject, 7, "text-model")
	require.NoError(t, err)
	require.Equal(t, []string{"", "test-private-relay-credential"}, permissions.cached)
	_, _, err = a.AssistantCredential(ctx, grant.Handle, "bad-subject", 7, "text-model")
	require.Error(t, err)
	_, _, err = a.AssistantCredential(ctx, grant.Handle, grant.Subject, 8, "text-model")
	require.Error(t, err)
	sources.err = service.ErrInvalidToken
	_, _, err = a.AssistantCredential(ctx, grant.Handle, grant.Subject, 7, "text-model")
	require.Error(t, err)
	require.Equal(t, 2, permissions.calls)
}
func TestMediaAssistantCredentialCannotOutliveRevokedOrExpiredSession(t *testing.T) {
	a, m, _, _ := fixture(t)
	a.cfg.Permissions = &assistantPermissions{}
	grant := grantFixture(t, a)
	require.NoError(t, a.Revoke(context.Background(), grant.Handle))
	_, _, err := a.AssistantCredential(context.Background(), grant.Handle, grant.Subject, 7, "text-model")
	require.Error(t, err)
	grant = grantFixture(t, a)
	m.FastForward(61 * time.Minute)
	_, _, err = a.AssistantCredential(context.Background(), grant.Handle, grant.Subject, 7, "text-model")
	require.Error(t, err)
}

type assistantIssuer struct {
	assistantPermissions
	issue func(context.Context, string, time.Time) (*service.MediaAssistantCredential, error)
}

func (p *assistantIssuer) MediaAssistantKey(ctx context.Context, _, _ int64, _, cached string, expiry time.Time) (*service.MediaAssistantCredential, error) {
	return p.issue(ctx, cached, expiry)
}

func TestMediaAssistantShortCredentialCacheRotatesAtCredentialExpiry(t *testing.T) {
	a, m, _, _ := fixture(t)
	grant := grantFixture(t, a)
	var seen []string
	a.cfg.Permissions = &assistantIssuer{issue: func(_ context.Context, cached string, _ time.Time) (*service.MediaAssistantCredential, error) {
		seen = append(seen, cached)
		return &service.MediaAssistantCredential{Key: "sk-short-lived-fixture", ExpiresAt: time.Now().Add(time.Minute)}, nil
	}}
	_, credential, err := a.AssistantCredential(t.Context(), grant.Handle, grant.Subject, 7, "text-model")
	require.NoError(t, err)
	cacheKey := a.prefix + "assistant:" + digest(grant.Handle) + ":7"
	require.Positive(t, m.TTL(cacheKey))
	require.LessOrEqual(t, m.TTL(cacheKey), time.Until(credential.ExpiresAt)+time.Second)
	m.FastForward(2 * time.Minute)
	_, _, err = a.AssistantCredential(t.Context(), grant.Handle, grant.Subject, 7, "text-model")
	require.NoError(t, err)
	require.Equal(t, []string{"", ""}, seen, "a short credential must not remain cached for the longer session TTL")
}

func TestMediaAssistantUnlockSurvivesCancellationAndHasDeadline(t *testing.T) {
	for _, stall := range []bool{false, true} {
		t.Run(fmt.Sprint("stalled-redis-", stall), func(t *testing.T) {
			a, m, _, _ := fixture(t)
			grant := grantFixture(t, a)
			observed := make(chan struct{}, 1)
			m.Server().SetPreHook(func(_ *server.Peer, command string, _ ...string) bool {
				if strings.EqualFold(command, "EVAL") {
					select {
					case observed <- struct{}{}:
					default:
					}
					return stall // Leave the actual Redis socket unanswered.
				}
				return false
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			a.cfg.Permissions = &assistantIssuer{issue: func(context.Context, string, time.Time) (*service.MediaAssistantCredential, error) {
				cancel()
				return nil, service.ErrGroupNotAllowed
			}}
			done := make(chan error, 1)
			go func() {
				_, _, err := a.AssistantCredential(ctx, grant.Handle, grant.Subject, 7, "text-model")
				done <- err
			}()
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case err := <-done:
				require.Error(t, err)
			case <-timer.C:
				t.Fatal("Redis unlock exceeded its bounded cleanup budget")
			}
			select {
			case <-observed:
			default:
				t.Fatal("caller cancellation prevented Redis lock cleanup")
			}
			lockKey := a.prefix + "assistant:" + digest(grant.Handle) + ":7:lock"
			require.Equal(t, stall, m.Exists(lockKey))
		})
	}
}

func TestMediaAssistantInvalidIssuanceCannotPublishCredential(t *testing.T) {
	for _, mode := range []string{"revoked-during-issuance", "expired", "outlives-session", "nil", "empty-key", "empty-subject", "invalid-model", "busy-lock", "replacement-lock"} {
		t.Run(mode, func(t *testing.T) {
			a, m, _, _ := fixture(t)
			grant := grantFixture(t, a)
			cacheKey := a.prefix + "assistant:" + digest(grant.Handle) + ":7"
			calls := 0
			a.cfg.Permissions = &assistantIssuer{issue: func(ctx context.Context, _ string, expiry time.Time) (*service.MediaAssistantCredential, error) {
				calls++
				credential := &service.MediaAssistantCredential{Key: "sk-private-fixture", ExpiresAt: expiry}
				switch mode {
				case "revoked-during-issuance":
					require.NoError(t, a.Revoke(ctx, grant.Handle))
				case "expired":
					credential.ExpiresAt = time.Now().Add(-time.Second)
				case "outlives-session":
					credential.ExpiresAt = expiry.Add(time.Second)
				case "nil":
					return nil, nil
				case "empty-key":
					credential.Key = ""
				case "replacement-lock":
					m.Set(cacheKey+":lock", "new-owner-proof")
				}
				return credential, nil
			}}
			subject, model := grant.Subject, "text-model"
			if mode == "empty-subject" {
				subject = ""
			}
			if mode == "invalid-model" {
				model = "text-model\n"
			}
			if mode == "busy-lock" {
				require.NoError(t, m.Set(cacheKey+":lock", "another-request"))
			}
			_, _, err := a.AssistantCredential(t.Context(), grant.Handle, subject, 7, model)
			if mode == "replacement-lock" {
				require.NoError(t, err)
				proof, getErr := m.Get(cacheKey + ":lock")
				require.NoError(t, getErr)
				require.Equal(t, "new-owner-proof", proof)
			} else {
				require.Error(t, err)
				require.False(t, m.Exists(cacheKey))
			}
			if mode == "empty-subject" || mode == "invalid-model" || mode == "busy-lock" {
				require.Zero(t, calls)
			}
		})
	}
}
