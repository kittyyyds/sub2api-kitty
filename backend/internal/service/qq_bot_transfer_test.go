package service

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestQQBotTransferableBalancePreservesGrantRestrictions(t *testing.T) {
	for _, tc := range []struct{ balance, grants, want string }{
		{"10.00000000", "3.00000000", "7.00000000"},
		{"2.00000000", "3.00000000", "0.00000000"},
		{"0.00000003", "0.00000001", "0.00000002"},
	} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		tx, err := db.Begin()
		require.NoError(t, err)
		mock.ExpectQuery("SELECT COALESCE\\(SUM\\(remaining_amount\\),0\\)::text").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"amount"}).AddRow(tc.grants))
		got, err := qqBotTransferableBalance(context.Background(), tx, 1, tc.balance)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}
