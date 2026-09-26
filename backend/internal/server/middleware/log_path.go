package middleware

import "strings"

// Reference IDs are bearer download capabilities, not ordinary public names.
// Both the context logger and access logger must redact them before attachment.
func sanitizedLogPath(path string) string {
	if strings.HasPrefix(path, "/media/video-references/") {
		return "/media/video-references/:id"
	}
	return path
}
