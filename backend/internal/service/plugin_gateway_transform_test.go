package service

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayPromptMutationBoundary(t *testing.T) {
	for _, tc := range []struct {
		before, after, field string
		ok                   bool
	}{
		{`{"model":"x","instructions":"old","n":9007199254740993}`, `{"model":"x","instructions":"new\n\nold","n":9007199254740993}`, "instructions", true},
		{`{"model":"x"}`, `{"model":"y","instructions":"new"}`, "instructions", false},
		{`{"instructions":"old"}`, `{"instructions":"new"}`, "instructions", false},
		{`{"messages":[{"role":"user","content":"old"}]}`, `{"messages":[{"role":"system","content":"new"},{"role":"user","content":"old"}]}`, "messages", true},
		{`{"messages":[{"role":"user","content":"old"}]}`, `{"messages":[{"role":"system","content":"new"}]}`, "messages", false},
		{`{"system":[{"type":"text","text":"old","cache_control":{"type":"ephemeral"}}]}`, `{"system":[{"type":"text","text":"new"},{"type":"text","text":"old","cache_control":{"type":"ephemeral"}}]}`, "system", true},
		{`{"model":"x"}`, `null`, "instructions", false},
		{`{"n":9007199254740993}`, `{"n":9007199254740992}`, "instructions", false},
	} {
		err := validateGatewayPromptMutation([]byte(tc.before), []byte(tc.after), tc.field)
		if tc.ok {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestGatewayPromptPinnedCompatibilityAndManagedStartup(t *testing.T) {
	var manifest PluginManifest
	require.NoError(t, json.Unmarshal([]byte(gatewayPromptPinnedManifest), &manifest))
	host := PluginHostInfo{Version: "2.9.9-kitty.3", BuildType: "release"}
	require.True(t, EvaluatePluginCompatibility(manifest, host).Tested)
	require.False(t, EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.9.9-kitty.2"}).Compatible)
	i := &PluginInstallation{Manifest: manifest, State: PluginStateEnabled, SignatureStatus: PluginSignatureTrusted, BinarySHA256: manifest.Files[manifest.Runtimes[manifest.RuntimeKey()].Path], Bindings: []PluginBinding{{Enabled: true, Capability: PluginCapabilityGatewayTransform, Platform: "gateway", AccountType: "any"}}}
	m := &PluginManager{repo: managedPluginRepo{rows: []*PluginInstallation{i}}}
	require.NoError(t, m.ValidateManagedStartup(context.Background()))
	i.BinarySHA256 = "modified"
	require.Error(t, m.ValidateManagedStartup(context.Background()))
	manifest.Version = "1.2.2"
	require.False(t, EvaluatePluginCompatibility(manifest, host).Compatible)
}

// Execute only with an explicitly provided package in an isolated Linux container.
func TestGatewayPromptRealPackage(t *testing.T) {
	path := os.Getenv("SUB2API_TEST_GATEWAY_PROMPT_PACKAGE")
	if path == "" {
		t.Skip("isolated package not supplied")
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	ctx := context.Background()
	root := t.TempDir()
	cfg := testPluginConfig(root, false)
	host := PluginHostInfo{Version: "2.9.9-kitty.3", BuildType: "release"}
	installer := NewPluginPackageInstaller(cfg, host)
	i, err := installer.Install(ctx, bytes.NewReader(data), nil)
	require.NoError(t, err)
	require.Equal(t, PluginSignatureTrusted, i.SignatureStatus)
	require.True(t, i.Compatibility.Compatible)
	socket := filepath.Join(root, "sock")
	require.NoError(t, os.MkdirAll(socket, 0700))
	r, err := startPluginRuntime(ctx, i, 10*time.Second, socket, nil)
	require.NoError(t, err)
	defer r.kill()
	m := NewPluginManager(nil, pluginTokenEncryptor{}, cfg, host, nil)
	m.transformRoute.Store(&pluginRoute{capability: PluginCapabilityGatewayTransform, pluginID: 1, runtime: r, rolloutPercent: 100})
	require.NoError(t, r.validateAndApplyConfig(ctx, []byte(`{"rules":[]}`)))
	original := []byte(`{ "model":"m", "instructions":"original", "input":"hello", "stream":true }`)
	out, err := m.TransformGatewayRequest(ctx, 7, 8, "responses", "m", original)
	require.NoError(t, err)
	require.Equal(t, string(original), string(out))
	rules := []byte(`{"rules":[{"group_ids":[7],"models":[],"enabled":true,"prompt":"GENERAL","protocols":["anthropic","responses","chat_completions"]},{"group_ids":[7],"models":["special"],"enabled":true,"prompt":"SPECIFIC","protocols":["anthropic","responses","chat_completions"]}]}`)
	require.NoError(t, r.validateAndApplyConfig(ctx, rules))
	for _, tc := range []struct{ name, protocol, body, field, want string }{
		{"responses", "responses", `{"model":"m","instructions":"OLD","input":[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}],"tools":[{"type":"function","name":"f","parameters":{"type":"object"}}],"stream":true}`, "instructions", "GENERAL\n\nOLD"},
		{"anthropic-string", "anthropic", `{"model":"m","system":"OLD","messages":[{"role":"user","content":"hello"}],"max_tokens":12}`, "system", "GENERAL\n\nOLD"},
		{"anthropic-block", "anthropic", `{"model":"m","system":[{"type":"text","text":"OLD","cache_control":{"type":"ephemeral"}}],"messages":[]}`, "system.0.text", "GENERAL"},
		{"chat", "chat_completions", `{"model":"m","messages":[{"role":"system","content":"OLD"},{"role":"user","content":"hello"}],"tools":[],"stream":true}`, "messages.0.content", "GENERAL"},
		{"priority", "responses", `{"model":"special","input":"hello"}`, "instructions", "SPECIFIC"},
		{"case-sensitive", "responses", `{"model":"Special","input":"hello"}`, "instructions", "GENERAL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := m.TransformGatewayRequest(ctx, 7, 8, tc.protocol, gjson.Get(tc.body, "model").String(), []byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, tc.want, gjson.GetBytes(out, tc.field).String())
		})
	}
	for _, gid := range []int64{0, 9} {
		out, err := m.TransformGatewayRequest(ctx, gid, 8, "responses", "m", original)
		require.NoError(t, err)
		require.Equal(t, string(original), string(out))
	}
	inherited := []byte(`{"type":"response.create","input":"second"}`)
	out, err = m.TransformGatewayRequest(ctx, 7, 8, "responses", "special", inherited)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(out, "model").Exists())
	require.Equal(t, "SPECIFIC", gjson.GetBytes(out, "instructions").String())
	out, err = m.TransformGatewayRequest(ctx, 7, 8, "gemini", "m", original)
	require.NoError(t, err)
	require.Equal(t, string(original), string(out))
	require.Error(t, r.validateAndApplyConfig(ctx, []byte(`{"rules":[{"group_ids":[7],"enabled":true,"prompt":"A"},{"group_ids":[7],"enabled":true,"prompt":"B"}]}`)))
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := m.TransformGatewayRequest(ctx, 7, 8, "responses", "m", original)
			if e != nil || gjson.GetBytes(out, "instructions").String() != "GENERAL\n\noriginal" {
				t.Errorf("concurrent transform failed: %v", e)
			}
		}()
	}
	wg.Wait()
	require.Zero(t, r.inFlight.Load())
	// The OAuth route must survive gateway route removal, and vice versa.
	oauth := &pluginRoute{pluginID: 99, rolloutPercent: 100}
	m.route.Store(oauth)
	m.detachCapabilityRuntimes(PluginCapabilityGatewayTransform)
	require.Same(t, oauth, m.route.Load())
	m.transformRoute.Store(&pluginRoute{pluginID: 1, runtime: r, rolloutPercent: 100})
	m.detachCapabilityRuntimes(PluginCapabilityOpenAIOAuthOutbound)
	require.True(t, m.HasGatewayPrompt())
	// A second instance restores the signed artifact into its own empty filesystem.
	repo := &gatewayArtifactRepo{pluginTokenRepository: pluginTokenRepository{installation: i}, artifact: data}
	m2 := NewPluginManager(repo, pluginTokenEncryptor{}, testPluginConfig(t.TempDir(), false), host, nil)
	restored, err := m2.ensureLocalInstallation(ctx, i)
	require.NoError(t, err)
	require.NotEqual(t, i.InstallPath, restored.InstallPath)
	require.True(t, samePluginPackage(i, restored))
	require.NoError(t, os.MkdirAll(filepath.Join(m2.installer.RootDir(), "runtime"), 0700))
	r2, err := m2.prepareRuntime(ctx, restored, true)
	require.NoError(t, err)
	defer r2.kill()
	require.NoError(t, r2.checkHealth(ctx))
}

type gatewayArtifactRepo struct {
	pluginTokenRepository
	artifact []byte
}

func (r *gatewayArtifactRepo) GetArtifact(context.Context, int64) ([]byte, error) {
	return r.artifact, nil
}
