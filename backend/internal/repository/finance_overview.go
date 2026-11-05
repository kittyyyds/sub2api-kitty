package repository

import (
	"context"
	"database/sql"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

const financeUsageSQL = `SELECT jsonb_build_object('records',count(*),
 'valuation',COALESCE(sum(actual_cost) FILTER(WHERE actual_cost>=0 AND actual_cost<'Infinity'::numeric),0),
 'invalid_records',count(*) FILTER(WHERE NOT(actual_cost>=0 AND actual_cost<'Infinity'::numeric)),
 'subscription_valuation',COALESCE(sum(actual_cost) FILTER(WHERE billing_type=1 AND actual_cost>=0 AND actual_cost<'Infinity'::numeric),0),
 'source','retained_usage_logs_actual_cost_not_cash_or_proven_debits') FROM usage_logs WHERE created_at>=$1 AND created_at<$2`

// Never relabel users.total_recharged (which includes affiliate transfers) as
// cash. Currency is an immutable order snapshot or explicitly unknown. Refund
// amounts are wallet credit units, not gateway cash; net cash is not inferred.
const financeCashSQL = `WITH selected AS (
 SELECT CASE WHEN upper(trim(provider_snapshot->>'currency')) ~ '^[A-Z]{3}$' THEN upper(trim(provider_snapshot->>'currency')) ELSE 'UNKNOWN' END AS currency,
 order_type,pay_amount,paid_at,refund_at,refund_amount,status
 FROM payment_orders WHERE (paid_at>=$1 AND paid_at<$2) OR (refund_at>=$1 AND refund_at<$2)
), grouped AS (
 SELECT currency,order_type,
 count(*) FILTER(WHERE paid_at>=$1 AND paid_at<$2) AS paid_orders,
 COALESCE(sum(pay_amount) FILTER(WHERE paid_at>=$1 AND paid_at<$2 AND pay_amount>=0 AND pay_amount<'Infinity'::numeric),0) AS received,
 count(*) FILTER(WHERE paid_at>=$1 AND paid_at<$2 AND NOT(pay_amount>=0 AND pay_amount<'Infinity'::numeric)) AS invalid_receipts,
 count(*) FILTER(WHERE refund_at>=$1 AND refund_at<$2 AND status IN ('REFUNDED','PARTIALLY_REFUNDED')) AS refunded_orders,
 COALESCE(sum(refund_amount) FILTER(WHERE refund_at>=$1 AND refund_at<$2 AND status IN ('REFUNDED','PARTIALLY_REFUNDED') AND refund_amount>=0 AND refund_amount<'Infinity'::numeric),0) AS refunded_credits
 FROM selected GROUP BY currency,order_type
) SELECT COALESCE(jsonb_agg(to_jsonb(g) ORDER BY currency,order_type),'[]') FROM grouped g`

const financeBalancesSQL = `SELECT jsonb_build_object('users',count(*),
 'positive',COALESCE(sum(balance) FILTER(WHERE balance>=0 AND balance<'Infinity'::numeric),0),
 'negative',COALESCE(sum(balance) FILTER(WHERE balance<0 AND balance>'-Infinity'::numeric),0),
 'invalid',count(*) FILTER(WHERE NOT(balance>'-Infinity'::numeric AND balance<'Infinity'::numeric)),
 'scope','current_non_deleted_users_wallet_includes_bonus_not_revenue') FROM users WHERE deleted_at IS NULL`

const financeCreditsSQL = `SELECT jsonb_build_object(
 'affiliate_transfers',COALESCE((SELECT sum(amount) FROM user_affiliate_ledger WHERE action='transfer' AND created_at>=$1 AND created_at<$2),0),
 'recharge_bonus_granted',COALESCE((SELECT sum(amount) FROM recharge_bonus_ledger WHERE entry_type='grant' AND created_at>=$1 AND created_at<$2),0),
 'recharge_bonus_revoked',COALESCE((SELECT sum(amount) FROM recharge_bonus_ledger WHERE entry_type='revoke' AND created_at>=$1 AND created_at<$2),0),
 'manual_credits_complete',false)`

const financeMonitoringSQL = `WITH sources AS (
 SELECT 'v3_idle_probe'::text AS source,count(*) AS records,count(cost_usd) AS priced,
 COALESCE(sum(cost_usd) FILTER(WHERE cost_usd>=0 AND cost_usd<'Infinity'::numeric),0) AS recorded_cost,
 count(*) FILTER(WHERE cost_usd IS NULL OR NOT(cost_usd>=0 AND cost_usd<'Infinity'::numeric)) AS unknown,
 min(started_at) AS earliest_retained FROM channel_monitor_v3_probe_results WHERE started_at>=$1 AND started_at<$2
 UNION ALL
 SELECT 'pelican_group_result',count(*),count(cost_usd),
 COALESCE(sum(cost_usd) FILTER(WHERE cost_usd>=0 AND cost_usd<'Infinity'::numeric),0),
 count(*) FILTER(WHERE cost_usd IS NULL OR cost_incomplete OR NOT(cost_usd>=0 AND cost_usd<'Infinity'::numeric)),
 min(created_at) FROM pelican_group_test_results WHERE created_at>=$1 AND created_at<$2
) SELECT jsonb_agg(to_jsonb(s)) FROM sources s`

func (r *channelRepository) ReadFinanceOverview(ctx context.Context, from, until time.Time) (*service.FinanceOverview, error) {
	if !service.ValidateFinanceRange(from, until) {
		return nil, service.ErrUpstreamOperationsValidation
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	out := &service.FinanceOverview{From: from, Until: until, QueriedAt: time.Now().UTC()}
	if err = tx.QueryRowContext(ctx, financeUsageSQL, from, until).Scan(&out.Usage); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, financeCashSQL, from, until).Scan(&out.Cash); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, financeBalancesSQL).Scan(&out.Balances); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, financeCreditsSQL, from, until).Scan(&out.Credits); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, financeMonitoringSQL, from, until).Scan(&out.Monitoring); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}
