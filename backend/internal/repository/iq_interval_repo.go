package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"time"
)

func (r *scheduledTestPlanRepository) ListDueIQ(ctx context.Context, now time.Time) ([]*service.ScheduledTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,account_id,model_id,cron_expression,enabled,max_results,auto_recover,last_run_at,next_run_at,created_at,updated_at,pelican_config,running_until FROM scheduled_test_plans p WHERE enabled=true AND EXISTS(SELECT 1 FROM accounts a WHERE a.id=account_id AND a.deleted_at IS NULL) AND pelican_config->>'schedule_mode'='completion_interval' AND next_run_at<=$1 AND (running_until IS NULL OR running_until<$1) AND NOT EXISTS (SELECT 1 FROM scheduled_test_plans other WHERE other.account_id=p.account_id AND other.id<>p.id AND other.running_until>$1) ORDER BY next_run_at,id LIMIT 64`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlans(rows)
}
func (r *scheduledTestPlanRepository) FinishIQ(ctx context.Context, p *service.ScheduledTestPlan, until, finished time.Time) error {
	next := finished.Add(time.Duration(p.PelicanConfig.IntervalSeconds) * time.Second)
	_, err := r.db.ExecContext(ctx, `UPDATE scheduled_test_plans SET running_until=NULL,last_run_at=$3,next_run_at=CASE WHEN updated_at=$5 THEN $4 ELSE next_run_at END WHERE id=$1 AND running_until=$2`, p.ID, until, finished, next, p.UpdatedAt)
	return err
}
func (r *scheduledTestPlanRepository) IQSummaries(ctx context.Context, ids []int64) ([]service.IQSummary, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT p.account_id,p.id,p.enabled,p.model_id,p.pelican_config,p.running_until,p.next_run_at,
 h.id,h.status,LEFT(h.response_text,256),LEFT(h.error_message,4096),h.finished_at,h.pelican_config
 FROM scheduled_test_plans p JOIN accounts a ON a.id=p.account_id AND a.deleted_at IS NULL
 LEFT JOIN LATERAL(SELECT id,status,response_text,error_message,finished_at,pelican_config FROM scheduled_test_results WHERE plan_id=p.id ORDER BY id DESC LIMIT 20) h ON true
 WHERE p.account_id=ANY($1) AND p.pelican_config->>'schedule_mode'='completion_interval'
 ORDER BY p.account_id,h.id DESC`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]service.IQSummary, 0)
	indices := map[int64]int{}
	for rows.Next() {
		var s service.IQSummary
		var cfg, rcfg []byte
		var id sql.NullInt64
		var status, answer, message sql.NullString
		var finished sql.NullTime
		if err = rows.Scan(&s.AccountID, &s.PlanID, &s.Enabled, &s.Model, &cfg, &s.RunningUntil, &s.NextRunAt, &id, &status, &answer, &message, &finished, &rcfg); err != nil {
			return nil, err
		}
		index, exists := indices[s.AccountID]
		if !exists {
			var config service.PelicanTestConfig
			if err = json.Unmarshal(cfg, &config); err != nil {
				return nil, err
			}
			s.IntervalSeconds = config.IntervalSeconds
			s.Reasoning = config.ReasoningEffort
			s.Results = []service.IQResult{}
			index = len(out)
			indices[s.AccountID] = index
			out = append(out, s)
		}
		if id.Valid {
			var config service.PelicanTestConfig
			if len(rcfg) > 0 {
				if err = json.Unmarshal(rcfg, &config); err != nil {
					return nil, err
				}
			}
			result := service.IQResult{ID: id.Int64, Verdict: config.IQVerdict, Answer: answer.String, Model: config.ModelID, Reasoning: config.ReasoningEffort, FinishedAt: finished.Time}
			if result.Verdict == "" {
				result.Verdict = "other"
				if status.String != "success" {
					result.Verdict = "error"
				}
			}
			if result.Verdict == "error" {
				result.Error = service.IQErrorSummary(message.String)
			}
			out[index].Results = append(out[index].Results, result)
		}
	}
	return out, rows.Err()
}
