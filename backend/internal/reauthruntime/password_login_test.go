package reauthruntime

import (
	"context"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestPasswordLoginCommandHasFixedProxyAndNoAmbientEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://unselected.invalid:9999")
	cmd := passwordLoginCommand(context.Background(), "/runtime", "/tmp/private", "fixture@example.invalid", "secret-password", "secret-totp", "http://user:secret-proxy@127.0.0.1:7777")
	args := strings.Join(cmd.Args, " ")
	require.NotContains(t, args, "secret-")
	env := strings.Join(cmd.Env, "\n")
	require.Contains(t, env, "CHATGPT_PROXY_URL=http://user:secret-proxy@127.0.0.1:7777")
	require.NotContains(t, env, "unselected")
	require.Contains(t, env, "CHATGPT_PROXY_MAX_ATTEMPTS=1")
	require.Equal(t, "/tmp/private", cmd.Dir)
}
func TestPasswordLoginRejectsMissingProxyOrUnavailableRuntime(t *testing.T) {
	m := New(t.TempDir(), "2.10.7", "http://127.0.0.1", "test")
	_, err := m.PasswordLogin(context.Background(), "e", "p", "t", "")
	require.Error(t, err)
	m.mu.Lock()
	m.status.State = "unavailable"
	m.retryAt = time.Now().Add(time.Hour)
	m.mu.Unlock()
	_, err = m.PasswordLogin(context.Background(), "e", "p", "t", "http://127.0.0.1:7777")
	require.ErrorContains(t, err, "runtime unavailable")
	require.Empty(t, m.loginSlots)
}
