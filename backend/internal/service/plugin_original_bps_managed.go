package service

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
)

// This is the unmodified author's signed 0.6.12 manifest. Other releases must
// be reviewed separately; package signature and file verification still apply.
//
//go:embed plugin_original_bps_manifest.json
var originalBPSManifest []byte

func isPinnedOriginalBPS(manifest PluginManifest) bool {
	var pinned PluginManifest
	if json.Unmarshal(originalBPSManifest, &pinned) != nil {
		return false
	}
	a, err := json.Marshal(manifest)
	if err != nil {
		return false
	}
	b, err := json.Marshal(pinned)
	return err == nil && bytes.Equal(a, b)
}

func isReviewedOriginalBPSManaged(row *PluginInstallation) bool {
	if row == nil || row.PluginKey != "local.oai-basispoints" ||
		row.Version != "0.6.12" || row.SignatureStatus != PluginSignatureTrusted ||
		!isPinnedOriginalBPS(row.Manifest) {
		return false
	}
	runtime, ok := row.Manifest.Runtimes[row.Manifest.RuntimeKey()]
	if !ok || row.Manifest.Files[runtime.Path] == "" || row.BinarySHA256 != row.Manifest.Files[runtime.Path] {
		return false
	}
	for _, b := range row.Bindings {
		if b.Enabled && (b.Capability != PluginCapabilityOpenAIOAuthOutbound || b.Platform != PlatformOpenAI || b.AccountType != AccountTypeOAuth) {
			return false
		}
	}
	return true
}

func managedPluginRequiresLeader(row *PluginInstallation) bool {
	return row != nil && row.PluginKey == "local.oai-basispoints"
}

func (m *PluginManager) managedPluginAllowed(row *PluginInstallation) bool {
	if !managedPluginRequiresLeader(row) {
		return true
	}
	c := managedinstance.Current()
	if c == nil {
		return true
	}
	_, allowed := c.LeadershipContext()
	return allowed && isReviewedOriginalBPSManaged(row)
}

// The controller retains its lease until this hook finishes. Request-owned
// streams complete naturally before we terminate the author's background loop.
func (m *PluginManager) stopManagedRuntimes() {
	if c := managedinstance.Current(); c != nil {
		for {
			s := c.Snapshot()
			if !s.LeaseHealthy || (s.ActiveRequests == 0 && s.ActiveSSE == 0 && s.BackgroundActive == 0) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	m.mu.Lock()
	var runtimes []*pluginRuntime
	for id, runtime := range m.runtimes {
		if managedPluginRequiresLeader(runtime.installation) {
			runtimes = append(runtimes, runtime)
			m.removeRuntimeLocked(id)
		}
	}
	m.mu.Unlock()
	for _, runtime := range runtimes {
		runtime.drainUntilComplete()
	}
}

// Author-side probes obtain identities through this host service. Deny new
// probes once draining begins while leaving KV persistence available for the
// work already in progress. No change to the signed plugin is needed.
func managedPluginHostAccess(pluginKey string) bool {
	if pluginKey != "local.oai-basispoints" {
		return true
	}
	if c := managedinstance.Current(); c != nil {
		_, allowed := c.LeadershipContext()
		return allowed
	}
	return true
}
