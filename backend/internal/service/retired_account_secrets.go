package service

import (
	"maps"
	"strings"
)

// IsRetiredAccountSecretKey protects historical transient credentials on read and
// export paths. No retired feature is executed or configured by this guard.
func IsRetiredAccountSecretKey(key string) bool {
	return strings.HasPrefix(key, "codex_turn_ticket:") ||
		strings.HasPrefix(key, "codex_ticket_") || strings.HasPrefix(key, "codex_harvest_")
}

func RedactRetiredAccountSecrets(extra map[string]any) map[string]any {
	result := maps.Clone(extra)
	for key := range result {
		if IsRetiredAccountSecretKey(key) {
			delete(result, key)
		}
	}
	return result
}
