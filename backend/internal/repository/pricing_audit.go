package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *channelRepository) ReadPricingAuditInventory(ctx context.Context, limit int) ([]service.PricingAuditInventory, error) {
	// Select one non-secret JSON field, never SELECT credentials or account DTOs.
	rows, err := r.db.QueryContext(ctx, `SELECT ag.group_id, a.id,
	 CASE WHEN jsonb_typeof(a.credentials->'model_mapping')='object' THEN a.credentials->'model_mapping' ELSE '{}'::jsonb END
	 FROM accounts a JOIN account_groups ag ON ag.account_id=a.id
	 WHERE a.deleted_at IS NULL ORDER BY ag.group_id,a.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.PricingAuditInventory{}
	for rows.Next() {
		var item service.PricingAuditInventory
		var raw []byte
		if err := rows.Scan(&item.GroupID, &item.AccountID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item.Models); err != nil {
			item.Models = nil
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
