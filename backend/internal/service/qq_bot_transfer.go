package service

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"
)

// Kitty's users.balance already excludes frozen media holds. Do not subtract
// them twice. Outstanding grant balances, including expired grants awaiting
// settlement, remain non-transferable to preserve their original restrictions.
func qqBotTransferableBalance(ctx context.Context, tx *sql.Tx, uid int64, balance string) (string, error) {
	var reserved string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(remaining_amount),0)::text
FROM recharge_bonus_grants WHERE user_id=$1 AND status='active' AND remaining_amount>0`, uid).Scan(&reserved); err != nil {
		return "", err
	}
	available, err := decimal.NewFromString(balance)
	if err != nil {
		return "", fmt.Errorf("invalid transferable balance")
	}
	bonus, err := decimal.NewFromString(reserved)
	if err != nil {
		return "", fmt.Errorf("invalid reserved grant balance")
	}
	return decimal.Max(decimal.Zero, available.Sub(bonus)).StringFixed(8), nil
}
