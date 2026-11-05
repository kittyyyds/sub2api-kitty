package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrProviderHubInput = infraerrors.BadRequest("PROVIDER_HUB_INVALID", "Invalid supplier, wallet or procurement record")
var ErrProviderHubConflict = infraerrors.Conflict("PROVIDER_HUB_CHANGED", "Configuration changed or the account belongs to another wallet; refresh before saving")
var ErrProviderHubUnavailable = infraerrors.ServiceUnavailable("PROVIDER_HUB_UNAVAILABLE", "Supplier management is unavailable")

type ProviderHubCommand struct {
	Action              string    `json:"-"`
	ID                  int64     `json:"-"`
	SupplierID          int64     `json:"supplier_id"`
	Revision            int64     `json:"revision"`
	Name                string    `json:"name"`
	Identity            string    `json:"identity"`
	SiteType            string    `json:"site_type"`
	BaseURL             string    `json:"base_url"`
	Notes               string    `json:"notes"`
	Active              bool      `json:"active"`
	Currency            string    `json:"currency"`
	CreditCurrency      string    `json:"credit_currency"`
	LowBalance          *float64  `json:"low_balance"`
	SyncIntervalMinutes int       `json:"sync_interval_minutes"`
	AccountIDs          []int64   `json:"account_ids"`
	Reassign            bool      `json:"reassign"`
	OperationID         string    `json:"operation_id"`
	Kind                string    `json:"kind"`
	Cash                float64   `json:"cash"`
	FXRate              float64   `json:"fx_rate"`
	Credits             float64   `json:"credits"`
	Bonus               float64   `json:"bonus"`
	Reference           string    `json:"reference"`
	Note                string    `json:"note"`
	OccurredAt          time.Time `json:"occurred_at"`
	Model               string    `json:"model"`
	Method              string    `json:"method"`
	Multiplier          float64   `json:"multiplier"`
	InputPrice          float64   `json:"input_price"`
	OutputPrice         float64   `json:"output_price"`
	CacheReadPrice      float64   `json:"cache_read_price"`
	CacheWritePrice     float64   `json:"cache_write_price"`
	UnitPrice           float64   `json:"unit_price"`
	FXToUSD             *float64  `json:"fx_to_usd"`
	APIKeyID            int64     `json:"api_key_id"`
	RequestKey          string    `json:"request_key"`
	Amount              float64   `json:"amount"`
	AuthMode            string    `json:"auth_mode"`
	Username            string    `json:"username"`
	RemoteUserID        string    `json:"remote_user_id"`
	Secret              string    `json:"secret,omitempty"`
	SecretCipher        string    `json:"-"`
}

type ProviderHubWalletAuthorization struct {
	WalletID       int64
	Revision       int64
	SupplierID     int64
	SiteType       string
	BaseURL        string
	Currency       string
	CreditCurrency string
	AuthMode       string
	Username       string
	RemoteUserID   string
	Cipher         string
	AccountIDs     []int64
}

type ProviderHubRepository interface {
	ReadProviderHubPurchases(context.Context, int64, int, int) (json.RawMessage, error)
	LockProviderHubImport(context.Context, int64) (func(), error)
	ReadProviderHub(context.Context) (json.RawMessage, error)
	MutateProviderHub(context.Context, ProviderHubCommand) (json.RawMessage, error)
	ProviderHubAuthorization(context.Context, int64) (*ProviderHubWalletAuthorization, error)
	SaveProviderHubObservation(context.Context, *ProviderHubWalletAuthorization, *float64, string, string, string, *time.Time) error
	ClaimProviderHubSync(context.Context, int) ([]int64, error)
	RotateProviderHubCredential(context.Context, *ProviderHubWalletAuthorization, string) error
	BeginProviderHubKeyCreation(context.Context, int64, string, string, string) (bool, error)
	FinishProviderHubKeyCreation(context.Context, string, bool) error
	ReadProviderOperations(context.Context, ProviderOperationsFilter) (json.RawMessage, error)
}

type ProviderOperationsFilter struct {
	From       time.Time `json:"from"`
	Until      time.Time `json:"until"`
	SupplierID int64     `json:"supplier_id"`
	WalletID   int64     `json:"wallet_id"`
	AccountID  int64     `json:"account_id"`
	GroupID    int64     `json:"group_id"`
	Model      string    `json:"model"`
	Dimension  string    `json:"dimension"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	Timezone   string    `json:"timezone"`
}

var hubCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func hubNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1e12 }
func ValidateProviderHubCommand(c *ProviderHubCommand) error {
	if c == nil || c.ID < 0 || c.Revision < 0 || len(c.Notes) > 2000 || len(c.Note) > 2000 || len(c.Reference) > 256 || len(c.OperationID) > 128 {
		return ErrProviderHubInput
	}
	c.Name = strings.TrimSpace(c.Name)
	c.Identity = strings.TrimSpace(c.Identity)
	for _, v := range []string{c.Name, c.Identity, c.Reference, c.Model, c.Username, c.RemoteUserID} {
		if strings.ContainsAny(v, "\r\n\x00") {
			return ErrProviderHubInput
		}
	}
	c.Currency = strings.ToUpper(strings.TrimSpace(c.Currency))
	c.CreditCurrency = strings.ToUpper(strings.TrimSpace(c.CreditCurrency))
	if c.FXToUSD != nil && (!hubNumber(*c.FXToUSD) || *c.FXToUSD <= 0) {
		return ErrProviderHubInput
	}
	if c.Currency == "USD" {
		v := 1.0
		c.FXToUSD = &v
	}
	switch c.Action {
	case "supplier":
		if len(c.Name) < 1 || len(c.Name) > 120 || !slicesContains([]string{"sub2api", "newapi", "manual"}, c.SiteType) {
			return ErrProviderHubInput
		}
		if c.SiteType != "manual" || c.BaseURL != "" {
			u, e := CanonicalNewAPISite(c.BaseURL)
			if e != nil {
				return ErrProviderHubInput
			}
			c.BaseURL = u
		}
	case "wallet":
		if c.SyncIntervalMinutes != 0 && (c.SyncIntervalMinutes < 5 || c.SyncIntervalMinutes > 1440) {
			return ErrProviderHubInput
		}
		if c.SupplierID <= 0 || len(c.Name) < 1 || len(c.Name) > 120 || len(c.Identity) < 1 || len(c.Identity) > 160 || !hubCurrency.MatchString(c.Currency) || !hubCurrency.MatchString(c.CreditCurrency) || (c.LowBalance != nil && !hubNumber(*c.LowBalance)) {
			return ErrProviderHubInput
		}
	case "bindings":
		if c.ID <= 0 || c.Revision <= 0 || len(c.AccountIDs) < 1 || len(c.AccountIDs) > 100 {
			return ErrProviderHubInput
		}
		seen := map[int64]bool{}
		for _, id := range c.AccountIDs {
			if id <= 0 || seen[id] {
				return ErrProviderHubInput
			}
			seen[id] = true
		}
	case "unbind", "void_purchase", "close_rule", "archive_supplier", "archive_wallet":
		if c.ID <= 0 || c.Revision <= 0 {
			return ErrProviderHubInput
		}
	case "purchase":
		if c.ID <= 0 || len(c.OperationID) < 12 || !slicesContains([]string{"topup", "refund", "opening"}, c.Kind) || !hubCurrency.MatchString(c.Currency) || !hubNumber(c.Cash) || !hubNumber(c.Credits) || !hubNumber(c.Bonus) || !hubNumber(c.FXRate) || c.FXRate <= 0 || c.Cash+c.Credits+c.Bonus <= 0 || c.OccurredAt.IsZero() || c.OccurredAt.After(time.Now().Add(time.Minute)) {
			return ErrProviderHubInput
		}
	case "rule":
		c.Model = strings.TrimSpace(c.Model)
		if c.ID <= 0 || c.Revision <= 0 || len(c.Model) < 1 || len(c.Model) > 160 || strings.ContainsAny(c.Model, "?[]\\") || (strings.Contains(c.Model, "*") && c.Model != "*") || !hubCurrency.MatchString(c.Currency) || !slicesContains([]string{"standard_multiplier", "account_credits", "tokens", "image", "video_second", "video_item", "request"}, c.Method) {
			return ErrProviderHubInput
		}
		for _, v := range []float64{c.Multiplier, c.InputPrice, c.OutputPrice, c.CacheReadPrice, c.CacheWritePrice, c.UnitPrice} {
			if !hubNumber(v) {
				return ErrProviderHubInput
			}
		}
	case "adjustment":
		if c.APIKeyID <= 0 || len(c.RequestKey) < 1 || len(c.RequestKey) > 512 || len(c.OperationID) < 12 || strings.TrimSpace(c.Reference) == "" || !hubCurrency.MatchString(c.Currency) || !hubNumber(c.Amount) {
			return ErrProviderHubInput
		}
	case "authorization":
		if c.ID <= 0 || c.Revision <= 0 || !slicesContains([]string{"none", "token", "password"}, c.AuthMode) || len(c.Username) > 320 || len(c.RemoteUserID) > 128 || len(c.Secret) > 16384 || strings.ContainsAny(c.Secret, "\r\n\x00") || (c.AuthMode == "password" && c.Username == "") {
			return ErrProviderHubInput
		}
	default:
		return ErrProviderHubInput
	}
	if (c.Action == "supplier" || c.Action == "wallet") && c.ID > 0 && c.Revision <= 0 {
		return ErrProviderHubInput
	}
	return nil
}
func slicesContains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func (s *ChannelService) providerHubRepo() (ProviderHubRepository, error) {
	if s == nil {
		return nil, ErrProviderHubUnavailable
	}
	r, ok := s.repo.(ProviderHubRepository)
	if !ok {
		return nil, ErrProviderHubUnavailable
	}
	return r, nil
}
func (s *ChannelService) ReadProviderHub(ctx context.Context) (json.RawMessage, error) {
	r, e := s.providerHubRepo()
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return r.ReadProviderHub(ctx)
}
func (s *ChannelService) MutateProviderHub(ctx context.Context, c ProviderHubCommand) (json.RawMessage, error) {
	if e := ValidateProviderHubCommand(&c); e != nil {
		return nil, e
	}
	r, e := s.providerHubRepo()
	if e != nil {
		return nil, e
	}
	if c.Action == "authorization" {
		if c.AuthMode != "none" {
			if s.providerProbe == nil || !s.providerProbe.newAPIFixedKey || s.providerProbe.newAPIEncryptor == nil {
				return nil, ErrNewAPIEncryptionKey
			}
			if c.Secret != "" {
				c.SecretCipher, e = s.providerProbe.newAPIEncryptor.Encrypt(c.Secret)
				if e != nil {
					return nil, ErrProviderHubUnavailable
				}
			}
		}
		c.Secret = ""
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return r.MutateProviderHub(ctx, c)
}
func (s *ChannelService) ProviderOperations(ctx context.Context, f ProviderOperationsFilter) (json.RawMessage, error) {
	if !ValidateFinanceRange(f.From, f.Until) || f.SupplierID < 0 || f.WalletID < 0 || f.AccountID < 0 || f.GroupID < 0 || len(f.Model) > 160 || !slicesContains([]string{"supplier", "wallet", "account", "model", "group"}, f.Dimension) || f.Page < 1 || f.Page > 1000000 || f.PageSize < 1 || f.PageSize > 100 {
		return nil, ErrProviderHubInput
	}
	if _, e := time.LoadLocation(f.Timezone); e != nil {
		return nil, ErrProviderHubInput
	}
	r, e := s.providerHubRepo()
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return r.ReadProviderOperations(ctx, f)
}
func (s *ChannelService) SetProviderHubProbe(p *UpstreamBillingProbeService) { s.providerProbe = p }
func (s *ChannelService) ProviderHubPurchases(ctx context.Context, id int64, page, size int) (json.RawMessage, error) {
	if id <= 0 || page < 1 || page > 1000000 || size < 1 || size > 100 {
		return nil, ErrProviderHubInput
	}
	r, e := s.providerHubRepo()
	if e != nil {
		return nil, e
	}
	return r.ReadProviderHubPurchases(ctx, id, page, size)
}
func providerHubStorageError(err error) error {
	if errors.Is(err, ErrProviderHubInput) || errors.Is(err, ErrProviderHubConflict) {
		return err
	}
	return ErrProviderHubUnavailable
}
