package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

const suanliImageTaskColumns = `id,user_id,api_key_id,account_id,idempotency_hash,request_hash,snapshot,upstream_task_id,status,result,created_at`

func scanSuanliImageTask(row interface{ Scan(...any) error }) (*service.SuanliImageTask, error) {
	t := &service.SuanliImageTask{}
	var snapshot []byte
	if err := row.Scan(&t.ID, &t.UserID, &t.APIKeyID, &t.AccountID, &t.IdempotencyHash, &t.RequestHash, &snapshot, &t.UpstreamTaskID, &t.Status, &t.Result, &t.CreatedAt); err != nil {
		return nil, err
	}
	return t, json.Unmarshal(snapshot, &t.Snapshot)
}
func (r *usageLogRepository) CreateSuanliImageTask(ctx context.Context, t *service.SuanliImageTask) (*service.SuanliImageTask, bool, error) {
	snapshot, err := json.Marshal(t.Snapshot)
	if err != nil {
		return nil, false, err
	}
	res, err := r.sql.ExecContext(ctx, `INSERT INTO image_supplier_tasks(id,user_id,api_key_id,account_id,idempotency_hash,request_hash,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb) ON CONFLICT(api_key_id,idempotency_hash) DO NOTHING`, t.ID, t.UserID, t.APIKeyID, t.AccountID, t.IdempotencyHash, t.RequestHash, string(snapshot))
	if err != nil {
		return nil, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if n == 1 {
		return t, true, nil
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT `+suanliImageTaskColumns+` FROM image_supplier_tasks WHERE api_key_id=$1 AND idempotency_hash=$2 AND user_id=$3`, t.APIKeyID, t.IdempotencyHash, t.UserID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, service.ErrImageTaskNotFound
	}
	old, err := scanSuanliImageTask(rows)
	return old, false, err
}
func (r *usageLogRepository) GetSuanliImageTask(ctx context.Context, id string, userID, keyID int64) (*service.SuanliImageTask, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT `+suanliImageTaskColumns+` FROM image_supplier_tasks WHERE id=$1 AND user_id=$2 AND api_key_id=$3`, id, userID, keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanSuanliImageTask(rows)
}
func (r *usageLogRepository) UpdateSuanliImageTask(ctx context.Context, t *service.SuanliImageTask, delay time.Duration) error {
	_, err := r.sql.ExecContext(ctx, `UPDATE image_supplier_tasks SET upstream_task_id=$2,status=$3,result=$4,next_poll_at=NOW()+($5*INTERVAL '1 second'),updated_at=NOW() WHERE id=$1 AND (status IN ('submitting','pending') OR (status='ready' AND $3 IN ('ready','completed')) OR (status='unknown' AND $3 IN ('pending','ready')))`, t.ID, t.UpstreamTaskID, t.Status, t.Result, int64(delay.Seconds()))
	return err
}
func (r *usageLogRepository) LeaseSuanliImageTasks(ctx context.Context, limit int) ([]*service.SuanliImageTask, error) {
	rows, err := r.sql.QueryContext(ctx, `WITH due AS(SELECT id FROM image_supplier_tasks WHERE status IN ('pending','ready','submitting') AND next_poll_at<=NOW() ORDER BY next_poll_at LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE image_supplier_tasks SET next_poll_at=NOW()+INTERVAL '90 seconds' WHERE id IN(SELECT id FROM due) RETURNING `+suanliImageTaskColumns, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*service.SuanliImageTask{}
	for rows.Next() {
		t, err := scanSuanliImageTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *usageLogRepository) ExpireSuanliImageResults(ctx context.Context) error {
	// Keep task IDs, ownership and immutable prices for audit; only temporary
	// result bytes expire. A small batch avoids long-running cleanup queries.
	_, err := r.sql.ExecContext(ctx, `UPDATE image_supplier_tasks SET result=NULL WHERE id IN(SELECT id FROM image_supplier_tasks WHERE status='completed' AND result IS NOT NULL AND updated_at<NOW()-INTERVAL '24 hours' ORDER BY updated_at LIMIT 20)`)
	return err
}
