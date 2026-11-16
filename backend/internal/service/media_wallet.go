package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrMediaWalletConflict        = errors.New("media wallet order conflicts with its immutable receipt")
	ErrMediaWalletState           = errors.New("media wallet order state does not permit this operation")
	ErrMediaWalletSubjectNotFound = errors.New("media wallet subject not found")
)

// Keys must be constructed from a verified SUB identity, not a caller-supplied
// email or New API user ID. No public HTTP handler is enabled by this contract.
type MediaWalletKey struct {
	UserID  int64  `json:"user_id"`
	OrderID string `json:"order_id"`
}

// Quote and PriceContract are server-owned pricing artifacts, not browser prices.
// AmountUSD is a decimal string in SUB's CNY-pegged USD-labelled site credits.
type MediaWalletReserve struct {
	Key           MediaWalletKey  `json:"key"`
	AmountUSD     string          `json:"amount_usd"`
	RequestHash   string          `json:"request_hash"`
	Quote         json.RawMessage `json:"quote"`
	PriceContract json.RawMessage `json:"price_contract"`
}

type MediaWalletOrder struct {
	Key             MediaWalletKey  `json:"key"`
	Fingerprint     string          `json:"fingerprint"`
	Currency        string          `json:"currency"`
	HoldUSD         string          `json:"hold_usd"`
	ActualUSD       string          `json:"actual_usd"`
	ReleasedUSD     string          `json:"released_usd"`
	ExpiredUSD      string          `json:"expired_usd"`
	BillingState    string          `json:"billing_state"`
	SubmissionState string          `json:"submission_state"`
	UpstreamID      string          `json:"upstream_id"`
	EvidenceHash    string          `json:"evidence_hash"`
	Quote           json.RawMessage `json:"quote"`
	PriceContract   json.RawMessage `json:"price_contract"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// This is intentionally not wired into production routes until authenticated
// identity mapping and the synchronous/asynchronous media adapters are verified.
type MediaWalletRepository interface {
	Reserve(context.Context, MediaWalletReserve) (*MediaWalletOrder, bool, error)
	Get(context.Context, MediaWalletKey) (*MediaWalletOrder, error)
	BeginSubmission(context.Context, MediaWalletKey) (*MediaWalletOrder, bool, error)
	RecordSubmission(context.Context, MediaWalletKey, string, string, string) (*MediaWalletOrder, error)
	Capture(context.Context, MediaWalletKey, string, string) (*MediaWalletOrder, error)
	Release(context.Context, MediaWalletKey, string) (*MediaWalletOrder, error)
}

// EnsureSubject is callable only after SUB's existing login/MFA flow has
// verified the user. ResolveSubject retains disabled users for reconciliation;
// the wallet repository separately rejects new spending by those users.
type MediaWalletIdentityRepository interface {
	EnsureSubject(context.Context, int64) (string, error)
	ResolveSubject(context.Context, string) (int64, error)
}

// These exact decimal strings are a point-in-time SUB view, not a reservation
// or a second media wallet. Available excludes due-but-not-yet-cleaned bonuses.
type MediaWalletBalance struct {
	Currency          string    `json:"currency"`
	Denomination      string    `json:"denomination"`
	AvailableUSD      string    `json:"available_usd"`
	FrozenUSD         string    `json:"frozen_usd"`
	BonusAvailableUSD string    `json:"bonus_available_usd"`
	AsOf              time.Time `json:"as_of"`
}

type MediaWalletBalanceReader interface {
	Balance(context.Context, int64) (*MediaWalletBalance, error)
}
