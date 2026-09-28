package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type prismGatewayFixture struct {
	t                       *testing.T
	starts, calls           int
	projects, conversations []string
	lastInput               []byte
	model                   string
	usage                   string
	failStatus              int
	loseStart               bool
}

func (u *prismGatewayFixture) DoWithTLS(r *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, proxy, id, concurrency)
}
func (u *prismGatewayFixture) Do(r *http.Request, _ string, id int64, concurrency int) (*http.Response, error) {
	u.calls++
	require.Equal(u.t, "prism.openai.com", r.URL.Host)
	require.Equal(u.t, "https", r.URL.Scheme)
	require.True(u.t, HTTPUpstreamRedirectsDisabled(r.Context()))
	require.Equal(u.t, "session=fixture-prism", r.Header.Get("Cookie"))
	require.Empty(u.t, r.Header.Get("Authorization"))
	require.Empty(u.t, r.Header.Get("x-codex-turn-state"))
	require.Empty(u.t, r.Header.Get("Chatgpt-Account-Id"))
	require.Equal(u.t, 1, concurrency)
	raw := []byte{}
	if r.Body != nil {
		raw, _ = io.ReadAll(r.Body)
	}
	status, wire := 200, `{}`
	switch {
	case r.URL.Path == "/api/projects":
		u.projects = append(u.projects, gjson.GetBytes(raw, "project_uuid").String())
	case r.URL.Path == "/api/y":
		wire = `{"token":"ys-fixture","url":"wss://prism.openai.com/y/sync"}`
	case r.URL.Path == "/api/backend/1/new":
		wire = `{"token":"sandbox-fixture","url":"https://prism.openai.com/s/sandboxes/proxy"}`
	case strings.HasSuffix(r.URL.Path, "/sandbox/resources-token"):
		wire = `{"access_token":"resource-fixture"}`
	case r.URL.Path == prismUpstreamEndpoint:
		u.starts++
		u.lastInput = raw
		u.conversations = append(u.conversations, gjson.GetBytes(raw, "conversationId").String())
		if u.loseStart {
			return nil, fmt.Errorf("network lost: fixture-prism secret must not appear")
		}
		if u.failStatus > 0 {
			status = u.failStatus
			wire = `{"error":"session=fixture-prism"}`
			break
		}
		wire = fmt.Sprintf(`{"request_id":"req_fixture","status":"completed","response":{"payload":{"id":"resp_fixture","model":%q,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello fixture"}]}],"usage":%s}}}`, u.model, u.usage)
	}
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}, nil
}

func prismGatewaySetup(t *testing.T) (*OpenAIGatewayService, *Account, *prismGatewayFixture) {
	t.Helper()
	t.Cleanup(func() { prismUncertainAccounts.Delete(int64(900)) })
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	gin.SetMode(gin.TestMode)
	u := &prismGatewayFixture{t: t, model: "gpt-test", usage: `{"input_tokens":20,"input_tokens_details":{"cached_tokens":5},"output_tokens":3,"total_tokens":23}`}
	svc := openAIClientToolsTestService(nil)
	svc.httpUpstream = u
	svc.cfg.Totp.EncryptionKeyConfigured = true
	svc.cfg.Totp.EncryptionKey = strings.Repeat("ab", 32)
	ciphertext, err := encryptPrismCookie("session=fixture-prism", svc.cfg)
	require.NoError(t, err)
	a := &Account{ID: 900, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Extra: map[string]any{PrismEnabledExtraKey: true}, Credentials: map[string]any{PrismCookieEncryptedKey: ciphertext, "access_token": "must-not-be-used", "model_mapping": map[string]any{"public-test": "gpt-test"}}}
	return svc, a, u
}

func prismTestContext(path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", path, bytes.NewReader(body))
	c.Request.Header.Set("X-Codex-Turn-State", "client-ticket-must-not-leak")
	return c, w
}

func TestPrismGatewayFullBootstrapContracts(t *testing.T) {
	for _, chat := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("chat=%t/stream=%t", chat, stream), func(t *testing.T) {
				svc, a, u := prismGatewaySetup(t)
				path, input := "/v1/responses", `"input":"Hello"`
				if chat {
					path = "/v1/chat/completions"
					input = `"messages":[{"role":"user","content":"Hello"}]`
				}
				body := []byte(fmt.Sprintf(`{"model":"public-test","stream":%t,%s}`, stream, input))
				c, w := prismTestContext(path, body)
				var result *OpenAIForwardResult
				var err error
				if chat {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, a, body, "", "")
				} else {
					result, err = svc.Forward(context.Background(), c, a, body)
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 200, w.Code)
				require.Equal(t, 1, u.starts)
				require.Equal(t, 20, result.Usage.InputTokens)
				require.Equal(t, 5, result.Usage.CacheReadInputTokens)
				require.Equal(t, 3, result.Usage.OutputTokens)
				require.Equal(t, "gpt-test", result.UpstreamResponseModel)
				require.Equal(t, prismUpstreamEndpoint, result.UpstreamEndpoint)
				require.Equal(t, "gpt-test", gjson.GetBytes(u.lastInput, "metadata.model").String())
				require.Equal(t, "Hello", gjson.GetBytes(u.lastInput, "input.0.content.0.text").String())
				require.Contains(t, w.Body.String(), "Hello fixture")
				require.NotContains(t, w.Body.String(), "fixture-prism")
				require.NotContains(t, w.Body.String(), "must-not-be-used")
				if stream {
					require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
					if chat {
						require.Contains(t, w.Body.String(), "[DONE]")
					} else {
						require.Equal(t, 1, strings.Count(w.Body.String(), "event: response.completed"))
						require.Contains(t, w.Body.String(), `"sequence_number":7`)
					}
				} else {
					require.True(t, json.Valid(w.Body.Bytes()))
					require.Equal(t, "public-test", gjson.GetBytes(w.Body.Bytes(), "model").String())
				}
			})
		}
	}
}

func TestPrismGatewayFastPolicyBeforeDispatch(t *testing.T) {
	cases := []struct {
		name       string
		groupForce bool
		tier       string
		ruleTier   string
		action     string
		wantStatus int
	}{
		{name: "neutral", wantStatus: http.StatusOK},
		{name: "group_force_rejected", groupForce: true, wantStatus: http.StatusBadRequest},
		{name: "missing_force_rejected", ruleTier: OpenAIFastTierMissing, action: OpenAIFastPolicyActionForcePriority, wantStatus: http.StatusBadRequest},
		{name: "group_block_preserved", groupForce: true, ruleTier: OpenAIFastTierPriority, action: BetaPolicyActionBlock, wantStatus: http.StatusForbidden},
		{name: "explicit_block_preserved", tier: OpenAIFastTierPriority, ruleTier: OpenAIFastTierPriority, action: BetaPolicyActionBlock, wantStatus: http.StatusForbidden},
		{name: "explicit_tier_rejected", tier: OpenAIFastTierPriority, wantStatus: http.StatusBadRequest},
		{name: "explicit_filter_preserved", tier: OpenAIFastTierPriority, ruleTier: OpenAIFastTierPriority, action: BetaPolicyActionFilter, wantStatus: http.StatusOK},
		{name: "group_filter_preserved", groupForce: true, ruleTier: OpenAIFastTierPriority, action: BetaPolicyActionFilter, wantStatus: http.StatusOK},
	}
	for _, chat := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("chat=%t/%s", chat, tc.name), func(t *testing.T) {
				svc, account, upstream := prismGatewaySetup(t)
				settings := DefaultOpenAIFastPolicySettings()
				if tc.action != "" {
					settings.Rules = []OpenAIFastPolicyRule{{
						ServiceTier: tc.ruleTier, Action: tc.action, Scope: BetaPolicyScopeAll,
						ModelWhitelist: []string{"gpt-test"}, FallbackAction: BetaPolicyActionPass,
						ErrorMessage: "Administrator blocked this Fast tier",
					}}
				}
				svc.settingService = newOpenAIGatewayServiceWithSettings(t, settings).settingService
				ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
					ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: tc.groupForce,
				})
				path, input := "/v1/responses", `"input":"Hello"`
				if chat {
					path, input = "/v1/chat/completions", `"messages":[{"role":"user","content":"Hello"}]`
				}
				tier := ""
				if tc.tier != "" {
					tier = fmt.Sprintf(`,"service_tier":%q`, tc.tier)
				}
				body := []byte(fmt.Sprintf(`{"model":"public-test",%s%s}`, input, tier))
				c, recorder := prismTestContext(path, body)
				c.Request = c.Request.WithContext(ctx)
				var result *OpenAIForwardResult
				var err error
				if chat {
					result, err = svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
				} else {
					result, err = svc.Forward(ctx, c, account, body)
				}
				require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
				if tc.wantStatus == http.StatusOK {
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, 1, upstream.starts)
					require.Equal(t, "gpt-test", gjson.GetBytes(upstream.lastInput, "metadata.model").String())
					require.False(t, gjson.GetBytes(upstream.lastInput, "service_tier").Exists())
					require.False(t, gjson.GetBytes(upstream.lastInput, "metadata.service_tier").Exists())
					require.Empty(t, result.ServiceTier)
					return
				}
				require.Error(t, err)
				require.Nil(t, result, "blocked or unsupported tiers must not create a billable result")
				require.Zero(t, upstream.calls, "policy must be enforced before creating a Prism workspace")
				require.True(t, IsResponseCommitted(c))
				var failover *UpstreamFailoverError
				require.NotErrorAs(t, err, &failover)
				if tc.wantStatus == http.StatusForbidden {
					var blocked *OpenAIFastBlockedError
					require.ErrorAs(t, err, &blocked)
					require.Equal(t, "permission_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
					require.Equal(t, "Administrator blocked this Fast tier", gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
				} else {
					require.Equal(t, "prism_service_tier_unsupported", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
				}
			})
		}
	}
}

func TestPrismGatewayUncertainAndUnbillableResultsNeverReplay(t *testing.T) {
	for _, mode := range []string{"missing_usage", "wrong_model", "no_model", "lost_start", "unauthorized", "rate_limited", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			svc, a, u := prismGatewaySetup(t)
			switch mode {
			case "missing_usage":
				u.usage = "null"
			case "wrong_model":
				u.model = "other"
			case "no_model":
				u.model = ""
			case "lost_start":
				u.loseStart = true
			case "unauthorized":
				u.failStatus = 401
			case "rate_limited":
				u.failStatus = 429
			case "redirect":
				u.failStatus = 302
			}
			body := []byte(`{"model":"public-test","input":"Hi"}`)
			c, w := prismTestContext("/v1/responses", body)
			result, err := svc.Forward(context.Background(), c, a, body)
			require.Error(t, err)
			require.Nil(t, result, "nil result prevents billing submission")
			var failover *UpstreamFailoverError
			require.NotErrorAs(t, err, &failover)
			require.Equal(t, 1, u.starts)
			require.True(t, IsResponseCommitted(c))
			require.True(t, json.Valid(w.Body.Bytes()))
			require.NotContains(t, w.Body.String(), "fixture-prism")
			require.NotContains(t, err.Error(), "fixture-prism")
		})
	}
}

func TestPrismGatewaySeparateCustomersGetSeparateWorkspaces(t *testing.T) {
	svc, a, u := prismGatewaySetup(t)
	for range 2 {
		body := []byte(`{"model":"public-test","input":"Same input","prompt_cache_key":"same-caller-id"}`)
		c, _ := prismTestContext("/v1/responses", body)
		_, err := svc.Forward(context.Background(), c, a, body)
		require.NoError(t, err)
	}
	require.Len(t, u.projects, 2)
	require.NotEqual(t, u.projects[0], u.projects[1])
	require.NotEqual(t, u.conversations[0], u.conversations[1])
}

func TestPrismUnknownRemoteTurnBlocksNextClientInstance(t *testing.T) {
	svc, a, u := prismGatewaySetup(t)
	u.loseStart = true
	body := []byte(`{"model":"public-test","input":"First"}`)
	c, _ := prismTestContext("/v1/responses", body)
	_, err := svc.Forward(context.Background(), c, a, body)
	require.Error(t, err)
	calls := u.calls
	u.loseStart = false
	c, w := prismTestContext("/v1/responses", body)
	result, err := svc.Forward(context.Background(), c, a, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, calls, u.calls)
	require.Equal(t, 409, w.Code)
	require.Contains(t, w.Body.String(), "prism_outcome_unknown")
}

func TestPrismGatewayDisabledAndUnsupportedNeverContactUpstream(t *testing.T) {
	for _, mode := range []string{"disabled", "tools", "image", "compact", "previous_response", "messages", "embeddings", "missing_cookie", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			svc, a, u := prismGatewaySetup(t)
			body := []byte(`{"model":"public-test","input":"Hi"}`)
			path := "/v1/responses"
			switch mode {
			case "disabled":
				t.Setenv("SUB2API_PRISM_ENABLED", "false")
			case "tools":
				body = []byte(`{"model":"public-test","input":"Hi","tools":[{"type":"function","name":"run"}]}`)
			case "image":
				body = []byte(`{"model":"public-test","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.test/secret"}]}]}`)
			case "compact":
				path = "/v1/responses/compact"
			case "previous_response":
				body = []byte(`{"model":"public-test","input":"Hi","previous_response_id":"resp_other_customer"}`)
			case "missing_cookie":
				delete(a.Credentials, PrismCookieEncryptedKey)
			case "conflict":
				a.Extra["openai_excel_bps"] = true
			}
			c, w := prismTestContext(path, body)
			var result *OpenAIForwardResult
			var err error
			switch mode {
			case "messages":
				result, err = svc.ForwardAsAnthropic(context.Background(), c, a, body, "", "")
			case "embeddings":
				result, err = svc.ForwardEmbeddings(context.Background(), c, a, body, "")
			default:
				result, err = svc.Forward(context.Background(), c, a, body)
			}
			require.Error(t, err)
			require.Nil(t, result)
			require.Zero(t, u.calls)
			require.GreaterOrEqual(t, w.Code, 400)
		})
	}
}

func TestPrismTextRequestPreservesHistoryAndInstructions(t *testing.T) {
	r, _, err := parsePrismTextRequest([]byte(`{"model":"m","instructions":"Rules","input":[{"role":"user","content":"First"},{"role":"assistant","content":"Answer"},{"role":"user","content":"Second"}],"reasoning":{"effort":"high"}}`), false)
	require.NoError(t, err)
	require.Len(t, r.Input, 4)
	require.Equal(t, "developer", r.Input[0].Role)
	require.Equal(t, "Answer", r.Input[2].Content[0].Text)
	require.Equal(t, "output_text", r.Input[2].Content[0].Type)
	require.Equal(t, "high", r.ReasoningEffort)
	for _, body := range []string{`null`, `[]`, `{"model":"m","input":"hi","temperature":0.9}`, `{"model":"m","input":"hi","reasoning":{"mode":"adaptive"}}`, `{"model":"m","input":[{"role":"tool","content":"hidden"}]}`, `{"model":"m","input":"hi","max_output_tokens":1}`} {
		_, _, err := parsePrismTextRequest([]byte(body), false)
		require.Error(t, err)
	}
}

func TestPrismAccountLeaseIsCancelableAndReleased(t *testing.T) {
	release, err := acquirePrismTurn(context.Background(), 54321)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = acquirePrismTurn(ctx, 54321)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	release()
	_, present := prismTurnSlots.Load(int64(54321))
	require.False(t, present)
	release, err = acquirePrismTurn(context.Background(), 54321)
	require.NoError(t, err)
	release()
}

func TestPrismRejectsNullMessageWithoutReusingHistory(t *testing.T) {
	for _, chat := range []bool{false, true} {
		key := "input"
		if chat {
			key = "messages"
		}
		body := []byte(fmt.Sprintf(`{"model":"m",%q:[{"role":"user","content":"First"},{"role":"assistant","content":null},{"role":"user","content":"Second"}]}`, key))
		_, _, err := parsePrismTextRequest(body, chat)
		require.Error(t, err)
	}
}

func TestPrismRoutingIsSeparateFromCodexTicketsAndWebsocket(t *testing.T) {
	svc, a, _ := prismGatewaySetup(t)
	a.Extra[PrismModelsExtraKey] = []string{"gpt-test"}
	require.False(t, isOpenAICodexTicketAccount(a, "gpt-test"))
	require.True(t, isOpenAICodexTicketAccount(a, "gpt-other"))
	require.Equal(t, "gpt-test", svc.openAICodexTicketOutboundModel(a, "public-test", false))
	require.False(t, svc.isOpenAIAccountTransportCompatible(a, OpenAIUpstreamTransportResponsesWebsocketV2Ingress, "public-test"))
	require.Error(t, svc.checkOpenAIWSBinding(a, "gpt-test", nil))
	before := openAITurnRouteFingerprint(a)
	a.Extra[PrismModelsExtraKey] = []string{"other"}
	require.NotEqual(t, before, openAITurnRouteFingerprint(a))
}

func TestPrismVerifiedResponseUsesSUBBillingWithoutCountingCacheTwice(t *testing.T) {
	svc, account, _ := prismGatewaySetup(t)
	body := []byte(`{"model":"public-test","input":"Hi"}`)
	c, _ := prismTestContext("/v1/responses", body)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{}
	billing := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	billing.billingService = NewBillingService(billing.cfg, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-test": {InputCostPerToken: 5e-6, OutputCostPerToken: 30e-6, CacheReadInputTokenCost: 0.5e-6},
	}})
	err = billing.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: result, APIKey: &APIKey{ID: 1001}, User: &User{ID: 2001}, Account: account})
	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 15, billingRepo.lastCmd.InputTokens)
	require.Equal(t, 5, billingRepo.lastCmd.CacheReadTokens)
	require.Equal(t, 3, billingRepo.lastCmd.OutputTokens)
	require.Equal(t, 23, usageRepo.lastLog.TotalTokens())
	require.InDelta(t, 15*5e-6+5*0.5e-6+3*30e-6, usageRepo.lastLog.TotalCost, 1e-12)
	require.Equal(t, 20, result.Usage.InputTokens, "raw response usage remains intact")
}
