package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCandyHistoryCompletionIsAtomic(t *testing.T) {
	for _, failPrune := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback on prune failure"}[failPrune], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE kitty_candy_test_runs SET status").WithArgs(int64(11), "passed", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			prune := mock.ExpectExec("DELETE FROM kitty_candy_test_runs").WithArgs(int64(2), 10)
			if failPrune {
				prune.WillReturnError(errors.New("database unavailable"))
				mock.ExpectRollback()
			} else {
				prune.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = (&CandyTestService{db: db}).finishRun(context.Background(), 11, 2, CandyTestRun{Status: "passed", Answer: "21"})
			if failPrune {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
