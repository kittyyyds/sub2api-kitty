package service

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func tiboRouteStatusResponse(status int) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"denied"}}`))}
}

type tiboRouteCase struct {
	svc      *OpenAIGatewayService
	account  *Account
	upstream *httpUpstreamRecorder
}

// tiboAccountRepo is a fake database row. Like the real repository it owns
// its row and hands out copies, so a background probe persisting a verdict
// never writes the Extra map a request is reading. Tests change the row with
// setAccount and read writes with updateFor.
type tiboAccountRepo struct {
	AccountRepository
	mu      sync.Mutex
	account *Account
	updates map[int64]map[string]any
	gets    atomic.Int32
}

func cloneTiboAccount(account *Account) *Account {
	copied := *account
	copied.Extra = maps.Clone(account.Extra)
	copied.Credentials = maps.Clone(account.Credentials)
	return &copied
}

func (r *tiboAccountRepo) setAccount(mutate func(*Account)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mutate(r.account)
}

func (r *tiboAccountRepo) updateFor(id int64) (map[string]any, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	update, ok := r.updates[id]
	return update, ok
}

func (r *tiboAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.gets.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account == nil || r.account.ID != id {
		return nil, errOpenAITiboAccountUnavailable
	}
	return cloneTiboAccount(r.account), nil
}

func (r *tiboAccountRepo) ListByPlatform(_ context.Context, _ string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account == nil {
		return nil, nil
	}
	return []Account{*cloneTiboAccount(r.account)}, nil
}

func (r *tiboAccountRepo) UpdateExtra(_ context.Context, id int64, extra map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updates == nil {
		r.updates = map[int64]map[string]any{}
	}
	r.updates[id] = extra
	if r.account != nil && r.account.ID == id {
		next := maps.Clone(r.account.Extra)
		if next == nil {
			next = map[string]any{}
		}
		maps.Copy(next, extra)
		r.account.Extra = next
	}
	return nil
}

func tiboProbeCompletion(model, answer string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{
		"model": model, "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": answer}}}},
	}})
	return b
}

func tiboProbeHTTPResponse(answer string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + string(tiboProbeCompletion(openAITiboProbeModel, answer)) + "\n\n"))}
}

func newTiboRouteCase(t *testing.T, responses ...*http.Response) *tiboRouteCase {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{responses: responses}
	svc := &OpenAIGatewayService{cfg: tiboEnabledTestConfig(), httpUpstream: upstream, cache: &stubGatewayCache{}, toolCorrector: NewCodexToolCorrector()}
	account := &Account{
		ID: 23141, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 20, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-chatgpt"},
		Extra:       map[string]any{"openai_passthrough": true},
	}
	svc.accountRepo = &tiboAccountRepo{account: cloneTiboAccount(account), updates: map[int64]map[string]any{}}
	return &tiboRouteCase{svc: svc, account: account, upstream: upstream}
}

func (tc *tiboRouteCase) hosts() []string {
	hosts := make([]string, 0, len(tc.upstream.requests))
	for _, req := range tc.upstream.requests {
		hosts = append(hosts, req.URL.Host)
	}
	return hosts
}

func requireTiboProbe(t *testing.T, body []byte) {
	t.Helper()
	require.Contains(t, string(body), openAITiboProbePrompt)
	require.Contains(t, string(body), `"model":"gpt-6-astra"`)
	require.Contains(t, string(body), `"effort":"xhigh"`)
}

func TestTiboRouteAppliesForOpenAIAPIKey(t *testing.T) {
	tc := newTiboRouteCase(t)
	tc.account.Type = AccountTypeAPIKey
	tc.account.Credentials = map[string]any{"api_key": "sk-test", "base_url": "http://127.0.0.1:18083"}
	require.True(t, tc.svc.openAITiboRouteApplies(tc.account))
	hold := tc.svc.openAITiboHoldFor(context.Background(), tc.account, time.Now())
	require.False(t, hold.parked)
	require.True(t, hold.until.IsZero())
	got := (&AccountTestService{openaiGatewayService: tc.svc}).OpenAITiboRouteStatuses(tc.account, time.Now())
	require.Len(t, got, 1)
	require.Equal(t, "http", got[0].Route)

	// gateway.openai_tibo_route.probe_api_key_accounts=false leaves API keys alone.
	disabled := false
	tc.svc.cfg.Gateway.OpenAITiboRoute.ProbeAPIKeyAccounts = &disabled
	require.False(t, tc.svc.openAITiboRouteApplies(tc.account))
	oauth := cloneTiboAccount(tc.account)
	oauth.Type = AccountTypeOAuth
	require.True(t, tc.svc.openAITiboRouteApplies(oauth))
}

// VM-bound / VM2-managed accounts, spark shadows and Excel OAuth never get a
// Tibo probe from this server; the global switch turns everything off.
func TestTiboRouteExcludedAccountsAndGlobalSwitch(t *testing.T) {
	tc := newTiboRouteCase(t)
	require.True(t, tc.svc.openAITiboRouteApplies(tc.account))

	vmBound := cloneTiboAccount(tc.account)
	vmBound.Extra["openai_vm_bound"] = true
	require.False(t, tc.svc.openAITiboRouteApplies(vmBound))
	require.Equal(t, openAITiboUnknown, tc.svc.probeOpenAITiboHTTP(context.Background(), vmBound).verdict)

	vmManaged := cloneTiboAccount(tc.account)
	vmManaged.Extra[OpenAIVMManagedExtraKey] = true
	require.False(t, tc.svc.openAITiboRouteApplies(vmManaged))

	shadow := cloneTiboAccount(tc.account)
	parent := int64(1)
	shadow.ParentAccountID = &parent
	require.False(t, tc.svc.openAITiboRouteApplies(shadow))

	unloadedProxy := cloneTiboAccount(tc.account)
	proxyID := int64(7)
	unloadedProxy.ProxyID = &proxyID
	require.Equal(t, openAITiboUnknown, tc.svc.probeOpenAITiboHTTP(context.Background(), unloadedProxy).verdict)
	require.Empty(t, tc.upstream.requests, "no probe may leave without its proxy or from a VM account")

	disabled := false
	tc.svc.cfg.Gateway.OpenAITiboRoute.Enabled = &disabled
	require.False(t, tc.svc.openAITiboRouteApplies(tc.account))
	require.Nil(t, (&AccountTestService{openaiGatewayService: tc.svc}).OpenAITiboRouteStatuses(tc.account, time.Now()))
	tc.svc.probeOpenAITiboRoutes(context.Background())
	tc.svc.openaiTiboProbeWG.Wait()
	require.Empty(t, tc.upstream.requests)
	tc.svc.StartOpenAITiboProbeLoop()
	require.Nil(t, tc.svc.openaiTiboLoopDone, "a disabled route never starts the loop")
}

func TestTiboProbeAPIKeyUsesResponsesBaseURL(t *testing.T) {
	tc := newTiboRouteCase(t, tiboProbeHTTPResponse("True"))
	tc.svc.cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	tc.account.Type = AccountTypeAPIKey
	tc.account.Credentials = map[string]any{"api_key": "sk-test", "base_url": "http://127.0.0.1:18083"}
	sample := tc.svc.probeOpenAITiboHTTP(context.Background(), tc.account)
	require.Equal(t, openAITiboHealthy, sample.verdict)
	require.Len(t, tc.upstream.requests, 1)
	require.Equal(t, "http://127.0.0.1:18083/v1/responses", tc.upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk-test", tc.upstream.requests[0].Header.Get("Authorization"))
	require.NotEqual(t, "chatgpt.com", tc.upstream.requests[0].Host)

	// The outbound URL policy of business requests applies to probes too.
	blocked := newTiboRouteCase(t, tiboProbeHTTPResponse("True"))
	blocked.svc.cfg.Security.URLAllowlist.Enabled = true
	blocked.account.Type = AccountTypeAPIKey
	blocked.account.Credentials = map[string]any{"api_key": "sk-test", "base_url": "https://not-allowed.example"}
	require.Equal(t, openAITiboUnknown, blocked.svc.probeOpenAITiboHTTP(context.Background(), blocked.account).verdict)
	require.Empty(t, blocked.upstream.requests)
}

// A cold account whose first probe is unknown has no confirmed verdict:
// unknown does not become a verdict.
func TestTiboRouteProbeFailureIsNotHealthy(t *testing.T) {
	tc := newTiboRouteCase(t, tiboRouteStatusResponse(http.StatusTooManyRequests))
	tc.tick(t)
	require.Len(t, tc.upstream.requests, 1)
	state := tc.svc.loadOpenAITiboAccountState(tc.account.ID)
	require.NotNil(t, state)
	require.Empty(t, state.http.verdict, "unknown samples never become a verdict")
	require.Equal(t, 1, state.http.unknownStreak)
	require.Equal(t, openAITiboUnknown, tc.svc.openAITiboEffective(tc.account.ID, openAITiboRouteHTTP, time.Now()))
}

func TestTiboRouteModelSwapIsDegraded(t *testing.T) {
	tc := newTiboRouteCase(t)
	body := "data: " + string(tiboProbeCompletion("gpt-5.6-luna", "True")) + "\n\n"
	tc.upstream.responses = []*http.Response{{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}}
	sample := tc.svc.probeOpenAITiboHTTP(context.Background(), tc.account)
	require.Equal(t, http.StatusOK, sample.status)
	require.Equal(t, openAITiboDegraded, sample.verdict)
	require.Equal(t, "", tc.upstream.lastProxyURL)
}

func TestTiboRouteProbeRetryAfterIsKept(t *testing.T) {
	tc := newTiboRouteCase(t)
	resp := tiboRouteStatusResponse(http.StatusTooManyRequests)
	resp.Header.Set("Retry-After", "600")
	tc.upstream.responses = []*http.Response{resp}
	sample := tc.svc.probeOpenAITiboHTTP(context.Background(), tc.account)
	require.Equal(t, openAITiboUnknown, sample.verdict)
	require.Equal(t, http.StatusTooManyRequests, sample.status)
	require.NotNil(t, sample.retryAt)
	require.WithinDuration(t, time.Now().Add(10*time.Minute), *sample.retryAt, 5*time.Second)
}

// The probe goes out as a Codex request (synthesis keeps the prompt, model and
// reasoning effort) and a True answer from astra is healthy.
func TestTiboProbeOAuthUsesCodexRequest(t *testing.T) {
	tc := newTiboRouteCase(t, tiboProbeHTTPResponse("True"))
	sample := tc.svc.probeOpenAITiboHTTP(context.Background(), tc.account)
	require.Equal(t, openAITiboHealthy, sample.verdict)
	require.Len(t, tc.upstream.requests, 1)
	require.Equal(t, "chatgpt.com", tc.upstream.requests[0].Host)
	require.Equal(t, "Bearer test-token", tc.upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "test-chatgpt", tc.upstream.requests[0].Header.Get("chatgpt-account-id"))
	requireTiboProbe(t, tc.upstream.bodies[0])
}

// A gpt-6-astra business response served by another model is hard evidence;
// the exact probe payload sent as a business request is not.
func TestTiboPassiveModelCheckOnForwardResult(t *testing.T) {
	tc := newTiboRouteCase(t)
	tc.svc.openaiTiboLoaded.Store(true)
	state := tc.svc.openAITiboAccountState(tc.account.ID)
	now := time.Now()
	state.mu.Lock()
	state.http.verdict, state.http.checkedAt, state.http.flippedAt = openAITiboHealthy, now, now.Add(-time.Hour)
	state.http.lastSampleAt, state.http.nextProbeAt = now, now.Add(time.Hour)
	state.mu.Unlock()

	probeBody, err := json.Marshal(openAITiboProbePayload())
	require.NoError(t, err)
	tc.svc.observeOpenAITiboForwardResult(tc.account, probeBody, &OpenAIForwardResult{UpstreamModel: "gpt-6-astra", UpstreamResponseModel: "gpt-5.6-luna"})
	require.Equal(t, openAITiboHealthy, tc.svc.openAITiboEffective(tc.account.ID, openAITiboRouteHTTP, time.Now()))

	tc.svc.observeOpenAITiboForwardResult(tc.account, []byte(`{"model":"gpt-6-astra","input":"hi"}`), &OpenAIForwardResult{UpstreamModel: "gpt-6-luna", UpstreamResponseModel: "gpt-5.6-luna"})
	require.Equal(t, openAITiboHealthy, tc.svc.openAITiboEffective(tc.account.ID, openAITiboRouteHTTP, time.Now()), "non-astra mismatch only logs")

	// Responses WebSocket turns are not sampled by the HTTP route check.
	tc.svc.observeOpenAITiboForwardResult(tc.account, []byte(`{"model":"gpt-6-astra","input":"hi"}`), &OpenAIForwardResult{OpenAIWSMode: true, UpstreamModel: "gpt-6-astra", UpstreamResponseModel: "gpt-5.6-luna"})
	require.Equal(t, openAITiboHealthy, tc.svc.openAITiboEffective(tc.account.ID, openAITiboRouteHTTP, time.Now()))

	tc.svc.observeOpenAITiboForwardResult(tc.account, []byte(`{"model":"gpt-6-astra","input":"hi"}`), &OpenAIForwardResult{UpstreamModel: "gpt-6-astra", UpstreamResponseModel: "gpt-5.6-luna"})
	state.mu.Lock()
	verdict, due := state.http.verdict, state.http.due(time.Now())
	state.mu.Unlock()
	require.Equal(t, openAITiboDegraded, verdict)
	require.True(t, due, "the confirmation probe is queued for the next loop tick")
	require.Empty(t, tc.upstream.requests, "a request never sends a probe itself")
}

// tiboSafeUpstream serves probe and business requests from separate queues
// and is safe for a background probe racing a business request.
type tiboSafeUpstream struct {
	mu         sync.Mutex
	probeGate  chan struct{}
	probes     int
	businesses int
}

func (u *tiboSafeUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	if strings.Contains(string(body), openAITiboProbePrompt) {
		if u.probeGate != nil {
			<-u.probeGate
		}
		u.mu.Lock()
		u.probes++
		u.mu.Unlock()
		return tiboProbeHTTPResponse("True"), nil
	}
	u.mu.Lock()
	u.businesses++
	u.mu.Unlock()
	return tiboProbeHTTPResponse("http ok"), nil
}

func (u *tiboSafeUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

// tick enrolls the account, removes the startup stagger of a cold account
// (it is due at once), runs one probe-loop tick and waits for its probes.
func (tc *tiboRouteCase) tick(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tc.svc.enrollOpenAITiboAccounts(ctx)
	if state := tc.svc.loadOpenAITiboAccountState(tc.account.ID); state != nil {
		state.mu.Lock()
		if state.http.cold() {
			state.http.nextProbeAt = time.Time{}
		}
		state.mu.Unlock()
	}
	tc.svc.probeOpenAITiboRoutes(ctx)
	tc.svc.openaiTiboProbeWG.Wait()
}

// tiboEnabledTestConfig turns Tibo probing on explicitly: the shipped default
// (and a config built without viper) is off in this fork.
func tiboEnabledTestConfig() *config.Config {
	cfg := &config.Config{}
	enabled := true
	cfg.Gateway.OpenAITiboRoute.Enabled = &enabled
	return cfg
}

// With the shipped default (gateway.openai_tibo_route.enabled=false) and with a
// config that never set the switch, the probe loop neither starts nor sends.
func TestTiboProbeLoopSendsNothingByDefault(t *testing.T) {
	for name, enabled := range map[string]*bool{"unset": nil, "default false": new(bool)} {
		t.Run(name, func(t *testing.T) {
			tc := newTiboRouteCase(t, tiboProbeHTTPResponse("True"))
			tc.svc.cfg.Gateway.OpenAITiboRoute.Enabled = enabled
			tc.svc.cfg.Gateway.OpenAITiboRoute.Interval = time.Nanosecond
			require.False(t, tc.svc.openAITiboRouteEnabled())
			require.False(t, tc.svc.openAITiboProbesRunHere())
			require.False(t, tc.svc.openAITiboRouteApplies(tc.account))

			tc.svc.StartOpenAITiboProbeLoop()
			require.Nil(t, tc.svc.openaiTiboLoopDone)
			for i := 0; i < 3; i++ {
				tc.svc.probeOpenAITiboRoutes(context.Background())
			}
			tc.svc.openaiTiboProbeWG.Wait()
			require.Empty(t, tc.upstream.requests)
			require.Nil(t, (&AccountTestService{openaiGatewayService: tc.svc}).OpenAITiboRouteStatuses(tc.account, time.Now()))
		})
	}
}
