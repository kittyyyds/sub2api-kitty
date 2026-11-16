package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func intelligenceScheduleKey(id int64) string {
	return fmt.Sprintf("%s%d", service.AccountIntelligenceSchedulePrefix, id)
}
func (r *accountRepository) IntelligenceScheduleGet(ctx context.Context, id int64) (*service.AccountIntelligenceSchedule, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT value FROM settings WHERE key=$1`, intelligenceScheduleKey(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var raw string
	if err = rows.Scan(&raw); err != nil {
		return nil, err
	}
	var p service.AccountIntelligenceSchedule
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, err
	}
	p.AccountID = id
	return &p, nil
}
func (r *accountRepository) IntelligenceScheduleSave(ctx context.Context, p *service.AccountIntelligenceSchedule, revision int64) (bool, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return false, err
	}
	var result sql.Result
	if revision == 0 {
		result, err = r.sql.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW()) ON CONFLICT(key) DO NOTHING`, intelligenceScheduleKey(p.AccountID), string(raw))
	} else {
		result, err = r.sql.ExecContext(ctx, `UPDATE settings SET value=$2,updated_at=NOW() WHERE key=$1 AND (value::jsonb->>'revision')::bigint=$3`, intelligenceScheduleKey(p.AccountID), string(raw), revision)
	}
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (r *accountRepository) IntelligenceSchedules(ctx context.Context) ([]service.AccountIntelligenceSchedule, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT a.id,s.value FROM settings s JOIN accounts a ON s.key=$1||a.id::text WHERE a.deleted_at IS NULL AND a.platform IN ('openai','anthropic')`, service.AccountIntelligenceSchedulePrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := []service.AccountIntelligenceSchedule{}
	for rows.Next() {
		var id int64
		var raw string
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var p service.AccountIntelligenceSchedule
		if json.Unmarshal([]byte(raw), &p) != nil || !p.Enabled || p.NextRunAt == nil {
			continue
		}
		p.AccountID = id
		plans = append(plans, p)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].NextRunAt.Before(*plans[j].NextRunAt) })
	return plans, rows.Err()
}
func (r *accountRepository) IntelligenceScheduleClaim(ctx context.Context, p *service.AccountIntelligenceSchedule, now, next time.Time) (bool, error) {
	if !p.Enabled || p.NextRunAt == nil || p.NextRunAt.After(now) {
		return false, nil
	}
	old, err := json.Marshal(p)
	if err != nil {
		return false, err
	}
	patch, err := json.Marshal(map[string]any{"next_run_at": next, "last_attempt_at": now, "last_error": ""})
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `UPDATE settings SET value=(value::jsonb || $3::jsonb)::text,updated_at=NOW() WHERE key=$1 AND value::jsonb=$2::jsonb`, intelligenceScheduleKey(p.AccountID), string(old), string(patch))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (r *accountRepository) IntelligenceScheduleError(ctx context.Context, p *service.AccountIntelligenceSchedule, attempt time.Time, message string) error {
	patch, _ := json.Marshal(map[string]string{"last_error": message})
	// A stale completion must not overwrite a user save or a newer run.
	_, err := r.sql.ExecContext(ctx, `UPDATE settings SET value=(value::jsonb || $4::jsonb)::text WHERE key=$1 AND (value::jsonb->>'revision')::bigint=$2 AND (value::jsonb->>'last_attempt_at')::timestamptz=$3`, intelligenceScheduleKey(p.AccountID), p.Revision, attempt, string(patch))
	return err
}
func (r *accountRepository) IntelligencePruneScheduled(ctx context.Context, id int64, keep int) error {
	if keep < 1 || keep > 1000 {
		return fmt.Errorf("invalid retention")
	}
	_, err := r.sql.ExecContext(ctx, `DELETE FROM account_intelligence_tests WHERE id IN (SELECT id FROM account_intelligence_tests WHERE account_id=$1 AND payload->>'source'='scheduled' AND status<>'running' ORDER BY created_at DESC,id DESC OFFSET $2)`, id, keep)
	return err
}

var _ service.AccountIntelligenceScheduleRepository = (*accountRepository)(nil)
