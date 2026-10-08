package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageLogRepository) ListVideoTaskSummaries(ctx context.Context, f service.VideoTaskListFilter) ([]service.VideoTaskSummary, error) {
	if err := f.ValidateScope(); err != nil {
		return nil, err
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 10000 {
		return nil, fmt.Errorf("invalid video task list scope")
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT jsonb_build_object(
 'id',v.id,'task_id',CASE WHEN v.billing_state<>'legacy' THEN COALESCE(NULLIF(v.upstream_task_id,''),v.id) ELSE COALESCE(v.upstream_task_id,'') END,'user_id',v.user_id,
 'billing_state',v.billing_state,'hold_amount',v.hold_amount,
 'api_key_id',v.api_key_id,'group_id',v.group_id,'account_id',v.account_id,
 'key_name',COALESCE(k.name,''),'group_name',COALESCE(g.name,''),
 'model',COALESCE(v.pending->>'model',''),'status',v.status,
 'resolution',COALESCE(v.pending->>'video_resolution',''),
 'seconds',COALESCE((v.pending->>'video_duration_seconds')::int,0),
 'price_snapshot',v.pending->'price_snapshot','usage_id',u.id,'actual_cost',u.actual_cost,
 'usage_recorded',COALESCE(u.video_count>0 AND (u.actual_cost>0 OR u.total_cost=0 OR u.rate_multiplier=0),false),
 'billed',EXISTS(SELECT 1 FROM usage_billing_dedup d WHERE d.api_key_id=v.api_key_id AND d.request_id='grok-video:'||v.upstream_task_id
   UNION ALL SELECT 1 FROM usage_billing_dedup_archive d WHERE d.api_key_id=v.api_key_id AND d.request_id='grok-video:'||v.upstream_task_id),
 'created_at',v.created_at,'updated_at',v.updated_at)
 FROM (SELECT * FROM video_tasks WHERE `+videoTaskFilterSQL+`
   ORDER BY created_at DESC,id DESC LIMIT $8 OFFSET $9) v
 LEFT JOIN api_keys k ON k.id=v.api_key_id LEFT JOIN groups g ON g.id=v.group_id
 LEFT JOIN LATERAL (SELECT id,actual_cost,total_cost,rate_multiplier,video_count FROM usage_logs
   WHERE api_key_id=v.api_key_id AND request_id='grok-video:'||v.upstream_task_id
   AND created_at>=v.created_at-interval '5 minutes' ORDER BY created_at DESC LIMIT 1) u ON true
 ORDER BY v.created_at DESC,v.id DESC`, append(videoTaskFilterArgs(f), f.Limit, f.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]service.VideoTaskSummary, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item service.VideoTaskSummary
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if !f.Admin {
			item.AccountID = 0
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Both the task page and its aggregate use the same half-open created_at range.
// No price configuration, credentials or historical snapshots are mutated.
const videoTaskFilterSQL = `($1::bigint=0 OR user_id=$1)
 AND ($2::bigint=0 OR api_key_id=$2) AND ($3::text='' OR status=$3)
 AND ($4::bigint=0 OR group_id=$4) AND ($5::text='' OR pending->>'model'=$5)
 AND created_at >= $6 AND created_at < $7`

func videoTaskFilterArgs(f service.VideoTaskListFilter) []any {
	return []any{f.UserID, f.APIKeyID, f.Status, f.GroupID, f.Model, f.StartAt, f.EndAt}
}

func (r *usageLogRepository) GetVideoTaskStats(ctx context.Context, f service.VideoTaskListFilter) (*service.VideoTaskStats, error) {
	if err := f.ValidateScope(); err != nil {
		return nil, err
	}
	rows, err := r.sql.QueryContext(ctx, `WITH matched AS (
 SELECT v.*, COALESCE(v.pending->>'model','') AS model,
   COALESCE(u.video_count>0 AND (u.actual_cost>0 OR u.total_cost=0 OR u.rate_multiplier=0),false) AS usage_recorded,
   COALESCE(u.actual_cost,0) AS recorded_cost,
   EXISTS(SELECT 1 FROM usage_billing_dedup d WHERE d.api_key_id=v.api_key_id AND d.request_id='grok-video:'||v.upstream_task_id
     UNION ALL SELECT 1 FROM usage_billing_dedup_archive d WHERE d.api_key_id=v.api_key_id AND d.request_id='grok-video:'||v.upstream_task_id) AS billed
 FROM (SELECT * FROM video_tasks WHERE `+videoTaskFilterSQL+`) v
 LEFT JOIN LATERAL (SELECT actual_cost,total_cost,rate_multiplier,video_count FROM usage_logs
   WHERE api_key_id=v.api_key_id AND request_id='grok-video:'||v.upstream_task_id
   AND created_at>=v.created_at-interval '5 minutes' ORDER BY created_at DESC,id DESC LIMIT 1) u ON true
 ) SELECT GROUPING(model)=1 AS is_total, jsonb_build_object(
   'model',COALESCE(model,''),'total',count(*),
   'succeeded',count(*) FILTER (WHERE status='done'),
   'failed',count(*) FILTER (WHERE status='failed'),
   'in_progress',count(*) FILTER (WHERE status IN ('submitting','pending')),
   'needs_review',count(*) FILTER (WHERE status IN ('unknown','needs_review')),
   'success_rate',100.0 * count(*) FILTER (WHERE status='done') / NULLIF(count(*) FILTER (WHERE status IN ('done','failed')),0),
   'successful_seconds',COALESCE(sum(COALESCE((pending->>'video_duration_seconds')::int,0)) FILTER (WHERE status='done'),0),
   'actual_cost',COALESCE(sum(recorded_cost) FILTER (WHERE billed AND usage_recorded),0),
   'unreconciled',count(*) FILTER (WHERE (billed AND NOT usage_recorded) OR (status='done' AND NOT (billed AND usage_recorded))),
   'average_completion_seconds',avg(GREATEST(EXTRACT(EPOCH FROM updated_at-created_at),0)) FILTER (WHERE status='done')
 ) FROM matched GROUP BY GROUPING SETS ((),(model))
 ORDER BY GROUPING(model) DESC, count(*) DESC, model`, videoTaskFilterArgs(f)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &service.VideoTaskStats{Models: make([]service.VideoTaskModelStats, 0), StartAt: f.StartAt, EndAt: f.EndAt}
	for rows.Next() {
		var total bool
		var raw []byte
		if err := rows.Scan(&total, &raw); err != nil {
			return nil, err
		}
		if total {
			if err := json.Unmarshal(raw, &result.VideoTaskMetrics); err != nil {
				return nil, err
			}
		} else {
			var model service.VideoTaskModelStats
			if err := json.Unmarshal(raw, &model); err != nil {
				return nil, err
			}
			result.Models = append(result.Models, model)
		}
	}
	return result, rows.Err()
}
