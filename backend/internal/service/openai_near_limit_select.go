package service

import (
	"context"
	"time"
)

// openAINearLimitCandidates lists fresh, eligible accounts whose 5h or 7d Codex
// window is at or above 95% (classifyOpenAINearLimitCandidate), best first.
//
// The scheduler snapshot is classified first (in memory), so a pool without a
// near-limit account costs no extra account read. Every per-request gate of
// this fork then applies to the few remaining accounts through
// resolveFreshSchedulableOpenAIAccount (strict failover tier, runtime blocks,
// active test holds, profit control, shadow parent health, proxy quarantine);
// accept adds caller-specific gates (scheduler transport/privacy/group).
//
// Excel BPS keeps its precedence: while the pool has a BPS-enabled account
// for the requested model, only BPS-enabled accounts are near-limit targets.
func (s *OpenAIGatewayService) openAINearLimitCandidates(
	ctx context.Context,
	groupID *int64,
	platform string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requireCompact bool,
	requiredCapability OpenAIEndpointCapability,
	accept func(*Account) bool,
) []openAINearLimitCandidate {
	if s == nil || skipOpenAINearLimitAccounts(ctx) || !openAINearLimitEnabled(s.cfg) {
		return nil
	}
	platform = NormalizeOpenAICompatiblePlatform(platform)
	if platform != PlatformOpenAI {
		return nil
	}
	accounts, err := s.listSchedulableAccounts(ctx, groupID, platform)
	if err != nil || len(accounts) == 0 {
		return nil
	}
	now := time.Now()
	var listed []*Account
	for i := range accounts {
		acc := &accounts[i]
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
		}
		if _, ok := classifyOpenAINearLimitCandidate(acc, now); !ok {
			continue
		}
		if !canClaimOpenAINearLimitPreference(acc.ID, now, true) {
			continue
		}
		listed = append(listed, acc)
	}
	var cands []openAINearLimitCandidate
	for _, acc := range listed {
		fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, acc, platform, requestedModel, requireCompact, requiredCapability)
		if fresh == nil || activeIntelligenceHold(ctx, fresh, groupID, requestedModel) {
			continue
		}
		if accept != nil && !accept(fresh) {
			continue
		}
		cand, ok := classifyOpenAINearLimitCandidate(fresh, now)
		if !ok {
			continue
		}
		cands = append(cands, cand)
	}
	sortOpenAINearLimitCandidates(cands)
	return cands
}

// trySelectOpenAINearLimitTarget acquires a slot on the best near-limit
// account. Its effective concurrency is capped by
// gateway.openai_near_limit_max_concurrency and one probe lease is held until
// the slot is released, so a 429/slow-first-token backoff takes effect before
// the next request piles onto the same account.
//
// Like the load-aware layer, the winner is re-read from the database before
// the slot is taken, and a proxy-pool account reserves its outbound proxy in
// the same operation.
func (s *OpenAIGatewayService) trySelectOpenAINearLimitTarget(
	ctx context.Context,
	groupID *int64,
	platform string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requireCompact bool,
	requiredCapability OpenAIEndpointCapability,
	accept ...func(*Account) bool,
) *AccountSelectionResult {
	var filter func(*Account) bool
	if len(accept) > 0 {
		filter = accept[0]
	}
	cands := s.openAINearLimitCandidates(ctx, groupID, platform, requestedModel, excludedIDs, requireCompact, requiredCapability, filter)
	if len(cands) == 0 {
		return nil
	}
	now := time.Now()
	lease := openAINearLimitProbeLease(s.cfg)
	for _, cand := range cands {
		acc := s.recheckSelectedOpenAIAccountFromDB(ctx, cand.account, groupID, platform, requestedModel, requireCompact, requiredCapability)
		if acc == nil || (filter != nil && !filter(acc)) {
			continue
		}
		latest, ok := classifyOpenAINearLimitCandidate(acc, now)
		if !ok {
			// The database row is newer than the snapshot (window reset).
			continue
		}
		result, err := s.tryAcquireAccountSlot(ctx, acc.ID, effectiveOpenAIAccountConcurrency(acc, s.cfg), &acc)
		if err != nil || result == nil || !result.Acquired {
			continue
		}
		if !claimOpenAINearLimitPreference(acc.ID, lease) {
			if result.ReleaseFunc != nil {
				result.ReleaseFunc()
			}
			continue
		}
		release := result.ReleaseFunc
		accountID := acc.ID
		wrapped := func() {
			releaseOpenAINearLimitPreference(accountID)
			if release != nil {
				release()
			}
		}
		// newAcquiredSelectionResult releases the slot (and the lease) on error.
		selection, err := s.newAcquiredSelectionResult(ctx, acc, wrapped)
		if err != nil {
			continue
		}
		if sessionHash != "" {
			_ = s.bindOpenAIStickySessionDuringSelection(ctx, groupID, sessionHash, acc.ID)
		}
		logOpenAINearLimitPrioritized(latest)
		return selection
	}
	return nil
}

// trySelectOpenAINearLimitAccount is the *Account (no slot) variant used by
// the legacy selectAccountForModelWithExclusions path. The caller binds the
// sticky session with its own rules.
func (s *OpenAIGatewayService) trySelectOpenAINearLimitAccount(
	ctx context.Context,
	groupID *int64,
	platform string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requireCompact bool,
	requiredCapability OpenAIEndpointCapability,
) *Account {
	cands := s.openAINearLimitCandidates(ctx, groupID, platform, requestedModel, excludedIDs, requireCompact, requiredCapability, func(candidate *Account) bool {
		return s.openAIAccountMatchesSchedulingGroup(candidate, groupID)
	})
	if len(cands) == 0 {
		return nil
	}
	logOpenAINearLimitPrioritized(cands[0])
	return cands[0].account
}
