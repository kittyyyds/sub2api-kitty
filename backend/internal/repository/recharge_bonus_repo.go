package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type rechargeBonusRepository struct {
	db *sql.DB
}

func NewRechargeBonusRepository(db *sql.DB) service.RechargeBonusRepository {
	return newRechargeBonusRepository(db)
}

func newRechargeBonusRepository(db *sql.DB) *rechargeBonusRepository {
	return &rechargeBonusRepository{db: db}
}

func (r *rechargeBonusRepository) ListActiveRules(ctx context.Context, now time.Time) ([]service.RechargeBonusRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("recharge bonus repository db is nil")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, recharge_amount::double precision, bonus_amount::double precision,
		       validity_days, starts_at, ends_at
		FROM recharge_bonus_rules
		WHERE enabled = TRUE
		  AND (starts_at IS NULL OR starts_at <= $1)
		  AND (ends_at IS NULL OR ends_at > $1)
		ORDER BY recharge_amount ASC, id ASC
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]service.RechargeBonusRule, 0)
	for rows.Next() {
		var rule service.RechargeBonusRule
		var startsAt, endsAt sql.NullTime
		if err := rows.Scan(&rule.ID, &rule.RechargeAmount, &rule.BonusAmount, &rule.ValidityDays, &startsAt, &endsAt); err != nil {
			return nil, err
		}
		if startsAt.Valid {
			rule.StartsAt = &startsAt.Time
		}
		if endsAt.Valid {
			rule.EndsAt = &endsAt.Time
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *rechargeBonusRepository) MatchRule(ctx context.Context, rechargeAmount float64, now time.Time) (*service.RechargeBonusRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("recharge bonus repository db is nil")
	}
	var rule service.RechargeBonusRule
	var startsAt, endsAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT id, recharge_amount::double precision, bonus_amount::double precision,
		       validity_days, starts_at, ends_at
		FROM recharge_bonus_rules
		WHERE enabled = TRUE
		  AND ABS(recharge_amount - $1) < 0.00000001
		  AND (starts_at IS NULL OR starts_at <= $2)
		  AND (ends_at IS NULL OR ends_at > $2)
		ORDER BY id DESC
		LIMIT 1
	`, rechargeAmount, now).Scan(
		&rule.ID, &rule.RechargeAmount, &rule.BonusAmount, &rule.ValidityDays, &startsAt, &endsAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if startsAt.Valid {
		rule.StartsAt = &startsAt.Time
	}
	if endsAt.Valid {
		rule.EndsAt = &endsAt.Time
	}
	return &rule, nil
}

func (r *rechargeBonusRepository) SnapshotOrder(ctx context.Context, exec service.RechargeBonusSQLExecutor, orderID int64, rule *service.RechargeBonusRule) error {
	if exec == nil || rule == nil || rule.BonusAmount <= 0 || rule.ValidityDays < 0 {
		return nil
	}
	_, err := exec.ExecContext(ctx, `
		INSERT INTO recharge_bonus_order_snapshots
		    (payment_order_id, rule_id, recharge_amount, bonus_amount, validity_days)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (payment_order_id) DO NOTHING
	`, orderID, rule.ID, rule.RechargeAmount, rule.BonusAmount, rule.ValidityDays)
	return err
}

func (r *rechargeBonusRepository) GrantForOrder(ctx context.Context, orderID, userID int64, now time.Time) (_ *service.RechargeBonusGrantResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("recharge bonus repository db is nil")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var ruleID sql.NullInt64
	var bonusAmount float64
	var validityDays int
	err = tx.QueryRowContext(ctx, `
		SELECT rule_id, bonus_amount::double precision, validity_days
		FROM recharge_bonus_order_snapshots
		WHERE payment_order_id = $1
	`, orderID).Scan(&ruleID, &bonusAmount, &validityDays)
	if errors.Is(err, sql.ErrNoRows) {
		return &service.RechargeBonusGrantResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	if bonusAmount <= 0 || validityDays < 0 {
		return &service.RechargeBonusGrantResult{}, nil
	}

	// NULL expresses permanent credit without inventing a distant expiry date.
	// The order snapshot, never the current campaign rule, controls fulfillment.
	var expiresAt *time.Time
	if validityDays > 0 {
		finiteExpiry := now.AddDate(0, 0, validityDays)
		expiresAt = &finiteExpiry
	}
	var grantID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO recharge_bonus_grants
		    (user_id, payment_order_id, rule_id, granted_amount, remaining_amount, expires_at)
		VALUES ($1, $2, $3, $4, $4, $5)
		ON CONFLICT (payment_order_id) DO NOTHING
		RETURNING id
	`, userID, orderID, nullableInt64(ruleID), bonusAmount, expiresAt).Scan(&grantID)
	if errors.Is(err, sql.ErrNoRows) {
		result := &service.RechargeBonusGrantResult{Applied: false}
		queryErr := tx.QueryRowContext(ctx, `
			SELECT id, granted_amount::double precision, remaining_amount::double precision, expires_at
			FROM recharge_bonus_grants
			WHERE payment_order_id = $1
		`, orderID).Scan(&result.GrantID, &result.BonusAmount, &result.Remaining, &result.ExpiresAt)
		if queryErr != nil {
			return nil, queryErr
		}
		return result, nil
	}
	if err != nil {
		return nil, err
	}

	var newBalance float64
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING balance::double precision
	`, bonusAmount, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO recharge_bonus_ledger (grant_id, user_id, request_id, entry_type, amount)
		VALUES ($1, $2, $3, 'grant', $4)
		ON CONFLICT (grant_id, request_id, entry_type) DO NOTHING
	`, grantID, userID, fmt.Sprintf("payment:%d", orderID), bonusAmount); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &service.RechargeBonusGrantResult{
		Applied:     true,
		GrantID:     grantID,
		BonusAmount: bonusAmount,
		Remaining:   bonusAmount,
		ExpiresAt:   expiresAt,
		NewBalance:  newBalance,
	}, nil
}

func (r *rechargeBonusRepository) GetUserSummary(ctx context.Context, userID int64, now time.Time) (*service.RechargeBonusSummary, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("recharge bonus repository db is nil")
	}
	if _, err := r.expireDueForUser(ctx, now, 500, &userID); err != nil {
		return nil, err
	}
	var summary service.RechargeBonusSummary
	var nextExpiry sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(remaining_amount), 0)::double precision,
		       MIN(expires_at), COUNT(*)
		FROM recharge_bonus_grants
		WHERE user_id = $1 AND status = 'active' AND remaining_amount > 0
		  AND (expires_at IS NULL OR expires_at > $2)
	`, userID, now).Scan(&summary.AvailableAmount, &nextExpiry, &summary.ActiveGrants)
	if err != nil {
		return nil, err
	}
	if nextExpiry.Valid {
		summary.NextExpiresAt = &nextExpiry.Time
	}
	return &summary, nil
}

func (r *rechargeBonusRepository) RevokeForRefund(ctx context.Context, orderID, userID int64, refundAmount, orderAmount float64, now time.Time) (_ float64, err error) {
	if r == nil || r.db == nil {
		return 0, errors.New("recharge bonus repository db is nil")
	}
	if refundAmount <= 0 || orderAmount <= 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var grantID int64
	var grantedAmount, remainingAmount float64
	err = tx.QueryRowContext(ctx, `
		SELECT id, granted_amount::double precision, remaining_amount::double precision
		FROM recharge_bonus_grants
		WHERE payment_order_id = $1 AND user_id = $2
		FOR UPDATE
	`, orderID, userID).Scan(&grantID, &grantedAmount, &remainingAmount)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	target := grantedAmount * refundAmount / orderAmount
	if target > grantedAmount {
		target = grantedAmount
	}
	revoked := target
	if revoked > remainingAmount {
		revoked = remainingAmount
	}
	if revoked <= 0.000000001 {
		return 0, nil
	}

	requestID := fmt.Sprintf("refund:%d", orderID)
	var ledgerID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO recharge_bonus_ledger (grant_id, user_id, request_id, entry_type, amount)
		VALUES ($1, $2, $3, 'revoke', $4)
		ON CONFLICT (grant_id, request_id, entry_type) DO NOTHING
		RETURNING id
	`, grantID, userID, requestID, revoked).Scan(&ledgerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	newRemaining := remainingAmount - revoked
	status := service.RechargeBonusStatusActive
	if newRemaining <= 0.000000001 {
		newRemaining = 0
		status = service.RechargeBonusStatusRevoked
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE recharge_bonus_grants
		SET remaining_amount = $1, revoked_amount = revoked_amount + $2,
		    status = $3, updated_at = $4
		WHERE id = $5
	`, newRemaining, revoked, status, now, grantID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET balance = balance - $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, revoked, userID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return revoked, nil
}

func (r *rechargeBonusRepository) ExpireDue(ctx context.Context, now time.Time, limit int) (*service.RechargeBonusExpiryResult, error) {
	return r.expireDueForUser(ctx, now, limit, nil)
}

func (r *rechargeBonusRepository) expireDueForUser(ctx context.Context, now time.Time, limit int, userID *int64) (_ *service.RechargeBonusExpiryResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("recharge bonus repository db is nil")
	}
	if limit <= 0 {
		limit = 500
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	query := `
		SELECT id, user_id, remaining_amount::double precision
		FROM recharge_bonus_grants
		WHERE status = 'active' AND remaining_amount > 0 AND expires_at IS NOT NULL AND expires_at <= $1`
	args := []any{now}
	if userID != nil {
		query += " AND user_id = $2"
		args = append(args, *userID)
	}
	query += " ORDER BY expires_at ASC, id ASC FOR UPDATE SKIP LOCKED LIMIT " + fmt.Sprintf("%d", limit)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type dueGrant struct {
		id     int64
		userID int64
		amount float64
	}
	due := make([]dueGrant, 0)
	for rows.Next() {
		var item dueGrant
		if err := rows.Scan(&item.id, &item.userID, &item.amount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		due = append(due, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	userIDs := make(map[int64]struct{})
	for _, item := range due {
		if _, err := tx.ExecContext(ctx, `
			UPDATE recharge_bonus_grants
			SET remaining_amount = 0, status = 'expired', updated_at = NOW()
			WHERE id = $1
		`, item.id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE users SET balance = balance - $1, updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
		`, item.amount, item.userID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recharge_bonus_ledger (grant_id, user_id, request_id, entry_type, amount)
			VALUES ($1, $2, $3, 'expire', $4)
			ON CONFLICT (grant_id, request_id, entry_type) DO NOTHING
		`, item.id, item.userID, fmt.Sprintf("expire:%d", item.id), item.amount); err != nil {
			return nil, err
		}
		userIDs[item.userID] = struct{}{}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result := &service.RechargeBonusExpiryResult{ExpiredGrants: len(due), UserIDs: make([]int64, 0, len(userIDs))}
	for id := range userIDs {
		result.UserIDs = append(result.UserIDs, id)
	}
	return result, nil
}

func nullableInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func (r *rechargeBonusRepository) expireUserInTx(ctx context.Context, tx *sql.Tx, userID int64, now time.Time) (float64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining_amount::double precision
		FROM recharge_bonus_grants
		WHERE user_id = $1 AND status = 'active' AND remaining_amount > 0 AND expires_at IS NOT NULL AND expires_at <= $2
		ORDER BY expires_at ASC, id ASC
		FOR UPDATE
	`, userID, now)
	if err != nil {
		return 0, err
	}
	type expiredGrant struct {
		id     int64
		amount float64
	}
	items := make([]expiredGrant, 0)
	for rows.Next() {
		var item expiredGrant
		if err := rows.Scan(&item.id, &item.amount); err != nil {
			_ = rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	total := 0.0
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
			UPDATE recharge_bonus_grants
			SET remaining_amount = 0, status = 'expired', updated_at = NOW()
			WHERE id = $1
		`, item.id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recharge_bonus_ledger (grant_id, user_id, request_id, entry_type, amount)
			VALUES ($1, $2, $3, 'expire', $4)
			ON CONFLICT (grant_id, request_id, entry_type) DO NOTHING
		`, item.id, userID, fmt.Sprintf("expire:%d", item.id), item.amount); err != nil {
			return 0, err
		}
		total += item.amount
	}
	if total > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE users SET balance = balance - $1, updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
		`, total, userID); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (r *rechargeBonusRepository) consumeInTx(ctx context.Context, tx *sql.Tx, userID int64, requestID string, amount float64, now time.Time) (float64, float64, error) {
	if amount <= 0 {
		return 0, 0, nil
	}
	expired, err := r.expireUserInTx(ctx, tx, userID, now)
	if err != nil {
		return 0, 0, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining_amount::double precision
		FROM recharge_bonus_grants
		WHERE user_id = $1 AND status = 'active' AND remaining_amount > 0
		  AND (expires_at IS NULL OR expires_at > $2)
		ORDER BY expires_at ASC NULLS LAST, id ASC
		FOR UPDATE
	`, userID, now)
	if err != nil {
		return 0, 0, err
	}
	type activeGrant struct {
		id        int64
		remaining float64
	}
	grants := make([]activeGrant, 0)
	for rows.Next() {
		var grant activeGrant
		if err := rows.Scan(&grant.id, &grant.remaining); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		grants = append(grants, grant)
	}
	if err := rows.Close(); err != nil {
		return 0, 0, err
	}

	left := amount
	consumed := 0.0
	for _, grant := range grants {
		if left <= 0 {
			break
		}
		use := grant.remaining
		if use > left {
			use = left
		}
		newRemaining := grant.remaining - use
		status := service.RechargeBonusStatusActive
		if newRemaining <= 0.000000001 {
			newRemaining = 0
			status = service.RechargeBonusStatusExhausted
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE recharge_bonus_grants
			SET remaining_amount = $1, status = $2, updated_at = NOW()
			WHERE id = $3
		`, newRemaining, status, grant.id); err != nil {
			return 0, 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recharge_bonus_ledger (grant_id, user_id, request_id, entry_type, amount)
			VALUES ($1, $2, $3, 'consume', $4)
			ON CONFLICT (grant_id, request_id, entry_type) DO NOTHING
		`, grant.id, userID, requestID, use); err != nil {
			return 0, 0, err
		}
		left -= use
		consumed += use
	}
	return consumed, expired, nil
}

type rechargeBonusBatchSettlement struct {
	ConsumedAmount float64
	RestoredAmount float64
	ExpiredAmount  float64
}

func (r *rechargeBonusRepository) reserveForBatchInTx(ctx context.Context, tx *sql.Tx, userID int64, batchID string, amount float64, now time.Time) (float64, error) {
	if amount <= 0 || batchID == "" {
		return 0, nil
	}
	if _, err := r.expireUserInTx(ctx, tx, userID, now); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining_amount::double precision
		FROM recharge_bonus_grants
		WHERE user_id = $1 AND status = 'active' AND remaining_amount > 0
		  AND (expires_at IS NULL OR expires_at > $2)
		ORDER BY expires_at ASC NULLS LAST, id ASC
		FOR UPDATE
	`, userID, now)
	if err != nil {
		return 0, err
	}
	type activeGrant struct {
		id        int64
		remaining float64
	}
	grants := make([]activeGrant, 0)
	for rows.Next() {
		var grant activeGrant
		if err := rows.Scan(&grant.id, &grant.remaining); err != nil {
			_ = rows.Close()
			return 0, err
		}
		grants = append(grants, grant)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	left := amount
	reserved := 0.0
	for _, grant := range grants {
		if left <= 0.000000001 {
			break
		}
		use := grant.remaining
		if use > left {
			use = left
		}
		newRemaining := grant.remaining - use
		status := service.RechargeBonusStatusActive
		if newRemaining <= 0.000000001 {
			newRemaining = 0
			status = service.RechargeBonusStatusExhausted
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE recharge_bonus_grants
			SET remaining_amount = $1, status = $2, updated_at = NOW()
			WHERE id = $3
		`, newRemaining, status, grant.id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recharge_bonus_batch_holds (batch_id, user_id, grant_id, held_amount)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (batch_id, grant_id) DO NOTHING
		`, batchID, userID, grant.id, use); err != nil {
			return 0, err
		}
		left -= use
		reserved += use
	}
	return reserved, nil
}

func (r *rechargeBonusRepository) settleBatchInTx(ctx context.Context, tx *sql.Tx, userID int64, batchID string, actualAmount float64, now time.Time) (*rechargeBonusBatchSettlement, error) {
	result := &rechargeBonusBatchSettlement{}
	if batchID == "" {
		return result, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT h.id, h.grant_id, h.held_amount::double precision, g.expires_at
		FROM recharge_bonus_batch_holds h
		JOIN recharge_bonus_grants g ON g.id = h.grant_id
		WHERE h.batch_id = $1 AND h.user_id = $2 AND h.status = 'held'
		ORDER BY g.expires_at ASC NULLS LAST, h.id ASC
		FOR UPDATE OF h, g
	`, batchID, userID)
	if err != nil {
		return nil, err
	}
	type allocation struct {
		id        int64
		grantID   int64
		held      float64
		expiresAt sql.NullTime
	}
	allocations := make([]allocation, 0)
	for rows.Next() {
		var item allocation
		if err := rows.Scan(&item.id, &item.grantID, &item.held, &item.expiresAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		allocations = append(allocations, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	left := actualAmount
	for _, item := range allocations {
		consumed := item.held
		if consumed > left {
			consumed = left
		}
		if consumed < 0 {
			consumed = 0
		}
		unused := item.held - consumed
		restored := 0.0
		expired := 0.0
		if unused > 0 {
			if !item.expiresAt.Valid || item.expiresAt.Time.After(now) {
				restored = unused
				if _, err := tx.ExecContext(ctx, `
					UPDATE recharge_bonus_grants
					SET remaining_amount = remaining_amount + $1, status = 'active', updated_at = NOW()
					WHERE id = $2
				`, restored, item.grantID); err != nil {
					return nil, err
				}
			} else {
				expired = unused
			}
		}
		if consumed > 0 {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO recharge_bonus_ledger (grant_id, user_id, request_id, entry_type, amount)
				VALUES ($1, $2, $3, 'consume', $4)
				ON CONFLICT (grant_id, request_id, entry_type) DO NOTHING
			`, item.grantID, userID, fmt.Sprintf("batch:%s:capture", batchID), consumed); err != nil {
				return nil, err
			}
		}
		if expired > 0 {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO recharge_bonus_ledger (grant_id, user_id, request_id, entry_type, amount)
				VALUES ($1, $2, $3, 'expire', $4)
				ON CONFLICT (grant_id, request_id, entry_type) DO NOTHING
			`, item.grantID, userID, fmt.Sprintf("batch:%s:expire", batchID), expired); err != nil {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE recharge_bonus_batch_holds
			SET consumed_amount = $1, restored_amount = $2, expired_amount = $3,
			    status = 'settled', settled_at = $4
			WHERE id = $5
		`, consumed, restored, expired, now, item.id); err != nil {
			return nil, err
		}
		left -= consumed
		result.ConsumedAmount += consumed
		result.RestoredAmount += restored
		result.ExpiredAmount += expired
	}
	return result, nil
}
