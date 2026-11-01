package reauthruntime

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBPSConnectionRejectsChangedRuntime(t *testing.T) {
	_, err := patchBPSConnectionSource("changed dependency")
	require.Error(t, err)
}
func TestBPSConnectionPinnedRuntimeSyntax(t *testing.T) {
	fixture := os.Getenv("BPS_PROTOCOL_FIXTURE")
	if fixture == "" {
		t.Skip("requires the verified runtime source fixture")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tosub2", "src"), 0700))
	for _, name := range []string{"protocol-login.mjs", "tls-transport.mjs"} {
		data, err := os.ReadFile(filepath.Join(fixture, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, "tosub2", "src", name), data, 0600))
	}
	for _, direct := range []bool{false, true} {
		tmp := t.TempDir()
		require.NoError(t, prepareBPSConnectionSource(root, tmp, direct))
		for _, name := range []string{"protocol-login.mjs", "tls-transport.mjs", "bps_connection_login.mjs"} {
			out, err := exec.Command("node", "--check", filepath.Join(tmp, "src", name)).CombinedOutput()
			require.NoError(t, err, string(out))
		}
		data, err := os.ReadFile(filepath.Join(tmp, "src", "protocol-login.mjs"))
		require.NoError(t, err)
		require.Contains(t, string(data), "callback_url:codex.callbackUrl")
		require.NotContains(t, string(data), "console.log(codex.callbackUrl)")
	}
}
func TestBPSConnectionCanceledBeforeLogin(t *testing.T) {
	m := New(t.TempDir(), "2.16.0", "http://127.0.0.1:1", "synthetic")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.BPSConnectionLogin(ctx, "fixture@example.invalid", "synthetic", "JBSWY3DPEHPK3PXP", "", "https://auth.openai.com/", "fixture")
	require.ErrorIs(t, err, context.Canceled)
}
func TestBPSConnectionChildDoesNotInheritSecrets(t *testing.T) {
	t.Setenv("OPENAI_REAUTH_WORKER_TOKEN", "must-not-inherit")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c := passwordLoginCommand(ctx, "/runtime", "/tmp/task", "fixture@example.invalid", "synthetic-password", "synthetic-totp", "")
	require.NotContains(t, strings.Join(c.Args, " "), "synthetic-password")
	require.NotContains(t, strings.Join(c.Env, "\n"), "must-not-inherit")
}
