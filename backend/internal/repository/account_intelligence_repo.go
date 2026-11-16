package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *accountRepository) expireIntelligence(ctx context.Context, ids []int64) error {
	_, err := r.sql.ExecContext(ctx, `UPDATE account_intelligence_tests SET status='error',finished_at=NOW(),payload=payload || jsonb_build_object('status','error','error','检测执行中断或超时，请重新测试。','finished_at',NOW()) WHERE account_id=ANY($1) AND status='running' AND created_at<NOW()-INTERVAL '6 minutes'`, pq.Array(ids))
	return err
}
func (r *accountRepository) IntelligenceFind(ctx context.Context, account int64, request string) (*service.AccountIntelligenceTest, error) {
	if err := r.expireIntelligence(ctx, []int64{account}); err != nil {
		return nil, err
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT payload FROM account_intelligence_tests WHERE account_id=$1 AND request_id=$2`, account, request)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var raw []byte
	if err = rows.Scan(&raw); err != nil {
		return nil, err
	}
	var item service.AccountIntelligenceTest
	err = json.Unmarshal(raw, &item)
	return &item, err
}
func (r *accountRepository) IntelligenceClaim(ctx context.Context, item *service.AccountIntelligenceTest) (*service.AccountIntelligenceTest, bool, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, false, err
	}
	rows, err := r.sql.QueryContext(ctx, `INSERT INTO account_intelligence_tests(id,account_id,request_id,status,created_at,payload) VALUES($1,$2,$3,'running',$4,$5) ON CONFLICT(account_id,request_id) DO UPDATE SET request_id=EXCLUDED.request_id RETURNING payload,(xmax=0)`, item.ID, item.AccountID, item.RequestID, item.CreatedAt, string(raw))
	if err != nil {
		var pe *pq.Error
		if errors.As(err, &pe) && pe.Code == "23505" {
			return nil, false, service.ErrAccountIntelligenceBusy
		}
		return nil, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	var data []byte
	var inserted bool
	if err = rows.Scan(&data, &inserted); err != nil {
		return nil, false, err
	}
	var saved service.AccountIntelligenceTest
	err = json.Unmarshal(data, &saved)
	return &saved, inserted, err
}
func (r *accountRepository) IntelligenceFinish(ctx context.Context, item *service.AccountIntelligenceTest) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = r.sql.ExecContext(ctx, `UPDATE account_intelligence_tests SET status=$2,finished_at=$3,payload=$4 WHERE id=$1 AND status='running'`, item.ID, item.Status, item.FinishedAt, string(raw))
	return err
}
func (r *accountRepository) IntelligenceHistory(ctx context.Context, ids []int64, limit, offset int) ([]service.AccountIntelligenceTest, error) {
	if err := r.expireIntelligence(ctx, ids); err != nil {
		return nil, err
	}
	// Limit each account before the combined result so large account lists remain bounded.
	rows, err := r.sql.QueryContext(ctx, `SELECT recent.payload FROM unnest($1::bigint[]) AS a(id) CROSS JOIN LATERAL (SELECT payload,created_at FROM account_intelligence_tests WHERE account_id=a.id ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3) AS recent ORDER BY recent.created_at DESC`, pq.Array(ids), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.AccountIntelligenceTest{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item service.AccountIntelligenceTest
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

var _ service.AccountIntelligenceRepository = (*accountRepository)(nil)
