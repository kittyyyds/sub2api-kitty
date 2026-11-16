package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// VM-routed accounts reach upstream through the vm2api bridge, whose runtime
// owns the final Codex identity. The ported identity/synthesis layers must
// leave them on the pre-port behavior.
func TestCodexIdentityLayersExcludeVMRoutedAccounts(t *testing.T) {
	for _, extra := range []map[string]any{
		{"openai_vm_bound": true},
		{"openai_vm_bound": true, OpenAIVMManagedExtraKey: true},
	} {
		account := &Account{ID: 9101, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Credentials: map[string]any{"chatgpt_account_id": "vm-account"}, Extra: extra}
		require.False(t, codexIdentityLaneAccount(account))
		require.False(t, isCodexOAuthAccount(account), "no synthesis, device profile or session identity")
		_, ok := resolveCodexDeviceProfile(account)
		require.False(t, ok)
		require.Equal(t, codexFingerprintOff, account.GetCodexFingerprintMode(), "missing mode stays off for VM")
		require.False(t, codexFingerprintConvergenceEnabled(account))
		require.Empty(t, codexAccountIdentityOverrideUA(account))

		// The account namespace mapping is the legacy one byte for byte.
		want := deriveStableUUIDv4(fmt.Sprintf(
			"sub2api:codex-account-identity:%s:user:%d:account:%s:kind:%s:value:%s",
			codexAccountIdentityNamespaceVersion, 7, codexAccountIdentityNamespace(account), "session", "client-session"))
		require.Equal(t, want, scopeCodexAccountIdentityValue(account, 7, "session", "client-session"))

		// An explicit administrator mode is still honored.
		account.Extra[codexFingerprintModeExtraKey] = "device"
		require.Equal(t, codexFingerprintDevice, account.GetCodexFingerprintMode())
	}

	direct := &Account{ID: 9102, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, isCodexOAuthAccount(direct))
	require.Equal(t, codexFingerprintFull, direct.GetCodexFingerprintMode(), "direct OAuth defaults to full")
	grok := &Account{ID: 9103, Platform: PlatformGrok, Type: AccountTypeOAuth}
	require.True(t, grok.UsesOpenAICodexProtocol(), "grok keeps the legacy protocol semantics")
	require.False(t, codexIdentityLaneAccount(grok))
}

func TestCodexDeviceUserAgentKeepsCLIPolicy(t *testing.T) {
	pinCodexTestVersion(t, "0.166.0")
	tui, ok := buildCodexDeviceUserAgent(codexClientFamilyTUI, codexDefaultPlatformProfile, "0.166.0")
	require.True(t, ok)
	require.Equal(t, "codex_cli_rs/0.166.0 (Ubuntu 26.4.0; x86_64) xterm-256color", tui,
		"the default family keeps this fork's codex_cli_rs identity without a surface trailer")
	exec, ok := buildCodexDeviceUserAgent(codexClientFamilyExec, codexDefaultPlatformProfile, "0.166.0")
	require.True(t, ok)
	require.Equal(t, "codex_exec/0.166.0 (Ubuntu 26.4.0; x86_64) xterm-256color (codex_exec; 0.166.0)", exec)
	identity := resolveCodexOutboundIdentity(exec)
	require.Equal(t, "codex_exec", identity.originator)
	require.Equal(t, exec, identity.userAgent)
}
