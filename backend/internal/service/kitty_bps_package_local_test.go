package service

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
)

func TestKittyBPSActualSignedPackage(t *testing.T) {
	root := os.Getenv("KITTY_BPS_DIST")
	if root == "" {
		t.Skip("explicit release directory required")
	}
	var pub struct {
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "publisher.public.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &pub); err != nil {
		t.Fatal(err)
	}
	var release struct {
		Package string `json:"package"`
	}
	raw, err = os.ReadFile(filepath.Join(root, "release.public.json"))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(raw, &release)
	pkg, err := os.ReadFile(filepath.Join(root, release.Package))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Plugins: config.PluginConfig{DataDir: t.TempDir(), TrustedPublishers: map[string]string{pub.KeyID: pub.PublicKey}, MaxUploadBytes: 64 << 20, MaxUncompressedBytes: 128 << 20, StartTimeoutSeconds: 15}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "2.17.7-kitty.7", BuildType: "release"})
	installed, err := installer.Install(ctx, bytes.NewReader(pkg), nil)
	if err != nil {
		t.Fatal(err)
	}
	if installed.SignatureStatus != PluginSignatureTrusted || installed.State != PluginStateDisabled || !installed.Compatibility.Compatible {
		t.Fatalf("unexpected install status: %+v", installed.Compatibility)
	}
	socket := filepath.Join(cfg.Plugins.DataDir, "runtime")
	if err = os.MkdirAll(socket, 0700); err != nil {
		t.Fatal(err)
	}
	runtime, err := startPluginRuntime(ctx, installed, 15*time.Second, socket, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.kill()
	if err = runtime.validateAndApplyConfig(ctx, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	test, err := runtime.api.TestConfig(ctx, &pluginv1.TestConfigRequest{ConfigJson: []byte("{}")})
	if err != nil || !test.Success {
		t.Fatal(test, err)
	}
	cfg.Plugins.TrustedPublishers = map[string]string{}
	if _, err = NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "2.17.7-kitty.7"}).Install(ctx, bytes.NewReader(pkg), nil); err == nil {
		t.Fatal("untrusted publisher accepted")
	}
	t.Log("signed package, file hashes, disabled installation, exact host compatibility, child runtime, normalized config and local self-test passed; missing trust rejected")
}
