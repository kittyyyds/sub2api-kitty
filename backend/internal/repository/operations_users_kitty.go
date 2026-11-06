package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (r *usageLogRepository) GetOperationsUserSuggestions(ctx context.Context, search string) ([]service.OperationsUserSuggestion, error) {
	out := []service.OperationsUserSuggestion{}
	if search == "" {
		return out, nil
	}
	e := r.operationsJSON(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(u)),'[]') FROM (SELECT id AS user_id,email FROM users WHERE role='user' AND deleted_at IS NULL AND POSITION(LOWER($1) IN LOWER(email))>0 ORDER BY email LIMIT 20)u`, []any{search}, &out)
	return out, e
}
func (r *usageLogRepository) GetOperationsUsersData(ctx context.Context, rangeStart, rangeEnd time.Time, search string, userID int64) (*service.OperationsUsersData, error) {
	out := &service.OperationsUsersData{Users: []service.OperationsUserSpendStat{}}
	e := r.operationsJSON(ctx, operationsUsageCTE+`, spend AS (
 SELECT user_id,sum(actual_cost) AS user_cost,COALESCE(sum(operations_cost),0) AS upstream_cost,count(*) FILTER(WHERE operations_cost IS NULL) AS unknown_cost FROM operations_usage GROUP BY user_id
 ) SELECT COALESCE(jsonb_agg(to_jsonb(v) ORDER BY user_cost DESC,user_id),'[]') FROM (
 SELECT u.id AS user_id,u.email,COALESCE(s.user_cost,0) AS user_cost,COALESCE(s.upstream_cost,0) AS upstream_cost,COALESCE(s.unknown_cost,0) AS unknown_cost
 FROM users u LEFT JOIN spend s ON s.user_id=u.id WHERE u.role='user' AND u.deleted_at IS NULL AND ($3='' OR POSITION(LOWER($3) IN LOWER(u.email))>0)
 ORDER BY user_cost DESC,u.id LIMIT CASE WHEN $3='' THEN 25 ELSE 100 END)v`, []any{rangeStart, rangeEnd, search}, &out.Users)
	if e != nil {
		return nil, e
	}
	if userID <= 0 {
		return out, nil
	}
	detail := &service.OperationsUserDetail{UserID: userID}
	e = scanSingleRow(ctx, r.sql, `SELECT COALESCE((SELECT email FROM users WHERE id=$1 AND role='user' AND deleted_at IS NULL),'')`, []any{userID}, &detail.Email)
	if e != nil {
		return nil, e
	}
	if detail.Email == "" {
		return out, nil
	}
	if e = r.operationsJSON(ctx, operationsGroupsSQL, []any{rangeStart, rangeEnd, userID}, &detail.Groups); e != nil {
		return nil, e
	}
	if e = r.operationsJSON(ctx, operationsModelsSQL, []any{rangeStart, rangeEnd, userID, int64(0)}, &detail.Models); e != nil {
		return nil, e
	}
	out.Detail = detail
	return out, nil
}
