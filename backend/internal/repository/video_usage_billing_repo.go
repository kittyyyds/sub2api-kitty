package repository

import (
	"context"
	"database/sql"
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
	return r.applyAtomicMediaUsage(ctx, cmd, log)
}

func (r *usageBillingRepository) ApplyImageUsage(ctx context.Context, cmd *service.UsageBillingCommand, log *service.UsageLog) (*service.UsageBillingApplyResult, error) {
	if r == nil || r.db == nil || cmd == nil || log == nil || log.ImageCount != 1 || log.VideoCount != 0 || !strings.HasPrefix(log.RequestID, "simg_") || log.RequestID != cmd.RequestID || log.APIKeyID != cmd.APIKeyID || log.UserID != cmd.UserID || log.AccountID != cmd.AccountID || cmd.PrepaidVideoTaskID != "" {
		return nil, errors.New("invalid atomic image usage billing input")
	}
	return r.applyAtomicMediaUsage(ctx, cmd, log)
}

func (r *usageBillingRepository) applyAtomicMediaUsage(ctx context.Context, cmd *service.UsageBillingCommand, log *service.UsageLog) (*service.UsageBillingApplyResult, error) {
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
		// New unified tasks already moved their charge into frozen_balance.
		// Capture it and write the usage receipt in this same transaction.
		var taskID, state string
		var hold float64
		captureErr := sql.ErrNoRows
		if cmd.PrepaidVideoTaskID != "" {
			if e := lockVideoBonusGrants(ctx, tx, cmd.UserID); e != nil {
				return nil, e
			}
			var lockedUser int64
			if e := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, cmd.UserID).Scan(&lockedUser); e != nil {
				return nil, e
			}
			captureErr = tx.QueryRowContext(ctx, `SELECT id,billing_state,hold_amount FROM video_tasks
			WHERE id=$1 AND upstream_task_id=$2 AND api_key_id=$3 AND user_id=$4 AND billing_state<>'legacy' FOR UPDATE`,
				cmd.PrepaidVideoTaskID, strings.TrimPrefix(cmd.RequestID, "grok-video:"), cmd.APIKeyID, cmd.UserID).Scan(&taskID, &state, &hold)
			if errors.Is(captureErr, sql.ErrNoRows) {
				return nil, errors.New("video prepayment evidence missing")
			}
		}
		originalBalanceCost := cmd.BalanceCost
		if captureErr == nil {
			if state != "held" {
				return nil, errors.New("video prepayment is not available for settlement")
			}
			captured, e := r.captureUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: cmd.UserID, APIKeyID: cmd.APIKeyID, BatchID: "video:" + taskID, HoldAmount: hold, ActualAmount: cmd.BalanceCost})
			if e != nil {
				return nil, e
			}
			result.NewBalance = captured.NewBalance
			if _, e = tx.ExecContext(ctx, `UPDATE video_tasks SET billing_state='captured',finished_at=NOW(),updated_at=NOW() WHERE id=$1`, taskID); e != nil {
				return nil, e
			}
			cmd.BalanceCost = 0
		} else if !errors.Is(captureErr, sql.ErrNoRows) {
			return nil, captureErr
		}
		if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
			cmd.BalanceCost = originalBalanceCost
			return nil, err
		}
		cmd.BalanceCost = originalBalanceCost
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
