package service

import "context"

// This runs inside the existing managed background worker. Wallets opt in to a
// bounded cadence; model requests never wait for a supplier management endpoint.
func (s *UpstreamBillingProbeService) runProviderHubDue(ctx context.Context) {
	release, acquired, e := s.tryAcquireLeaderLock(ctx, "sub2api:provider_hub_sync")
	if e != nil || !acquired {
		return
	}
	defer release()
	repo, e := s.providerHub.providerHubRepo()
	if e != nil {
		return
	}
	ids, e := repo.ClaimProviderHubSync(ctx, 3)
	if e != nil {
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		_ = s.providerHub.SyncProviderHubWallet(ctx, id)
	}
}
