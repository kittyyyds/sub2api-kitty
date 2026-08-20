package service

import (
	"context"
	"database/sql"
	"time"
)

const (
	RechargeBonusStatusActive    = "active"
	RechargeBonusStatusExhausted = "exhausted"
	RechargeBonusStatusExpired   = "expired"
	RechargeBonusStatusRevoked   = "revoked"
)

type RechargeBonusRule struct {
	ID             int64      `json:"id"`
	RechargeAmount float64    `json:"recharge_amount"`
	BonusAmount    float64    `json:"bonus_amount"`
	ValidityDays   int        `json:"validity_days"`
	StartsAt       *time.Time `json:"starts_at,omitempty"`
	EndsAt         *time.Time `json:"ends_at,omitempty"`
}

type RechargeBonusSummary struct {
	AvailableAmount float64    `json:"available_amount"`
	NextExpiresAt   *time.Time `json:"next_expires_at,omitempty"`
	ActiveGrants    int        `json:"active_grants"`
}

type RechargeBonusGrantResult struct {
	Applied     bool
	GrantID     int64
	BonusAmount float64
	Remaining   float64
	ExpiresAt   time.Time
	NewBalance  float64
}

type RechargeBonusExpiryResult struct {
	ExpiredGrants int
	UserIDs       []int64
}

// RechargeBonusSQLExecutor is implemented by *sql.DB, *sql.Tx, and Ent's
// generated transactional client.
type RechargeBonusSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type RechargeBonusRepository interface {
	ListActiveRules(ctx context.Context, now time.Time) ([]RechargeBonusRule, error)
	MatchRule(ctx context.Context, rechargeAmount float64, now time.Time) (*RechargeBonusRule, error)
	SnapshotOrder(ctx context.Context, exec RechargeBonusSQLExecutor, orderID int64, rule *RechargeBonusRule) error
	GrantForOrder(ctx context.Context, orderID, userID int64, now time.Time) (*RechargeBonusGrantResult, error)
	GetUserSummary(ctx context.Context, userID int64, now time.Time) (*RechargeBonusSummary, error)
	ExpireDue(ctx context.Context, now time.Time, limit int) (*RechargeBonusExpiryResult, error)
	RevokeForRefund(ctx context.Context, orderID, userID int64, refundAmount, orderAmount float64, now time.Time) (float64, error)
}
