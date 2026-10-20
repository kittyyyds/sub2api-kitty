package service

import (
	"context"
	"errors"
	"time"
)

// MediaSourceSession contains no reusable SUB access/refresh token. The source
// session must still have a live refresh credential on every media check.
type MediaSourceSession struct {
	UserID       int64     `json:"user_id"`
	TokenVersion int64     `json:"token_version"`
	FamilyID     string    `json:"family_id"`
	BindingHash  string    `json:"binding_hash"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (s *AuthService) MediaSourceFromToken(ctx context.Context, accessToken string) (*MediaSourceSession, error) {
	claims, err := s.ValidateToken(accessToken)
	if err != nil || claims == nil || claims.ExpiresAt == nil {
		return nil, ErrInvalidToken
	}
	source := &MediaSourceSession{UserID: claims.UserID, TokenVersion: claims.TokenVersion,
		FamilyID: claims.SessionID, BindingHash: claims.BindingHash, ExpiresAt: claims.ExpiresAt.Time}
	expires, err := s.mediaSourceExpiry(ctx, source)
	if err != nil {
		return nil, err
	}
	// The handoff follows the verified SUB login family, not its short-lived
	// access JWT. Every use still checks the live family and current user.
	source.ExpiresAt = expires
	return source, nil
}

func (s *AuthService) ValidateMediaSource(ctx context.Context, source *MediaSourceSession) error {
	_, err := s.mediaSourceExpiry(ctx, source)
	return err
}

func (s *AuthService) mediaSourceExpiry(ctx context.Context, source *MediaSourceSession) (time.Time, error) {
	if source == nil || source.UserID <= 0 || source.FamilyID == "" || len(source.FamilyID) > 128 ||
		!time.Now().Before(source.ExpiresAt) || s.userRepo == nil || s.refreshTokenCache == nil {
		return time.Time{}, ErrTokenRevoked
	}
	user, err := s.userRepo.GetByID(ctx, source.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return time.Time{}, ErrTokenRevoked
		}
		return time.Time{}, ErrServiceUnavailable
	}
	if user == nil || !user.IsActive() || source.TokenVersion != resolvedTokenVersion(user) {
		return time.Time{}, ErrTokenRevoked
	}
	bindingEnabled := false
	if s.settingService != nil {
		value, err := s.settingService.settingRepo.GetValue(ctx, SettingKeySessionBindingEnabled)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			return time.Time{}, ErrServiceUnavailable
		}
		bindingEnabled = value == "true"
	}
	if bindingEnabled {
		if source.BindingHash == "" {
			return time.Time{}, ErrSessionBindingMismatch
		}
		current := sessionBindingHashFromContext(ctx)
		if current == "" || current != source.BindingHash {
			if err := s.RevokeSessionFamily(ctx, source.FamilyID); err != nil {
				return time.Time{}, ErrServiceUnavailable
			}
			return time.Time{}, ErrSessionBindingMismatch
		}
	}
	hashes, err := s.refreshTokenCache.GetFamilyTokenHashes(ctx, source.FamilyID)
	if err != nil || len(hashes) > 256 {
		return time.Time{}, ErrServiceUnavailable
	}
	var expires time.Time
	// Family indexes can retain hashes after logout/rotation. Existence of the
	// index alone is NOT proof of an active session; inspect live token data.
	for _, hash := range hashes {
		data, err := s.refreshTokenCache.GetRefreshToken(ctx, hash)
		if errors.Is(err, ErrRefreshTokenNotFound) {
			continue
		}
		if err != nil {
			return time.Time{}, ErrServiceUnavailable
		}
		if data != nil && data.UserID == source.UserID && data.FamilyID == source.FamilyID &&
			data.TokenVersion == source.TokenVersion && data.BindingHash == source.BindingHash && time.Now().Before(data.ExpiresAt) {
			if data.ExpiresAt.After(expires) {
				expires = data.ExpiresAt
			}
		}
	}
	if expires.IsZero() {
		return time.Time{}, ErrTokenRevoked
	}
	return expires, nil
}
