package service

import (
	"maps"
	"strings"
)

// IsRetiredAccountExtraKey identifies obsolete credentials and configuration.
// Keep this boundary for old backups, stale caches and full-account writes;
// these keys no longer enable any gateway behavior. The Codex ticket keys are
// left over from the removed ticket harvesting and may hold cookies or proxy
// credentials, so they are never read, exported or written back. The
// openai_excel_bps* keys belong to the removed Excel BPS route and its 403
// automation; they no longer change routing, scheduling or account status.
func IsRetiredAccountExtraKey(key string) bool {
	return key == "upstream_billing_rate_limit" || strings.HasPrefix(key, "codex_ticket_") ||
		strings.HasPrefix(key, "codex_turn_ticket:") || key == "codex_turn_cookies" ||
		key == "codex_harvest_proxy_url" || key == "codex_skip_harvest" ||
		key == "openai_excel_bps" || strings.HasPrefix(key, "openai_excel_bps_")
}

// StripRetiredAccountExtra returns a copy without obsolete account material.
// It must not mutate shared scheduler snapshots or the caller's input.
func StripRetiredAccountExtra(extra map[string]any) map[string]any {
	clean := maps.Clone(extra)
	for key := range clean {
		if IsRetiredAccountExtraKey(key) {
			delete(clean, key)
		}
	}
	if mode, _ := clean["codex_fingerprint_mode"].(string); strings.TrimSpace(mode) == "single_machine_multi_window" {
		clean["codex_fingerprint_mode"] = "off"
	}
	return clean
}
