package service

import (
	"context"
	"encoding/json"
	"time"
)

// Who owns a bound account's credential. A sub_token binding is fed access
// tokens by Sub2API. A vm_managed binding has no token on this side at all: the
// runtime logs the account in and refreshes it, and every request goes through
// the bridge with nothing but the bridge secret.
const (
	OpenAIVMModeSubToken = "sub_token"
	OpenAIVMModeManaged  = "vm_managed"

	// OpenAIVMManagedExtraKey marks a vm_managed account. Only the managed
	// service sets it; the database guard keeps it in step with the binding.
	OpenAIVMManagedExtraKey = "openai_vm_managed"
	// OpenAIAuthModeVMManaged is the credential kind reported for such accounts.
	OpenAIAuthModeVMManaged = "vmManaged"
)

// OpenAIVMConfig deliberately never serializes the encrypted bridge secret.
type OpenAIVMConfig struct {
	Enabled          bool   `json:"enabled"`
	BaseURL          string `json:"base_url"`
	ProxyURL         string `json:"proxy_url"`
	SecretConfigured bool   `json:"secret_configured"`
	EncryptedSecret  string `json:"-"`
}

type OpenAIVMConfigInput struct {
	Enabled  bool   `json:"enabled"`
	BaseURL  string `json:"base_url"`
	ProxyURL string `json:"proxy_url"`
	Secret   string `json:"secret"`
}

type OpenAIVMBinding struct {
	AccountID   int64     `json:"account_id"`
	SlotID      string    `json:"slot_id"`
	Status      string    `json:"status"`
	RuntimeType string    `json:"runtime_type"`
	LastError   string    `json:"last_error"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Mode        string    `json:"mode"`
	// ManagedStatus and ManagedDetail are the last state synced from the bridge
	// for a vm_managed binding; the detail never holds a token or a password.
	ManagedStatus      string          `json:"managed_status,omitempty"`
	ManagedDetail      json.RawMessage `json:"managed_detail,omitempty"`
	ActivatedAt        *time.Time      `json:"activated_at,omitempty"`
	SyncedAt           *time.Time      `json:"synced_at,omitempty"`
	ManagedProxyID     int64           `json:"-"`
	OriginalProxyID    *int64          `json:"-"`
	OriginalProxyIDs   []int64         `json:"-"`
	OriginalFallbackID *int64          `json:"-"`
}

type OpenAIVMRepository interface {
	GetConfig(context.Context) (*OpenAIVMConfig, error)
	SaveConfig(context.Context, *OpenAIVMConfig) error
	GetBinding(context.Context, int64) (*OpenAIVMBinding, error)
	// WithLock serializes account operations across server instances; ID 0
	// exclusively locks global configuration against concurrent provisioning.
	WithLock(context.Context, int64, func() error) error
	CountBindings(context.Context) (int, error)
	Reserve(context.Context, int64, string, *Proxy) (*OpenAIVMBinding, error)
	SetState(context.Context, int64, string, string, string) error
	Restore(context.Context, int64) error
}

// OpenAIVMManagedRepository is the part of the binding store only vm_managed
// accounts use. The repository implements it next to OpenAIVMRepository.
type OpenAIVMManagedRepository interface {
	// ReserveManaged is Reserve for an account that carries the managed mark
	// and exactly one SOCKS5 proxy.
	ReserveManaged(ctx context.Context, id int64, runtime string, p *Proxy) (*OpenAIVMBinding, error)
	SetManaged(ctx context.Context, id int64, status string, detail json.RawMessage) error
	// MarkActivated reports true exactly once, at the first successful login.
	MarkActivated(ctx context.Context, id int64) (bool, error)
	ListManaged(ctx context.Context) ([]*OpenAIVMBinding, error)
	SetOriginalProxy(ctx context.Context, id, proxyID int64) error
	// ReleaseManaged drops the binding and its bridge route so the account can be deleted.
	ReleaseManaged(ctx context.Context, id int64) error
}

func (b *OpenAIVMBinding) Managed() bool { return b != nil && b.Mode == OpenAIVMModeManaged }

func openAIVMOwnerID(a *Account) int64 {
	if a == nil {
		return 0
	}
	if a.ParentAccountID != nil {
		return *a.ParentAccountID
	}
	return a.ID
}

func (a *Account) IsOpenAIVMBound() bool {
	if a == nil {
		return false
	}
	bound, _ := a.Extra["openai_vm_bound"].(bool)
	return bound
}

// IsOpenAIVMManaged reports whether the runtime, not Sub2API, holds this
// account's credential. Such an account must never reach a token code path.
func (a *Account) IsOpenAIVMManaged() bool {
	if a == nil {
		return false
	}
	managed, _ := a.Extra[OpenAIVMManagedExtraKey].(bool)
	return managed
}
