package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Raw credentials cross only the pinned, mutually authenticated backchannel.
type MediaTextKey struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Platform  string `json:"platform"`
	Key       string `json:"key,omitempty"`
}

func (s *APIKeyService) MediaTextKeys(ctx context.Context, userID, selectedID int64) ([]MediaTextKey, error) {
	groups, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	allowed := map[int64]Group{}
	for _, g := range groups {
		allowed[g.ID] = g
	}
	result := []MediaTextKey{}
	for page := 1; ; page++ {
		keys, _, err := s.apiKeyRepo.ListByUserID(ctx, userID, pagination.PaginationParams{Page: page, PageSize: 100}, APIKeyListFilters{})
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if k.UserID != userID || k.GroupID == nil || !k.IsActive() || k.IsExpired() || k.IsQuotaExhausted() || k.Key == "" || k.Name == mediaAssistantKeyName {
				continue
			}
			g, ok := allowed[*k.GroupID]
			if !ok || selectedID != 0 && k.ID != selectedID {
				continue
			}
			item := MediaTextKey{ID: k.ID, Name: k.Name, GroupID: g.ID, GroupName: g.Name, Platform: g.Platform}
			if selectedID != 0 {
				item.Key = k.Key
			}
			result = append(result, item)
		}
		if len(keys) < 100 {
			break
		}
	}
	return result, nil
}
