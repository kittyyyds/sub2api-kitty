package kittyrisk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// Exercise the actual HTTP transport and parser with local servers, including
// distinct-provider confirmation. No paid upstream participates in this test.
func TestKittyRemoteRestrictionConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, primary, secondary, want string
		disabled                       bool
	}{
		{"confirmed", "restricted", "restricted", "restricted", false},
		{"disagreement", "restricted", "allow", "allow", false},
		{"disabled", "restricted", "restricted", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, settings := gateService(t)
			cfg := defaultContentModerationConfig()
			cfg.Enabled = !tc.disabled
			cfg.ReviewRelease = svc.releaseID
			var first, second atomic.Int64
			server := func(verdict string, calls *atomic.Int64) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					require.Equal(t, "Bearer local-test-only", r.Header.Get("Authorization"))
					category, confidence := "safe", 0.01
					if verdict == "restricted" {
						category = ContentModerationRestrictedCategory
						confidence = 0.95
					}
					body, _ := json.Marshal(map[string]any{"disposition": verdict, "category": category, "confidence": confidence, "reason": "isolated verification"})
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(body)}}}})
				}))
			}
			a, b := server(tc.primary, &first), server(tc.secondary, &second)
			defer a.Close()
			defer b.Close()
			cfg.DeepSeekChannels = []ContentModerationDeepSeekChannel{
				{ID: "primary", Name: "test-primary", Provider: ContentModerationRemoteProviderDeepSeek, BaseURL: a.URL, Model: "deepseek-v4-flash", Enabled: true, TimeoutMS: 2000, APIKey: "local-test-only"},
				{ID: "confirm", Name: "test-confirm", Provider: ContentModerationRemoteProviderGLM, BaseURL: b.URL, Model: "glm-4.7-flash", Enabled: true, TimeoutMS: 2000, APIKey: "local-test-only"},
			}
			gateConfig(t, settings, cfg)
			result, _, err := svc.scanContentModerationRemotePool(context.Background(), cfg, contentModerationSecondLayerInput{Fragment: ContentModerationFragment{Text: "isolated sample"}})
			if tc.disabled {
				require.Error(t, err)
				require.Zero(t, first.Load())
				require.Zero(t, second.Load())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, result.normalizedDisposition())
				require.Positive(t, first.Load())
				require.Positive(t, second.Load())
				require.Equal(t, 2, result.RemoteVotes)
			}
		})
	}
}
