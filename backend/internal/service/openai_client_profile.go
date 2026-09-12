package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// OpenAIClientProfileExtraKey is an account-scoped, opt-in client identity and
// transport profile. It intentionally does not include Codex convergence IDs,
// conversation IDs, or tool/turn state. Those values are managed by the
// existing isolation layer and must remain stable for continuation.
const OpenAIClientProfileExtraKey = "openai_client_profile"

type OpenAIClientProfile struct {
	Enabled                bool   `json:"enabled"`
	Preset                 string `json:"preset"`
	Language               string `json:"language,omitempty"`
	ClientVersion          string `json:"client_version,omitempty"`
	HTTPVersion            string `json:"http_version,omitempty"`
	TLSProfile             string `json:"tls_profile,omitempty"`
	ConnectionReuse        string `json:"connection_reuse,omitempty"`
	MaxConnsPerHost        int    `json:"max_conns_per_host,omitempty"`
	MaxIdleConnsPerHost    int    `json:"max_idle_conns_per_host,omitempty"`
	IdleConnTimeoutSeconds int    `json:"idle_conn_timeout_seconds,omitempty"`
	WSMode                 string `json:"ws_mode,omitempty"`
	RetryBackoffInitialMS  int    `json:"retry_backoff_initial_ms,omitempty"`
	RetryBackoffMaxMS      int    `json:"retry_backoff_max_ms,omitempty"`
	RetryTotalBudgetMS     int    `json:"retry_total_budget_ms,omitempty"`
}

const (
	OpenAIClientPresetDefault = "default"
	OpenAIClientPresetMacOS   = "macos"
	OpenAIClientPresetLinux   = "linux"
	OpenAIClientPresetWindows = "windows"
)

// OpenAIClientProfilePresetUA returns a safe, internally consistent Codex UA.
// The profile changes the declared client environment only; it is not a claim
// that the server has become that operating system.
func OpenAIClientProfilePresetUA(preset, version string) string {
	version = NormalizeCodexClientVersion(version)
	if version == "" {
		version = CodexCanonicalClientVersion()
	}
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case OpenAIClientPresetMacOS:
		return fmt.Sprintf("codex-tui/%s (Mac OS X 14.5; arm64) iTerm2", version)
	case OpenAIClientPresetWindows:
		return fmt.Sprintf("codex-tui/%s (Windows 11; x86_64) WindowsTerminal", version)
	case OpenAIClientPresetLinux:
		return fmt.Sprintf("codex_cli_rs/%s (Ubuntu 22.04.0; x86_64) xterm-256color", version)
	default:
		return ""
	}
}

func (p *OpenAIClientProfile) clone() *OpenAIClientProfile {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

// GetOpenAIClientProfile returns only a valid, enabled OAuth OpenAI profile.
// A missing or disabled profile deliberately returns nil so existing accounts
// retain exactly their previous forwarding behavior.
func (a *Account) GetOpenAIClientProfile() *OpenAIClientProfile {
	if a == nil || a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth || a.IsCredentialShadow() || a.Extra == nil {
		return nil
	}
	p, err := parseOpenAIClientProfile(a.Extra[OpenAIClientProfileExtraKey])
	if err != nil || !p.Enabled {
		return nil
	}
	return p.clone()
}

func (a *Account) GetOpenAIWSMode() string {
	if p := a.GetOpenAIClientProfile(); p != nil {
		return p.WSMode
	}
	return "inherit"
}

func (a *Account) GetOpenAIClientLanguage() string {
	if p := a.GetOpenAIClientProfile(); p != nil {
		return p.Language
	}
	return ""
}

func (a *Account) GetOpenAIClientVersion() string {
	if p := a.GetOpenAIClientProfile(); p != nil {
		return p.ClientVersion
	}
	return ""
}

func parseOpenAIClientProfile(raw any) (*OpenAIClientProfile, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object")
	}
	for key := range m {
		switch key {
		case "enabled", "preset", "language", "client_version", "http_version", "tls_profile", "connection_reuse", "max_conns_per_host", "max_idle_conns_per_host", "idle_conn_timeout_seconds", "ws_mode", "retry_backoff_initial_ms", "retry_backoff_max_ms", "retry_total_budget_ms":
		default:
			return nil, fmt.Errorf("unsupported client profile field %q", key)
		}
	}
	p := &OpenAIClientProfile{Preset: OpenAIClientPresetDefault, HTTPVersion: "inherit", TLSProfile: "inherit", ConnectionReuse: "inherit", WSMode: "inherit"}
	var err error
	if p.Enabled, err = profileBool(m, "enabled", false); err != nil {
		return nil, err
	}
	if p.Preset, err = profileString(m, "preset", p.Preset); err != nil {
		return nil, err
	}
	if p.Language, err = profileString(m, "language", ""); err != nil {
		return nil, err
	}
	if p.ClientVersion, err = profileString(m, "client_version", ""); err != nil {
		return nil, err
	}
	if p.HTTPVersion, err = profileString(m, "http_version", p.HTTPVersion); err != nil {
		return nil, err
	}
	if p.TLSProfile, err = profileString(m, "tls_profile", p.TLSProfile); err != nil {
		return nil, err
	}
	if p.ConnectionReuse, err = profileString(m, "connection_reuse", p.ConnectionReuse); err != nil {
		return nil, err
	}
	if p.WSMode, err = profileString(m, "ws_mode", p.WSMode); err != nil {
		return nil, err
	}
	for key, dst := range map[string]*int{"max_conns_per_host": &p.MaxConnsPerHost, "max_idle_conns_per_host": &p.MaxIdleConnsPerHost, "idle_conn_timeout_seconds": &p.IdleConnTimeoutSeconds, "retry_backoff_initial_ms": &p.RetryBackoffInitialMS, "retry_backoff_max_ms": &p.RetryBackoffMaxMS, "retry_total_budget_ms": &p.RetryTotalBudgetMS} {
		if *dst, err = profileInt(m, key, 0); err != nil {
			return nil, err
		}
	}
	return p, validateOpenAIClientProfile(p)
}

func profileString(m map[string]any, key, fallback string) (string, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			return "", fmt.Errorf("%s must contain printable ASCII only", key)
		}
	}
	return strings.TrimSpace(s), nil
}
func profileBool(m map[string]any, key string, fallback bool) (bool, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return b, nil
}
func profileInt(m map[string]any, key string, fallback int) (int, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback, nil
	}
	var n int64
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int64:
		n = x
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || x != math.Trunc(x) || x < 0 || x > 3600000 {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		n = int64(x)
	case json.Number:
		var err error
		n, err = x.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if n < 0 || n > 3600000 {
		return 0, fmt.Errorf("%s is out of range", key)
	}
	return int(n), nil
}

func validateOpenAIClientProfile(p *OpenAIClientProfile) error {
	if p == nil {
		return nil
	}
	p.Preset = strings.ToLower(strings.TrimSpace(p.Preset))
	p.HTTPVersion = strings.ToLower(strings.TrimSpace(p.HTTPVersion))
	p.TLSProfile = strings.ToLower(strings.TrimSpace(p.TLSProfile))
	p.ConnectionReuse = strings.ToLower(strings.TrimSpace(p.ConnectionReuse))
	p.WSMode = strings.ToLower(strings.TrimSpace(p.WSMode))
	if p.Preset != "default" && p.Preset != "macos" && p.Preset != "linux" && p.Preset != "windows" {
		return fmt.Errorf("preset must be default, macos, linux, or windows")
	}
	if p.HTTPVersion != "inherit" && p.HTTPVersion != "http1" && p.HTTPVersion != "http2" {
		return fmt.Errorf("http_version must be inherit, http1, or http2")
	}
	if p.TLSProfile != "inherit" && p.TLSProfile != "chrome" && p.TLSProfile != "firefox" && p.TLSProfile != "safari" {
		return fmt.Errorf("tls_profile must be inherit, chrome, firefox, or safari")
	}
	if p.HTTPVersion == "http2" && p.TLSProfile != "inherit" {
		return fmt.Errorf("custom tls_profile currently requires http_version=inherit or http1")
	}
	if p.ConnectionReuse != "inherit" && p.ConnectionReuse != "reuse" && p.ConnectionReuse != "fresh" {
		return fmt.Errorf("connection_reuse must be inherit, reuse, or fresh")
	}
	if p.WSMode != "inherit" && p.WSMode != "off" && p.WSMode != "ctx_pool" && p.WSMode != "passthrough" {
		return fmt.Errorf("ws_mode must be inherit, off, ctx_pool, or passthrough")
	}
	if p.MaxIdleConnsPerHost > 0 && p.MaxConnsPerHost > 0 && p.MaxIdleConnsPerHost > p.MaxConnsPerHost {
		return fmt.Errorf("max_idle_conns_per_host cannot exceed max_conns_per_host")
	}
	if p.ClientVersion != "" && (NormalizeCodexClientVersion(p.ClientVersion) == "" || CompareVersions(p.ClientVersion, codexUpstreamMinVersion) < 0) {
		return fmt.Errorf("client_version must be a valid Codex version >= %s", codexUpstreamMinVersion)
	}
	if len(p.Language) > 64 || (p.Language != "" && !openAIClientLanguagePattern.MatchString(p.Language)) {
		return fmt.Errorf("language must be a valid Accept-Language value of at most 64 characters")
	}
	for _, limit := range []struct {
		name       string
		value, max int
	}{
		{"max_conns_per_host", p.MaxConnsPerHost, 256}, {"max_idle_conns_per_host", p.MaxIdleConnsPerHost, 256},
		{"idle_conn_timeout_seconds", p.IdleConnTimeoutSeconds, 600}, {"retry_backoff_initial_ms", p.RetryBackoffInitialMS, 30000},
		{"retry_backoff_max_ms", p.RetryBackoffMaxMS, 30000}, {"retry_total_budget_ms", p.RetryTotalBudgetMS, 120000},
	} {
		if limit.value < 0 || limit.value > limit.max {
			return fmt.Errorf("%s must be between 0 and %d", limit.name, limit.max)
		}
	}
	if p.RetryBackoffMaxMS > 0 && p.RetryBackoffInitialMS > p.RetryBackoffMaxMS {
		return fmt.Errorf("retry_backoff_max_ms must be >= retry_backoff_initial_ms")
	}
	return nil
}

var openAIClientLanguagePattern = regexp.MustCompile(`^(?:[A-Za-z]{1,8}(?:-[A-Za-z0-9]{1,8})*|\*)(?:\s*;\s*q=(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?))?(?:\s*,\s*(?:[A-Za-z]{1,8}(?:-[A-Za-z0-9]{1,8})*|\*)(?:\s*;\s*q=(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?))?)*$`)

// ApplyOpenAIClientProfileHeaders is a final, opt-in identity pass. It changes
// only the four declared client headers, never auth, tools or continuation IDs.
// Call after the existing canonical identity enforcement. Missing originator
// identifies non-Codex API contracts and must not be recreated here.
func ApplyOpenAIClientProfileHeaders(h http.Header, account *Account) {
	profile := account.GetOpenAIClientProfile()
	if profile == nil || h == nil || h.Get("originator") == "" {
		return
	}
	identity := resolveCodexOutboundIdentity(account.GetOpenAIUserAgent())
	if profile.ClientVersion != "" {
		identity.version = profile.ClientVersion
		identity.userAgent = openai.SetCodexUserAgentVersion(identity.userAgent, profile.ClientVersion)
	}
	h.Set("User-Agent", identity.userAgent)
	h.Set("Originator", identity.originator)
	h.Set("Version", identity.version)
	if profile.Language != "" {
		h.Set("Accept-Language", profile.Language)
	}
}

// UpdateOpenAIClientProfile performs an atomic JSONB key merge so settings from
// a dedicated modal cannot erase unrelated live account usage, quota or tokens.
// The repository preserves/creates the convergence seed atomically and publishes
// the updated account snapshot to scheduler caches.
func (s *adminServiceImpl) UpdateOpenAIClientProfile(ctx context.Context, id int64, profile map[string]any, fingerprintMode *string) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.IsCredentialShadow() {
		return nil, infraerrors.BadRequest("OPENAI_CLIENT_PROFILE_INVALID_TARGET", "client profiles require an OpenAI OAuth account")
	}
	if profile == nil {
		return nil, infraerrors.BadRequest("OPENAI_CLIENT_PROFILE_INVALID", "profile must be an object; use enabled=false to restore the existing settings")
	}
	updates := map[string]any{OpenAIClientProfileExtraKey: profile}
	if err := ValidateOpenAIClientProfileExtra(account.Platform, account.Type, updates); err != nil {
		return nil, err
	}
	if fingerprintMode != nil {
		switch *fingerprintMode {
		case "off", "device", "session", "full":
			updates[codexFingerprintModeExtraKey] = *fingerprintMode
		default:
			return nil, infraerrors.BadRequest("CODEX_FINGERPRINT_MODE_INVALID", "fingerprint_mode must be off, device, session, or full")
		}
	}
	if err := s.UpdateAccountExtra(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, id)
}

func validateAccountOpenAIClientProfileExtra(account *Account, extra map[string]any) error {
	if account == nil {
		return ErrAccountNotFound
	}
	if raw, exists := extra[OpenAIClientProfileExtraKey]; exists && raw != nil && account != nil && account.IsCredentialShadow() {
		return infraerrors.BadRequest("OPENAI_CLIENT_PROFILE_INVALID_TARGET", "client profiles must be configured on the parent OAuth account, not a credential shadow")
	}
	return ValidateOpenAIClientProfileExtra(account.Platform, account.Type, extra)
}

func ValidateOpenAIClientProfileExtra(platform, accountType string, extra map[string]any) error {
	if _, exists := extra[OpenAIClientProfileExtraKey]; !exists || extra[OpenAIClientProfileExtraKey] == nil {
		return nil
	}
	if platform != PlatformOpenAI || accountType != AccountTypeOAuth {
		return infraerrors.BadRequest("OPENAI_CLIENT_PROFILE_INVALID_TARGET", "openai_client_profile requires an OpenAI OAuth account")
	}
	if _, err := parseOpenAIClientProfile(extra[OpenAIClientProfileExtraKey]); err != nil {
		return infraerrors.BadRequest("OPENAI_CLIENT_PROFILE_INVALID", err.Error())
	}
	return nil
}
