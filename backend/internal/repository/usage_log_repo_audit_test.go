package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryGetUsageAuditStats(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	mock.ExpectQuery(`(?s)WITH filtered AS \(\s*SELECT usage_audit.*FROM filtered`).
		WillReturnRows(sqlmock.NewRows([]string{
			"total", "audited", "pass", "warning", "anomaly", "unverifiable", "not_applicable", "unknown", "missing",
			"avg_input", "max_input", "avg_output", "max_output",
		}).AddRow(10, 8, 4, 2, 1, 1, 0, 0, 2, 1.5, 12.0, 2.5, 20.0))

	dimensionColumns := []string{
		"key", "name", "total", "pass", "warning", "anomaly", "unverifiable", "not_applicable", "unknown",
		"avg_input", "max_input", "avg_output", "max_output",
	}
	mock.ExpectQuery(`(?s)dimensions AS.*FROM dimensions`).
		WillReturnRows(sqlmock.NewRows(dimensionColumns).AddRow("openai", "openai", 5, 3, 1, 1, 0, 0, 0, 1.0, 5.0, 2.0, 7.0))
	mock.ExpectQuery(`(?s)dimensions AS.*FROM dimensions`).
		WillReturnRows(sqlmock.NewRows(dimensionColumns).AddRow("42", "Primary", 4, 2, 1, 1, 0, 0, 0, 1.5, 6.0, 2.5, 8.0))
	mock.ExpectQuery(`(?s)dimensions AS.*FROM dimensions`).
		WillReturnRows(sqlmock.NewRows(dimensionColumns).AddRow("gpt-5.4", "gpt-5.4", 3, 1, 1, 1, 0, 0, 0, 2.0, 7.0, 3.0, 9.0))

	stats, err := repo.GetUsageAuditStats(context.Background(), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, int64(10), stats.TotalRecords)
	require.Equal(t, int64(8), stats.AuditedRecords)
	require.Equal(t, 80.0, stats.CoveragePct)
	require.Equal(t, int64(1), stats.Counts.Anomaly)
	require.Equal(t, int64(2), stats.Counts.Missing)
	require.Equal(t, 1.5, *stats.AverageInputDeviationPct)
	require.Equal(t, "openai", stats.ByPlatform[0].Key)
	require.Equal(t, "Primary", stats.ByAccount[0].Name)
	require.Equal(t, "gpt-5.4", stats.ByModel[0].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildUsageAuditWhereSupportsStatusAndDateRange(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	whereClause, args := buildUsageAuditWhere(usagestats.UsageLogFilters{
		AccountID:        12,
		UsageAuditStatus: string(service.UsageAuditStatusAnomaly),
		StartTime:        &start,
		EndTime:          &end,
	})

	require.Contains(t, whereClause, "account_id = $1")
	require.Contains(t, whereClause, "usage_audit->>'status' = $2")
	require.Contains(t, whereClause, "created_at >= $3")
	require.Contains(t, whereClause, "created_at < $4")
	require.Equal(t, []any{int64(12), string(service.UsageAuditStatusAnomaly), start, end}, args)

	missingWhere, missingArgs := buildUsageAuditWhere(usagestats.UsageLogFilters{UsageAuditStatus: service.UsageAuditFilterMissing})
	require.Contains(t, missingWhere, "usage_audit IS NULL")
	require.Empty(t, missingArgs)
}

func TestUsageAuditDeviationExpressionGuardsMalformedHistoricalValues(t *testing.T) {
	expression := usageAuditDeviationExpression("usage_audit #>> '{deviation_pct,input_tokens}'")
	require.Contains(t, expression, "CASE WHEN")
	require.Contains(t, expression, "::DOUBLE PRECISION")
	require.Contains(t, expression, "[eE]")
}
