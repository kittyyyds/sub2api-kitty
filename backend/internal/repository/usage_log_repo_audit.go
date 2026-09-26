package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const usageAuditDimensionLimit = 50

const usageAuditDecimalPattern = "'^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]+)?$'"

// JSON evidence is persisted from multiple versions of the gateway. Guard the
// cast so one malformed historical value cannot break the whole audit report.
func usageAuditDeviationExpression(path string) string {
	return fmt.Sprintf("CASE WHEN (%s) ~ %s THEN (%s)::DOUBLE PRECISION END", path, usageAuditDecimalPattern, path)
}

func (r *usageLogRepository) GetUsageAuditStats(ctx context.Context, filters usagestats.UsageLogFilters) (*service.UsageAuditStats, error) {
	whereClause, args := buildUsageAuditWhere(filters)
	stats := &service.UsageAuditStats{}
	var avgInput, maxInput, avgOutput, maxOutput sql.NullFloat64
	inputDeviation := usageAuditDeviationExpression("usage_audit #>> '{deviation_pct,input_tokens}'")
	outputDeviation := usageAuditDeviationExpression("usage_audit #>> '{deviation_pct,output_tokens}'")

	summaryQuery := fmt.Sprintf(`
		WITH filtered AS (
			SELECT usage_audit
			FROM usage_logs
			%s
		)
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE usage_audit IS NOT NULL),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'pass'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'warning'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'anomaly'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'unverifiable'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'not_applicable'),
			COUNT(*) FILTER (
				WHERE usage_audit IS NOT NULL
				  AND COALESCE(usage_audit->>'status', '') NOT IN ('pass', 'warning', 'anomaly', 'unverifiable', 'not_applicable')
			),
			COUNT(*) FILTER (WHERE usage_audit IS NULL),
			AVG(ABS(%s)),
			MAX(ABS(%s)),
			AVG(ABS(%s)),
			MAX(ABS(%s))
		FROM filtered
	`, whereClause, inputDeviation, inputDeviation, outputDeviation, outputDeviation)
	if err := scanSingleRow(
		ctx,
		r.sql,
		summaryQuery,
		args,
		&stats.TotalRecords,
		&stats.AuditedRecords,
		&stats.Counts.Pass,
		&stats.Counts.Warning,
		&stats.Counts.Anomaly,
		&stats.Counts.Unverifiable,
		&stats.Counts.NotApplicable,
		&stats.Counts.Unknown,
		&stats.Counts.Missing,
		&avgInput,
		&maxInput,
		&avgOutput,
		&maxOutput,
	); err != nil {
		return nil, err
	}
	stats.AverageInputDeviationPct = usageAuditNullableFloat64Ptr(avgInput)
	stats.MaximumInputDeviationPct = usageAuditNullableFloat64Ptr(maxInput)
	stats.AverageOutputDeviationPct = usageAuditNullableFloat64Ptr(avgOutput)
	stats.MaximumOutputDeviationPct = usageAuditNullableFloat64Ptr(maxOutput)
	if stats.TotalRecords > 0 {
		stats.CoveragePct = float64(stats.AuditedRecords) / float64(stats.TotalRecords) * 100
	}

	var err error
	stats.ByPlatform, err = r.queryUsageAuditDimensions(ctx, whereClause, args,
		"COALESCE("+usageLogEffectivePlatformExpr+", 'unknown')",
		"COALESCE("+usageLogEffectivePlatformExpr+", 'unknown')",
		"LEFT JOIN groups g ON g.id = f.group_id LEFT JOIN accounts a ON a.id = f.account_id",
	)
	if err != nil {
		return nil, err
	}
	stats.ByAccount, err = r.queryUsageAuditDimensions(ctx, whereClause, args,
		"COALESCE(f.account_id::TEXT, 'unknown')",
		"COALESCE(NULLIF(a.name, ''), CASE WHEN f.account_id IS NULL THEN 'Unknown account' ELSE 'Account #' || f.account_id::TEXT END)",
		"LEFT JOIN accounts a ON a.id = f.account_id",
	)
	if err != nil {
		return nil, err
	}
	stats.ByModel, err = r.queryUsageAuditDimensions(ctx, whereClause, args,
		"COALESCE(NULLIF(TRIM(f.requested_model), ''), NULLIF(TRIM(f.model), ''), 'unknown')",
		"COALESCE(NULLIF(TRIM(f.requested_model), ''), NULLIF(TRIM(f.model), ''), 'unknown')",
		"",
	)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *usageLogRepository) queryUsageAuditDimensions(
	ctx context.Context,
	whereClause string,
	args []any,
	keyExpression string,
	nameExpression string,
	joins string,
) (result []service.UsageAuditDimensionStats, err error) {
	inputDeviation := usageAuditDeviationExpression("usage_audit #>> '{deviation_pct,input_tokens}'")
	outputDeviation := usageAuditDeviationExpression("usage_audit #>> '{deviation_pct,output_tokens}'")
	query := fmt.Sprintf(`
		WITH filtered AS (
			SELECT *
			FROM usage_logs
			%s
		), dimensions AS (
			SELECT
				%s AS dimension_key,
				%s AS dimension_name,
				f.usage_audit
			FROM filtered f
			%s
			WHERE f.usage_audit IS NOT NULL
		)
		SELECT
			dimension_key,
			dimension_name,
			COUNT(*),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'pass'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'warning'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'anomaly'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'unverifiable'),
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'not_applicable'),
			COUNT(*) FILTER (
				WHERE COALESCE(usage_audit->>'status', '') NOT IN ('pass', 'warning', 'anomaly', 'unverifiable', 'not_applicable')
			),
			AVG(ABS(%s)),
			MAX(ABS(%s)),
			AVG(ABS(%s)),
			MAX(ABS(%s))
		FROM dimensions
		GROUP BY dimension_key, dimension_name
		ORDER BY
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'anomaly') DESC,
			COUNT(*) FILTER (WHERE usage_audit->>'status' = 'warning') DESC,
			COUNT(*) DESC,
			dimension_name ASC
		LIMIT %d
	`, whereClause, keyExpression, nameExpression, joins, inputDeviation, inputDeviation, outputDeviation, outputDeviation, usageAuditDimensionLimit)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	result = make([]service.UsageAuditDimensionStats, 0)
	for rows.Next() {
		var item service.UsageAuditDimensionStats
		var avgInput, maxInput, avgOutput, maxOutput sql.NullFloat64
		if err = rows.Scan(
			&item.Key,
			&item.Name,
			&item.Total,
			&item.Counts.Pass,
			&item.Counts.Warning,
			&item.Counts.Anomaly,
			&item.Counts.Unverifiable,
			&item.Counts.NotApplicable,
			&item.Counts.Unknown,
			&avgInput,
			&maxInput,
			&avgOutput,
			&maxOutput,
		); err != nil {
			return nil, err
		}
		item.AverageInputDeviationPct = usageAuditNullableFloat64Ptr(avgInput)
		item.MaximumInputDeviationPct = usageAuditNullableFloat64Ptr(maxInput)
		item.AverageOutputDeviationPct = usageAuditNullableFloat64Ptr(avgOutput)
		item.MaximumOutputDeviationPct = usageAuditNullableFloat64Ptr(maxOutput)
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func buildUsageAuditWhere(filters usagestats.UsageLogFilters) (string, []any) {
	conditions := make([]string, 0, 12)
	args := make([]any, 0, 12)
	appendID := func(column string, value int64) {
		if value <= 0 {
			return
		}
		conditions = append(conditions, fmt.Sprintf("%s = $%d", column, len(args)+1))
		args = append(args, value)
	}
	appendID("user_id", filters.UserID)
	appendID("api_key_id", filters.APIKeyID)
	appendID("account_id", filters.AccountID)
	appendID("group_id", filters.GroupID)
	if requestID := strings.TrimSpace(filters.RequestID); requestID != "" {
		conditions = append(conditions, fmt.Sprintf("request_id = $%d", len(args)+1))
		args = append(args, requestID)
	}
	conditions, args = appendUsageLogModelWhereCondition(conditions, args, filters.Model, filters.ModelFilterSource)
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
	}
	conditions, args = appendUsageLogBillingModeWhereCondition(conditions, args, filters.BillingMode)
	conditions, args = appendUsageAuditStatusWhereCondition(conditions, args, filters.UsageAuditStatus, "")
	if filters.UpstreamModelMismatch != nil {
		conditions = append(conditions, upstreamModelMismatchCondition("upstream_model_mismatch", *filters.UpstreamModelMismatch))
	}
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
	}
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *filters.EndTime)
	}
	return buildWhere(conditions), args
}

func usageAuditNullableFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}
