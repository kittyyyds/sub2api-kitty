package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"reflect"
	"time"
)

func (r *usageLogRepository) ListVideoPricingGroups(ctx context.Context, after int64, limit int) ([]service.VideoPricingGroup, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT id,COALESCE(model_pricing,'[]'::jsonb) FROM groups WHERE platform='video' AND deleted_at IS NULL AND id>$1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.VideoPricingGroup{}
	for rows.Next() {
		var g service.VideoPricingGroup
		var raw []byte
		if err = rows.Scan(&g.ID, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &g.Pricing); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *usageLogRepository) CompareAndSwapVideoPricing(ctx context.Context, id int64, old, next []service.ChannelModelPricing) (bool, error) {
	a, err := json.Marshal(old)
	if err != nil {
		return false, err
	}
	b, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	// Compare normalized known fields after locking the row. Never overwrite an
	// administrator's concurrent change, and never change group membership/status.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(model_pricing,'[]'::jsonb) FROM groups WHERE id=$1 AND platform='video' AND deleted_at IS NULL FOR UPDATE`, id).Scan(&raw)
	if err != nil {
		return false, err
	}
	var current []service.ChannelModelPricing
	if err = json.Unmarshal(raw, &current); err != nil {
		return false, err
	}
	normalized, _ := json.Marshal(current)
	if string(normalized) != string(a) {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE groups SET model_pricing=$2::jsonb,updated_at=NOW() WHERE id=$1`, id, string(b)); err != nil {
		return false, err
	}
	before := map[string]service.ChannelModelPricing{}
	for _, p := range old {
		if len(p.Models) == 1 {
			before[p.Models[0]] = p
		}
	}
	for _, p := range next {
		if p.Video == nil || len(p.Models) != 1 || p.PerRequestPrice == nil {
			continue
		}
		previous, exists := before[p.Models[0]]
		if exists && previous.Video != nil && previous.PerRequestPrice != nil && *previous.PerRequestPrice == *p.PerRequestPrice && previous.Video.ReferencePerSecond == p.Video.ReferencePerSecond && reflect.DeepEqual(previous.Video.ResolutionPrices, p.Video.ResolutionPrices) {
			continue
		}
		oldRaw, _ := json.Marshal(previous)
		newRaw, _ := json.Marshal(p)
		if _, err = tx.ExecContext(ctx, `INSERT INTO kitty_video_price_changes(group_id,model,old_price,new_price) VALUES($1,$2,$3::jsonb,$4::jsonb)`, id, p.Models[0], string(oldRaw), string(newRaw)); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func (r *usageLogRepository) ListVideoPriceChanges(ctx context.Context) ([]service.VideoPriceChange, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT created_at,group_id,model,old_price,new_price FROM kitty_video_price_changes ORDER BY id DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changes := []service.VideoPriceChange{}
	for rows.Next() {
		var change service.VideoPriceChange
		var at time.Time
		var oldRaw, newRaw []byte
		if err = rows.Scan(&at, &change.GroupID, &change.Model, &oldRaw, &newRaw); err != nil {
			return nil, err
		}
		var old, next service.ChannelModelPricing
		if err = json.Unmarshal(oldRaw, &old); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(newRaw, &next); err != nil {
			return nil, err
		}
		change.At = at.UTC().Format(time.RFC3339)
		if old.PerRequestPrice != nil {
			change.Old = *old.PerRequestPrice
		}
		if next.PerRequestPrice != nil {
			change.New = *next.PerRequestPrice
		}
		if old.Video != nil {
			change.OldReference = old.Video.ReferencePerSecond
		}
		if next.Video != nil {
			change.NewReference = next.Video.ReferencePerSecond
		}
		changes = append(changes, change)
	}
	// Existing admin UI shows the latest item last.
	for i, j := 0, len(changes)-1; i < j; i, j = i+1, j-1 {
		changes[i], changes[j] = changes[j], changes[i]
	}
	return changes, rows.Err()
}
