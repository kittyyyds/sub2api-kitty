package service

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestManagedPluginStartupPinnedKittyBPS(t *testing.T) {
	fresh := func() *PluginInstallation {
		var manifest PluginManifest
		require.NoError(t, json.Unmarshal([]byte(kittyBPSManagedManifest), &manifest))
		return &PluginInstallation{Manifest: manifest, State: PluginStateEnabled, SignatureStatus: PluginSignatureTrusted, BinarySHA256: manifest.Files[manifest.Runtimes[manifest.RuntimeKey()].Path], Bindings: []PluginBinding{{Enabled: true, Capability: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}}}
	}
	cases := []struct {
		name   string
		change func(*PluginInstallation)
		ok     bool
	}{
		{"reviewed", func(*PluginInstallation) {}, true},
		{"unsigned", func(p *PluginInstallation) { p.SignatureStatus = PluginSignatureUnsigned }, false},
		{"different-binary", func(p *PluginInstallation) { p.BinarySHA256 = "different" }, false},
		{"different-version", func(p *PluginInstallation) { p.Manifest.Version = "0.1.2" }, false},
		{"changed-manifest", func(p *PluginInstallation) { p.Manifest.Name = "changed" }, false},
		{"unexpected-capability", func(p *PluginInstallation) { p.Bindings[0].Capability = PluginCapabilityGatewayTransform }, false},
		{"unexpected-platform", func(p *PluginInstallation) { p.Bindings[0].Platform = "gateway" }, false},
		{"unexpected-account", func(p *PluginInstallation) { p.Bindings[0].AccountType = "apikey" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := fresh()
			tc.change(p)
			m := &PluginManager{repo: managedPluginRepo{rows: []*PluginInstallation{p}}}
			err := m.ValidateManagedStartup(context.Background())
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
