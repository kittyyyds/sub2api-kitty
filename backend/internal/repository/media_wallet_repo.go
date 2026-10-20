package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type mediaWalletRepository struct{ db *sql.DB }

func NewMediaWalletRepository(db *sql.DB) service.MediaWalletRepository {
	return &mediaWalletRepository{db: db}
}

func NewMediaWalletBalanceReader(db *sql.DB) service.MediaWalletBalanceReader {
	return &mediaWalletRepository{db: db}
}

func (r *mediaWalletRepository) Balance(ctx context.Context, userID int64) (*service.MediaWalletBalance, error) {
	if userID <= 0 {
		return nil, service.ErrMediaWalletSubjectNotFound
	}
	result := &service.MediaWalletBalance{Currency: "USD", Denomination: "cny_pegged_site_credit"}
	// A single statement sees user and grants at the same MVCC snapshot. No
	// cleanup, writes, bonus reissue or float conversion occurs on this read.
	err := r.db.QueryRowContext(ctx, `SELECT
	 (u.balance-COALESCE(b.due,0))::numeric(20,8)::text,
	 COALESCE(u.frozen_balance,0)::numeric(20,8)::text,
	 COALESCE(b.available,0)::numeric(20,8)::text, statement_timestamp()
	 FROM users u LEFT JOIN LATERAL (
	  SELECT SUM(remaining_amount) FILTER(WHERE expires_at IS NOT NULL AND expires_at<=statement_timestamp()) AS due,
	  SUM(remaining_amount) FILTER(WHERE expires_at IS NULL OR expires_at>statement_timestamp()) AS available
	  FROM recharge_bonus_grants WHERE user_id=u.id AND status='active' AND remaining_amount>0
	 ) b ON TRUE WHERE u.id=$1 AND u.status='active' AND u.deleted_at IS NULL`, userID).
		Scan(&result.AvailableUSD, &result.FrozenUSD, &result.BonusAvailableUSD, &result.AsOf)
	return result, err
}

var mediaOrderID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var mediaMoney = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.[0-9]{1,8})?$`)

func validMediaWalletKey(key service.MediaWalletKey) bool {
	return key.UserID > 0 && mediaOrderID.MatchString(key.OrderID)
}

func mediaWalletAmount(raw string) (decimal.Decimal, error) {
	if !mediaMoney.MatchString(raw) {
		return decimal.Zero, errors.New("media wallet amount must be a bounded nonnegative decimal with at most 8 places")
	}
	return decimal.NewFromString(raw)
}

func validMediaHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

const mediaWalletColumns = `order_id,user_id,fingerprint,currency,hold_amount::text,actual_amount::text,released_amount::text,expired_amount::text,billing_state,submission_state,upstream_id,evidence_hash,quote,price_contract,updated_at`

func scanMediaWallet(row interface{ Scan(...any) error }) (*service.MediaWalletOrder, error) {
	o := &service.MediaWalletOrder{}
	err := row.Scan(&o.Key.OrderID, &o.Key.UserID, &o.Fingerprint, &o.Currency, &o.HoldUSD, &o.ActualUSD, &o.ReleasedUSD, &o.ExpiredUSD,
		&o.BillingState, &o.SubmissionState, &o.UpstreamID, &o.EvidenceHash, &o.Quote, &o.PriceContract, &o.UpdatedAt)
	return o, err
}

func (r *mediaWalletRepository) Get(ctx context.Context, key service.MediaWalletKey) (*service.MediaWalletOrder, error) {
	if !validMediaWalletKey(key) {
		return nil, errors.New("invalid media wallet identity")
	}
	return scanMediaWallet(r.db.QueryRowContext(ctx, `SELECT `+mediaWalletColumns+` FROM kitty_media_wallet_orders WHERE order_id=$1 AND user_id=$2`, key.OrderID, key.UserID))
}

// All paths acquire bonus grants, user, then order in the existing billing lock
// order. The order row, rather than a timeout or an in-memory flag, is the receipt.
func (r *mediaWalletRepository) lock(ctx context.Context, key service.MediaWalletKey, active bool) (*sql.Tx, error) {
	if !validMediaWalletKey(key) {
		return nil, errors.New("invalid media wallet identity")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err = lockVideoBonusGrants(ctx, tx, key.UserID); err != nil {
		tx.Rollback()
		return nil, err
	}
	var id int64
	query := `SELECT id FROM users WHERE id=$1`
	if active {
		query += ` AND deleted_at IS NULL AND status='active'`
	}
	err = tx.QueryRowContext(ctx, query+` FOR UPDATE`, key.UserID).Scan(&id)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (r *mediaWalletRepository) Reserve(ctx context.Context, cmd service.MediaWalletReserve) (*service.MediaWalletOrder, bool, error) {
	amount, err := mediaWalletAmount(cmd.AmountUSD)
	if err != nil {
		return nil, false, err
	}
	if !validMediaHash(cmd.RequestHash) || len(cmd.Quote) == 0 || len(cmd.Quote) > 64<<10 || len(cmd.PriceContract) == 0 || len(cmd.PriceContract) > 64<<10 {
		return nil, false, errors.New("invalid media wallet price artifacts")
	}
	var quote struct {
		ID           string `json:"quote_id"`
		SnapshotID   string `json:"snapshot_id"`
		Currency     string `json:"currency"`
		Denomination string `json:"denomination"`
		Amount       string `json:"sell_usd_credit"`
		CNYPerCredit string `json:"cny_per_usd_credit"`
	}
	var contract struct {
		Schema     string `json:"schema"`
		SnapshotID string `json:"snapshot_id"`
	}
	if json.Unmarshal(cmd.Quote, &quote) != nil || json.Unmarshal(cmd.PriceContract, &contract) != nil ||
		quote.Currency != "USD" || quote.Denomination != "cny_pegged_site_credit" || quote.CNYPerCredit != "1" ||
		!validMediaHash(quote.ID) || !validMediaHash(quote.SnapshotID) || contract.SnapshotID != quote.SnapshotID || contract.Schema != "kitty-media-wallet-quote-v1" {
		return nil, false, errors.New("media wallet requires a server-owned USD-credit quote and matching contract")
	}
	quoted, err := mediaWalletAmount(quote.Amount)
	if err != nil || !quoted.Equal(amount) {
		return nil, false, errors.New("media wallet hold does not equal the trusted quote")
	}
	cmd.AmountUSD = amount.StringFixed(8)
	// A retry may use documents read back from JSONB. Normalize object key
	// ordering before hashing, without converting precise numbers to float64.
	for _, document := range []*json.RawMessage{&cmd.Quote, &cmd.PriceContract} {
		decoder := json.NewDecoder(bytes.NewReader(*document))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, false, err
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			return nil, false, err
		}
		*document = canonical
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return nil, false, err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(raw))
	tx, err := r.lock(ctx, cmd.Key, true)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var stored string
	err = tx.QueryRowContext(ctx, `INSERT INTO kitty_media_wallet_orders(order_id,user_id,fingerprint,hold_amount,quote,price_contract)
	 VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb) ON CONFLICT(order_id) DO NOTHING RETURNING order_id`,
		cmd.Key.OrderID, cmd.Key.UserID, fingerprint, cmd.AmountUSD, string(cmd.Quote), string(cmd.PriceContract)).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		existing, getErr := scanMediaWallet(tx.QueryRowContext(ctx, `SELECT `+mediaWalletColumns+` FROM kitty_media_wallet_orders WHERE order_id=$1 AND user_id=$2 FOR UPDATE`, cmd.Key.OrderID, cmd.Key.UserID))
		if getErr != nil {
			return nil, false, getErr
		}
		if existing.Fingerprint != fingerprint {
			return nil, false, service.ErrMediaWalletConflict
		}
		return existing, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	bonus := newRechargeBonusRepository(r.db)
	if _, err = bonus.reserveForBatchInTx(ctx, tx, cmd.Key.UserID, "media:"+cmd.Key.OrderID, amount.InexactFloat64(), time.Now().UTC()); err != nil {
		return nil, false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance-$1,frozen_balance=COALESCE(frozen_balance,0)+$1,updated_at=NOW()
	 WHERE id=$2 AND balance >= $1`, cmd.AmountUSD, cmd.Key.UserID)
	if err != nil {
		return nil, false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if n != 1 {
		return nil, false, service.ErrBatchImageInsufficientBalance
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO kitty_media_wallet_ledger(order_id,entry_type,held_amount,actual_amount,released_amount,expired_amount,evidence_hash)
	 VALUES($1,'hold',$2,0,0,0,$3)`, cmd.Key.OrderID, cmd.AmountUSD, cmd.RequestHash)
	if err != nil {
		return nil, false, err
	}
	o, err := scanMediaWallet(tx.QueryRowContext(ctx, `SELECT `+mediaWalletColumns+` FROM kitty_media_wallet_orders WHERE order_id=$1`, cmd.Key.OrderID))
	if err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return o, true, nil
}

func (r *mediaWalletRepository) BeginSubmission(ctx context.Context, key service.MediaWalletKey) (*service.MediaWalletOrder, bool, error) {
	tx, err := r.lock(ctx, key, true)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	o, err := scanMediaWallet(tx.QueryRowContext(ctx, `SELECT `+mediaWalletColumns+` FROM kitty_media_wallet_orders WHERE order_id=$1 AND user_id=$2 FOR UPDATE`, key.OrderID, key.UserID))
	if err != nil {
		return nil, false, err
	}
	if o.BillingState != "held" {
		return nil, false, service.ErrMediaWalletState
	}
	if o.SubmissionState != "reserved" {
		return o, false, nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE kitty_media_wallet_orders SET submission_state='submitting',updated_at=NOW() WHERE order_id=$1`, key.OrderID)
	if err != nil {
		return nil, false, err
	}
	o.SubmissionState = "submitting"
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return o, true, nil
}

// Only the caller that wins BeginSubmission may submit. An unknown result stays
// frozen until provider evidence resolves it; retrying must not send it again.
func (r *mediaWalletRepository) RecordSubmission(ctx context.Context, key service.MediaWalletKey, state, upstreamID, evidence string) (*service.MediaWalletOrder, error) {
	if (state != "accepted" && state != "rejected" && state != "unknown") || !validMediaHash(evidence) || len(upstreamID) > 512 || state == "unknown" && upstreamID != "" {
		return nil, errors.New("invalid media submission outcome")
	}
	tx, err := r.lock(ctx, key, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	o, err := scanMediaWallet(tx.QueryRowContext(ctx, `SELECT `+mediaWalletColumns+` FROM kitty_media_wallet_orders WHERE order_id=$1 AND user_id=$2 FOR UPDATE`, key.OrderID, key.UserID))
	if err != nil {
		return nil, err
	}
	if o.SubmissionState == state && o.UpstreamID == upstreamID && o.EvidenceHash == evidence {
		return o, nil
	}
	// An accepted async job may subsequently fail. Preserve its provider ID
	// while recording that confirmed failure before releasing the hold.
	allowed := o.SubmissionState == "submitting" || o.SubmissionState == "unknown" || o.SubmissionState == "accepted" && state == "rejected"
	if o.BillingState != "held" || !allowed || o.UpstreamID != "" && o.UpstreamID != upstreamID {
		return nil, service.ErrMediaWalletState
	}
	_, err = tx.ExecContext(ctx, `UPDATE kitty_media_wallet_orders SET submission_state=$2,upstream_id=$3,evidence_hash=$4,updated_at=NOW() WHERE order_id=$1`, key.OrderID, state, upstreamID, evidence)
	if err != nil {
		return nil, err
	}
	o.SubmissionState, o.UpstreamID, o.EvidenceHash = state, upstreamID, evidence
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return o, nil
}

func (r *mediaWalletRepository) Capture(ctx context.Context, key service.MediaWalletKey, actual, evidence string) (*service.MediaWalletOrder, error) {
	return r.finish(ctx, key, actual, evidence, "captured")
}

func (r *mediaWalletRepository) Release(ctx context.Context, key service.MediaWalletKey, evidence string) (*service.MediaWalletOrder, error) {
	return r.finish(ctx, key, "0", evidence, "released")
}

func (r *mediaWalletRepository) finish(ctx context.Context, key service.MediaWalletKey, actualRaw, evidence, state string) (*service.MediaWalletOrder, error) {
	actual, err := mediaWalletAmount(actualRaw)
	if err != nil {
		return nil, err
	}
	if !validMediaHash(evidence) {
		return nil, errors.New("media settlement evidence hash is required")
	}
	tx, err := r.lock(ctx, key, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	o, err := scanMediaWallet(tx.QueryRowContext(ctx, `SELECT `+mediaWalletColumns+` FROM kitty_media_wallet_orders WHERE order_id=$1 AND user_id=$2 FOR UPDATE`, key.OrderID, key.UserID))
	if err != nil {
		return nil, err
	}
	var priorEvidence string
	if err = tx.QueryRowContext(ctx, `SELECT settlement_hash FROM kitty_media_wallet_orders WHERE order_id=$1`, key.OrderID).Scan(&priorEvidence); err != nil {
		return nil, err
	}
	if o.BillingState != "held" {
		if o.BillingState == state && priorEvidence == evidence && decimal.RequireFromString(o.ActualUSD).Equal(actual) {
			return o, nil
		}
		return nil, service.ErrMediaWalletConflict
	}
	if state == "captured" && o.SubmissionState != "accepted" || state == "released" && o.SubmissionState != "reserved" && o.SubmissionState != "rejected" {
		return nil, service.ErrMediaWalletState
	}
	hold := decimal.RequireFromString(o.HoldUSD)
	if actual.GreaterThan(hold) {
		return nil, service.ErrBatchImageSettlementCostExceedsHold
	}
	bonus := newRechargeBonusRepository(r.db)
	settlement, err := bonus.settleBatchInTx(ctx, tx, key.UserID, "media:"+key.OrderID, actual.InexactFloat64(), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	expired := decimal.NewFromFloat(settlement.ExpiredAmount).Round(8)
	released := hold.Sub(actual).Sub(expired)
	if released.IsNegative() {
		return nil, errors.New("media wallet bonus allocation exceeds its hold")
	}
	result, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,frozen_balance=COALESCE(frozen_balance,0)-$2,updated_at=NOW()
	 WHERE id=$3 AND COALESCE(frozen_balance,0)>=$2`, released.StringFixed(8), hold.StringFixed(8), key.UserID)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, errors.New("media wallet frozen balance is insufficient")
	}
	_, err = tx.ExecContext(ctx, `UPDATE kitty_media_wallet_orders SET billing_state=$2,actual_amount=$3,released_amount=$4,expired_amount=$5,settlement_hash=$6,updated_at=NOW() WHERE order_id=$1`,
		key.OrderID, state, actual.StringFixed(8), released.StringFixed(8), expired.StringFixed(8), evidence)
	if err != nil {
		return nil, err
	}
	entry := "capture"
	if state == "released" {
		entry = "release"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO kitty_media_wallet_ledger(order_id,entry_type,actual_amount,released_amount,expired_amount,evidence_hash)
	 VALUES($1,$2,$3,$4,$5,$6)`, key.OrderID, entry, actual.StringFixed(8), released.StringFixed(8), expired.StringFixed(8), evidence)
	if err != nil {
		return nil, err
	}
	o, err = scanMediaWallet(tx.QueryRowContext(ctx, `SELECT `+mediaWalletColumns+` FROM kitty_media_wallet_orders WHERE order_id=$1`, key.OrderID))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return o, nil
}
