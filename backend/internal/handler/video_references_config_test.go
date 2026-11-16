//go:build unit

package handler

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVideoReferencePublicBaseConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, base string
		enabled    bool
	}{
		{"internal_http_disables_reference_upload", "http://kitty-prod-sub-app:8080", false},
		{"https_public_root_enables_reference_upload", "https://kittyapi.xyz", true},
		{"url_with_path_disables_reference_upload", "https://kittyapi.xyz/v1", false},
		{"empty_url_disables_reference_upload", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VIDEO_REFERENCE_PUBLIC_BASE_URL", tc.base)
			t.Setenv("VIDEO_REFERENCE_STORAGE_DIR", t.TempDir())
			h := NewVideoReferenceHandler()
			require.Equal(t, tc.enabled, h.store != nil)
			if tc.enabled {
				require.Equal(t, tc.base, h.baseURL)
			}
		})
	}
}
