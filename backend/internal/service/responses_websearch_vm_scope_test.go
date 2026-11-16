//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func enableHostedResponsesSearchForTest(t *testing.T) {
	t.Helper()
	previousManager, previousConfig := getWebSearchManager(), webSearchEmulationCache.Load()
	t.Cleanup(func() {
		SetWebSearchManager(previousManager)
		if previousConfig != nil {
			webSearchEmulationCache.Store(previousConfig)
		} else {
			webSearchEmulationCache.Store((*cachedWebSearchEmulationConfig)(nil))
		}
	})
	SetWebSearchManager(websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "fixture"}}, nil))
	webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{config: &WebSearchEmulationConfig{Enabled: true, Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "fixture"}}}, expiresAt: time.Now().Add(time.Minute).UnixNano()})
}

// Hosted Responses web_search never reaches VM-bound / VM2-managed accounts:
// even with the global switch and the account override on, the VM bridge
// receives the client's body byte for byte and no search round is started.
func TestHostedResponsesSearchLeavesVMRoutedForwardUntouched(t *testing.T) {
	enableHostedResponsesSearchForTest(t)
	gin.SetMode(gin.TestMode)
	// Owned search history would be rewritten on an eligible account; it must
	// reach the VM bridge unchanged as well.
	body := `{"model":"gpt-5.4","stream":false,"tools":[{"type":"web_search"}],"input":[{"type":"web_search_call","id":"` + responsesWebSearchIDPrefix + `vm","status":"completed"},{"type":"message","role":"user","content":"hi"}]}`
	for _, extra := range []map[string]any{
		{"openai_vm_bound": true, featureKeyWebSearchEmulation: WebSearchModeEnabled},
		{"openai_vm_bound": true, OpenAIVMManagedExtraKey: true, featureKeyWebSearchEmulation: WebSearchModeEnabled},
	} {
		var received []byte
		vm, repo, _ := vmTestService(t, func(w http.ResponseWriter, r *http.Request) {
			received, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"resp_vm","object":"response","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":3,"output_tokens":1}}`)
		})
		repo.account.Extra = extra
		repo.b = &OpenAIVMBinding{AccountID: 7, Status: "ready", Mode: extraManagedMode(extra)}
		require.False(t, openAIResponsesWebSearchAccountEligible(repo.account))
		direct := &httpUpstreamRecorder{}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, openaiVM: vm, accountRepo: vm.accounts, cache: &stubGatewayCache{}, httpUpstream: direct, settingService: newSettingServiceForWebSearchTest(true)}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		c.Set("api_key", &APIKey{ID: 41, UserID: 7})
		result, err := svc.Forward(context.Background(), c, repo.account, []byte(body))
		require.NoError(t, err)
		require.Equal(t, body, string(received), "VM bridge body must be the client body byte for byte")
		require.Equal(t, "resp_vm", result.ResponseID)
		require.False(t, result.HasHostedSearchRounds())
		require.Empty(t, direct.requests)
		var public map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &public))
		require.NotContains(t, recorder.Body.String(), responsesWebSearchFunctionPrefix)
	}
}

// OpenAI OAuth / setup-token accounts keep native upstream search (their
// bodies are synthesized into the Codex shape); API-key accounts follow the
// source fork's switches.
func TestHostedResponsesSearchAccountScope(t *testing.T) {
	for _, tc := range []struct {
		account *Account
		want    bool
	}{
		{&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, true},
		{&Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, true},
		{&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, false},
		{&Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, false},
		{&Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}, false},
		{&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_vm_bound": true}}, false},
		{nil, false},
	} {
		require.Equal(t, tc.want, openAIResponsesWebSearchAccountEligible(tc.account))
	}

	// An ineligible account is forwarded as is, owned history included.
	body := []byte(`{"model":"m","input":[{"type":"web_search_call","id":"` + responsesWebSearchIDPrefix + `x"}]}`)
	c, _ := hostedSearchTestContext(t, string(body), 77)
	svc := &OpenAIGatewayService{}
	_, err := svc.forwardOpenAIResponsesWithWebSearch(context.Background(), c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, func(next []byte) (*OpenAIForwardResult, error) {
		require.Equal(t, string(body), string(next))
		return &OpenAIForwardResult{}, nil
	})
	require.NoError(t, err)
}

func extraManagedMode(extra map[string]any) string {
	if managed, _ := extra[OpenAIVMManagedExtraKey].(bool); managed {
		return OpenAIVMModeManaged
	}
	return ""
}
