package service

import (
	"encoding/json"
	"math"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Common account policy is applied before a slot is provisioned or can log in.
// Login credentials and alternate upstream routes never belong in this policy.
type OpenAIVMManagedSettings struct {
	Notes               *string            `json:"notes"`
	GroupIDs            []int64            `json:"group_ids"`
	Concurrency         int                `json:"concurrency"`
	Priority            int                `json:"priority"`
	RateMultiplier      *float64           `json:"rate_multiplier"`
	GroupRateMultiplier *float64           `json:"group_rate_multiplier"`
	LoadFactor          *int               `json:"load_factor"`
	ExpiresAt           *int64             `json:"expires_at"`
	AutoPauseOnExpired  *bool              `json:"auto_pause_on_expired"`
	Credentials         map[string]any     `json:"credentials"`
	Extra               map[string]any     `json:"extra"`
	GroupAllowedModels  map[int64][]string `json:"group_allowed_models"`
}

var managedPolicyCredentials = map[string]bool{
	"model_mapping": true, "temp_unschedulable_enabled": true, "temp_unschedulable_rules": true,
}
var managedPolicyExtra = map[string]bool{
	"cost_multiplier": true, "upstream_request_id_header": true,
	"openai_passthrough": true, "openai_responses_flatten_namespaces": true,
	"openai_long_context_billing_enabled": true, "codex_cli_only": true, "codex_cli_only_allow_app_server": true,
	"openai_oauth_responses_websockets_v2_mode": true, "openai_oauth_responses_websockets_v2_enabled": true,
	"openai_image_size_tiers": true,
}

func (in *OpenAIVMManagedSettings) validate() error {
	bad := func() error {
		return infraerrors.BadRequest("OPENAI_VM_SETTINGS_INVALID", "Invalid VM2 account settings; login credentials and direct upstream routes are not accepted")
	}
	if in == nil {
		return nil
	}
	if in.Concurrency < 1 || in.Concurrency > 256 || in.Priority < 0 || in.Priority > 1000000 {
		return bad()
	}
	for _, rate := range []*float64{in.RateMultiplier, in.GroupRateMultiplier} {
		if rate != nil && (math.IsNaN(*rate) || math.IsInf(*rate, 0) || *rate < 0 || *rate > 1000000) {
			return bad()
		}
	}
	if in.LoadFactor != nil && (*in.LoadFactor < 1 || *in.LoadFactor > 10000) {
		return bad()
	}
	for k := range in.Credentials {
		if !managedPolicyCredentials[k] {
			return bad()
		}
	}
	for k := range in.Extra {
		if !managedPolicyExtra[k] {
			return bad()
		}
	}
	if err := ValidateAccountCostMultiplierExtra(in.Extra); err != nil {
		return bad()
	}
	if err := ValidateGroupAllowedModels(in.GroupAllowedModels); err != nil {
		return err
	}
	groups := map[int64]bool{}
	for _, id := range in.GroupIDs {
		if id <= 0 {
			return bad()
		}
		groups[id] = true
	}
	for id := range in.GroupAllowedModels {
		if !groups[id] {
			return bad()
		}
	}
	raw, err := json.Marshal(in)
	if err != nil || len(raw) > 32768 {
		return bad()
	}
	return nil
}

func (in *OpenAIVMManagedSettings) apply(target *CreateAccountInput) {
	if in == nil {
		return
	}
	target.Notes, target.GroupIDs = in.Notes, append([]int64(nil), in.GroupIDs...)
	target.Concurrency, target.Priority = in.Concurrency, in.Priority
	target.RateMultiplier, target.GroupRateMultiplier, target.LoadFactor = in.RateMultiplier, in.GroupRateMultiplier, in.LoadFactor
	target.ExpiresAt, target.AutoPauseOnExpired = in.ExpiresAt, in.AutoPauseOnExpired
	// Each account receives its own nested maps; normalizers may mutate them.
	raw, _ := json.Marshal(in.Credentials)
	_ = json.Unmarshal(raw, &target.Credentials)
	if target.Credentials == nil {
		target.Credentials = map[string]any{}
	}
	raw, _ = json.Marshal(in.Extra)
	_ = json.Unmarshal(raw, &target.Extra)
	if target.Extra == nil {
		target.Extra = map[string]any{}
	}
	target.Extra[OpenAIVMManagedExtraKey] = true
}
