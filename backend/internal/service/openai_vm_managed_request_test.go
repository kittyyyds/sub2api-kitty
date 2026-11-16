package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

// managedRequestFixture is a logged-in managed account behind a fake bridge.
func managedRequestFixture(t *testing.T, h http.HandlerFunc) (*OpenAIGatewayService, *OpenAIVMService, *managedTestRepo, *managedTestAccounts, *httpUpstreamRecorder) {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	proxy := int64(900)
	account := &Account{ID: 7, Name: "managed", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, ProxyID: &proxy,
		Credentials: map[string]any{"chatgpt_account_id": "org-7", "email": "demo@example.test"},
		Extra:       map[string]any{OpenAIVMManagedExtraKey: true, "openai_vm_bound": true}}
	accounts := &managedTestAccounts{a: account}
	base := &vmTestRepo{account: account, c: OpenAIVMConfig{Enabled: true, BaseURL: server.URL, ProxyURL: server.URL, EncryptedSecret: "encrypted:" + vmTestSecret, SecretConfigured: true}}
	base.b = &OpenAIVMBinding{AccountID: 7, Mode: OpenAIVMModeManaged, Status: "ready", CreatedAt: time.Now()}
	repo := &managedTestRepo{vmTestRepo: base, accounts: accounts}
	vm := NewOpenAIVMService(repo, accounts, &managedTestProxies{}, vmTestEncryptor{})
	direct := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, openaiVM: vm, accountRepo: accounts, cache: &stubGatewayCache{}, httpUpstream: direct}
	svc.rateLimitService = &RateLimitService{accountRepo: accounts, cfg: &config.Config{}}
	return svc, vm, repo, accounts, direct
}

func managedAccount(accounts *managedTestAccounts) *Account {
	a, _ := accounts.GetByID(context.Background(), 7)
	return a
}

func TestManagedAccountNeverYieldsATokenAndIsNeverDisabledForLackingOne(t *testing.T) {
	svc, _, _, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
	account := managedAccount(accounts)
	token, kind, err := svc.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Empty(t, token)
	require.Equal(t, OpenAIAuthModeVMManaged, kind)

	// The token provider would otherwise treat "no refresh token" as a dead
	// account and disable it permanently.
	provider := NewOpenAITokenProvider(accounts, nil, nil)
	_, err = provider.GetAccessToken(context.Background(), account)
	require.ErrorIs(t, err, errOpenAIVMManagedNoToken)
	require.Empty(t, accounts.errors)
	require.Equal(t, StatusActive, accounts.snapshot().Status)

	refresher := NewOpenAITokenRefresher(nil, accounts)
	require.False(t, refresher.CanRefresh(account), "the background refresh must skip a managed account")
	plain := &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, refresher.CanRefresh(plain))

	_, err = NewOpenAIOAuthService(nil, nil).RefreshAccountToken(context.Background(), account)
	require.ErrorIs(t, err, errOpenAIVMManagedNoToken)
	require.NoError(t, validateOpenAIWSBearerToken(account, ""))
	require.Error(t, validateOpenAIWSBearerToken(plain, ""))
	require.False(t, openAIVMEligible(account), "the token-fed slot flow must not adopt a managed account")
}

func TestManagedForwardSendsNoCredentialThroughTheBridge(t *testing.T) {
	body := `{"model":"gpt-5.4","input":"hi","stream":false}`
	var seen http.Header
	svc, _, _, accounts, direct := managedRequestFixture(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/accounts/7/responses", r.URL.Path)
		seen = r.Header.Clone()
		got, _ := io.ReadAll(r.Body)
		require.JSONEq(t, body, string(got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_managed","object":"response","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":5,"output_tokens":2}}`)
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Authorization", "Bearer client-key")
	c.Request.Header.Set("X-VM2API-Access-Token", "spoofed-by-client")
	c.Request.Header.Set("X-VM2API-ChatGPT-Account-ID", "spoofed-by-client")
	c.Request.Header.Set("Chatgpt-Account-Id", "spoofed-by-client")
	result, err := svc.Forward(context.Background(), c, managedAccount(accounts), []byte(body))
	require.NoError(t, err)
	require.Equal(t, "resp_managed", result.ResponseID)
	require.EqualValues(t, 5, result.Usage.InputTokens, "usage still comes back for billing")
	// Only the bridge secret authenticates; no account credential or identity travels.
	require.Equal(t, "Bearer "+vmTestSecret, seen.Get("Authorization"))
	require.Empty(t, seen.Get("X-VM2API-Access-Token"))
	require.Empty(t, seen.Get("X-VM2API-ChatGPT-Account-ID"))
	require.Empty(t, seen.Get("Chatgpt-Account-Id"))
	require.Empty(t, direct.requests, "nothing may go to OpenAI directly")
}

func TestManagedWebSocketRouteCarriesNoCredential(t *testing.T) {
	_, vm, repo, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected HTTP request") })
	route, handled, err := vm.websocketRoute(context.Background(), managedAccount(accounts), "", http.Header{"X-VM2API-Access-Token": {"spoofed"}, "Chatgpt-Account-Id": {"spoofed"}, "X-Codex-Turn-State": {"kept"}})
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "Bearer "+vmTestSecret, route.headers.Get("Authorization"))
	require.Empty(t, route.headers.Get("X-VM2API-Access-Token"))
	require.Empty(t, route.headers.Get("X-VM2API-ChatGPT-Account-ID"))
	require.Empty(t, route.headers.Get("Chatgpt-Account-Id"))
	require.Equal(t, "kept", route.headers.Get("X-Codex-Turn-State"))
	// Binding and account must agree on who owns the credential, whatever token is offered.
	unmarked := managedAccount(accounts)
	delete(unmarked.Extra, OpenAIVMManagedExtraKey)
	_, handled, err = vm.websocketRoute(context.Background(), unmarked, "some-token", nil)
	require.True(t, handled)
	require.ErrorContains(t, err, "disagree")
	repo.b.Mode = OpenAIVMModeSubToken
	_, handled, err = vm.websocketRoute(context.Background(), managedAccount(accounts), "some-token", nil)
	require.True(t, handled)
	require.ErrorContains(t, err, "disagree")
	// A token-fed binding still requires its token.
	plain := managedAccount(accounts)
	delete(plain.Extra, OpenAIVMManagedExtraKey)
	_, handled, err = vm.websocketRoute(context.Background(), plain, "", nil)
	require.True(t, handled)
	require.ErrorContains(t, err, "bearer access token")
}

func TestManagedAccountFailsClosedWithoutAUsableBinding(t *testing.T) {
	_, vm, repo, accounts, direct := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
	account := managedAccount(accounts)
	request := func() *http.Request {
		req, _ := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{}`))
		return req
	}
	for name, binding := range map[string]*OpenAIVMBinding{
		"no binding":     nil,
		"awaiting login": {AccountID: 7, Mode: OpenAIVMModeManaged, Status: "awaiting_login"},
		"failed create":  {AccountID: 7, Mode: OpenAIVMModeManaged, Status: "error"},
		"being deleted":  {AccountID: 7, Mode: OpenAIVMModeManaged, Status: "deleting"},
		"wrong mode":     {AccountID: 7, Mode: OpenAIVMModeSubToken, Status: "ready"},
	} {
		repo.b = binding
		resp, handled, err := vm.RoundTrip(request(), account)
		require.Truef(t, handled, "%s: fell through to the direct path", name)
		require.Errorf(t, err, name)
		require.Nilf(t, resp, name)
		bound, err := vm.RouteBound(context.Background(), account)
		require.NoError(t, err)
		require.Truef(t, bound, "%s: the gateway would take the direct path", name)
		_, handled, err = vm.websocketRoute(context.Background(), account, "", nil)
		require.Truef(t, handled, "%s: direct WebSocket fallback", name)
		require.Errorf(t, err, name)
	}
	require.Empty(t, direct.requests)
	// The mark alone is enough: after its slot is released the account carries
	// no bound flag any more, and still must not reach OpenAI directly.
	repo.b = nil
	released := managedAccount(accounts)
	delete(released.Extra, "openai_vm_bound")
	_, handled, err := vm.RoundTrip(request(), released)
	require.True(t, handled, "a released managed account fell through to the direct path")
	require.Error(t, err)
	bound, err := vm.RouteBound(context.Background(), released)
	require.NoError(t, err)
	require.True(t, bound)
	_, handled, err = vm.websocketRoute(context.Background(), released, "", nil)
	require.True(t, handled)
	require.Error(t, err)
	// And an account whose mark was lost is refused too.
	repo.b = &OpenAIVMBinding{AccountID: 7, Mode: OpenAIVMModeManaged, Status: "ready"}
	unmarked := managedAccount(accounts)
	delete(unmarked.Extra, OpenAIVMManagedExtraKey)
	_, handled, err = vm.RoundTrip(request(), unmarked)
	require.True(t, handled)
	require.Error(t, err)
}

// Whatever a caller put on the request, nothing credential-like reaches the
// bridge for a managed account: the bridge would refuse it, and a refused
// request is a failed request.
func TestManagedRoundTripStripsCredentialHeaders(t *testing.T) {
	var seen http.Header
	_, vm, _, accounts, _ := managedRequestFixture(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		_, _ = io.WriteString(w, `{}`)
	})
	req, _ := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer leftover-token")
	req.Header.Set("X-VM2API-Access-Token", "leftover-token")
	req.Header.Set("X-VM2API-ChatGPT-Account-ID", "org-other")
	req.Header.Set("Chatgpt-Account-Id", "org-other")
	req.Header.Set("Cookie", "session=1")
	resp, handled, err := vm.RoundTrip(req, managedAccount(accounts))
	require.NoError(t, err)
	require.True(t, handled)
	_ = resp.Body.Close()
	require.Equal(t, "Bearer "+vmTestSecret, seen.Get("Authorization"))
	for _, name := range []string{"X-VM2API-Access-Token", "X-VM2API-ChatGPT-Account-ID", "Chatgpt-Account-Id", "Cookie"} {
		require.Emptyf(t, seen.Get(name), "%s reached the bridge", name)
	}
}

func TestManagedAuxiliaryRequestsNeverLeaveWithAnEmptyToken(t *testing.T) {
	var paths []string
	_, vm, _, accounts, _ := managedRequestFixture(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		require.Equal(t, "Bearer "+vmTestSecret, r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-5.5"}]}`)
	})
	account := managedAccount(accounts)
	// The model manifest is answered by the slot through the bridge.
	models, _ := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/codex/models?client_version=1", nil)
	models.Header.Set("Authorization", "Bearer ")
	resp, handled, err := vm.RoundTrip(models, account)
	require.NoError(t, err)
	require.True(t, handled)
	_ = resp.Body.Close()
	require.Equal(t, []string{"GET /v1/accounts/7/models"}, paths)
	// Anything else that would need the account's own token is refused locally.
	for _, target := range []string{"https://chatgpt.com/backend-api/wham/usage", "https://chatgpt.com/backend-api/codex/responses/input_tokens", "https://chatgpt.com/backend-api/settings/account_user_setting"} {
		req, _ := http.NewRequest(http.MethodPost, target, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer ")
		resp, handled, err = vm.RoundTrip(req, account)
		require.Truef(t, handled, target)
		require.ErrorIsf(t, err, errOpenAIVMManagedUnsupported, target)
		require.Nil(t, resp)
	}
	require.Len(t, paths, 1, "a refused request must not reach the bridge")
}

func TestManagedUpstreamErrorsPauseButNeverDisable(t *testing.T) {
	svc, _, _, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) {})
	account := managedAccount(accounts)
	// Bodies that permanently disable an ordinary OAuth account.
	for _, tc := range []struct {
		status int
		body   string
	}{
		{401, `{"error":{"code":"token_invalidated","message":"token revoked"}}`},
		{401, `{"detail":"Unauthorized"}`},
		{401, `{"error":{"message":"expired"}}`},
		{402, `{"detail":{"code":"deactivated_workspace"}}`},
		{403, `{"error":{"message":"forbidden"}}`},
		{400, `{"error":{"message":"organization has been disabled"}}`},
		{409, `{"error":{"code":"account_not_ready"}}`},
		{503, `{"error":{"code":"pool_unavailable"}}`},
	} {
		svc.handleOpenAIAccountUpstreamError(context.Background(), account, tc.status, http.Header{}, []byte(tc.body))
	}
	state := accounts.snapshot()
	require.Empty(t, accounts.errors, "a managed account's login is the runtime's to judge")
	require.Equal(t, StatusActive, state.Status)
	require.True(t, state.Schedulable)
	require.NotEmpty(t, accounts.pauses)
	for _, reason := range accounts.pauses {
		require.True(t, strings.HasPrefix(reason, openAIVMManagedPausePrefix), reason)
	}
	// A 403 is also how a refused prompt arrives; it must not bench the account.
	before := len(accounts.pauses)
	svc.handleOpenAIAccountUpstreamError(context.Background(), account, 403, http.Header{}, []byte(`{"error":{"message":"policy"}}`))
	require.Len(t, accounts.pauses, before)
	// Which answers let another account take the request.
	for status, want := range map[int]bool{401: true, 409: true, 503: true, 403: false, 400: false, 404: false, 500: false} {
		require.Equalf(t, want, shouldFailoverOpenAIVMManagedResponse(account, status), "status %d", status)
	}
	require.False(t, shouldFailoverOpenAIVMManagedResponse(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, 401))
	// An ordinary account is still handled the old way.
	plainRepo := &managedTestAccounts{a: &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"refresh_token": "rt"}}}
	plain, _ := plainRepo.GetByID(context.Background(), 9)
	(&RateLimitService{accountRepo: plainRepo, cfg: &config.Config{}}).HandleUpstreamError(context.Background(), plain, 401, http.Header{}, []byte(`{"detail":"Unauthorized"}`))
	require.NotEmpty(t, plainRepo.errors, "the guard must not weaken handling of token-holding accounts")
}

// What each answer from a managed slot does to the account, one status at a time.
func TestManagedUpstreamErrorPauseTable(t *testing.T) {
	for _, tc := range []struct {
		status   int
		pause    string
		failover bool
	}{
		{401, "auth", true},
		{409, "slot_unavailable", true},
		{503, "slot_unavailable", true},
		{529, "slot_unavailable", true},
		{429, "rate_limited", false},
		{400, "", false},
		{402, "", false},
		{403, "", false},
		{404, "", false},
		{500, "", false},
	} {
		svc, _, _, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) {})
		got := svc.rateLimitService.HandleUpstreamError(context.Background(), managedAccount(accounts), tc.status, http.Header{}, []byte(`{}`))
		require.Equalf(t, tc.failover, got, "status %d", tc.status)
		if tc.pause == "" {
			require.Emptyf(t, accounts.pauses, "status %d must not pause the account", tc.status)
		} else {
			require.Equalf(t, []string{openAIVMManagedPausePrefix + tc.pause}, accounts.pauses, "status %d", tc.status)
		}
		require.Emptyf(t, accounts.errors, "status %d", tc.status)
	}
	// Retry-After from the runtime bounds the pause, within limits.
	for header, want := range map[string]time.Duration{"": time.Minute, "20": 20 * time.Second, "86400": 10 * time.Minute, "soon": time.Minute, "-5": time.Minute} {
		require.Equalf(t, want, openAIVMManagedRetryAfter(http.Header{"Retry-After": {header}}, time.Minute), "Retry-After %q", header)
	}
}

func TestManagedAdminModelSyncRequestCarriesNoToken(t *testing.T) {
	_, _, _, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
	tests := &AccountTestService{accountRepo: accounts}
	req, err := tests.buildOpenAIOAuthUpstreamModelsRequest(context.Background(), managedAccount(accounts))
	require.NoError(t, err, "a managed account has no token and must still be able to list its models")
	require.Empty(t, req.Header.Get("Authorization"))
	plain := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}}
	_, err = tests.buildOpenAIOAuthUpstreamModelsRequest(context.Background(), plain)
	require.Error(t, err, "a token-holding account without a token is still an error")
}

func TestManagedAccountIsNotProbedOrCountedWithAToken(t *testing.T) {
	_, _, _, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
	account := managedAccount(accounts)
	updates, err := (&AccountUsageService{}).probeOpenAICodexSnapshot(context.Background(), account)
	require.NoError(t, err)
	require.Nil(t, updates)
	require.True(t, shouldEstimateOpenAIInputTokensLocally(account))
	require.False(t, shouldEstimateOpenAIInputTokensLocally(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	_, _, _, _, err = (&OpenAIQuotaService{accountRepo: accounts, privacyClientFactory: func(string) (*req.Client, error) { return nil, nil }}).prepareUpstreamCall(context.Background(), 7)
	require.ErrorIs(t, err, errOpenAIVMManagedNoToken)
}

// The ordinary account editor must not be able to drop the managed mark, add
// it to another account, or hand a managed account a token or a proxy.
func TestManagedMarkSurvivesOrdinaryAccountEdits(t *testing.T) {
	proxy := int64(900)
	managed := &Account{ID: 1, Name: "managed", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, ProxyID: &proxy,
		Credentials: map[string]any{"email": "demo@example.test"},
		Extra:       map[string]any{OpenAIVMManagedExtraKey: true, "openai_vm_bound": true}}
	plain := &Account{ID: 2, Name: "plain", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "at", "refresh_token": "rt"}, Extra: map[string]any{}}
	repo := &upstreamBillingProbeAdminRepo{&upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: managed, 2: plain}}}
	svc := &adminServiceImpl{accountRepo: repo}

	// A form that submits an extra object without the mark does not remove it.
	updated, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Name: "renamed", Extra: map[string]any{"note": "x"}})
	require.NoError(t, err)
	require.True(t, updated.IsOpenAIVMManaged())
	require.Equal(t, "renamed", updated.Name)
	// And a form cannot switch it off explicitly.
	updated, err = svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{OpenAIVMManagedExtraKey: false}})
	require.NoError(t, err)
	require.True(t, updated.IsOpenAIVMManaged())
	// Nor can a form mark an account that holds its own token.
	updated, err = svc.UpdateAccount(context.Background(), 2, &UpdateAccountInput{Extra: map[string]any{OpenAIVMManagedExtraKey: true}})
	require.NoError(t, err)
	require.False(t, updated.IsOpenAIVMManaged())

	other := int64(42)
	for name, in := range map[string]*UpdateAccountInput{
		"token": {Credentials: map[string]any{"access_token": "pasted"}},
		"proxy": {ProxyID: &other},
		"type":  {Type: AccountTypeAPIKey},
	} {
		_, err = svc.UpdateAccount(context.Background(), 1, in)
		require.Errorf(t, err, name)
	}
	stored, _ := repo.GetByID(context.Background(), 1)
	require.False(t, openAIVMManagedHasToken(stored.Credentials))
	require.Equal(t, proxy, *stored.ProxyID)

	_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1, 2}, Extra: map[string]any{OpenAIVMManagedExtraKey: true}})
	require.Equal(t, "OPENAI_VM_MANAGED_MARK", infraerrors.Reason(err))
	_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Credentials: map[string]any{"refresh_token": "rt"}})
	require.Equal(t, "OPENAI_VM_MANAGED_TOKEN", infraerrors.Reason(err))
	require.Empty(t, repo.bulkUpdates)

	_, err = svc.CreateAccount(context.Background(), &CreateAccountInput{Name: "x", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAIVMManagedExtraKey: true}, ProxyID: &proxy})
	require.Error(t, err, "only the managed service creates managed accounts")
	// Created by the managed service, the account starts closed: it has no login yet.
	created, err := svc.CreateAccount(withOpenAIVMManagedCreate(context.Background()), &CreateAccountInput{Name: "new managed", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{}, Extra: map[string]any{OpenAIVMManagedExtraKey: true}, ProxyID: &proxy, SkipDefaultGroupBind: true})
	require.NoError(t, err)
	require.True(t, created.IsOpenAIVMManaged())
	require.False(t, created.Schedulable, "a managed account must not be schedulable before its first login")
	ordinary, err := svc.CreateAccount(context.Background(), &CreateAccountInput{Name: "ordinary", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "k"}, SkipDefaultGroupBind: true})
	require.NoError(t, err)
	require.True(t, ordinary.Schedulable)
	_, err = svc.DuplicateAccount(context.Background(), 1, "admin", "op-1")
	require.Equal(t, "OPENAI_VM_MANAGED_NO_DUPLICATE", infraerrors.Reason(err))
	_, err = svc.CreateShadow(context.Background(), 1, ShadowOptions{Name: "shadow"})
	require.Equal(t, "SPARK_SHADOW_MANAGED_PARENT_UNSUPPORTED", infraerrors.Reason(err), "a shadow would borrow a token that is not here")
}
