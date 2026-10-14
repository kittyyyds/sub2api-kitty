package repository

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageLogRepository) ListImageAccountModelBindings(ctx context.Context, id int64) ([]service.VideoAccountModelBinding, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT COALESCE(a.credentials->>'base_url',''),COALESCE(a.credentials->'model_mapping','{}'::jsonb),COALESCE(ag.allowed_models,'[]'::jsonb)
 FROM accounts a JOIN account_groups ag ON ag.account_id=a.id WHERE ag.group_id=$1 AND a.platform='openai' AND a.type='apikey' AND a.deleted_at IS NULL ORDER BY a.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service.VideoAccountModelBinding
	for rows.Next() {
		var b service.VideoAccountModelBinding
		var mapping, allowed []byte
		if err := rows.Scan(&b.BaseURL, &mapping, &allowed); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(mapping, &b.Models); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(allowed, &b.AllowedModels); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (r *usageLogRepository) ListImagePricingGroups(ctx context.Context, after int64, limit int) ([]service.ImagePricingGroup, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT id,name,COALESCE(model_pricing,'[]'::jsonb),COALESCE(image_model_prices,'{}'::jsonb) FROM groups WHERE platform='openai' AND allow_image_generation=true AND deleted_at IS NULL AND id>$1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.ImagePricingGroup{}
	for rows.Next() {
		var g service.ImagePricingGroup
		var p, prices []byte
		if err := rows.Scan(&g.ID, &g.Name, &p, &prices); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(p, &g.Pricing); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(prices, &g.Prices); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func (r *usageLogRepository) CompareAndSwapImagePricing(ctx context.Context, old, next service.ImagePricingGroup) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout='750ms'`); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `SET LOCAL statement_timeout='5s'`); err != nil {
		return false, err
	}
	var raw, prices []byte
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(model_pricing,'[]'::jsonb),COALESCE(image_model_prices,'{}'::jsonb) FROM groups WHERE id=$1 AND platform='openai' AND allow_image_generation=true AND deleted_at IS NULL FOR UPDATE`, old.ID).Scan(&raw, &prices)
	if err != nil {
		return false, err
	}
	var current []service.ChannelModelPricing
	var currentPrices map[string]map[string]float64
	if err = json.Unmarshal(raw, &current); err != nil {
		return false, err
	}
	if err = json.Unmarshal(prices, &currentPrices); err != nil {
		return false, err
	}
	if !reflect.DeepEqual(current, old.Pricing) || !reflect.DeepEqual(currentPrices, old.Prices) {
		return false, nil
	}
	// Preserve unknown JSON fields when updating existing pricing entries.
	var original []map[string]json.RawMessage
	if err = json.Unmarshal(raw, &original); err != nil {
		return false, err
	}
	byModel := map[string]map[string]json.RawMessage{}
	for i, p := range current {
		key, _ := json.Marshal(p.Models)
		byModel[string(key)] = original[i]
	}
	merged := make([]map[string]json.RawMessage, 0, len(next.Pricing))
	for _, p := range next.Pricing {
		var item map[string]json.RawMessage
		b, e := json.Marshal(p)
		if e != nil {
			return false, e
		}
		if e = json.Unmarshal(b, &item); e != nil {
			return false, e
		}
		key, _ := json.Marshal(p.Models)
		{
			for k, v := range byModel[string(key)] {
				if _, ok := item[k]; !ok {
					item[k] = v
				}
			}
		}
		merged = append(merged, item)
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return false, err
	}
	p, err := json.Marshal(next.Prices)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE groups SET model_pricing=$2::jsonb,image_model_prices=$3::jsonb,updated_at=NOW() WHERE id=$1`, old.ID, string(b), string(p)); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
