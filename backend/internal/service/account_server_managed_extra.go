package service

import "maps"

// Server-managed account extra keys are written only by the gateway itself.
// Account edits and imports never set them and exports never include them.
func isServerManagedAccountExtraKey(key string) bool {
	return IsOpenAITiboVerdictExtraKey(key)
}

// MergeServerManagedAccountExtra drops server-managed keys supplied by an
// account edit and keeps the stored ones. The repository repeats this under
// the row lock so a concurrent gateway write is not overwritten by a stale
// admin snapshot.
func MergeServerManagedAccountExtra(extra, current map[string]any) map[string]any {
	result := maps.Clone(extra)
	for key := range result {
		if isServerManagedAccountExtraKey(key) {
			delete(result, key)
		}
	}
	for key, value := range current {
		if isServerManagedAccountExtraKey(key) {
			if result == nil {
				result = make(map[string]any)
			}
			result[key] = value
		}
	}
	return result
}

// RedactServerManagedAccountExtra strips server-managed state from exports
// without changing the source account or unrelated backup fields.
func RedactServerManagedAccountExtra(extra map[string]any) map[string]any {
	redacted := maps.Clone(extra)
	for key := range redacted {
		if isServerManagedAccountExtraKey(key) {
			delete(redacted, key)
		}
	}
	return redacted
}
