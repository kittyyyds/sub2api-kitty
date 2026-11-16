//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestVideoGroupBindingRejectsCrossPlatformBeforeMutation(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectQuery("SELECT EXISTS").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(mismatch))
		err = validateVideoGroupBindingPlatforms(context.Background(), db, []int64{205}, []int64{25})
		if mismatch {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}
