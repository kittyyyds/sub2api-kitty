package service

import (
	"encoding/json"
	"strings"
	"time"
)

// What an administrator sees for a VM2-managed account. Derived from the
// bridge's lifecycle state and the slot's own report; never from a guess about
// the credential made on this side.
const (
	OpenAIVMManagedAvailable      = "available"
	OpenAIVMManagedAwaitingLogin  = "awaiting_login"
	OpenAIVMManagedNeedsRelogin   = "needs_relogin"
	OpenAIVMManagedNeedsAttention = "needs_attention"
	OpenAIVMManagedReloginRunning = "relogin_running"
	OpenAIVMManagedRestricted     = "restricted"
	OpenAIVMManagedPaused         = "paused"
	OpenAIVMManagedStopped        = "stopped"
	OpenAIVMManagedSlotError      = "slot_error"
	OpenAIVMManagedProvisioning   = "provisioning"
	OpenAIVMManagedStarting       = "starting"
	OpenAIVMManagedError          = "error"
	OpenAIVMManagedDeleting       = "deleting"
	OpenAIVMManagedUnknown        = "unknown"
)

// openAIVMManagedSnapshot mirrors the bridge's managed snapshot. The bridge has
// already reduced it to non-secret fields.
type openAIVMManagedSnapshot struct {
	Status              string          `json:"status"`
	Schedulable         bool            `json:"schedulable"`
	ScheduleState       string          `json:"schedule_state"`
	RestrictionReason   string          `json:"restriction_reason"`
	RestrictionText     string          `json:"restriction_text"`
	RestrictionUntil    int64           `json:"restriction_until"`
	UnschedulableReason string          `json:"unschedulable_reason"`
	HasToken            bool            `json:"has_token"`
	RefreshError        string          `json:"refresh_error"`
	Email               string          `json:"email"`
	ChatGPTAccountID    string          `json:"chatgpt_account_id"`
	PlanType            string          `json:"plan_type"`
	SubscriptionEnd     string          `json:"subscription_end"`
	MaxConcurrency      int             `json:"max_concurrency"`
	Usage               json.RawMessage `json:"usage"`
	AutoRelogin         *struct {
		LastResult     string `json:"last_result"`
		NeedsAttention bool   `json:"needs_attention"`
	} `json:"auto_relogin"`
}

type openAIVMManagedRemote struct {
	State         string                   `json:"state"`
	Ready         bool                     `json:"ready"`
	Starting      bool                     `json:"starting"`
	VMID          string                   `json:"vm_id"`
	LastErrorCode string                   `json:"last_error_code"`
	Snapshot      *openAIVMManagedSnapshot `json:"snapshot"`
}

// Reasons with which the runtime says "this slot's login is gone".
var openAIVMManagedCredentialReasons = map[string]bool{
	"no_credential": true, "oauth_revoked": true, "oauth_invalid_grant": true, "oauth_no_refresh": true,
	"oauth_cleared": true, "oauth_expired": true, "credential_refresh_failed": true, "credential_blocked": true,
}

func openAIVMManagedReloginStatus(snap *openAIVMManagedSnapshot) string {
	if snap != nil && snap.AutoRelogin != nil {
		if snap.AutoRelogin.NeedsAttention {
			return OpenAIVMManagedNeedsAttention
		}
		if snap.AutoRelogin.LastResult == "running" {
			return OpenAIVMManagedReloginRunning
		}
	}
	return OpenAIVMManagedNeedsRelogin
}

// deriveOpenAIVMManagedStatus turns a bridge reading into the status shown to
// the administrator, plus the time until which a restriction is known to last
// (zero when the runtime gave none).
func deriveOpenAIVMManagedStatus(remote *openAIVMManagedRemote, activated bool, now time.Time) (string, time.Time) {
	if remote == nil {
		return OpenAIVMManagedUnknown, time.Time{}
	}
	snap := remote.Snapshot
	switch remote.State {
	case "provisioning":
		return OpenAIVMManagedProvisioning, time.Time{}
	case "deleting":
		return OpenAIVMManagedDeleting, time.Time{}
	case "failed":
		return OpenAIVMManagedError, time.Time{}
	}
	if remote.Starting && (remote.State == "awaiting_login" || remote.State == "ready") {
		// The slot boots in the background (a KVM guest takes a minute or more).
		return OpenAIVMManagedStarting, time.Time{}
	}
	if remote.State == "awaiting_login" && remote.LastErrorCode == "activate_failed" {
		// Logged in, but the slot would not start; the bridge tries again shortly.
		return OpenAIVMManagedSlotError, time.Time{}
	}
	switch remote.State {
	case "awaiting_login":
		if !activated {
			return OpenAIVMManagedAwaitingLogin, time.Time{}
		}
		return openAIVMManagedReloginStatus(snap), time.Time{}
	case "ready":
	default:
		return OpenAIVMManagedUnknown, time.Time{}
	}
	if snap == nil {
		// The runtime could not be read. Routing follows the bridge's lifecycle
		// record; nothing about the account is changed on a missing reading.
		return OpenAIVMManagedUnknown, time.Time{}
	}
	if !snap.HasToken || snap.RefreshError != "" || openAIVMManagedCredentialReasons[snap.UnschedulableReason] {
		return openAIVMManagedReloginStatus(snap), time.Time{}
	}
	if status := strings.TrimSpace(snap.Status); status != "" && status != "running" {
		return OpenAIVMManagedStopped, time.Time{}
	}
	switch snap.ScheduleState {
	case "restricted":
		var until time.Time
		if snap.RestrictionUntil > 0 {
			if t := time.UnixMilli(snap.RestrictionUntil); t.After(now) {
				until = t
			}
		}
		return OpenAIVMManagedRestricted, until
	case "off":
		return OpenAIVMManagedPaused, time.Time{}
	case "unschedulable":
		if snap.UnschedulableReason == "organization_disabled" {
			return OpenAIVMManagedNeedsAttention, time.Time{}
		}
		return OpenAIVMManagedSlotError, time.Time{}
	}
	if !snap.Schedulable {
		return OpenAIVMManagedPaused, time.Time{}
	}
	return OpenAIVMManagedAvailable, time.Time{}
}

// openAIVMManagedBindingStatus maps the bridge's lifecycle state to the binding
// status the request path checks ("ready" is the only one that routes).
func openAIVMManagedBindingStatus(state string) string {
	switch state {
	case "ready":
		return "ready"
	case "awaiting_login":
		return "awaiting_login"
	case "failed":
		return "error"
	}
	return ""
}

// openAIVMManagedUsageExtra converts the slot's usage view into the account
// fields the usage columns already read. Only windows the runtime reported are
// written, so a slot that has not served a request yet shows nothing.
func openAIVMManagedUsageExtra(raw json.RawMessage) map[string]any {
	var view struct {
		Limits map[string]any `json:"limits"`
		Source struct {
			UpdatedAt string `json:"updated_at"`
		} `json:"snapshot"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &view) != nil || len(view.Limits) == 0 {
		return nil
	}
	updates := map[string]any{}
	for _, window := range []string{"5h", "7d"} {
		used, ok := view.Limits["used_"+window+"_percent"].(float64)
		if !ok {
			continue
		}
		updates["codex_"+window+"_used_percent"] = used
		if at, ok := view.Limits["reset_"+window+"_at"].(string); ok && strings.TrimSpace(at) != "" {
			updates["codex_"+window+"_reset_at"] = at
		}
		if seconds, ok := view.Limits["reset_"+window+"_seconds"].(float64); ok && seconds >= 0 {
			updates["codex_"+window+"_reset_after_seconds"] = int(seconds)
		}
		if minutes, ok := view.Limits["window_"+window+"_minutes"].(float64); ok && minutes > 0 {
			updates["codex_"+window+"_window_minutes"] = int(minutes)
		}
	}
	if len(updates) == 0 {
		return nil
	}
	if strings.TrimSpace(view.Source.UpdatedAt) != "" {
		updates["codex_usage_updated_at"] = view.Source.UpdatedAt
	}
	return updates
}

// Fields of the slot report that are kept for the managed panel. The bridge
// already reduces its snapshot to these; filtering again here means a field
// added on the other side is never stored or shown by accident.
var openAIVMManagedDetailFields = []string{
	"status", "schedulable", "schedule_state", "restriction_reason", "restriction_text", "restriction_until",
	"unschedulable_reason", "unschedulable_text", "has_token", "has_refresh", "refresh_error", "expires_at",
	"email", "chatgpt_account_id", "plan_type", "subscription_end", "runtime_type", "execution",
	"max_concurrency", "proxy_label", "proxy_status", "usage", "credits", "reset_credits", "auto_relogin",
}

// openAIVMManagedDetail keeps the allow-listed fields of a slot snapshot. The
// saved-login entry is rebuilt from its four public fields: it says that a
// login is saved on the runtime, never what the login is.
func openAIVMManagedDetail(raw json.RawMessage) json.RawMessage {
	var in map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &in) != nil || in == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(openAIVMManagedDetailFields)+1)
	for _, name := range openAIVMManagedDetailFields {
		if value, ok := in[name]; ok {
			out[name] = value
		}
	}
	var saved *struct {
		Email        string `json:"email"`
		HasPassword  bool   `json:"has_password"`
		HasMFASecret bool   `json:"has_mfa_secret"`
		MFAMode      string `json:"mfa_mode,omitempty"`
	}
	if json.Unmarshal(in["saved_login"], &saved) == nil && saved != nil {
		if encoded, err := json.Marshal(saved); err == nil {
			out["saved_login"] = encoded
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return encoded
}
