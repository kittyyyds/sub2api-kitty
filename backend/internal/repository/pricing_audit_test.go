package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPricingAuditInventoryOnlyProjectsModelMappings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &channelRepository{db: db}
	mock.ExpectQuery(`SELECT ag.group_id, a.id,\s+CASE WHEN jsonb_typeof\(a.credentials->'model_mapping'\)='object' THEN a.credentials->'model_mapping' ELSE '\{\}'::jsonb END\s+FROM accounts a JOIN account_groups ag ON ag.account_id=a.id\s+WHERE a.deleted_at IS NULL ORDER BY ag.group_id,a.id LIMIT \$1`).WithArgs(1001).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "id", "model_mapping"}).AddRow(2, 5, `{"public-model":"mapped-model"}`).AddRow(3, 6, `{"invalid":123}`))
	rows, err := r.ReadPricingAuditInventory(context.Background(), 1001)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "mapped-model", rows[0].Models["public-model"])
	require.Empty(t, rows[1].Models)
	require.NoError(t, mock.ExpectationsWereMet())
}
