package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Supply only the pinned installed executable in a network-isolated container.
// This test uses synthetic inputs and no database, credentials or paid supplier.
func TestGatewayPromptInstalledBinaryLongConversation(t *testing.T) {
	path := os.Getenv("SUB2API_TEST_GATEWAY_PROMPT_BINARY")
	if path == "" {
		t.Skip("isolated installed executable not supplied")
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	const expected = "f7c597afeb8decce60e6ea9fc6b266cc19cc4892d4ce6ed1f0b9060fc34f7832"
	require.Equal(t, expected, hex.EncodeToString(sum[:]))
	var manifest PluginManifest
	require.NoError(t, json.Unmarshal([]byte(gatewayPromptPinnedManifest), &manifest))
	installation := &PluginInstallation{PluginKey: gatewayPromptPluginID, Version: "1.2.1", BinaryPath: path, BinarySHA256: expected, Manifest: manifest}
	dir, err := os.MkdirTemp("", "gp-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	r, err := startPluginRuntime(context.Background(), installation, 10*time.Second, dir, nil)
	require.NoError(t, err)
	defer r.kill()
	require.NoError(t, r.validateAndApplyConfig(context.Background(), []byte(`{"rules":[{"group_ids":[7],"enabled":true,"prompt":"GENERAL","protocols":["responses"]}]}`)))
	m := &PluginManager{}
	m.transformRoute.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100, runtime: r})
	for _, length := range []int{8 << 20, GatewayPromptMaxBody - 256} {
		body := []byte(`{"model":"m","instructions":"OLD","input":[{"role":"user","content":"` + strings.Repeat("a", length) + `"}],"tools":[{"type":"function","name":"keep"}],"stream":true}`)
		require.Less(t, len(body), GatewayPromptMaxBody)
		out, err := m.TransformGatewayRequest(context.Background(), 7, 8, "responses", "m", body)
		require.NoError(t, err)
		require.Equal(t, "GENERAL\n\nOLD", gjson.GetBytes(out, "instructions").String())
		require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(out, "input").Raw)
		require.Equal(t, gjson.GetBytes(body, "tools").Raw, gjson.GetBytes(out, "tools").Raw)
		require.True(t, gjson.GetBytes(out, "stream").Bool())
	}
	require.Zero(t, r.inFlight.Load())
}
