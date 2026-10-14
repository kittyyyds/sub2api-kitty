package middleware

import (
	"strings"
	"testing"
)

func TestSanitizedLogPathHidesVideoReferenceCapability(t *testing.T) {
	token := strings.Repeat("ab", 32)
	if got := sanitizedLogPath("/media/video-references/" + token); got != "/media/video-references/:id" {
		t.Fatalf("capability was not redacted: %q", got)
	}
	for _, path := range []string{"/v1/video-references", "/canvas-api/v1/video-references", "/v1/videos/task/content", "/health"} {
		if sanitizedLogPath(path) != path {
			t.Fatalf("ordinary route changed: %s", path)
		}
	}
}
