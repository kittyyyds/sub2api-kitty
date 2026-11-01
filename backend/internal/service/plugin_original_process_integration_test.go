//go:build kitty_original_bps_integration

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
	"time"
)

type originalBPSProcessRepo struct{ pluginTokenRepository }

func (r *originalBPSProcessRepo) MarkRuntimeHealthy(context.Context, int64, string, string) error {
	return nil
}

func TestOriginalBPSManagedSignedProcess(t *testing.T) {
	controller, _ := originalBPSController(t)
	cfg := testPluginConfig(t.TempDir(), false)
	public, err := os.ReadFile(os.Getenv("BPS_HOST_TEST_PUBLIC_KEY"))
	require.NoError(t, err)
	cfg.Plugins.TrustedPublishers["oai-basispoints-v1"] = strings.TrimSpace(string(public))
	host := PluginHostInfo{Version: "2.19.0-kitty.2", BuildType: "release"}
	f, err := os.Open(os.Getenv("BPS_HOST_TEST_PACKAGE"))
	require.NoError(t, err)
	defer f.Close()
	installed, err := NewPluginPackageInstaller(cfg, host).Install(context.Background(), f, nil)
	require.NoError(t, err)
	installed.ID = 5
	installed.State = PluginStateEnabled
	installed.ConfigEncrypted = "ENC:{\"enabled_models\":[],\"auto_degradation_enabled\":false}"
	installed.Bindings = originalBPSFixture(t).Bindings
	repo := &originalBPSProcessRepo{pluginTokenRepository{installation: installed}}
	m := NewPluginManager(repo, pluginTokenEncryptor{}, cfg, host, newFakePluginKVStore())
	require.NoError(t, m.ValidateManagedStartup(context.Background()))
	require.NoError(t, m.Start(context.Background()))
	defer m.Stop()
	require.Nil(t, m.route.Load().runtime, "standby spawned signed process")
	require.NoError(t, controller.SetRole("active"))
	controller.Step(context.Background())
	require.NoError(t, m.reconcileOnce(context.Background()))
	runtime := m.route.Load().runtime
	require.NotNil(t, runtime)
	require.NoError(t, runtime.checkHealth(context.Background()))
	require.True(t, runtime.beginRequest())
	controller.Drain()
	controller.Step(context.Background())
	require.Eventually(t, func() bool { return runtime.draining.Load() }, time.Second, time.Millisecond)
	require.False(t, runtime.client.Exited(), "normal drain killed a live response")
	require.True(t, controller.Snapshot().LeadershipOwned, "lease released before child stopped")
	runtime.finishRequest()
	require.Eventually(t, func() bool { controller.Step(context.Background()); return controller.Snapshot().LeadershipReleased }, 3*time.Second, 10*time.Millisecond)
	require.True(t, runtime.client.Exited(), "old signed child survived handoff")
	require.Nil(t, m.route.Load())
	t.Log("signed author process: standby absent; active healthy; response drained; child exited before lease release")
}
