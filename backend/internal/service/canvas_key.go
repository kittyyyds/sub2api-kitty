package service

import "context"

// GetOwnedForCanvas resolves a key using the authenticated panel identity. It
// does not authenticate generation: the original gateway middleware still does.
// Refresh the user separately because GetByID's relation omits allowed groups.
func (s *APIKeyService) GetOwnedForCanvas(ctx context.Context, userID, keyID int64) (*APIKey, error) {
	key, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	if key == nil || userID <= 0 || key.UserID != userID {
		return nil, ErrAPIKeyNotFound
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	key.User = user
	s.compileAPIKeyIPRules(key)
	return key, nil
}
