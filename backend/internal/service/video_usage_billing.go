package service

import (
	"context"
	"errors"
)

// VideoUsageBillingRepository commits the receipt together with the existing
// billing dedup and money transaction. A retry may repair a historical receipt
// only after its billing fingerprint matches; it never applies money twice.
type VideoUsageBillingRepository interface {
	ApplyVideoUsage(context.Context, *UsageBillingCommand, *UsageLog) (*UsageBillingApplyResult, error)
}

var ErrVideoUsageEvidenceMissing = errors.New("video usage requires its original billing evidence")

func applyVideoUsageBilling(ctx context.Context, log *UsageLog, p *postUsageBillingParams, deps *billingDeps, repo VideoUsageBillingRepository) error {
	cmd := buildUsageBillingCommand(log.RequestID, log, p)
	if cmd == nil || cmd.RequestID == "" || deps == nil || repo == nil {
		return errors.New("atomic video usage billing is unavailable")
	}
	cmd.PrepaidVideoTaskID = p.PrepaidVideoTaskID
	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	result, err := repo.ApplyVideoUsage(billingCtx, cmd, log)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("atomic video usage billing returned no result")
	}
	if !result.Applied {
		deps.deferredService.ScheduleLastUsedUpdate(p.Account.ID)
		return nil
	}
	if result.APIKeyQuotaExhausted {
		if invalidator, ok := p.APIKeyService.(apiKeyAuthCacheInvalidator); ok && p.APIKey != nil {
			if key := p.APIKey.AuthCacheInvalidationKey(); key != "" {
				invalidator.InvalidateAuthCacheByKey(billingCtx, key)
			}
		}
	}
	finalizePostUsageBilling(billingCtx, p, deps, result)
	return nil
}
