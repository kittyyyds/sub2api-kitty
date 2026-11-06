package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

const operationsGroupsSQL = operationsUsageCTE + `, grouped AS (
 SELECT CASE WHEN COALESCE(ul.group_id,0)=0 THEN 0 WHEN g.id IS NULL OR g.deleted_at IS NOT NULL THEN -2 ELSE g.id END AS group_id,
 sum(ul.actual_cost) AS user_cost,COALESCE(sum(ul.operations_cost),0) AS upstream_cost,
 count(*) FILTER(WHERE ul.operations_cost IS NULL) AS unknown_cost,count(*) AS requests
 FROM operations_usage ul JOIN users u ON u.id=ul.user_id LEFT JOIN groups g ON g.id=ul.group_id
 WHERE u.role='user' AND ($3::bigint=0 OR ul.user_id=$3) GROUP BY 1
), entries AS (
 SELECT g.id AS group_id,g.name AS group_name,g.platform,g.rate_multiplier,
 COALESCE(ug.user_cost,0) AS user_cost,COALESCE(ug.upstream_cost,0) AS upstream_cost,COALESCE(ug.unknown_cost,0) AS unknown_cost
 FROM groups g LEFT JOIN grouped ug ON ug.group_id=g.id WHERE g.deleted_at IS NULL AND ($3::bigint=0 OR ug.requests>0)
 UNION ALL SELECT group_id,CASE WHEN group_id=0 THEN 'Unassigned' ELSE 'Deleted groups' END,'custom',1,user_cost,upstream_cost,unknown_cost FROM grouped WHERE group_id IN(0,-2)
) SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY group_id),'[]') FROM entries e`

const operationsModelsSQL = operationsUsageCTE + ` SELECT COALESCE(jsonb_agg(to_jsonb(m) ORDER BY user_cost DESC,model),'[]') FROM (
 SELECT COALESCE(NULLIF(trim(ul.requested_model),''),ul.model) AS model,
 sum(ul.actual_cost) AS user_cost,COALESCE(sum(ul.operations_cost),0) AS upstream_cost,count(*) FILTER(WHERE ul.operations_cost IS NULL) AS unknown_cost,
 array_agg(DISTINCT COALESCE(NULLIF(g.platform,''),a.platform,'custom')) AS platforms
 FROM operations_usage ul JOIN users u ON u.id=ul.user_id LEFT JOIN groups g ON g.id=ul.group_id LEFT JOIN accounts a ON a.id=ul.account_id
 WHERE u.role='user' AND ($3::bigint=0 OR ul.user_id=$3)
 AND ($4::bigint=0 OR ($4=-1 AND COALESCE(ul.group_id,0)=0) OR ($4=-2 AND COALESCE(ul.group_id,0)<>0 AND (g.id IS NULL OR g.deleted_at IS NOT NULL)) OR ul.group_id=$4)
 GROUP BY 1) m`

func (r *usageLogRepository) operationsJSON(ctx context.Context, q string, args []any, out any) error {
	var raw []byte
	if e := scanSingleRow(ctx, r.sql, q, args, &raw); e != nil {
		return e
	}
	return json.Unmarshal(raw, out)
}

func (r *usageLogRepository) GetOperationsDashboardData(ctx context.Context, todayStart, todayEnd, rangeStart, rangeEnd, trendStart, trendEnd time.Time, userTimezone string, groupID int64) (*service.OperationsDashboardData, error) {
	data := &service.OperationsDashboardData{}
	if e := r.loadOperationsMetrics(ctx, data, todayStart, todayEnd, rangeStart, rangeEnd); e != nil {
		return nil, e
	}
	e := r.operationsJSON(ctx, operationsUsageCTE+`SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY date),'[]') FROM (
 SELECT to_char(ul.created_at AT TIME ZONE $3,'YYYY-MM-DD') AS date,COALESCE(sum(ul.operations_cost),0) AS upstream_cost,sum(ul.actual_cost) AS user_cost,
 CASE WHEN count(*) FILTER(WHERE ul.operations_cost IS NULL)=0 THEN sum(ul.actual_cost-ul.operations_cost) END AS profit
 FROM operations_usage ul JOIN users u ON u.id=ul.user_id WHERE u.role='user' GROUP BY 1)t`, []any{trendStart, trendEnd, userTimezone}, &data.Trend)
	if e != nil {
		return nil, e
	}
	if e = r.operationsJSON(ctx, operationsGroupsSQL, []any{rangeStart, rangeEnd, int64(0)}, &data.Groups); e != nil {
		return nil, e
	}
	if e = r.operationsJSON(ctx, operationsModelsSQL, []any{rangeStart, rangeEnd, int64(0), groupID}, &data.Models); e != nil {
		return nil, e
	}
	return data, nil
}
