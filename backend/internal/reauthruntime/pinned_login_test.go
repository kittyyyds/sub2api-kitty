package reauthruntime

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedLoginRejectsUnknownRuntime(t *testing.T) {
	_, err := pinnedTransport([]byte("unknown revision"))
	require.Error(t, err)
	_, err = pinTransportSource("unknown layout")
	require.Error(t, err)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tosub2", "src"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tosub2", "src", "protocol-login.mjs"), []byte("unknown"), 0600))
	require.Error(t, preparePinnedLoginSource(root, t.TempDir()))
}
func TestPinnedLoginGuardsEveryNetworkPath(t *testing.T) {
	source := strings.Join([]string{
		"  async configure(proxy, { force = false } = {}) {",
		"  async prepareProxy(proxyTemplate, validationUrl) {",
		"const canRotate = proxySupportsSessionRotation(template);",
		"verifyTls: shouldVerifyTlsForProxy(proxy),",
		"  async request(method, url, options = {}) {",
		"const riskControl = isRiskControlResult(result);",
	}, "\n")
	data, err := pinTransportSource(source)
	require.NoError(t, err)
	text := string(data)
	require.Equal(t, 4, strings.Count(text, "requireSelectedProxy("))
	require.Contains(t, text, "const canRotate = false;")
	require.Contains(t, text, "verifyTls: true,")
	require.Contains(t, text, "PROXY_SECURITY_CHECK_REQUIRES_INTERACTION")
	_, err = pinTransportSource(source + "\nconst canRotate = proxySupportsSessionRotation(template);")
	require.Error(t, err)
}
