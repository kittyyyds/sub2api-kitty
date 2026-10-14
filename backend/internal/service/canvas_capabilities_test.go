package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanvasVideoCapabilitiesFollowValidatedContract(t *testing.T) {
	for id, spec := range suanliVideoModels {
		t.Run(id, func(t *testing.T) {
			capability := CanvasCapabilities(id, PlatformVideo)
			require.True(t, capability.Supported)
			require.Equal(t, []string{spec.resolution}, capability.Resolutions)
			var payload map[string]any
			if capability.Family == "sd25" {
				payload = map[string]any{"model": id, "prompt": "test", "images": []string{"https://example.com/reference.png"}, "resolution": spec.resolution, "seconds": capability.Seconds.Fixed}
			} else {
				dimension := map[string]string{"480p": "480x480", "720p": "720x720", "1080p": "1080x1080", "2k": "1440x1440"}[spec.resolution]
				payload = map[string]any{"model": id, "prompt": "test", "seconds": capability.Seconds.Max, "size": dimension,
					"reference_images": []map[string]string{{"url": "https://example.com/reference.png", "role": "reference_image"}}}
			}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			_, _, err = normalizeSuanliVideoRequest(GrokMediaEndpointVideosGenerations, body, "application/json")
			require.NoError(t, err)
			payload["seconds"] = capability.Seconds.Max + 1
			body, _ = json.Marshal(payload)
			_, _, err = normalizeSuanliVideoRequest(GrokMediaEndpointVideosGenerations, body, "application/json")
			require.Error(t, err)
		})
	}
}

func TestCanvasUnknownVideoCatalogIsNotAnExecutableModel(t *testing.T) {
	capability := CanvasCapabilities("supplier-new-model", PlatformVideo)
	require.Equal(t, "video", capability.MediaType)
	require.False(t, capability.Supported)
	require.Empty(t, capability.Operations)
	require.False(t, CanvasCapabilities("text-embedding-3-large", PlatformOpenAI).Supported)
	require.True(t, CanvasCapabilities("gpt-5.6-sol", PlatformOpenAI).Supported)
}
