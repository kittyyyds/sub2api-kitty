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
		// This fixed package only transforms an individual request. Each instance
		// restores and verifies its copy and reconciles the shared configuration.
		if isPinnedGatewayPrompt(row.Manifest) && row.SignatureStatus == PluginSignatureTrusted &&
			row.BinarySHA256 == row.Manifest.Files[row.Manifest.Runtimes[row.Manifest.RuntimeKey()].Path] {
			reviewed := true
			for _, binding := range row.Bindings {
				if binding.Enabled && (binding.Capability != PluginCapabilityGatewayTransform || binding.Platform != "gateway" || binding.AccountType != "any") {
					reviewed = false
				}
			}
			if reviewed {
				continue
			}
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
