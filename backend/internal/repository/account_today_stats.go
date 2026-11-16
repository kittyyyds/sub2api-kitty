package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// GetAccountTodayStatsBatch reads both windows in one account-indexed pass.
// Historical costs retain their recorded account multiplier; group/customer
// pricing and the account's current settings must not revalue past requests.
func (r *usageLogRepository) GetAccountTodayStatsBatch(ctx context.Context, accountIDs []int64, today time.Time) (map[int64]*service.AccountTodayStats, error) {
	result := make(map[int64]*service.AccountTodayStats, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}
	const query = `
		SELECT account_id,
			COUNT(*) FILTER (WHERE created_at >= $2),
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) FILTER (WHERE created_at >= $2), 0),
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)) FILTER (WHERE created_at >= $2), 0),
			COALESCE(SUM(total_cost) FILTER (WHERE created_at >= $2), 0),
			COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $2), 0),
			COUNT(*),
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0),
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0),
			COALESCE(SUM(actual_cost), 0)
		FROM usage_logs
		WHERE account_id = ANY($1)
		GROUP BY account_id`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), today)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var accountID int64
		stats := &service.AccountTodayStats{}
		if err := rows.Scan(&accountID, &stats.Requests, &stats.Tokens, &stats.Cost, &stats.StandardCost, &stats.UserCost, &stats.LifetimeRequests, &stats.LifetimeTokens, &stats.LifetimeCost, &stats.LifetimeUserCost); err != nil {
			return nil, err
		}
		result[accountID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range accountIDs {
		if result[id] == nil {
			result[id] = &service.AccountTodayStats{}
		}
	}
	return result, nil
}
