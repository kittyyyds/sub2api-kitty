package service

import "context"

// GetGroup resolves the administrator-selected group for model publication.
func (s *GatewayService) GetGroup(ctx context.Context, id int64) (*Group, error) {
	return s.groupRepo.GetByID(ctx, id)
}
