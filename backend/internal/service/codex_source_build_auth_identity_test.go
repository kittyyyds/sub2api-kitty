//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestCodexSourceBuildAuthIdentityIsRequestAndAccountBound(t *testing.T) {
	codexSourceTestState(t)
	// 401 自动重载/重刷新链未移植（见移植说明）；这里只校验凭据面身份绑定。
	account := &Account{ID: 4242, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "access", "refresh_token": "refresh", "chatgpt_account_id": "acct"}}
	headers := codexSourceTestContext(codexSourceTestUA, "codex_cli_rs", "0.0.0").Request.Header
	base := context.Background()
	bound := withCodexSourceBuildAuthIdentity(base, account, headers)
	ua, _ := CodexAuthIdentityForContext(bound)
	require.Contains(t, ua, "/0.166.0", "private claim alone must match the actual auth account")
	for _, id := range []int64{account.ID, account.ID + 1} {
		ua, origin := CodexAuthIdentityForContext(openai.WithCodexAuthAccount(bound, id))
		if id == account.ID {
			require.Equal(t, codexSourceTestUA, ua)
			require.Equal(t, "codex_cli_rs", origin)
		} else {
			require.Contains(t, ua, "/0.166.0")
		}
	}
	ua, _ = CodexAuthIdentityForContext(openai.WithCodexAuthAccount(base, account.ID))
	require.Contains(t, ua, "/0.166.0", "ordinary refresh cannot inherit a prior request claim")
	unbound := withCodexSourceBuildAuthIdentity(bound, account, http.Header{"User-Agent": {"curl/0.0.0"}, "Originator": {"codex_cli_rs"}})
	ua, _ = CodexAuthIdentityForContext(openai.WithCodexAuthAccount(unbound, account.ID))
	require.Contains(t, ua, "/0.166.0", "invalid replacement headers clear inherited request claims")
}
