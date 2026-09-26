//go:build unit

package repository

import (
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGrokMediaVideoOwnerAccountQueryScopesOwnership(t *testing.T) {
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ars.gateway_request_id = $1")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ar.user_id = $2")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ar.media_api_key_id = $3")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ul.session_id = ar.session_id")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ul.user_id = ar.user_id")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ul.api_key_id = ar.media_api_key_id")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ul.group_id IS NOT DISTINCT FROM $4")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "ul.video_count > 0")
	require.Contains(t, grokMediaVideoOwnerAccountQuery, "'video_generation', 'video_edit', 'video_extension'")
}

func TestFindGrokMediaVideoOwnerAccountID(t *testing.T) {
	tests := []struct {
		name      string
		rows      *sqlmock.Rows
		want      int64
		wantError string
	}{
		{
			name: "unique owner",
			rows: sqlmock.NewRows([]string{"account_id"}).
				AddRow(int64(12286)),
			want: 12286,
		},
		{
			name: "duplicate rows for the same owner remain unambiguous",
			rows: sqlmock.NewRows([]string{"account_id"}).
				AddRow(int64(12286)).
				AddRow(int64(12286)),
			want: 12286,
		},
		{
			name:      "no owner",
			rows:      sqlmock.NewRows([]string{"account_id"}),
			wantError: sql.ErrNoRows.Error(),
		},
		{
			name: "ambiguous owners fail closed",
			rows: sqlmock.NewRows([]string{"account_id"}).
				AddRow(int64(12286)).
				AddRow(int64(12287)),
			wantError: "ambiguous grok video owner account",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
				require.NoError(t, mock.ExpectationsWereMet())
			})
			groupID := int64(24)
			mock.ExpectQuery(regexp.QuoteMeta(grokMediaVideoOwnerAccountQuery)).
				WithArgs("video-request", int64(2), int64(421), groupID).
				WillReturnRows(tt.rows)
			mock.ExpectClose()
			repo := &usageLogRepository{sql: db}

			accountID, lookupErr := repo.FindGrokMediaVideoOwnerAccountID(
				t.Context(), " video-request ", 2, 421, &groupID,
			)
			if tt.wantError != "" {
				require.Error(t, lookupErr)
				require.Contains(t, lookupErr.Error(), tt.wantError)
				require.Zero(t, accountID)
			} else {
				require.NoError(t, lookupErr)
				require.Equal(t, tt.want, accountID)
			}
		})
	}
}

func TestFindGrokMediaVideoOwnerAccountIDSupportsUngroupedUsage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	mock.ExpectQuery(regexp.QuoteMeta(grokMediaVideoOwnerAccountQuery)).
		WithArgs("video-request", int64(2), int64(421), nil).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(12286)))
	mock.ExpectClose()
	repo := &usageLogRepository{sql: db}

	accountID, err := repo.FindGrokMediaVideoOwnerAccountID(t.Context(), "video-request", 2, 421, nil)

	require.NoError(t, err)
	require.Equal(t, int64(12286), accountID)
}

func TestFindGrokMediaVideoOwnerAccountIDPropagatesDatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	dbErr := errors.New("database unavailable")
	mock.ExpectQuery(regexp.QuoteMeta(grokMediaVideoOwnerAccountQuery)).
		WillReturnError(dbErr)
	mock.ExpectClose()
	repo := &usageLogRepository{sql: db}

	accountID, err := repo.FindGrokMediaVideoOwnerAccountID(t.Context(), "video-request", 2, 421, nil)

	require.ErrorIs(t, err, dbErr)
	require.Zero(t, accountID)
}

func TestFindGrokMediaVideoOwnerAccountIDRejectsInvalidScope(t *testing.T) {
	repo := &usageLogRepository{}
	accountID, err := repo.FindGrokMediaVideoOwnerAccountID(t.Context(), "", 2, 421, nil)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Zero(t, accountID)
}
