package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const mediaAssistantKeyName = "媒体画布助手（自动短期会话）"

// MediaAssistantCredential crosses only the pinned mTLS backchannel. It must
// never appear in a browser DTO, URL, log, canvas, or assistant history.
type MediaAssistantCredential struct {
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expires_at"`
}

// MediaAssistantKey uses the normal relay credential and billing path. The
// authority scopes its cache to a validated media session and selected group.
func (s *APIKeyService) MediaAssistantKey(ctx context.Context, userID, groupID int64, model, cached string, expiry time.Time) (*MediaAssistantCredential, error) {
	now := time.Now()
	if userID <= 0 || groupID <= 0 || model == "" || len(model) > 512 || model != strings.TrimSpace(model) || strings.ContainsAny(model, "\r\n\x00") || !now.Before(expiry) || expiry.After(now.Add(16*time.Minute)) {
		return nil, ErrGroupNotAllowed
	}
	groups, err := s.MediaPermissions(ctx, userID, []MediaModelCandidate{{Model: model, Kind: "text"}})
	if err != nil {
		return nil, err
	}
	permitted := false
	for _, g := range groups {
		if g.ID == groupID && len(g.Models) == 1 {
			permitted = true
		}
	}
	if !permitted || groupID <= 0 || !time.Now().Before(expiry) || expiry.After(time.Now().Add(16*time.Minute)) {
		return nil, ErrGroupNotAllowed
	}
	if cached != "" {
		key, err := s.GetByKeyUncached(ctx, cached)
		if err != nil || key == nil || key.UserID != userID || key.GroupID == nil || *key.GroupID != groupID || !key.IsActive() || key.IsExpired() || key.IsQuotaExhausted() || key.ExpiresAt == nil || key.ExpiresAt.After(expiry) {
			return nil, ErrGroupNotAllowed
		}
		return &MediaAssistantCredential{Key: cached, ExpiresAt: *key.ExpiresAt}, nil
	}
	// Expired automatic credentials otherwise remain in api_keys and consume
	// the user's key-count limit forever. Only prune expired assistant records;
	// keep audit history through the normal tombstone deletion path.
	keys, _, err := s.apiKeyRepo.ListByUserID(ctx, userID, pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "expires_at", SortOrder: "asc"}, APIKeyListFilters{Search: mediaAssistantKeyName})
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if key.UserID == userID && key.Name == mediaAssistantKeyName && key.ConcurrencyLimit == 1 && key.ExpiresAt != nil && !now.Before(*key.ExpiresAt) {
			if err := s.Delete(ctx, key.ID, userID); err != nil {
				return nil, err
			}
		}
	}
	if err := s.checkAPIKeyCreateLimits(ctx, userID); err != nil {
		return nil, err
	}
	raw, err := s.GenerateKey()
	if err != nil {
		return nil, err
	}
	key := &APIKey{UserID: userID, Key: raw, Name: mediaAssistantKeyName, GroupID: &groupID, Status: StatusActive, ExpiresAt: &expiry, ConcurrencyLimit: 1}
	if err := s.apiKeyRepo.Create(ctx, key); err != nil {
		return nil, err
	}
	s.InvalidateAuthCacheByKey(ctx, key.AuthCacheInvalidationKey())
	return &MediaAssistantCredential{Key: raw, ExpiresAt: expiry}, nil
}
