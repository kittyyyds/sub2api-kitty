package service

import "context"

// ListPlazaGroups keeps ChannelService compatible with the billing-preview
// catalog provider. The dedicated ModelPlazaService owns the catalog logic;
// this adapter uses the same repositories and pricing fallback for callers
// that still receive a ChannelService from the provider graph.
func (s *ChannelService) ListPlazaGroups(ctx context.Context) ([]PlazaGroup, error) {
	if s == nil {
		return nil, nil
	}
	return NewModelPlazaService(s.repo, s.groupRepo, s.pricingService, nil, nil, nil).ListGroups(ctx)
}
