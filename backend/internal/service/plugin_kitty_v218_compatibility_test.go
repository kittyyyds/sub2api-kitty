package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKittyV219PinnedInstalledPluginCompatibility(t *testing.T) {
	for name, raw := range map[string]string{"prompt": gatewayPromptPinnedManifest} {
		t.Run(name, func(t *testing.T) {
			var manifest PluginManifest
			require.NoError(t, json.Unmarshal([]byte(raw), &manifest))
			result := EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.19.0-kitty.5"})
			require.True(t, result.Compatible)
			require.True(t, result.Tested)
			for _, version := range []string{"2.18.1", "2.18.1-kitty.2", "2.19.0"} {
				require.False(t, EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: version}).Compatible)
			}
			manifest.Name += " modified"
			require.False(t, EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.19.0-kitty.5"}).Compatible)
			require.NoError(t, json.Unmarshal([]byte(raw), &manifest))
			manifest.Requires.TransportAPI++
			require.False(t, EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.19.0-kitty.5"}).Compatible)
		})
	}
}

func TestRetiredBPSPluginsCannotBeReenabled(t *testing.T) {
	for _, id := range []string{"local.oai-basispoints", "local.kitty.oai-basispoints"} {
		manifest := PluginManifest{ID: id, Requires: PluginRequirements{Sub2API: ">=2.0.0", PluginProtocol: 1, TransportAPI: 1, UIBridge: 1}}
		result := EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.19.0-kitty.5"})
		require.False(t, result.Compatible)
		require.Equal(t, "incompatible", result.Status)
	}
}

func TestKittyV219FinancePinnedPluginCompatibility(t *testing.T) {
	var manifest PluginManifest
	require.NoError(t, json.Unmarshal([]byte(gatewayPromptPinnedManifest), &manifest))
	for _, version := range []string{"2.19.0-kitty.6", "2.19.0-kitty.7", "2.19.0-kitty.8", "2.19.0-kitty.9", "2.19.0-kitty.10", "2.19.0-kitty.11", "2.19.0-kitty.12", "2.19.0-kitty.13", "2.19.0-kitty.14", "2.19.0-kitty.15"} {
		require.True(t, EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: version}).Compatible)
	}
	manifest.Name += " modified"
	require.False(t, EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.19.0-kitty.6"}).Compatible)
	manifest.ID = "local.oai-basispoints"
	require.False(t, EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.19.0-kitty.6"}).Compatible)
}
