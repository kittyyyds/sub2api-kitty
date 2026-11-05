package repository

import (
	"context"
	"database/sql"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (r *channelRepository) ReadPermanentUsage(ctx context.Context) (*service.PermanentUsageSnapshot, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	out := &service.PermanentUsageSnapshot{QueriedAt: time.Now().UTC()}
	if err = tx.QueryRowContext(ctx, `SELECT to_jsonb(s) FROM kitty_permanent_usage_state s WHERE singleton`).Scan(&out.State); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT jsonb_build_object('requests',count(*),'input_tokens',COALESCE(sum(input_tokens),0),'output_tokens',COALESCE(sum(output_tokens),0),
 'cache_creation_tokens',COALESCE(sum(cache_creation_tokens),0),'cache_read_tokens',COALESCE(sum(cache_read_tokens),0),
 'recorded_valuation',COALESCE(sum(actual_cost),0),'standard_valuation',COALESCE(sum(total_cost),0),
 'subscription_valuation',COALESCE(sum(actual_cost) FILTER(WHERE billing_type=1),0),'invalid_records',count(*) FILTER(WHERE invalid_metrics),
 'oldest_observation',min(observed_at),'newest_observation',max(observed_at)) FROM kitty_permanent_usage`).Scan(&out.Totals); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}
func (r *channelRepository) BackfillPermanentUsage(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx, `SELECT kitty_backfill_permanent_usage(500)`).Scan(&n)
	return n, err
}
