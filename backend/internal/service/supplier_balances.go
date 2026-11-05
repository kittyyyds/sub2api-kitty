package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type SupplierBalanceThreshold struct {
	Wallet    *float64 `json:"wallet"`
	Remaining *float64 `json:"remaining"`
}
type SupplierBalanceThresholds map[string]SupplierBalanceThreshold
type SupplierBalanceRow struct {
	AccountID     int64      `json:"account_id"`
	Name          string     `json:"name"`
	Platform      string     `json:"platform"`
	Host          string     `json:"host"`
	Source        string     `json:"source"`
	Status        string     `json:"status"`
	Unit          string     `json:"unit"`
	Wallet        *float64   `json:"wallet"`
	Remaining     *float64   `json:"remaining"`
	Unlimited     bool       `json:"unlimited"`
	ReceivedAt    *time.Time `json:"received_at"`
	FreshUntil    *time.Time `json:"fresh_until"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	Alerts        []string   `json:"alerts"`
}
type SupplierBalanceSnapshot struct {
	Rows       []SupplierBalanceRow      `json:"rows"`
	Total      int64                     `json:"total"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"page_size"`
	QueriedAt  time.Time                 `json:"queried_at"`
	Thresholds SupplierBalanceThresholds `json:"thresholds"`
}
type SupplierBalanceRepository interface {
	ReadSupplierBalances(context.Context, int, int) (*SupplierBalanceSnapshot, error)
	SaveSupplierBalanceThresholds(context.Context, SupplierBalanceThresholds) error
}

var supplierCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func ValidateSupplierBalanceThresholds(v SupplierBalanceThresholds) error {
	if v == nil || len(v) > 32 {
		return errors.New("invalid supplier balance thresholds")
	}
	for unit, threshold := range v {
		if !supplierCurrency.MatchString(unit) {
			return errors.New("invalid threshold currency")
		}
		for _, n := range []*float64{threshold.Wallet, threshold.Remaining} {
			if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0) || *n < 0 || *n > 1e12) {
				return errors.New("invalid balance threshold")
			}
		}
	}
	return nil
}

// BuildSupplierBalanceRow receives only a narrow projection. Authentication
// tokens, raw response data, query strings and URL credentials never leave it.
func BuildSupplierBalanceRow(id int64, name, platform, baseURL string, raw []byte, thresholds SupplierBalanceThresholds, now time.Time) SupplierBalanceRow {
	r := SupplierBalanceRow{AccountID: id, Name: name, Platform: platform, Status: "unknown", Source: "unknown", Alerts: []string{}}
	if u, err := url.Parse(baseURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" {
		r.Host = strings.ToLower(u.Hostname())
	}
	if len(raw) == 0 || string(raw) == "null" {
		return r
	}
	var b UpstreamBalanceSnapshot
	if json.Unmarshal(raw, &b) != nil {
		r.Status = "invalid"
		return r
	}
	r.ReceivedAt, r.FreshUntil = b.ReceivedAt, b.FreshUntil
	if !b.LastAttemptAt.IsZero() {
		r.LastAttemptAt = &b.LastAttemptAt
	}
	if unit, ok := b.Data["unit"].(string); ok && len(unit) <= 32 {
		r.Unit = unit
	}
	if source, ok := b.Data["source"].(string); ok && source == "new_api" {
		r.Source = "new_api"
	} else {
		r.Source = "sub2api_usage"
	}
	r.Unlimited, _ = b.Data["unlimited"].(bool)
	if n, ok := opsNumber(b.Data["wallet_balance"]); ok {
		r.Wallet = &n
	}
	if n, ok := opsNumber(b.Data["remaining"]); ok && !r.Unlimited {
		r.Remaining = &n
	}
	switch b.Status {
	case "ok", "failed", "unsupported":
		r.Status = b.Status
	default:
		r.Status = "unknown"
	}
	if r.Status != "ok" {
		return r
	}
	if !opsFresh(b.ReceivedAt, b.FreshUntil, now) {
		r.Status = "stale"
		return r
	}
	if valid, ok := b.Data["is_valid"].(bool); ok && !valid {
		r.Status = "invalid"
		return r
	}
	if r.Wallet == nil && r.Remaining == nil && !r.Unlimited {
		r.Status = "unknown"
		return r
	}
	if threshold, ok := thresholds[r.Unit]; ok {
		if threshold.Wallet != nil && r.Wallet != nil && *r.Wallet <= *threshold.Wallet {
			r.Alerts = append(r.Alerts, "wallet")
		}
		if threshold.Remaining != nil && r.Remaining != nil && *r.Remaining <= *threshold.Remaining {
			r.Alerts = append(r.Alerts, "remaining")
		}
	}
	return r
}
func (s *ChannelService) SupplierBalances(ctx context.Context, page, size int) (*SupplierBalanceSnapshot, error) {
	if page < 1 || page > 1000000 || size < 1 || size > 100 {
		return nil, errors.New("invalid supplier balance page")
	}
	r, ok := s.repo.(SupplierBalanceRepository)
	if !ok {
		return nil, errors.New("supplier balance source unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return r.ReadSupplierBalances(ctx, page, size)
}
func (s *ChannelService) SaveSupplierBalanceThresholds(ctx context.Context, v SupplierBalanceThresholds) error {
	if err := ValidateSupplierBalanceThresholds(v); err != nil {
		return err
	}
	r, ok := s.repo.(SupplierBalanceRepository)
	if !ok {
		return errors.New("supplier balance source unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return r.SaveSupplierBalanceThresholds(ctx, v)
}
