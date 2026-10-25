package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"time"
)

type retainedMediaRepository struct{ db *sql.DB }

func NewRetainedMediaRepository(db *sql.DB) service.RetainedMediaRepository {
	return &retainedMediaRepository{db}
}

const retainedColumns = `id::text,user_id,source_key,sha256,kind,name,mime_type,bytes,status,created_at,expires_at`

func scanRetained(row interface{ Scan(...any) error }) (*service.RetainedMedia, error) {
	a := &service.RetainedMedia{}
	e := row.Scan(&a.ID, &a.UserID, &a.SourceKey, &a.SHA256, &a.Kind, &a.Name, &a.MimeType, &a.Bytes, &a.Status, &a.CreatedAt, &a.ExpiresAt)
	return a, e
}
func (r *retainedMediaRepository) Get(ctx context.Context, user int64, id string) (*service.RetainedMedia, error) {
	return scanRetained(r.db.QueryRowContext(ctx, `SELECT `+retainedColumns+` FROM kitty_retained_media WHERE id=$1 AND user_id=$2`, id, user))
}
func (r *retainedMediaRepository) BySource(ctx context.Context, user int64, source string) (*service.RetainedMedia, error) {
	return scanRetained(r.db.QueryRowContext(ctx, `SELECT `+retainedColumns+` FROM kitty_retained_media WHERE source_key=$1 AND user_id=$2`, source, user))
}
func (r *retainedMediaRepository) Create(ctx context.Context, a *service.RetainedMedia) (*service.RetainedMedia, error) {
	stored, e := scanRetained(r.db.QueryRowContext(ctx, `INSERT INTO kitty_retained_media(id,user_id,source_key,sha256,kind,name,mime_type,bytes)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(user_id,source_key) DO NOTHING RETURNING `+retainedColumns, a.ID, a.UserID, a.SourceKey, a.SHA256, a.Kind, a.Name, a.MimeType, a.Bytes))
	if errors.Is(e, sql.ErrNoRows) {
		stored, e = r.BySource(ctx, a.UserID, a.SourceKey)
	}
	if e == nil && stored.SHA256 != a.SHA256 {
		return nil, service.ErrStorageChanged
	}
	return stored, e
}
func (r *retainedMediaRepository) List(ctx context.Context, user int64, offset int) ([]service.RetainedMedia, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT `+retainedColumns+` FROM kitty_retained_media WHERE user_id=$1 ORDER BY created_at DESC,id LIMIT 100 OFFSET $2`, user, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []service.RetainedMedia{}
	for rows.Next() {
		a, e := scanRetained(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

const receiptColumns = `order_id,asset_id::text,days,amount::text,before_expiry,after_expiry,created_at`

func scanStorageReceipt(row interface{ Scan(...any) error }) (*service.StorageReceipt, error) {
	o := &service.StorageReceipt{}
	e := row.Scan(&o.OrderID, &o.AssetID, &o.Days, &o.Amount, &o.Before, &o.After, &o.CreatedAt)
	return o, e
}
func (r *retainedMediaRepository) Orders(ctx context.Context, user int64, offset int) ([]service.StorageReceipt, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT `+receiptColumns+` FROM kitty_media_storage_orders WHERE user_id=$1 ORDER BY created_at DESC,order_id LIMIT 100 OFFSET $2`, user, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []service.StorageReceipt{}
	for rows.Next() {
		o, e := scanStorageReceipt(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}
func (r *retainedMediaRepository) Extend(ctx context.Context, user int64, id string, cmd service.StorageExtension) (*service.StorageReceipt, error) {
	if _, e := uuid.Parse(cmd.RequestID); e != nil {
		return nil, service.ErrStorageInvalid
	}
	if cmd.Days < 1 || cmd.Days > 365 || cmd.ExpectedExpiry.IsZero() {
		return nil, service.ErrStorageInvalid
	}
	key := service.MediaWalletKey{UserID: user, OrderID: fmt.Sprintf("storage-%d-%s", user, cmd.RequestID)}
	wallet := &mediaWalletRepository{r.db}
	// Keep existing bonus -> user lock ordering, then asset. Cleaner only locks assets.
	tx, e := wallet.lock(ctx, key, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	prior, e := scanStorageReceipt(tx.QueryRowContext(ctx, `SELECT `+receiptColumns+` FROM kitty_media_storage_orders WHERE order_id=$1 AND user_id=$2`, key.OrderID, user))
	if e == nil {
		if prior.AssetID != id || prior.Days != cmd.Days || !prior.Before.Equal(cmd.ExpectedExpiry) {
			return nil, service.ErrStorageChanged
		}
		return prior, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	a, e := scanRetained(tx.QueryRowContext(ctx, `SELECT `+retainedColumns+` FROM kitty_retained_media WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, user))
	if e != nil {
		return nil, e
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return nil, e
	}
	if a.Status != "active" || !a.ExpiresAt.After(now) {
		return nil, service.ErrStorageExpired
	}
	if !a.ExpiresAt.Equal(cmd.ExpectedExpiry) {
		return nil, service.ErrStorageChanged
	}
	rate := decimal.RequireFromString("0.01")
	if a.Kind == "video" {
		rate = decimal.RequireFromString("0.10")
	}
	amount := rate.Mul(decimal.NewFromInt(int64(cmd.Days))).StringFixed(8)
	after := a.ExpiresAt.Add(time.Duration(cmd.Days) * 24 * time.Hour)
	evidence := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s|%s", key.OrderID, id, cmd.Days, a.ExpiresAt.UTC().Format(time.RFC3339Nano), amount))))
	quote, _ := json.Marshal(map[string]any{"quote_id": evidence, "snapshot_id": evidence, "currency": "USD", "denomination": "cny_pegged_site_credit", "cny_per_usd_credit": "1", "sell_usd_credit": amount, "service": "media_storage_extension", "asset_id": id, "days": cmd.Days, "before": a.ExpiresAt, "after": after})
	contract, _ := json.Marshal(map[string]string{"schema": "kitty-media-wallet-quote-v1", "snapshot_id": evidence, "rate_per_day": rate.StringFixed(2)})
	_, created, e := wallet.reserveAtomic(ctx, service.MediaWalletReserve{Key: key, AmountUSD: amount, RequestHash: evidence, Quote: quote, PriceContract: contract}, tx)
	if e != nil {
		return nil, e
	}
	if !created {
		return nil, service.ErrStorageChanged
	}
	// Local storage fulfilment is not an upstream generation request.
	_, e = tx.ExecContext(ctx, `UPDATE kitty_media_wallet_orders SET submission_state='accepted',upstream_id=$2,evidence_hash=$3 WHERE order_id=$1`, key.OrderID, "storage:"+id, evidence)
	if e != nil {
		return nil, e
	}
	if _, e = wallet.finishAtomic(ctx, key, amount, evidence, "captured", tx); e != nil {
		return nil, e
	}
	// Recheck against the DB clock AFTER all locks/accounting. Crossing expiry rolls everything back.
	result, e := tx.ExecContext(ctx, `UPDATE kitty_retained_media SET expires_at=$3,updated_at=clock_timestamp() WHERE id=$1 AND user_id=$2 AND status='active' AND expires_at>clock_timestamp()`, id, user, after)
	if e != nil {
		return nil, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return nil, e
	}
	if n != 1 {
		return nil, service.ErrStorageExpired
	}
	receipt, e := scanStorageReceipt(tx.QueryRowContext(ctx, `INSERT INTO kitty_media_storage_orders(order_id,user_id,asset_id,days,amount,before_expiry,after_expiry)
 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+receiptColumns, key.OrderID, user, id, cmd.Days, amount, a.ExpiresAt, after))
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return receipt, nil
}

// Locks stay held during unlink, so renewal cannot charge for a deleted file.
// Tombstones survive unlink/commit crashes and make cleanup safely retryable.
func (r *retainedMediaRepository) Cleanup(ctx context.Context, remove func(string) error) (int, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT id::text FROM kitty_retained_media WHERE status<>'deleted' AND expires_at<=clock_timestamp() ORDER BY expires_at LIMIT 50 FOR UPDATE SKIP LOCKED`)
	if e != nil {
		return 0, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	for _, id := range ids {
		if e = remove(id); e != nil {
			return 0, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE kitty_retained_media SET status='deleted',updated_at=clock_timestamp() WHERE id=$1`, id); e != nil {
			return 0, e
		}
	}
	return len(ids), tx.Commit()
}
