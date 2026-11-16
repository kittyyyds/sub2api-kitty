package service

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoUnifiedCapability4KNormalization(t *testing.T) {
	for _, tc := range []struct {
		name        string
		resolutions []string
		resolution  string
	}{
		{"lowercase", []string{"4k"}, "4k"},
		{"official spelling", []string{"4k"}, " 4K "},
		{"uppercase capability", []string{"4K"}, "4k"},
		{"fixed resolution default", []string{"4K"}, ""},
		{"multiple resolutions", []string{"720p", "4k"}, "4K"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := unifiedFixtureSpec()
			spec.Resolutions = tc.resolutions
			require.NoError(t, spec.Validate())
			body, err := json.Marshal(map[string]any{
				"model": "verified-4k-fixture", "prompt": "A landscape", "seconds": 5,
				"resolution": tc.resolution, "aspect_ratio": "16:9",
			})
			require.NoError(t, err)
			input, err := PrepareUnifiedVideoInput(body, "application/json", spec)
			require.NoError(t, err)
			require.Equal(t, "4k", input.Resolution)
			out, err := json.Marshal(input)
			require.NoError(t, err)
			require.Contains(t, string(out), `"resolution":"4k"`)
			// Validation must not rewrite shared capability slices.
			require.Equal(t, tc.resolutions, spec.Resolutions)
		})
	}
}

func TestVideoUnifiedCapability4KMultipart(t *testing.T) {
	spec := unifiedFixtureSpec()
	spec.Resolutions = []string{"4k"}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"model": "verified-4k-fixture", "prompt": "A landscape", "seconds": "5",
		"resolution": "4K", "aspect_ratio": "16:9",
	} {
		require.NoError(t, w.WriteField(key, value))
	}
	require.NoError(t, w.Close())
	input, err := PrepareUnifiedVideoInput(body.Bytes(), w.FormDataContentType(), spec)
	require.NoError(t, err)
	require.Equal(t, "4k", input.Resolution)
	capability := (&VideoModelConfig{Spec: spec}).Capability()
	require.True(t, capability.Supported)
	require.Equal(t, []string{"4k"}, capability.Resolutions)
}

func TestVideoUnifiedCapabilityResolutionStillRequiresModelSupport(t *testing.T) {
	for _, tc := range []struct {
		resolutions []string
		resolution  string
	}{
		{[]string{"720p", "1080p"}, "4k"},
		{[]string{"4k"}, "720p"},
		{[]string{"4k"}, "8k"},
		{[]string{"720p", "4k"}, ""},
	} {
		spec := unifiedFixtureSpec()
		spec.Resolutions = tc.resolutions
		body, err := json.Marshal(map[string]any{
			"model": "fixture", "prompt": "A landscape", "seconds": 5, "resolution": tc.resolution,
		})
		require.NoError(t, err)
		_, err = PrepareUnifiedVideoInput(body, "application/json", spec)
		require.Error(t, err, "resolutions=%v requested=%q", tc.resolutions, tc.resolution)
	}
	spec := unifiedFixtureSpec()
	spec.Resolutions = []string{"8k"}
	require.Error(t, spec.Validate())
}

func TestVideoUnifiedCapabilityDocumentedSD4K(t *testing.T) {
	for _, model := range []string{
		"优质sd2.0vip4k(9-3-3不排队速度极快不卡脸4K满血)",
		"官转稳定-sd2-4k(933满血卡脸4K)",
		"官方直连sd2.0(满血全参不排队4K)",
	} {
		spec, ok := UnifiedVideoSpecification(model)
		require.True(t, ok, model)
		require.Equal(t, []string{"4k"}, spec.Resolutions, model)
		// Only the explicitly scoped official model enables a no-video-reference
		// contract. The two other 4K contracts remain unchanged.
		require.Equal(t, model == "官方直连sd2.0(满血全参不排队4K)", spec.Supported, model)
		require.False(t, spec.ReferenceBilled, model)
	}
}
