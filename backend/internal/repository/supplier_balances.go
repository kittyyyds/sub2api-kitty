package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

const supplierBalanceThresholdKey = "supplier_balance_alert_thresholds_v1"

func (r *channelRepository) ReadSupplierBalances(ctx context.Context, page, size int) (*service.SupplierBalanceSnapshot, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	out := &service.SupplierBalanceSnapshot{Rows: []service.SupplierBalanceRow{}, Thresholds: service.SupplierBalanceThresholds{}, Page: page, PageSize: size, QueriedAt: time.Now().UTC()}
	var config string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM settings WHERE key=$1),'{}')`, supplierBalanceThresholdKey).Scan(&config); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(config), &out.Thresholds); err != nil {
		return nil, err
	}
	if err = service.ValidateSupplierBalanceThresholds(out.Thresholds); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE deleted_at IS NULL AND type='apikey'`).Scan(&out.Total); err != nil {
		return nil, err
	}
	// Read only the endpoint hostname input and balance snapshot, never full
	// credentials/extra, OAuth tokens or raw upstream response bodies.
	rows, err := tx.QueryContext(ctx, `SELECT id,name,platform,COALESCE(credentials->>'base_url',''),extra->'upstream_billing_probe'->'balance' FROM accounts WHERE deleted_at IS NULL AND type='apikey' ORDER BY id LIMIT $1 OFFSET $2`, size, (page-1)*size)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var name, platform, base string
		var raw []byte
		if err = rows.Scan(&id, &name, &platform, &base, &raw); err != nil {
			return nil, err
		}
		out.Rows = append(out.Rows, service.BuildSupplierBalanceRow(id, name, platform, base, raw, out.Thresholds, out.QueriedAt))
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}
func (r *channelRepository) SaveSupplierBalanceThresholds(ctx context.Context, v service.SupplierBalanceThresholds) error {
	if err := service.ValidateSupplierBalanceThresholds(v); err != nil {
		return err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=EXCLUDED.updated_at`, supplierBalanceThresholdKey, string(raw))
	return err
}

var _ service.SupplierBalanceRepository = (*channelRepository)(nil)
