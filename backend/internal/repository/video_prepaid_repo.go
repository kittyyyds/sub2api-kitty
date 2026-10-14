package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Creation and the balance hold share a transaction. The locked user row also
// serializes concurrent admissions across multiple gateway processes.
func (r *usageLogRepository) CreatePrepaidVideoTask(ctx context.Context, t *service.VideoTask, limit int) (*service.VideoTask, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if err = lockVideoBonusGrants(ctx, tx, t.UserID); err != nil {
		return nil, false, err
	}
	var user int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR UPDATE`, t.UserID).Scan(&user); err != nil {
		return nil, false, err
	}
	if t.IdempotencyKey != "" {
		existing, err := scanVideoTask(tx.QueryRowContext(ctx, `SELECT `+videoTaskColumns+` FROM video_tasks WHERE api_key_id=$1 AND idempotency_key=$2`, t.APIKeyID, t.IdempotencyKey))
		if err == nil {
			var hash string
			if err = tx.QueryRowContext(ctx, `SELECT request_hash FROM video_tasks WHERE id=$1`, existing.ID).Scan(&hash); err != nil {
				return nil, false, err
			}
			if hash != t.RequestHash {
				return nil, false, service.ErrUsageBillingRequestConflict
			}
			return existing, false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, err
		}
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM video_tasks WHERE user_id=$1 AND status IN ('submitting','pending','unknown','needs_review')`, t.UserID).Scan(&active); err != nil {
		return nil, false, err
	}
	if active >= limit {
		return nil, false, errors.New("进行中的视频任务已达到并发上限")
	}
	raw, err := json.Marshal(t.Pending)
	if err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO video_tasks(id,user_id,api_key_id,group_id,account_id,subscription_id,pending,idempotency_key,request_hash,hold_amount,billing_state)
	VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,NULLIF($8,''),$9,$10,'held')`, t.ID, t.UserID, t.APIKeyID, t.GroupID, t.AccountID, t.SubscriptionID, string(raw), t.IdempotencyKey, t.RequestHash, t.Pending.PrepaidAmount)
	if err != nil {
		return nil, false, err
	}
	var quota, used float64
	if err = tx.QueryRowContext(ctx, `SELECT quota,quota_used FROM api_keys WHERE id=$1 AND user_id=$2 AND status='active' AND deleted_at IS NULL AND (expires_at IS NULL OR expires_at>NOW()) FOR UPDATE`, t.APIKeyID, t.UserID).Scan(&quota, &used); err != nil {
		return nil, false, err
	}
	var keyHolds float64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(hold_amount),0) FROM video_tasks WHERE api_key_id=$1 AND billing_state IN ('held','refund_pending') AND id<>$2`, t.APIKeyID, t.ID).Scan(&keyHolds); err != nil {
		return nil, false, err
	}
	if quota > 0 && used+keyHolds+t.Pending.PrepaidAmount > quota {
		return nil, false, errors.New("视频预计费用超过密钥剩余额度（包含在途任务）")
	}
	var withinWindows bool
	if err = tx.QueryRowContext(ctx, `SELECT
	 (rate_limit_5h<=0 OR (CASE WHEN window_5h_start IS NULL OR window_5h_start+INTERVAL '5 hours'<=NOW() THEN 0 ELSE usage_5h END)+$2<=rate_limit_5h)
	 AND (rate_limit_1d<=0 OR (CASE WHEN window_1d_start IS NULL OR window_1d_start+INTERVAL '1 day'<=NOW() THEN 0 ELSE usage_1d END)+$2<=rate_limit_1d)
	 AND (rate_limit_7d<=0 OR (CASE WHEN window_7d_start IS NULL OR window_7d_start+INTERVAL '7 days'<=NOW() THEN 0 ELSE usage_7d END)+$2<=rate_limit_7d)
	 FROM api_keys WHERE id=$1`, t.APIKeyID, keyHolds+t.Pending.PrepaidAmount).Scan(&withinWindows); err != nil {
		return nil, false, err
	}
	if !withinWindows {
		return nil, false, errors.New("视频预计费用超过密钥周期额度（包含在途任务）")
	}
	billing := &usageBillingRepository{db: r.db, bonus: newRechargeBonusRepository(r.db)}
	_, err = billing.reserveUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: t.UserID, APIKeyID: t.APIKeyID, BatchID: "video:" + t.ID, HoldAmount: t.Pending.PrepaidAmount})
	if err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return t, true, nil
}

// The task's held state is the receipt proving a reservation happened. Locking
// it makes failure/refund and successful capture mutually exclusive.
func (r *usageLogRepository) RefundVideoTask(ctx context.Context, id, message string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var user, key int64
	var amount float64
	var state string
	if err = tx.QueryRowContext(ctx, `SELECT user_id FROM video_tasks WHERE id=$1`, id).Scan(&user); err != nil {
		return err
	}
	if err = lockVideoBonusGrants(ctx, tx, user); err != nil {
		return err
	}
	var lockedUser int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, user).Scan(&lockedUser); err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT user_id,api_key_id,hold_amount,billing_state FROM video_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&user, &key, &amount, &state)
	if err != nil {
		return err
	}
	if state == "refunded" {
		return nil
	}
	if state != "held" && state != "refund_pending" {
		return errors.New("视频任务没有有效的预扣记录")
	}
	billing := &usageBillingRepository{db: r.db, bonus: newRechargeBonusRepository(r.db)}
	_, err = billing.captureUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: user, APIKeyID: key, BatchID: "video:" + id, HoldAmount: amount, ActualAmount: 0})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE video_tasks SET billing_state='refunded',status='failed',last_error=$2,next_poll_at=NULL,finished_at=NOW(),updated_at=NOW() WHERE id=$1`, id, message)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Ordinary usage and batch holds lock grants before updating the user balance.
// Keep the same order, including grants whose funds are already frozen.
func lockVideoBonusGrants(ctx context.Context, tx *sql.Tx, userID int64) error {
	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout='3s'`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM recharge_bonus_grants WHERE user_id=$1 ORDER BY expires_at ASC NULLS LAST,id ASC FOR UPDATE`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *usageLogRepository) MarkVideoRefundPending(ctx context.Context, id string) error {
	_, err := r.sql.ExecContext(ctx, `UPDATE video_tasks SET billing_state='refund_pending',next_poll_at=NOW(),updated_at=NOW() WHERE id=$1 AND billing_state='held'`, id)
	return err
}

func (r *usageLogRepository) PrepaidVideoReferenceHolds(ctx context.Context) (map[string]time.Time, error) {
	var legacy bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM video_tasks WHERE billing_state='legacy' AND status IN ('submitting','pending','unknown','needs_review') AND NOT (pending ? 'reference_ids'))`).Scan(&legacy); err != nil {
		return nil, err
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT pending->'reference_ids',finished_at FROM video_tasks
	WHERE (finished_at IS NULL OR finished_at>NOW()-INTERVAL '8 days')
	AND jsonb_array_length(COALESCE(pending->'reference_ids','[]'::jsonb))>0 LIMIT 20001`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	if legacy {
		out["legacy-untracked"] = time.Time{}
	}
	count := 0
	for rows.Next() {
		count++
		if count > 20000 {
			return nil, errors.New("素材引用扫描超出单次上限，保留现有素材")
		}
		var raw []byte
		var done sql.NullTime
		if err = rows.Scan(&raw, &done); err != nil {
			return nil, err
		}
		var ids []string
		if err = json.Unmarshal(raw, &ids); err != nil {
			return nil, err
		}
		for _, id := range ids {
			old, exists := out[id]
			if exists && old.IsZero() {
				continue
			}
			if !done.Valid {
				out[id] = time.Time{}
			} else if !exists || old.Before(done.Time) {
				out[id] = done.Time
			}
		}
	}
	return out, rows.Err()
}
