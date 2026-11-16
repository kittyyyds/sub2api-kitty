//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Tests in this file cover the points where the ported OpenAI
// account-protection scheduling deliberately differs from the source fork
// because of this fork's VM bridge, runtime block fastpath and switches.

// portProbeVMRepo answers only the binding lookup a probe guard performs.
type portProbeVMRepo struct {
	OpenAIVMRepository
	bindings map[int64]*OpenAIVMBinding
	err      error
}

func (r *portProbeVMRepo) GetBinding(_ context.Context, id int64) (*OpenAIVMBinding, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.bindings[id], nil
}

func portProbeOAuthAccount(id int64) *Account {
	return &Account{
		ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "token", "chatgpt_account_id": "chatgpt"},
		Extra:       map[string]any{},
	}
}

// The binding table is authoritative: an account whose row does not carry the
// VM flag yet (older snapshot) is still refused, and a lookup failure fails
// closed. Nothing is sent and no token is read.
func TestCodexProbeRefusesAccountsBoundInVMTable(t *testing.T) {
	for name, repo := range map[string]*portProbeVMRepo{
		"binding without flag": {bindings: map[int64]*OpenAIVMBinding{91: {AccountID: 91, Status: "ready"}}},
		"managed binding":      {bindings: map[int64]*OpenAIVMBinding{91: {AccountID: 91, Status: "ready", Mode: OpenAIVMModeManaged}}},
		"lookup failure":       {err: errors.New("db unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			svc.openaiVM = NewOpenAIVMService(repo, nil, nil, nil)
			account := portProbeOAuthAccount(91)

			req, err := http.NewRequest(http.MethodPost, chatgptCodexURL, nil)
			require.NoError(t, err)
			resp, err := svc.doOpenAICodexProbe(req, "", account)
			require.ErrorIs(t, err, errOpenAICodexProbeVMAccount)
			require.Nil(t, resp)

			require.Equal(t, openAITiboUnknown, svc.probeOpenAITiboHTTP(context.Background(), account).verdict)
			require.Empty(t, upstream.requests)
		})
	}
}

// A probe never falls back to this server's own egress, and a proxy-pool
// account (whose proxy is chosen by the business slot reservation) is skipped.
func TestCodexProbeRequiresTheAccountProxyRoute(t *testing.T) {
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	proxyID := int64(7)
	unloaded := portProbeOAuthAccount(92)
	unloaded.ProxyID = &proxyID
	req, err := http.NewRequest(http.MethodPost, chatgptCodexURL, nil)
	require.NoError(t, err)
	_, err = svc.doOpenAICodexProbe(req, "", unloaded)
	require.ErrorIs(t, err, errOpenAICodexProbeNoRoute)

	pool := portProbeOAuthAccount(93)
	pool.ProxyID = &proxyID
	pool.ProxyIDs = []int64{7, 8}
	req, err = http.NewRequest(http.MethodPost, chatgptCodexURL, nil)
	require.NoError(t, err)
	_, err = svc.doOpenAICodexProbe(req, "socks5://127.0.0.1:1", pool)
	require.ErrorIs(t, err, errOpenAICodexProbeNoRoute)
	require.False(t, svc.openAITiboRouteApplies(pool))
	reset := time.Now().Add(time.Hour)
	pool.RateLimitResetAt = &reset
	pool.Extra = openai429RecoveryEligibleExtra(time.Now())
	require.False(t, openAIAccountEligibleFor429RecoveryProbe(pool, time.Now()))
	require.Empty(t, upstream.requests)
}

// gateway.openai_429_recovery_probe_disabled keeps the full cooldown.
func TestOpenAI429RecoveryProbeGlobalSwitch(t *testing.T) {
	resetOpenAI429TestState(t)
	repo := &openai429RecoveryRepo{}
	account := openai429RecoveryEligibleAccount(94, time.Now())
	repo.store(account)
	cfg := &config.Config{}
	cfg.Gateway.OpenAI429RecoveryProbeDisabled = true
	svc := NewRateLimitService(repo, nil, cfg, nil, nil)
	scheduled := 0
	svc.openai429RecoveryAfter = func(time.Duration, func()) *time.Timer {
		scheduled++
		return dummyOpenAI429RecoveryTimer()
	}
	svc.ConsiderOpenAI429Recovery(account)
	svc.markOpenAIFormalCooldown(account, time.Now().Add(time.Hour))
	require.Zero(t, scheduled)
	require.Empty(t, svc.openai429RecoveryStates)
}

// gateway.openai_429_soft_threshold=1 restores "cool down on the first 429".
func TestOpenAI429SoftThresholdConfig(t *testing.T) {
	resetOpenAI429TestState(t)
	account := &Account{ID: 95, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	cfg := &config.Config{}
	cfg.Gateway.OpenAI429SoftThreshold = 1
	immediate := NewRateLimitService(&rateLimitAccountRepoStub{}, nil, cfg, nil, nil)
	require.True(t, immediate.shouldCooldownOpenAI429(context.Background(), account, http.Header{}, nil))

	resetOpenAI429TestState(t)
	defaults := NewRateLimitService(&rateLimitAccountRepoStub{}, nil, &config.Config{}, nil, nil)
	for i := 1; i < quota429CooldownThreshold; i++ {
		require.False(t, defaults.shouldCooldownOpenAI429(context.Background(), account, http.Header{}, nil), "429 #%d stays soft", i)
	}
	require.True(t, defaults.shouldCooldownOpenAI429(context.Background(), account, http.Header{}, nil))
}

// Billing 429s and Ollama Cloud usage accounts are not "soft": they keep the
// account-billing / usage-window handling they had before the port.
func TestOpenAISoft429KeepsBillingAndOllamaHandling(t *testing.T) {
	resetOpenAI429TestState(t)
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 96, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	billing := []byte(`{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`)
	require.True(t, IsUpstreamBillingError(http.StatusTooManyRequests, billing))
	require.True(t, svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, billing))
	require.Equal(t, 1, repo.setErrorCalls, "a billing 429 still stops the account at once")
	require.Zero(t, openAI429Count(account.ID))

	ollama := ollama429Account(97, PlatformOpenAI)
	ollamaRepo := newOllama429Repo(ollama)
	scheduler := newOllama429SchedulerStub(true)
	ollamaSvc, _ := ollama429Fixture(t, ollamaRepo, scheduler)
	require.False(t, ollamaSvc.HandleUpstreamError(context.Background(), ollama, http.StatusTooManyRequests, http.Header{}, nil))
	require.Equal(t, 1, scheduler.count(), "the first Ollama usage 429 still cools down and probes")
	require.NotEmpty(t, ollamaRepo.ifLaterWrites)
	require.Zero(t, openAI429Count(ollama.ID))
}

// An edge / anti-bot challenge is not account evidence. It extends this fork's
// HTML-403 guard: no penalty and no "disable" verdict (which would add a
// runtime block per failover hop here); 403 still fails over in the gateway.
func TestOpenAI403EdgeChallengeSkipsAccountPenalty(t *testing.T) {
	for name, body := range map[string]string{
		"chatgpt holding page":     "<html>\n <head>\n <meta http-equiv=\"refresh\" content=\"360\"></head>\n <body></body>\n</html>",
		"cloudflare just a moment": `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body>Checking your browser</body></html>`,
		"head-first challenge":     `<head><title>Just a moment...</title></head>`,
		"bare text challenge":      `Attention Required! Enable JavaScript and cookies to continue`,
	} {
		t.Run(name, func(t *testing.T) {
			h := newOpenAI403TestHarness(t, 98, 1, 2, 3)
			for i := 0; i < openAI403DisableThreshold+1; i++ {
				require.False(t, h.handle(body))
			}
			h.requireNoAccountPenalty(t)
		})
	}

	// A structured account-level 403 that merely mentions cloudflare is still
	// an account 403.
	h := newOpenAI403TestHarness(t, 99, 1)
	require.True(t, h.handle(`{"error":{"message":"workspace forbidden by cloudflare policy"}}`))
	require.Equal(t, 1, h.repo.tempCalls)

	// The gateway fails a 403 over to another account regardless of the verdict.
	require.True(t, (&OpenAIGatewayService{}).shouldFailoverUpstreamError(http.StatusForbidden))
}

// gateway.openai_credits_guard_disabled turns the guard off for every account;
// VM2-managed accounts and spark shadows are never guarded here.
func TestOpenAICreditsGuardScopeAndGlobalSwitch(t *testing.T) {
	t.Cleanup(func() { SetOpenAICreditsGuardEnabled(true) })
	account := newCreditsGuardTestAccount(901, nil)
	require.True(t, account.IsOpenAICreditsGuardActive())

	managed := newCreditsGuardTestAccount(902, map[string]any{OpenAIVMManagedExtraKey: true})
	require.False(t, managed.IsOpenAICreditsGuardActive(), "the slot runtime owns a managed account's limits")
	parent := int64(901)
	shadow := newCreditsGuardTestAccount(903, nil)
	shadow.ParentAccountID = &parent
	require.False(t, shadow.IsOpenAICreditsGuardActive())
	vmBound := newCreditsGuardTestAccount(904, map[string]any{"openai_vm_bound": true})
	require.True(t, vmBound.IsOpenAICreditsGuardActive(), "a VM-bound account's windows come from response headers as usual")

	SetOpenAICreditsGuardEnabled(false)
	require.False(t, account.IsOpenAICreditsGuardActive())
	exhausted := newCreditsGuardTestAccount(905, map[string]any{
		"codex_7d_used_percent":  100.0,
		"codex_7d_reset_at":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"codex_usage_updated_at": time.Now().UTC().Format(time.RFC3339),
		"auto_pause_7d_disabled": true,
	})
	paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), &exhausted)
	require.False(t, paused)
	SetOpenAICreditsGuardEnabled(true)
	paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), &exhausted)
	require.True(t, paused)
	require.Equal(t, "credits_guard_exhausted_7d", decision.reason)
}

// Tibo probing runs only on the node that owns background jobs: a gateway-only
// replica neither starts the loop nor ticks nor records passive evidence.
func TestTiboProbesOnlyRunOnBackgroundNodes(t *testing.T) {
	tc := newTiboRouteCase(t, tiboProbeHTTPResponse("True"))
	tc.svc.cfg.Runtime.Role = config.RuntimeRoleGateway
	require.True(t, tc.svc.openAITiboRouteEnabled())
	require.False(t, tc.svc.openAITiboProbesRunHere())

	tc.tick(t)
	require.Empty(t, tc.upstream.requests)
	tc.svc.StartOpenAITiboProbeLoop()
	require.Nil(t, tc.svc.openaiTiboLoopDone)
	tc.svc.observeOpenAITiboForwardResult(tc.account, []byte(`{"model":"gpt-6-astra","input":"hi"}`), &OpenAIForwardResult{UpstreamModel: "gpt-6-astra", UpstreamResponseModel: "gpt-5.6-luna"})
	require.Nil(t, tc.svc.loadOpenAITiboAccountState(tc.account.ID))

	// A full node starts the loop once, and it can be stopped and started again
	// (managed handoff). The warm-up keeps the first tick away from this test.
	tc.svc.cfg.Runtime.Role = config.RuntimeRoleFull
	tc.svc.StartOpenAITiboProbeLoop()
	tc.svc.StartOpenAITiboProbeLoop()
	require.NotNil(t, tc.svc.openaiTiboLoopDone)
	tc.svc.StopOpenAITiboProbeLoop()
	require.Nil(t, tc.svc.openaiTiboLoopDone)
	tc.svc.StartOpenAITiboProbeLoop()
	require.NotNil(t, tc.svc.openaiTiboLoopDone)
	tc.svc.StopOpenAITiboProbeLoop()
	require.Empty(t, tc.upstream.requests)
}

// Near-limit preference keeps this fork's Excel BPS precedence and yields to
// a quality hold; it re-reads the winner before taking the slot.
func TestNearLimitRespectsForkGates(t *testing.T) {
	resetOpenAI429TestState(t)
	ctx := context.Background()
	groupID := int64(4300)
	near := Account{
		ID: 43001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Concurrency: 2, Priority: 9, GroupIDs: []int64{groupID}, Extra: nearLimitExtra(98, time.Minute),
	}
	normal := Account{
		ID: 43002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Concurrency: 2, Priority: 0, GroupIDs: []int64{groupID},
		Extra: map[string]any{"codex_5h_used_percent": 10.0, "codex_usage_updated_at": time.Now().Format(time.RFC3339)},
	}
	newService := func(accounts ...Account) *OpenAIGatewayService {
		return &OpenAIGatewayService{
			accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
			cfg:                &config.Config{},
			concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		}
	}

	selection := newService(near, normal).trySelectOpenAINearLimitTarget(ctx, &groupID, PlatformOpenAI, "", "gpt-5.1", nil, false, "")
	require.NotNil(t, selection)
	require.Equal(t, near.ID, selection.Account.ID)
	require.False(t, canClaimOpenAINearLimitPreference(near.ID, time.Now(), false), "the probe lease is held with the slot")
	selection.ReleaseFunc()
	require.True(t, canClaimOpenAINearLimitPreference(near.ID, time.Now(), false))

	// Excluded accounts and an admin-set quality pause are never near-limit targets.
	require.Nil(t, newService(near, normal).trySelectOpenAINearLimitTarget(ctx, &groupID, PlatformOpenAI, "", "gpt-5.1", map[int64]struct{}{near.ID: {}}, false, ""))
	held := near
	held.Extra = nearLimitExtra(98, time.Minute)
	held.Extra["active_test_all_models_paused"] = true
	require.Nil(t, newService(held, normal).trySelectOpenAINearLimitTarget(ctx, &groupID, PlatformOpenAI, "", "gpt-5.1", nil, false, ""))

	// The global switch turns the layer off.
	disabled := newService(near, normal)
	disabled.cfg.Gateway.OpenAINearLimitDisabled = true
	require.Nil(t, disabled.trySelectOpenAINearLimitTarget(ctx, &groupID, PlatformOpenAI, "", "gpt-5.1", nil, false, ""))
}

// Excel BPS was removed: a stale openai_excel_bps flag no longer changes the
// route, so such an ordinary Codex account is probed like any other.
func TestProbesIgnoreRetiredExcelBPSFlag(t *testing.T) {
	tc := newTiboRouteCase(t)
	stale := cloneTiboAccount(tc.account)
	stale.Extra["openai_excel_bps"] = true
	require.True(t, tc.svc.openAITiboRouteApplies(stale))

	limited := openai429RecoveryEligibleAccount(97001, time.Now())
	limited.Extra["openai_excel_bps"] = true
	require.True(t, openAIAccountEligibleFor429RecoveryProbe(limited, time.Now()))

	// Retired Excel OAuth credentials are never probed.
	excel := cloneTiboAccount(tc.account)
	excel.Credentials = map[string]any{"client_id": "app_fnr0pYvVwwFDocDumLG3H2Bp", "access_token": "excel"}
	require.False(t, tc.svc.openAITiboRouteApplies(excel))
}

// An account whose model whitelist excludes gpt-6-astra never receives astra
// business traffic, so it is not probed either.
func TestTiboSkipsAccountsThatDoNotServeAstra(t *testing.T) {
	tc := newTiboRouteCase(t)
	apiKey := cloneTiboAccount(tc.account)
	apiKey.Type = AccountTypeAPIKey
	apiKey.Extra = map[string]any{}
	apiKey.Credentials = map[string]any{"api_key": "sk-test", "model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}
	require.False(t, apiKey.IsModelSupported(openAITiboProbeModel))
	require.False(t, tc.svc.openAITiboRouteApplies(apiKey))

	apiKey.Credentials["model_mapping"] = map[string]any{"gpt-6-astra": "gpt-6-astra"}
	require.True(t, tc.svc.openAITiboRouteApplies(apiKey))
	require.True(t, tc.svc.openAITiboRouteApplies(tc.account), "an OAuth account without a whitelist serves astra")
}
