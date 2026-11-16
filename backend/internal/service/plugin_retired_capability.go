package service

import (
	"errors"
	"log/slog"
	"strconv"
)

// The OpenAI OAuth outbound transport capability only served the removed
// Kitty BPS WebSocket plugin (local.kitty.bps-ws). Installation rows for it may
// remain in the database: they are still listed so an administrator can
// disable and uninstall them, but they are never started, enabled, tested or
// configured, and new packages declaring the capability are rejected.
const pluginRetiredCapabilityMessage = "OpenAI OAuth 出站能力（BPS 插件）已移除，此插件不会再启动；请停用后卸载"

var errPluginCapabilityRetired = errors.New(pluginRetiredCapabilityMessage)

// pluginManifestRetired reports a manifest that declares the retired OpenAI
// OAuth outbound capability.
func pluginManifestRetired(manifest PluginManifest) bool {
	for _, capability := range manifest.Capabilities {
		if capability.ID == PluginCapabilityOpenAIOAuthOutbound {
			return true
		}
	}
	return false
}

func pluginInstallationRetired(installation *PluginInstallation) bool {
	return installation != nil && pluginManifestRetired(installation.Manifest)
}

// logRetiredInstallations writes one log line per installation package that
// the database still marks enabled, instead of failing reconciliation.
func (m *PluginManager) logRetiredInstallations(installations []*PluginInstallation) {
	for _, installation := range installations {
		if !pluginInstallationRetired(installation) {
			continue
		}
		if installation.State != PluginStateEnabled && installation.State != PluginStateStarting && !hasEnabledPluginBinding(installation.Bindings) {
			continue
		}
		key := strconv.FormatInt(installation.ID, 10) + ":" + installation.BinarySHA256
		if _, logged := m.retiredLogged.LoadOrStore(key, struct{}{}); logged {
			continue
		}
		slog.Warn("plugin_retired_capability_skipped",
			"plugin_id", installation.ID,
			"plugin_key", installation.PluginKey,
			"version", installation.Version,
			"reason", pluginRetiredCapabilityMessage)
	}
}
