package repository

// Read-only summaries still used by supplier and procurement management.
const providerCashSQL = `WITH selected AS (
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

const providerBalancesSQL = `SELECT jsonb_build_object('users',count(*),
 'positive',COALESCE(sum(balance) FILTER(WHERE balance>=0 AND balance<'Infinity'::numeric),0),
 'negative',COALESCE(sum(balance) FILTER(WHERE balance<0 AND balance>'-Infinity'::numeric),0),
 'invalid',count(*) FILTER(WHERE NOT(balance>'-Infinity'::numeric AND balance<'Infinity'::numeric)),
 'scope','current_non_deleted_users_wallet_includes_bonus_not_revenue') FROM users WHERE deleted_at IS NULL`

const providerMonitoringSQL = `WITH sources AS (
 SELECT 'v3_idle_probe'::text AS source,count(*) AS records,count(cost_usd) AS priced,
 COALESCE(sum(cost_usd) FILTER(WHERE cost_usd>=0 AND cost_usd<'Infinity'::numeric),0) AS recorded_cost,
 count(*) FILTER(WHERE cost_usd IS NULL OR NOT(cost_usd>=0 AND cost_usd<'Infinity'::numeric)) AS unknown,
 min(started_at) AS earliest_retained FROM channel_monitor_v3_probe_results WHERE started_at>=$1 AND started_at<$2
) SELECT jsonb_agg(to_jsonb(s)) FROM sources s`
