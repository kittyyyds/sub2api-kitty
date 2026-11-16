package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const codexSourceTestBootstrapUA = "codex_cli_rs/0.0.0 (Ubuntu 26.4.0; x86_64) xterm-256color"
const codexSourceTestUA = codexSourceTestBootstrapUA + " (codex_cli_rs; 0.0.0)"

func codexSourceTestState(t *testing.T) {
	t.Helper()
	codexCanonicalUAMu.RLock()
	previousResolver := codexCanonicalUAResolver
	codexCanonicalUAMu.RUnlock()
	previousEnforcement := codexIdentityEnforcement.Load()
	codexSessionIdentities.Lock()
	previousEntries := codexSessionIdentities.entries
	codexSessionIdentities.entries = make(map[string]codexSessionIdentitySnapshot)
	codexSessionIdentities.Unlock()
	SetCodexCanonicalUserAgentResolver(func() string { return buildCodexCLIUserAgent("0.166.0") })
	SetCodexIdentityEnforcementEnabled(true)
	t.Cleanup(func() {
		SetCodexCanonicalUserAgentResolver(previousResolver)
		SetCodexIdentityEnforcementEnabled(previousEnforcement)
		codexSessionIdentities.Lock()
		codexSessionIdentities.entries = previousEntries
		codexSessionIdentities.Unlock()
	})
}

func codexSourceTestContext(ua, originator, version string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", ua)
	c.Request.Header.Set("Originator", originator)
	if version != "" {
		c.Request.Header.Set("Version", version)
	}
	c.Request.Header.Set("Session-Id", "source-build-session")
	c.Request.Header.Set("Thread-Id", "source-build-thread")
	c.Set("api_key", &APIKey{ID: 6789, UserID: 6788})
	return c
}

func TestCodexSourceBuildIdentityRequiresCompleteOfficialShape(t *testing.T) {
	for _, tc := range []struct {
		name, ua, origin string
		want             bool
	}{
		{"bootstrap", codexSourceTestBootstrapUA, "codex_cli_rs", true},
		{"initialized", codexSourceTestUA, "codex_cli_rs", true},
		{"official_tui_trailer", codexSourceTestBootstrapUA + " (codex-tui; 0.0.0)", "codex_cli_rs", true},
		{"missing_platform", "codex_cli_rs/0.0.0", "codex_cli_rs", false},
		{"origin_only", "curl/0.0.0", "codex_cli_rs", false},
		{"version_only", "other/0.0.0 (Ubuntu 26.4.0; x86_64) xterm", "other", false},
		{"embedded_codex", "Mozilla " + codexSourceTestUA, "codex_cli_rs", false},
		{"missing_origin", codexSourceTestUA, "", false},
		{"mismatched_origin", codexSourceTestUA, "codex_exec", false},
		{"unofficial_trailer", codexSourceTestBootstrapUA + " (other; 0.0.0)", "codex_cli_rs", false},
		{"mismatched_trailer_version", codexSourceTestBootstrapUA + " (codex_cli_rs; 0.166.0)", "codex_cli_rs", false},
		{"old_stable", strings.ReplaceAll(codexSourceTestUA, "0.0.0", "0.137.0"), "codex_cli_rs", false},
		{"other_zero_version", strings.ReplaceAll(codexSourceTestUA, "0.0.0", "0.0.1"), "codex_cli_rs", false},
		{"injection", codexSourceTestUA + "\r\nX: 1", "codex_cli_rs", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, ok := codexSourceBuildIdentity(tc.ua, tc.origin)
			require.Equal(t, tc.want, ok)
			if ok {
				require.Equal(t, tc.ua, identity.userAgent)
				require.Equal(t, tc.origin, identity.originator)
				require.Equal(t, "0.0.0", identity.version)
			}
		})
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	headers := codexSourceTestContext(codexSourceTestUA, "codex_cli_rs", "0.0.0").Request.Header
	_, ok := codexSourceBuildIdentityFromHeaders(headers, account)
	require.True(t, ok)
	for _, other := range []*Account{nil, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, {Platform: PlatformGrok, Type: AccountTypeOAuth}} {
		_, ok = codexSourceBuildIdentityFromHeaders(headers, other)
		require.False(t, ok)
	}
	headers.Add("Version", "0.0.0")
	_, ok = codexSourceBuildIdentityFromHeaders(headers, account)
	require.False(t, ok, "duplicate version cannot establish a single identity")
	headers.Set("Version", "0.166.0")
	_, ok = codexSourceBuildIdentityFromHeaders(headers, account)
	require.False(t, ok, "a conflicting version header is not the observed source identity")
}

func TestCodexSourceBuildSessionDoesNotChangeCanonicalOrStablePolicy(t *testing.T) {
	codexSourceTestState(t)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 678, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "synthetic-source-account"}}
	ordinary := codexSourceTestContext("codex_cli_rs/0.137.0 (Ubuntu 26.4.0; x86_64) xterm-256color", "codex_cli_rs", "0.137.0")
	require.Equal(t, "0.166.0", svc.codexSessionClientVersion(ordinary, account, nil))
	source := codexSourceTestContext(codexSourceTestUA, "codex_cli_rs", "0.0.0")
	require.Equal(t, "0.0.0", svc.codexSessionClientVersion(source, account, nil))
	profile, _ := svc.codexWireProfileForRequest(source, account, nil)
	wantProfile, _ := resolveCodexWireProfile("0.166.0", codexWireProfileModeAuto)
	require.Equal(t, wantProfile.Name, profile.Name, "source version must not silently downgrade the configured wire profile")
	require.Equal(t, "0.166.0", svc.codexSessionClientVersion(codexSourceTestContext("curl/0.0.0", "codex_cli_rs", "0.0.0"), account, nil))
	require.Equal(t, "0.166.0", svc.codexSessionClientVersion(codexSourceTestContext("codex_cli_rs/0.137.0 (Ubuntu 26.4.0; x86_64) xterm-256color", "codex_cli_rs", "0.137.0"), account, nil))
	require.Equal(t, "0.166.0", CodexCanonicalClientVersion())
	require.Equal(t, codexCLIVersion, codexClientVersionFromUA(codexSourceTestUA), "global minimum still treats unbound zero version as invalid")
	SetCodexCanonicalUserAgentResolver(func() string { return codexSourceTestUA })
	require.Equal(t, codexCLIVersion, CodexCanonicalClientVersion(), "no global source-build exception")
}
