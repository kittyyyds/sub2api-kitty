//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

func codexTestJWTExpiry(exp int64) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp))) + ".signature"
}

func kittyCacheOwnerJWT(workspace, user string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"https://api.openai.com/auth":{"chatgpt_account_id":%q,"chatgpt_user_id":%q}}`, workspace, user))) + ".signature"
}

func TestKittyOAuthOpaqueExpiryAndJWTPrecedence(t *testing.T) {
	now := time.Now()
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "opaque", "expires_at": now.Add(time.Minute).Format(time.RFC3339)}}
	require.Equal(t, "proactive_expiry", codexAutomaticRefreshReason(a, now))
	a.Credentials["access_token"] = codexTestJWTExpiry(now.Add(time.Hour).Unix())
	require.Empty(t, codexAutomaticRefreshReason(a, now), "JWT exp is authoritative over stale display expiry")
}

func TestKittyOAuthOpaqueCacheCannotReplaceDurableCredential(t *testing.T) {
	a := &Account{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "durable"}}
	cache := newOpenAITokenCacheStub()
	cache.tokens[OpenAITokenCacheKey(a)] = "unknown-cache-owner"
	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, "durable", token)
}
func TestKittyModelAliasExactAndUnknown(t *testing.T) {
	for input, want := range map[string]string{
		"gpt-5.6sol": "gpt-5.6-sol", "gpt-5.6terra": "gpt-5.6-terra", "gpt-5.6luna": "gpt-5.6-luna",
		"gpt-6.1-sol-high": "gpt-6.1-sol", "gpt-6.1-terra-minimal": "gpt-6.1-terra", "openai/gpt-6-astra": "gpt-6-astra",
		"gpt-5.6-sol-openai-compact": "gpt-5.6-sol", "gpt-5.6-my-private": "", "gpt-5.99-future": "", "codex-private-team": "", "not-gpt-5-custom": "",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex-spark", "gpt-5.5-codex-high": "gpt-5.5",
	} {
		t.Run(input, func(t *testing.T) { require.Equal(t, want, normalizeKnownOpenAICodexModel(input)) })
	}
	for _, input := range []string{"gpt-5.6-my-private", "gpt-5.99-future", "codex-private-team", "not-gpt-5-custom"} {
		require.Equal(t, input, normalizeCodexModel(input))
	}
}
func TestKittyOAuthSnapshotIsIndependent(t *testing.T) {
	account := &Account{Credentials: map[string]any{"nested": map[string]any{"list": []any{map[string]any{"mode": "old"}}}}}
	snapshot := snapshotOAuthRefreshAccount(account)
	account.Credentials["nested"].(map[string]any)["list"].([]any)[0].(map[string]any)["mode"] = "new"
	require.Equal(t, "old", snapshot.Credentials["nested"].(map[string]any)["list"].([]any)[0].(map[string]any)["mode"])
}

func (r *rateLimitAccountRepoStub) SetOpenAIOAuthRefreshErrorIfUnchanged(ctx context.Context, id int64, expected map[string]any, proxy *int64, reason string) (bool, error) {
	return true, r.SetError(ctx, id, reason)
}
func (r *tokenRefreshAccountRepo) SetOpenAIOAuthRefreshErrorIfUnchanged(ctx context.Context, id int64, expected map[string]any, proxy *int64, reason string) (bool, error) {
	account := r.accountsByID[id]
	if account == nil {
		account = r.lastAccount
	}
	if account == nil || !reflect.DeepEqual(account.Credentials, expected) || !reflect.DeepEqual(account.ProxyID, proxy) {
		return false, nil
	}
	return true, r.SetError(ctx, id, reason)
}
func (r *openAIRecoveryRepo) SetOpenAIOAuthRefreshErrorIfUnchanged(ctx context.Context, id int64, expected map[string]any, proxy *int64, reason string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !openAIRefreshTestStateMatches(r.account, id, expected, proxy) {
		return false, nil
	}
	r.setError++
	return true, nil
}
func TestKittyOAuthFailureCannotQuarantineNewAuthorization(t *testing.T) {
	account, repo, _, provider := newOpenAIRecoveryFixture()
	repo.account.Credentials["access_token"] = "new-authorization"
	provider.disableAccountMissingRefreshToken(context.Background(), account, "old expired attempt")
	require.Zero(t, repo.setError)
}

func TestKittyOAuthWSCompatibilitySeparatesUserNotTokenRotation(t *testing.T) {
	account, _, _, _ := newOpenAIRecoveryFixture()
	original := normalizeOpenAIWSHandshakeCompatibility(account, make(http.Header))
	account.Credentials["access_token"] = "rotated"
	account.Credentials["refresh_token"] = "rotated"
	require.Equal(t, original, normalizeOpenAIWSHandshakeCompatibility(account, make(http.Header)))
	account.Credentials["chatgpt_user_id"] = "different-user"
	require.NotEqual(t, original, normalizeOpenAIWSHandshakeCompatibility(account, make(http.Header)))
}
func TestKittyOAuthShadowResponseOriginFollowsParentOwner(t *testing.T) {
	parent, repo, _, _ := newOpenAIRecoveryFixture()
	shadow := &Account{ID: parent.ID + 1, ParentAccountID: &parent.ID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc := &OpenAIGatewayService{accountRepo: repo}
	first := responseAuthOwnerContext(101, 201)
	_, err := svc.prepareCodexAccountIdentitySource(context.Background(), first, shadow)
	require.NoError(t, err)
	svc.bindOpenAIResponseAuthOwner(context.Background(), first, shadow, "resp_shadow")
	repo.account.Credentials["chatgpt_user_id"] = "another-parent-user"
	next := responseAuthOwnerContext(101, 201)
	_, err = svc.prepareCodexAccountIdentitySource(context.Background(), next, shadow)
	require.NoError(t, err)
	require.ErrorIs(t, svc.validateOpenAIResponseAuthOwner(context.Background(), next, shadow, "resp_shadow"), errOpenAIResponseAuthOwnerMismatch)
}

func TestKittyOAuthShadowHTTP401CannotReplayWithReauthorizedParent(t *testing.T) {
	parent, repo, executor, provider := newOpenAIRecoveryFixture()
	parent.Credentials["access_token"] = kittyCacheOwnerJWT("same-account", "old-user")
	parent.Credentials["chatgpt_user_id"] = "old-user"
	shadow := &Account{ID: parent.ID + 1, ParentAccountID: &parent.ID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	repo.account.Credentials["access_token"] = kittyCacheOwnerJWT("same-account", "new-user")
	repo.account.Credentials["chatgpt_user_id"] = "new-user"
	calls := 0
	upstream := &openAIRecoveryUpstream{do: func(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 401, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`))}, nil
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, httpUpstream: upstream, openAITokenProvider: provider}
	req, err := http.NewRequest(http.MethodPost, chatgptCodexURL, strings.NewReader(`{}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+parent.GetOpenAIAccessToken())
	response, err := svc.doOpenAIUpstream(req, "", shadow)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 401, response.StatusCode)
	require.Equal(t, 1, calls)
	require.Zero(t, executor.calls.Load())
}

func TestKittyOAuthNamespaceUsesCurrentTokenUser(t *testing.T) {
	a, _, _, _ := newOpenAIRecoveryFixture()
	a.Credentials["id_token"] = kittyCacheOwnerJWT("same-account", "new-user")
	a.Credentials["chatgpt_user_id"] = "stale-display-user"
	require.Equal(t, "chatgpt:same-account:user:new-user", codexAccountIdentityNamespace(a))
}

func TestKittyOAuthProxyChangeInvalidatesOldRejection(t *testing.T) {
	a, repo, executor, provider := newOpenAIRecoveryFixture()
	provider.refreshAPI.recordOpenAIRefreshFailure(a, &openai.RefreshTokenError{Permanent: true})
	newProxy := int64(99)
	repo.account.ProxyID = &newProxy
	require.Nil(t, provider.refreshAPI.openAIRefreshFailure(repo.account), "old-route rejection is not evidence against the new proxy")
	svc := &OpenAIGatewayService{openAITokenProvider: provider}
	headers := http.Header{"Authorization": {"Bearer rejected-access"}}
	_, _, recovered := svc.reloadOpenAIOAuthHeaders(context.Background(), a, headers)
	require.False(t, recovered, "an old-route request cannot replay under newly configured route credentials")
	provider.RecordUnauthorizedAfterRefresh(context.Background(), a, a.GetOpenAIAccessToken())
	require.Nil(t, provider.refreshAPI.openAIRefreshFailure(repo.account))
	_, err := provider.RefreshAfterUnauthorized(context.Background(), a, a.GetOpenAIAccessToken())
	require.Error(t, err)
	require.Zero(t, executor.calls.Load())
}
