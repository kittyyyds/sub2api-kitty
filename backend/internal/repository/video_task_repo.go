package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.VideoTaskRepository = (*usageLogRepository)(nil)

func (r *usageLogRepository) CreateVideoTask(ctx context.Context, t *service.VideoTask) error {
	body, err := json.Marshal(t.Pending)
	if err != nil {
		return err
	}
	_, err = r.sql.ExecContext(ctx, `INSERT INTO video_tasks
        (id,user_id,api_key_id,group_id,account_id,subscription_id,pending)
        VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, t.ID, t.UserID, t.APIKeyID, t.GroupID, t.AccountID, t.SubscriptionID, string(body))
	return err
}

func (r *usageLogRepository) AcceptVideoTask(ctx context.Context, t *service.VideoTask) error {
	body, err := json.Marshal(t.Pending)
	if err != nil {
		return err
	}
	res, err := r.sql.ExecContext(ctx, `UPDATE video_tasks SET upstream_task_id=$2,pending=$3::jsonb,
        status='pending',next_poll_at=NOW()+INTERVAL '30 seconds',updated_at=NOW()
        WHERE id=$1 AND status IN ('submitting','unknown')`, t.ID, t.UpstreamTaskID, string(body))
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("video task acceptance state conflict")
	}
	return err
}

const videoTaskColumns = `id,COALESCE(upstream_task_id,''),user_id,api_key_id,group_id,account_id,
    subscription_id,pending,status,attempts,created_at,billing_state`

func scanVideoTask(row interface{ Scan(...any) error }) (*service.VideoTask, error) {
	t := &service.VideoTask{}
	var body []byte
	var sub sql.NullInt64
	if err := row.Scan(&t.ID, &t.UpstreamTaskID, &t.UserID, &t.APIKeyID, &t.GroupID, &t.AccountID,
		&sub, &body, &t.Status, &t.Attempts, &t.CreatedAt, &t.BillingState); err != nil {
		return nil, err
	}
	if sub.Valid {
		t.SubscriptionID = &sub.Int64
	}
	if err := json.Unmarshal(body, &t.Pending); err != nil {
		return nil, fmt.Errorf("decode video billing snapshot: %w", err)
	}
	return t, nil
}

func (r *usageLogRepository) FindVideoTask(ctx context.Context, id string, userID, keyID int64) (*service.VideoTask, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT `+videoTaskColumns+` FROM video_tasks
        WHERE (upstream_task_id=$1 OR id=$1) AND user_id=$2 AND api_key_id=$3`, id, userID, keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanVideoTask(rows)
}

func (r *usageLogRepository) LeaseVideoTasks(ctx context.Context, limit int) ([]*service.VideoTask, error) {
	// The lease expires after crashes; no paid submission is retried here.
	rows, err := r.sql.QueryContext(ctx, `WITH due AS (
        SELECT id FROM video_tasks WHERE (status='pending' OR billing_state='refund_pending') AND next_poll_at<=NOW()
        ORDER BY next_poll_at LIMIT $1 FOR UPDATE SKIP LOCKED
    ) UPDATE video_tasks SET next_poll_at=NOW()+INTERVAL '3 minutes',attempts=attempts+1,
        updated_at=NOW() WHERE id IN (SELECT id FROM due) RETURNING `+videoTaskColumns, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]*service.VideoTask, 0)
	for rows.Next() {
		t, err := scanVideoTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *usageLogRepository) SetVideoTaskState(ctx context.Context, id, status, message string) error {
	if len(message) > 512 {
		message = message[:512]
	}
	// Terminal state never regresses when a customer poll races the worker;
	// a done task with conflicting billing evidence may require manual review.
	_, err := r.sql.ExecContext(ctx, `UPDATE video_tasks SET status=$2,last_error=$3,
        next_poll_at=CASE WHEN $2='pending' THEN NOW()+INTERVAL '30 seconds' ELSE NULL END,
		finished_at=CASE WHEN $2 IN ('done','failed') THEN COALESCE(finished_at,NOW()) ELSE finished_at END,
        updated_at=NOW() WHERE id=$1 AND (status NOT IN ('done','failed')
        OR (status='done' AND $2='needs_review'))`, id, status, message)
	return err
}

func (r *usageLogRepository) VideoTaskWasBilled(ctx context.Context, requestID string, keyID int64) (bool, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT EXISTS (
        SELECT 1 FROM usage_billing_dedup WHERE request_id=$1 AND api_key_id=$2
        UNION ALL SELECT 1 FROM usage_billing_dedup_archive WHERE request_id=$1 AND api_key_id=$2
    )`, requestID, keyID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	var exists bool
	err = rows.Scan(&exists)
	return exists, err
}

// A zero-cost failed-billing placeholder has a positive quoted total. It is not
// a completed receipt. Explicitly free videos have a zero total or multiplier.
func (r *usageLogRepository) VideoTaskUsageRecorded(ctx context.Context, requestID string, keyID int64) (bool, error) {
	var recorded bool
	err := scanSingleRow(ctx, r.sql, `SELECT EXISTS (SELECT 1 FROM usage_logs
		WHERE request_id=$1 AND api_key_id=$2 AND video_count>0
		AND (actual_cost>0 OR total_cost=0 OR rate_multiplier=0))`, []any{requestID, keyID}, &recorded)
	return recorded, err
}
