//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func retiredBPSInstallation(state string, enabled bool) *PluginInstallation {
	return &PluginInstallation{
		ID: 41, PluginKey: "local.kitty.bps-ws", Version: "1.0.1", State: state, BinarySHA256: "abc",
		Manifest: PluginManifest{Capabilities: []PluginCapability{{
			ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth,
		}}},
		Bindings: []PluginBinding{{
			Capability: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth,
			Enabled: enabled, RolloutPercent: 100,
		}},
	}
}

type retiredPluginRepo struct {
	PluginRepository
	rows []*PluginInstallation
}

func (r retiredPluginRepo) List(context.Context) ([]*PluginInstallation, error) { return r.rows, nil }

func (r retiredPluginRepo) GetByID(_ context.Context, id int64) (*PluginInstallation, error) {
	for _, row := range r.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return nil, context.Canceled
}

func TestRetiredBPSPluginIsSkippedAtBootAndManagedUpdate(t *testing.T) {
	row := retiredBPSInstallation(PluginStateEnabled, true)
	m := &PluginManager{
		repo:               retiredPluginRepo{rows: []*PluginInstallation{row}},
		runtimes:           make(map[int64]*pluginRuntime),
		localInstallations: make(map[int64]*PluginInstallation),
	}
	// Reconciliation never starts the retired runtime and does not fail.
	require.NoError(t, m.reconcileOnce(context.Background()))
	require.NoError(t, m.reconcileOnce(context.Background()))
	require.Empty(t, m.runtimes)
	require.Nil(t, m.transformRoute.Load())
	// An enabled stale row must not block a managed update.
	require.NoError(t, m.ValidateManagedStartup(context.Background()))
}

func TestRetiredBPSPluginCannotBeEnabledOrRestored(t *testing.T) {
	row := retiredBPSInstallation(PluginStateDisabled, false)
	m := &PluginManager{
		repo:               retiredPluginRepo{rows: []*PluginInstallation{row}},
		runtimes:           make(map[int64]*pluginRuntime),
		localInstallations: make(map[int64]*PluginInstallation),
	}
	_, err := m.Enable(context.Background(), row.ID, true, 100)
	require.ErrorIs(t, err, errPluginCapabilityRetired)
	_, err = m.ensureLocalInstallation(context.Background(), row)
	require.ErrorIs(t, err, errPluginCapabilityRetired)

	listed, err := m.List(context.Background())
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, pluginRetiredCapabilityMessage, listed[0].RuntimeMessage)
	require.False(t, listed[0].Compatibility.Compatible)
}

func TestRetiredOutboundCapabilityIsRejectedForNewPackages(t *testing.T) {
	manifest := PluginManifest{Capabilities: []PluginCapability{{
		ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth,
	}}}
	require.True(t, pluginManifestRetired(manifest))
	require.False(t, pluginManifestRetired(PluginManifest{Capabilities: []PluginCapability{{
		ID: PluginCapabilityGatewayTransform, Platform: "gateway", AccountType: "any",
	}}}))
	result := EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "0.2.21-custom.1-kitty.15"})
	require.False(t, result.Compatible)
	require.Equal(t, pluginRetiredCapabilityMessage, result.Message)
}
