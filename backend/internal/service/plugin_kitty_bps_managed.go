package service

import (
	"bytes"
	"encoding/json"
)

// Only this previously installed, signed package has been reviewed here.
// Init/config/health use process-local state; outbound work only runs for an
// explicitly forwarded request. It has no timer, queue or shared replay cache.
// Package restoration, signature checks and normal semver checks still apply.
const kittyBPSManagedManifest = `{"schema_version":1,"id":"local.kitty.oai-basispoints","name":"Kitty BPS · SUB 适配版","version":"0.1.1","description":"按已选账号与模型使用 BPS；保留 SUB 计费与代理；安装默认停用。","author":"Kitty local adaptation; CPR upstream 2han9wen71an; Sub2API contributors","requires":{"sub2api":">=2.17.7-kitty.5 <2.18.0","recommended_sub2api_version":"2.17.7-kitty.5","tested_sub2api_versions":["2.17.7-kitty.5"],"plugin_protocol":1,"transport_api":1,"ui_bridge":1},"capabilities":[{"id":"openai.oauth.outbound_transport.v1","platform":"openai","account_type":"oauth"}],"runtimes":{"linux-amd64":{"path":"runtimes/linux-amd64/plugin"},"windows-amd64":{"path":"runtimes/windows-amd64/plugin.exe"}},"ui":{"entrypoint":"ui/index.html"},"files":{"runtimes/linux-amd64/plugin":"177c1b74c588bbd734ac43f6002257b1a977631cdafd7a8166ec114eabaef91c","runtimes/windows-amd64/plugin.exe":"127d6f31628b99c3588b6213f8ffc3a5a68fa4164f37000ced4efd7161543f02","ui/app.js":"38472dcd87d0c5c8b009104d5e0d67201017b6cf181fc31de5a8ac5b411802ca","ui/index.html":"4bd53d8162b1ae6dcc7eccd87c216e0188baed3595d3dc4c50a116a65da60307","ui/style.css":"30ee713cb9b99c3a3094c60351648f6f9e69982c3c37030a0fd67df171b15a2a","licenses/CPR-Basispoints-MIT.txt":"e59f5fc1fa58223d37eded40e538898379aa18c9bf264803b19d2319ffbd1a64","licenses/SUB-LGPL-3.0.txt":"f831e7eed577481687a9bc0b48024e5e40b6f655fcde073ede964b50be5d55d9"}}`

const kittyBPSPreparedManifest = `{"schema_version":1,"id":"local.kitty.oai-basispoints","name":"Kitty BPS · SUB 适配版","version":"0.1.2","description":"按已选账号与模型使用 BPS；保留 SUB 计费与代理；安装默认停用。","author":"Kitty local adaptation; CPR upstream 2han9wen71an; Sub2API contributors","requires":{"sub2api":">=2.17.7-kitty.5 <2.18.0","recommended_sub2api_version":"2.17.7-kitty.7","tested_sub2api_versions":["2.17.7-kitty.7"],"plugin_protocol":1,"transport_api":1,"ui_bridge":1},"capabilities":[{"id":"openai.oauth.outbound_transport.v1","platform":"openai","account_type":"oauth"}],"runtimes":{"linux-amd64":{"path":"runtimes/linux-amd64/plugin"},"windows-amd64":{"path":"runtimes/windows-amd64/plugin.exe"}},"ui":{"entrypoint":"ui/index.html"},"files":{"runtimes/linux-amd64/plugin":"13e2e39c7d14690cdffed2ee4b6321f768c2fc34d2d8643550fc18e00ba92796","runtimes/windows-amd64/plugin.exe":"3aa28510f609ff3d5a9c6fe2abe2241070a39721434b4cb3cc13479cad8035b4","ui/app.js":"38472dcd87d0c5c8b009104d5e0d67201017b6cf181fc31de5a8ac5b411802ca","ui/index.html":"2c1e54c46d1e281898a37975e1fe5ff1df9baa7dbd9fddb6bfa197edf80de6ff","ui/style.css":"30ee713cb9b99c3a3094c60351648f6f9e69982c3c37030a0fd67df171b15a2a","licenses/CPR-Basispoints-MIT.txt":"e59f5fc1fa58223d37eded40e538898379aa18c9bf264803b19d2319ffbd1a64","licenses/SUB-LGPL-3.0.txt":"f831e7eed577481687a9bc0b48024e5e40b6f655fcde073ede964b50be5d55d9"}}`

func isReviewedKittyBPSManaged(row *PluginInstallation) bool {
	if row == nil || row.SignatureStatus != PluginSignatureTrusted {
		return false
	}
	var pinned PluginManifest
	raw := kittyBPSManagedManifest
	if row.Manifest.Version == "0.1.2" {
		raw = kittyBPSPreparedManifest
	}
	if json.Unmarshal([]byte(raw), &pinned) != nil {
		return false
	}
	a, err := json.Marshal(row.Manifest)
	if err != nil {
		return false
	}
	b, err := json.Marshal(pinned)
	if err != nil || !bytes.Equal(a, b) {
		return false
	}
	runtime, ok := pinned.Runtimes[pinned.RuntimeKey()]
	if !ok || pinned.Files[runtime.Path] == "" || row.BinarySHA256 != pinned.Files[runtime.Path] {
		return false
	}
	for _, binding := range row.Bindings {
		if binding.Enabled && (binding.Capability != PluginCapabilityOpenAIOAuthOutbound || binding.Platform != PlatformOpenAI || binding.AccountType != AccountTypeOAuth) {
			return false
		}
	}
	return true
}

func isPinnedKittyBPSPrepared(manifest PluginManifest) bool {
	var pinned PluginManifest
	if json.Unmarshal([]byte(kittyBPSPreparedManifest), &pinned) != nil {
		return false
	}
	actual, err := json.Marshal(manifest)
	if err != nil {
		return false
	}
	expected, err := json.Marshal(pinned)
	return err == nil && bytes.Equal(actual, expected)
}
