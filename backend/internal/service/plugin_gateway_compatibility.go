package service

import (
	"bytes"
	"encoding/json"
	"sync/atomic"
)

const gatewayPromptPluginID = "local.sub2api.gateway-group-prompt"
const gatewayPromptPublisherID = "gateway-group-prompt-2026-v1"
const gatewayPromptPublisherKey = "mBfDrRIPB64kU1qSfK6Wk/O5Wi/dfJpmTvalHuDzdJk="
const gatewayPromptPackageSHA256 = "26b4e22bdf0279bfa7475930fe54423c54c3fca7e9003540a838341e56a9a80a"
const gatewayPromptPinnedManifest = `{"schema_version":1,"id":"local.sub2api.gateway-group-prompt","name":"Gateway Group Prompt","version":"1.2.1","description":"Injects administrator configured prompts by API key group and requested model.","author":"Sub2API local plugin","requires":{"sub2api":">=2.8.5 <=2.9.1","recommended_sub2api_version":"2.9.1","tested_sub2api_versions":["2.8.5","2.9.1"],"plugin_protocol":1,"transport_api":1,"ui_bridge":1},"capabilities":[{"id":"gateway.request_transform.v1","platform":"gateway","account_type":"any"}],"runtimes":{"windows-amd64":{"path":"runtimes/windows-amd64/plugin.exe"},"linux-amd64":{"path":"runtimes/linux-amd64/plugin"},"linux-arm64":{"path":"runtimes/linux-arm64/plugin"}},"ui":{"entrypoint":"ui/index.html"},"files":{"runtimes/windows-amd64/plugin.exe":"0971d228c1c78e3635afb818983777ff6eb2fbdf6255686a40617a7533e52cc3","runtimes/linux-amd64/plugin":"f7c597afeb8decce60e6ea9fc6b266cc19cc4892d4ce6ed1f0b9060fc34f7832","runtimes/linux-arm64/plugin":"74c130e251168241b1405e570acb3aaa13313393aeaefd14a246008c7338ef3c","ui/index.html":"37fa6205dd669b273cd837157689408953472db85ec9c958f05f0c019083df46"}}`

// This is a release-specific compatibility attestation, not a semver bypass
// for other plugins. The installer additionally requires the exact package hash.
func isPinnedGatewayPrompt(m PluginManifest) bool {
	if m.ID != gatewayPromptPluginID || m.Version != "1.2.1" {
		return false
	}
	var pinned PluginManifest
	if json.Unmarshal([]byte(gatewayPromptPinnedManifest), &pinned) != nil {
		return false
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(pinned)
	return bytes.Equal(a, b)
}

func pluginInstallationCapability(i *PluginInstallation) string {
	if i != nil && len(i.Manifest.Capabilities) == 1 && i.Manifest.Capabilities[0].ID == PluginCapabilityGatewayTransform {
		return PluginCapabilityGatewayTransform
	}
	return PluginCapabilityOpenAIOAuthOutbound
}

// routeSlot returns the gateway transform route, the only capability that still
// publishes a runtime. Retired capabilities get a detached empty slot, so a
// stray load reads nil and a stray store cannot replace the transform route.
func (m *PluginManager) routeSlot(capability string) *atomic.Pointer[pluginRoute] {
	if capability == PluginCapabilityGatewayTransform {
		return &m.transformRoute
	}
	return new(atomic.Pointer[pluginRoute])
}

func hasEnabledCapability(bindings []PluginBinding, capability string) bool {
	for _, b := range bindings {
		if !b.Enabled || b.Capability != capability {
			continue
		}
		if capability == PluginCapabilityGatewayTransform && b.Platform == "gateway" && b.AccountType == "any" {
			return true
		}
		if capability == PluginCapabilityOpenAIOAuthOutbound && b.Platform == PlatformOpenAI && b.AccountType == AccountTypeOAuth {
			return true
		}
	}
	return false
}

func hasEnabledPluginBinding(bindings []PluginBinding) bool {
	return hasEnabledOpenAIBinding(bindings) || hasEnabledCapability(bindings, PluginCapabilityGatewayTransform)
}
