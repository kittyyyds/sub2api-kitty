package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIsReservedEmail_DingTalkDomain(t *testing.T) {
	require.True(t, isReservedEmail("dingtalk-123@dingtalk-connect.invalid"))
	require.True(t, isReservedEmail("DINGTALK-456@DINGTALK-CONNECT.INVALID")) // case-insensitive
	require.False(t, isReservedEmail("real@dingtalk.com"))
}

type mediaSourceUserRepo struct {
	UserRepository
	user *User
}

func (r *mediaSourceUserRepo) GetByID(context.Context, int64) (*User, error) { return r.user, nil }

type mediaSourceRefresh struct {
	RefreshTokenCache
	live *RefreshTokenData
	err  error
}

func (r *mediaSourceRefresh) DeleteTokenFamily(context.Context, string) error {
	r.live = nil
	return nil
}

type mediaSourceSettings struct {
	SettingRepository
	err error
}

func (s mediaSourceSettings) GetValue(context.Context, string) (string, error) { return "true", s.err }

func (r *mediaSourceRefresh) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return []string{"stale-logout-hash", "current-refresh-hash"}, r.err
}

func (r *mediaSourceRefresh) GetRefreshToken(_ context.Context, hash string) (*RefreshTokenData, error) {
	if hash == "stale-logout-hash" || r.live == nil {
		return nil, ErrRefreshTokenNotFound
	}
	return r.live, nil
}

func TestMediaSourceChecksActualLiveCredentialsAndCurrentUser(t *testing.T) {
	ctx := context.Background()
	u := &User{ID: 42, Email: "sso-test@example.invalid", PasswordHash: "original-password-hash", Status: "active"}
	cache := &mediaSourceRefresh{live: &RefreshTokenData{UserID: u.ID, TokenVersion: resolvedTokenVersion(u), FamilyID: "family", ExpiresAt: time.Now().Add(time.Hour)}}
	a := &AuthService{cfg: &config.Config{}, userRepo: &mediaSourceUserRepo{user: u}, refreshTokenCache: cache}
	a.cfg.JWT.Secret = "isolated-media-source-test-secret"
	a.cfg.JWT.AccessTokenExpireMinutes = 5
	token, err := a.generateAccessToken(u, "family", "")
	require.NoError(t, err)
	source, err := a.MediaSourceFromToken(ctx, token)
	require.NoError(t, err)
	require.Equal(t, u.ID, source.UserID)
	require.Equal(t, cache.live.ExpiresAt, source.ExpiresAt, "handoff follows the live refresh family, beyond the five-minute access token")
	_, err = a.MediaSourceFromToken(ctx, "pre-MFA-not-an-access-token")
	require.Error(t, err)
	for _, kind := range []string{"logout", "password", "disabled", "expired source", "expired refresh", "other user", "other family", "cache down"} {
		t.Run(kind, func(t *testing.T) {
			oldUser := *u
			oldLive := *cache.live
			oldSource := *source
			defer func() { *u = oldUser; cache.live = &oldLive; *source = oldSource; cache.err = nil }()
			switch kind {
			case "logout":
				cache.live = nil
			case "password":
				u.PasswordHash = "changed-password-hash"
			case "disabled":
				u.Status = "disabled"
			case "expired source":
				source.ExpiresAt = time.Now().Add(-time.Second)
			case "expired refresh":
				cache.live.ExpiresAt = time.Now().Add(-time.Second)
			case "other user":
				cache.live.UserID++
			case "other family":
				cache.live.FamilyID = "other-family"
			case "cache down":
				cache.err = ErrServiceUnavailable
			}
			require.Error(t, a.ValidateMediaSource(ctx, source))
		})
	}
	// A refreshed credential in the same family keeps the handoff valid without
	// persisting any original access/refresh token in the media session store.
	cache.live.ExpiresAt = time.Now().Add(2 * time.Hour)
	require.NoError(t, a.ValidateMediaSource(ctx, source))
	// Enabling source binding also applies to the first login handoff. A settings
	// outage cannot silently relax the configured IP/UA policy on the media path.
	binding := &SessionBinding{IP: "192.0.2.1", UserAgent: "original-browser"}
	source.BindingHash = binding.Hash()
	cache.live.BindingHash = source.BindingHash
	a.settingService = NewSettingService(mediaSourceSettings{}, a.cfg)
	require.NoError(t, a.ValidateMediaSource(WithSessionBinding(ctx, binding), source))
	a.settingService = NewSettingService(mediaSourceSettings{err: ErrServiceUnavailable}, a.cfg)
	require.ErrorIs(t, a.ValidateMediaSource(WithSessionBinding(ctx, binding), source), ErrServiceUnavailable)
	a.settingService = NewSettingService(mediaSourceSettings{}, a.cfg)
	require.ErrorIs(t, a.ValidateMediaSource(ctx, source), ErrSessionBindingMismatch)
	require.Nil(t, cache.live, "missing network binding revokes the family")
}
