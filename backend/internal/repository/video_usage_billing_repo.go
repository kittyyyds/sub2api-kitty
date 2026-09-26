package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.VideoUsageBillingRepository = (*usageBillingRepository)(nil)

// ApplyVideoUsage makes a successful video receipt a condition of committing
// the charge. Existing dedup keys (including the archive) verify the original
// fingerprint before permitting receipt repair, without applying money again.
func (r *usageBillingRepository) ApplyVideoUsage(ctx context.Context, cmd *service.UsageBillingCommand, log *service.UsageLog) (*service.UsageBillingApplyResult, error) {
	if r == nil || r.db == nil || cmd == nil || log == nil || log.VideoCount <= 0 ||
		strings.TrimSpace(log.RequestID) == "" || log.RequestID != cmd.RequestID ||
		log.APIKeyID != cmd.APIKeyID || log.UserID != cmd.UserID || log.AccountID != cmd.AccountID {
		return nil, errors.New("invalid atomic video usage billing input")
	}
	cmd.Normalize()
	log.ActualCost = service.QuantizeUsageBillingAmount(log.ActualCost)
	log.TotalCost = service.QuantizeUsageBillingAmount(log.TotalCost)
	if service.QuantizeUsageBillingAmount(log.ActualCost) != cmd.BalanceCost+cmd.SubscriptionCost {
		return nil, errors.New("video receipt does not match the billing amount")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	applied, err := r.claimUsageBillingKey(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	result := &service.UsageBillingApplyResult{Applied: applied}
	if applied {
		if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
			return nil, err
		}
	}
	// A repaired receipt belongs to the original charge date, not today's retry.
	if err := tx.QueryRowContext(ctx, `SELECT MIN(created_at) FROM (
		SELECT created_at FROM usage_billing_dedup WHERE request_id=$1 AND api_key_id=$2
		UNION ALL SELECT created_at FROM usage_billing_dedup_archive WHERE request_id=$1 AND api_key_id=$2
	) billed`, cmd.RequestID, cmd.APIKeyID).Scan(&log.CreatedAt); err != nil {
		return nil, err
	}
	billedAt := log.CreatedAt
	if !applied {
		// Recovery observed the charge, not the original response latency.
		log.DurationMs, log.FirstTokenMs = nil, nil
	}
	inserted, err := (&usageLogRepository{sql: tx}).createSingle(ctx, tx, log)
	if err != nil {
		return nil, err
	}
	if !inserted {
		// Older code wrote actual_cost=0 after a failed charge. Correct only that
		// exact receipt after the dedup fingerprint has proved the successful bill.
		// Paid rows and rows with different identity/price are never overwritten.
		if _, err := tx.ExecContext(ctx, `UPDATE usage_logs SET actual_cost=$3,
			rate_multiplier=$4, billing_mode=$5, video_count=$6,
			video_resolution=$7, video_duration_seconds=$8, created_at=$15
			WHERE request_id=$1 AND api_key_id=$2 AND actual_cost=0 AND total_cost=$9
			AND user_id=$10 AND account_id=$11 AND model=$12 AND video_count>0
			AND group_id IS NOT DISTINCT FROM $13 AND subscription_id IS NOT DISTINCT FROM $14`,
			cmd.RequestID, cmd.APIKeyID, log.ActualCost, log.RateMultiplier, log.BillingMode,
			log.VideoCount, log.VideoResolution, log.VideoDurationSeconds, log.TotalCost,
			log.UserID, log.AccountID, log.Model, log.GroupID, log.SubscriptionID, billedAt); err != nil {
			return nil, err
		}
		var matches bool
		if err := tx.QueryRowContext(ctx, `SELECT actual_cost=$3 AND total_cost=$4
			AND user_id=$5 AND account_id=$6 AND model=$7 AND video_count>0
			AND group_id IS NOT DISTINCT FROM $8 AND subscription_id IS NOT DISTINCT FROM $9, created_at
			FROM usage_logs WHERE request_id=$1 AND api_key_id=$2`, cmd.RequestID, cmd.APIKeyID,
			log.ActualCost, log.TotalCost, log.UserID, log.AccountID, log.Model, log.GroupID, log.SubscriptionID).Scan(&matches, &log.CreatedAt); err != nil {
			return nil, err
		}
		if !matches {
			return nil, service.ErrUsageBillingRequestConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
