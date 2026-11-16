package service

import (
	"context"
	"errors"
)

// ValidateManagedStartup reads plugin state without changing it. Enabled
// plugins need a separately reviewed multi-instance lifecycle contract.
func (m *PluginManager) ValidateManagedStartup(ctx context.Context) error {
	if m == nil || m.repo == nil {
		return errors.New("managed plugin state is unavailable")
	}
	rows, err := m.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row == nil {
			return errors.New("managed plugin state is invalid")
		}
		// Retired OpenAI OAuth outbound (BPS) rows never start, so they cannot
		// affect a managed update even if the database still marks them enabled.
		if pluginInstallationRetired(row) || managedPluginLifecycleReviewed(row, row.Manifest.RuntimeKey()) {
			continue
		}
		if row.State == PluginStateStarting || row.State == PluginStateEnabled {
			return errors.New("enabled plugins require a reviewed multi-instance lifecycle before a managed update")
		}
		for _, binding := range row.Bindings {
			if binding.Enabled {
				return errors.New("enabled plugin bindings require a reviewed multi-instance lifecycle before a managed update")
			}
		}
	}
	return nil
}

// The pinned prompt package processes individual host-owned requests.
// It does not run an account pool, refresh credentials, write usage, or own
// scheduled work. Each host restores its own verified runtime; the host
// lifecycle drains active HTTP/SSE/WS requests before stopping that runtime.
// Other packages, versions and binding scopes still require a separate review.
func managedPluginLifecycleReviewed(row *PluginInstallation, runtimeKey string) bool {
	if !isPinnedGatewayPrompt(row.Manifest) {
		return false
	}
	capability, platform, accountType := PluginCapabilityGatewayTransform, "gateway", "any"
	runtime, exists := row.Manifest.Runtimes[runtimeKey]
	expectedHash := row.Manifest.Files[runtime.Path]
	if !exists || expectedHash == "" || row.SignatureStatus != PluginSignatureTrusted || row.BinarySHA256 != expectedHash {
		return false
	}
	for _, binding := range row.Bindings {
		if binding.Enabled && (binding.Capability != capability || binding.Platform != platform || binding.AccountType != accountType) {
			return false
		}
	}
	return true
}
