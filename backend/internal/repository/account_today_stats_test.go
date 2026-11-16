package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAccountTodayStatsBatchReadsBothWindowsAndEmptyAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &usageLogRepository{sql: db}
	today := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT account_id,.*FILTER.*FROM usage_logs.*WHERE account_id = ANY`).
		WithArgs("{7,8}", today).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "requests", "tokens", "cost", "standard", "user", "lifetime_requests", "lifetime_tokens", "lifetime_cost", "lifetime_user_cost"}).
			AddRow(7, 2, 200, 0.6, 1.0, 0.2, 90, 12000, 31.5, 14.2))
	stats, err := r.GetAccountTodayStatsBatch(context.Background(), []int64{7, 8}, today)
	require.NoError(t, err)
	require.Equal(t, int64(200), stats[7].Tokens)
	require.Equal(t, int64(12000), stats[7].LifetimeTokens)
	require.Equal(t, 31.5, stats[7].LifetimeCost)
	require.Equal(t, int64(90), stats[7].LifetimeRequests)
	require.Equal(t, 14.2, stats[7].LifetimeUserCost)
	require.Zero(t, stats[8].LifetimeCost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountTodayStatsBatchReturnsDatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT account_id,`).WillReturnError(errors.New("query timeout"))
	r := &usageLogRepository{sql: db}
	_, err = r.GetAccountTodayStatsBatch(context.Background(), []int64{7}, time.Now())
	require.ErrorContains(t, err, "query timeout")
	require.NoError(t, mock.ExpectationsWereMet())
}
