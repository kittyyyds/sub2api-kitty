package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Preserve Kitty's completed-credit definition, including its existing refund
// semantics. PAY codes are implementation records of orders, not a second credit.
const operationsRechargeCreditsCTE = `WITH credits AS (
 SELECT po.user_id,po.amount,po.completed_at AS at FROM payment_orders po JOIN users u ON u.id=po.user_id WHERE u.role='user' AND po.order_type='balance' AND po.completed_at IS NOT NULL
 UNION ALL SELECT rc.used_by,rc.value,rc.used_at FROM redeem_codes rc JOIN users u ON u.id=rc.used_by WHERE u.role='user' AND rc.status='used' AND rc.type IN ('balance','admin_balance') AND rc.value>0 AND rc.code NOT LIKE 'PAY-%'
) `

// GetOperationsRechargeRanking keeps population totals ahead of the row limit.
func (r *usageLogRepository) GetOperationsRechargeRanking(ctx context.Context, start, end time.Time, limit int) (*service.OperationsRechargeRankingData, error) {
	data := &service.OperationsRechargeRankingData{Users: make([]service.OperationsRechargeRankingEntry, 0)}
	const query = operationsRechargeCreditsCTE + `, per_user AS (
  SELECT u.id,u.email,SUM(c.amount) AS amount,COUNT(*) AS recharge_count
  FROM credits c JOIN users u ON u.id=c.user_id WHERE c.at >= $1 AND c.at < $2 GROUP BY u.id,u.email
 ) SELECT id,email,amount,recharge_count,SUM(amount) OVER() AS total_amount,
 COUNT(*) OVER() AS total_users,SUM(recharge_count) OVER() AS total_count
 FROM per_user ORDER BY amount DESC,id ASC LIMIT $3`
	rows, err := r.sql.QueryContext(ctx, query, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("query operations recharge ranking: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entry service.OperationsRechargeRankingEntry
		if err = rows.Scan(&entry.UserID, &entry.Email, &entry.Amount, &entry.RechargeCount, &data.TotalAmount, &data.TotalUsers, &data.TotalCount); err != nil {
			return nil, fmt.Errorf("scan recharge ranking: %w", err)
		}
		data.Users = append(data.Users, entry)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
