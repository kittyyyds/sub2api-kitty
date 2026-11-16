// Package vmbridge implements the separately deployed, account-scoped vm2api bridge.
package vmbridge

import (
	"context"
	"encoding/json"
	"net/http"
)

// ProvisionRequest contains an access token only. Refresh tokens are never accepted.
type ProvisionRequest struct {
	AccountID        int64  `json:"-"`
	Name             string `json:"name"`
	AccessToken      string `json:"access_token"`
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	UpstreamProxyURL string `json:"upstream_proxy_url"`
	RuntimeType      string `json:"runtime_type"`
}

type Identity struct {
	AccessToken      string `json:"access_token"`
	ChatGPTAccountID string `json:"chatgpt_account_id"`
}

// Resources identifies only resources belonging to this bridge account. APIKey
// is encrypted at rest and must never be returned by the management API.
type Resources struct {
	VMID    string `json:"vm_id"`
	KeyID   string `json:"key_id"`
	APIKey  string `json:"api_key"`
	ProxyID string `json:"proxy_id,omitempty"`
}

// Provider operations must be idempotent and account-scoped. Provision may
// return partial resources with an error so cleanup can still recover them.
// Forward accepts a request whose body is the untouched Responses body.
type Provider interface {
	Provision(context.Context, ProvisionRequest) (Resources, error)
	UpdateIdentity(context.Context, Resources, Identity) error
	Delete(context.Context, Resources) error
	Forward(context.Context, Resources, *http.Request) (*http.Response, error)
}

// ReadinessChecker is optional. Implementations backed by an external control
// plane can verify that the persisted slot and scoped key still exist before a
// management status is shown as ready. A failed check never mutates the
// lifecycle record; the next status request may recover automatically.
type ReadinessChecker interface {
	CheckReady(context.Context, Resources) error
}

// Binding modes. A sub_token binding is fed access tokens by Sub2API, which
// keeps the refresh token. A vm_managed binding has no credential on the
// Sub2API side at all: the runtime logs the account in, holds and refreshes
// its tokens, and the bridge only relays management operations to it.
const (
	ModeSubToken  = "sub_token"
	ModeVMManaged = "vm_managed"
)

// ManagedProvisionRequest creates a slot before any credential exists.
type ManagedProvisionRequest struct {
	AccountID        int64  `json:"-"`
	Name             string `json:"name"`
	UpstreamProxyURL string `json:"upstream_proxy_url"`
	RuntimeType      string `json:"runtime_type"`
}

// ManagedSnapshot is what the runtime reports about one managed slot. It never
// carries a token, a password, a 2FA secret or a proxy credential.
type ManagedSnapshot struct {
	Status              string           `json:"status"`
	Schedulable         bool             `json:"schedulable"`
	ScheduleState       string           `json:"schedule_state,omitempty"`
	RestrictionReason   string           `json:"restriction_reason,omitempty"`
	RestrictionText     string           `json:"restriction_text,omitempty"`
	RestrictionUntil    int64            `json:"restriction_until,omitempty"`
	UnschedulableReason string           `json:"unschedulable_reason,omitempty"`
	UnschedulableText   string           `json:"unschedulable_text,omitempty"`
	HasToken            bool             `json:"has_token"`
	HasRefresh          bool             `json:"has_refresh"`
	RefreshError        string           `json:"refresh_error,omitempty"`
	ExpiresAt           string           `json:"expires_at,omitempty"`
	Email               string           `json:"email,omitempty"`
	ChatGPTAccountID    string           `json:"chatgpt_account_id,omitempty"`
	PlanType            string           `json:"plan_type,omitempty"`
	SubscriptionEnd     string           `json:"subscription_end,omitempty"`
	RuntimeType         string           `json:"runtime_type,omitempty"`
	Execution           string           `json:"execution,omitempty"`
	MaxConcurrency      int              `json:"max_concurrency,omitempty"`
	ProxyLabel          string           `json:"proxy_label,omitempty"`
	ProxyStatus         string           `json:"proxy_status,omitempty"`
	Usage               json.RawMessage  `json:"usage,omitempty"`
	Credits             json.RawMessage  `json:"credits,omitempty"`
	ResetCredits        json.RawMessage  `json:"reset_credits,omitempty"`
	SavedLogin          *SavedLoginState `json:"saved_login,omitempty"`
	AutoRelogin         json.RawMessage  `json:"auto_relogin,omitempty"`
}

// SavedLoginState says a login is saved on the runtime; the login itself stays there.
type SavedLoginState struct {
	Email        string `json:"email"`
	HasPassword  bool   `json:"has_password"`
	HasMFASecret bool   `json:"has_mfa_secret"`
	MFAMode      string `json:"mfa_mode,omitempty"`
}

// ManagedCall is one relayed management operation on a managed slot. Body is
// the caller's JSON, already size-limited by the HTTP layer; it may hold a
// password or a 2FA secret and must be neither stored nor logged.
type ManagedCall struct {
	Operation string
	Body      json.RawMessage
}

// ManagedProvider is optional. A provider that implements it lets the bridge
// offer vm_managed bindings; without it those routes answer 501.
type ManagedProvider interface {
	// ProvisionManaged creates the slot, binds its exit and issues the scoped
	// key, leaving the slot stopped, unscheduled and without a credential.
	ProvisionManaged(context.Context, ManagedProvisionRequest) (Resources, error)
	// Snapshot reads the slot's current state from the runtime.
	Snapshot(context.Context, Resources) (*ManagedSnapshot, error)
	// Activate starts a logged-in slot and opens it to scheduling.
	Activate(context.Context, Resources) error
	// Rebind moves the slot to another exit and returns the new proxy row ID.
	Rebind(ctx context.Context, resources Resources, upstreamProxyURL string) (string, error)
	// Call relays one whitelisted operation and returns the runtime's public
	// JSON answer. A *CallError carries a status and code safe to pass on.
	Call(context.Context, Resources, ManagedCall) (json.RawMessage, error)
}

// CallError is a relayed operation's failure, reduced to what may be shown:
// the runtime's HTTP status and its short error code. No upstream body.
type CallError struct {
	Status int
	Code   string
}

func (e *CallError) Error() string { return "managed operation failed: " + e.Code }
