package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func kittyChatIdentityContext(user, key int64) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("api_key", &APIKey{ID: key, UserID: user})
	return c
}

func TestKittyChatIdentityBridgeExplicitEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, body, session, thread string
		headers                     http.Header
		valid                       bool
	}{
		{name: "body", body: `{"client_metadata":{"session_id":"s","thread_id":"t"}}`, session: "s", thread: "t", valid: true},
		{name: "header", body: `{}`, headers: http.Header{"Session-Id": []string{"s"}, "Thread-Id": []string{"t"}}, session: "s", thread: "t", valid: true},
		{name: "body_precedes_header", body: `{"client_metadata":{"session_id":"s","thread_id":"t"}}`, headers: http.Header{"Session-Id": []string{"other"}, "Thread-Id": []string{"other"}}, session: "s", thread: "t", valid: true},
		{name: "thread_only", body: `{"client_metadata":{"thread_id":"t"}}`, session: "t", thread: "t", valid: true},
		{name: "parent_only", body: `{"client_metadata":{"parent_thread_id":"parent"}}`},
		{name: "parent_without_child", body: `{"client_metadata":{"session_id":"s","parent_thread_id":"parent"}}`},
		{name: "parent_with_child_turn", body: `{"client_metadata":{"session_id":"s","parent_thread_id":"parent","turn_id":"child"}}`, session: "s", thread: "child", valid: true},
		{name: "turn_only", body: `{"client_metadata":{"turn_id":"turn"}}`},
		{name: "cache_hint_only", body: `{"prompt_cache_key":"cache"}`},
		{name: "duplicate_body_key", body: `{"client_metadata":{"session_id":"s","session_id":"other"}}`},
		{name: "duplicate_header", body: `{}`, headers: http.Header{"Session-Id": []string{"s", "other"}}},
		{name: "control_character", body: `{"client_metadata":{"session_id":"s\n"}}`},
		{name: "typed_identity", body: `{"client_metadata":{"session_id":42}}`},
		{name: "ambiguous_metadata", body: `{"client_metadata":{"session_id":"s","turn_id":false}}`},
		{name: "invalid_utf8", body: string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := kittyChatIdentityContext(101, 202)
			if tc.headers != nil {
				c.Request.Header = tc.headers
			}
			session, thread, valid := nativeReasoningTargetChatIdentity(c, []byte(tc.body))
			require.Equal(t, tc.valid, valid)
			if valid {
				require.Equal(t, tc.session, session)
				require.Equal(t, tc.thread, thread)
			}
		})
	}
}

func TestKittyChatIdentityBridgeStagingRequiresServerAuthorization(t *testing.T) {
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "synthetic-owner"}}
	body := []byte(`{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"hello"}],"client_metadata":{"session_id":"body-session","thread_id":"body-thread","turn_id":"body-turn","unrelated":"must-not-cross"}}`)
	for _, enabled := range []bool{false, true} {
		c := kittyChatIdentityContext(101, 202)
		c.Request.Header.Set("X-Unrelated", "preserve")
		original := c.Request.Header.Clone()
		restore := stageGPT61SolChatClientSession(c, account, body, "gpt-6.1-sol", enabled)
		if enabled {
			require.Equal(t, "body-session", c.GetHeader("session-id"))
			require.Equal(t, "body-thread", c.GetHeader("thread-id"))
			require.Equal(t, "body-turn", c.GetHeader("turn-id"))
			require.NotContains(t, c.GetHeader(openAIWSTurnMetadataHeader), "must-not-cross")
		} else {
			require.Empty(t, c.GetHeader("thread-id"))
		}
		require.Equal(t, "preserve", c.GetHeader("X-Unrelated"))
		restore()
		require.Equal(t, original, c.Request.Header)
	}
	engine, _, _, _ := nativeReplayFixture(t)
	svc := &OpenAIGatewayService{}
	c := kittyChatIdentityContext(101, 202)
	require.False(t, svc.resolveNativeReasoningChatAuthorization(c, "gpt-6.1-sol").targetReplayEnabled(), "default off")
	svc.SetNativeReasoningReplayRuntime(nativeReplayPolicyReaderFunc(func(context.Context) (NativeReasoningReplayRuntimeConfig, error) {
		return NativeReasoningReplayRuntimeConfig{Mode: NativeReasoningReplayModeReplay, ModelUserIDs: map[string][]int64{"gpt-6.1-sol": {101}}}, nil
	}), engine)
	require.True(t, svc.resolveNativeReasoningChatAuthorization(c, "gpt-6.1-sol").targetReplayEnabled())
	require.False(t, svc.resolveNativeReasoningChatAuthorization(kittyChatIdentityContext(102, 202), "gpt-6.1-sol").targetReplayEnabled())
	require.False(t, svc.resolveNativeReasoningChatAuthorization(c, "gpt-6-astra").targetReplayEnabled())
}

func TestKittyChatIdentityBridgeReplayScopesStayIsolated(t *testing.T) {
	engine, _, fixture, completion := nativeReplayFixture(t)
	fixture.Parameters = json.RawMessage(strings.ReplaceAll(string(fixture.Parameters), "gpt-5.5", "gpt-6.1-sol"))
	completion.Response = json.RawMessage(strings.ReplaceAll(string(completion.Response), "gpt-5.5", "gpt-6.1-sol"))
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "synthetic-owner"}}
	svc := &OpenAIGatewayService{nativeReplayRuntime: &nativeReasoningRuntime{engine: engine, limits: engine.limits}}
	makeRequest := func(user, key int64, session, thread, token string) NativeReasoningReplayRequest {
		c := kittyChatIdentityContext(user, key)
		body, _ := json.Marshal(map[string]any{"messages": fixture.Messages, "client_metadata": map[string]string{"session_id": session, "thread_id": thread}})
		auth := nativeReasoningChatAuthorization{mode: NativeReasoningReplayModeReplay, model: "gpt-6.1-sol", modelRestricted: true, userID: user, apiKeyID: key}
		n := svc.beginNativeReasoningChat(c, account, body, auth)
		require.NotNil(t, n)
		r := fixture
		r.Scope = n.request.Scope
		r.Scope.Model = "gpt-6.1-sol"
		r.Scope.CredentialIdentity = nativeReasoningCredentialIdentity(c, account, token)
		return r
	}
	original := makeRequest(101, 202, "session", "thread", nativeChatJWT("owner", 1))
	require.NoError(t, engine.Commit(context.Background(), original, completion))
	for _, tc := range []struct {
		name string
		req  NativeReasoningReplayRequest
		hit  bool
	}{
		{"same", original, true},
		{"refreshed_same_principal", makeRequest(101, 202, "session", "thread", nativeChatJWT("owner", 2)), true},
		{"other_user", makeRequest(102, 202, "session", "thread", nativeChatJWT("owner", 1)), false},
		{"other_key", makeRequest(101, 203, "session", "thread", nativeChatJWT("owner", 1)), false},
		{"other_session", makeRequest(101, 202, "other", "thread", nativeChatJWT("owner", 1)), false},
		{"other_thread", makeRequest(101, 202, "session", "other", nativeChatJWT("owner", 1)), false},
		{"other_principal", makeRequest(101, 202, "session", "thread", nativeChatJWT("other", 1)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := nativeReplayContinuation(tc.req, completion)
			replacements, err := engine.Restore(context.Background(), r)
			require.NoError(t, err)
			if tc.hit {
				require.Len(t, replacements, 1)
			} else {
				require.Empty(t, replacements)
			}
		})
	}
	changed := nativeReplayContinuation(original, completion)
	changed.Messages = append([]json.RawMessage(nil), changed.Messages...)
	changed.Messages[0] = json.RawMessage(strings.ReplaceAll(string(changed.Messages[0]), "declared", "different"))
	replacements, err := engine.Restore(context.Background(), changed)
	require.NoError(t, err)
	require.Empty(t, replacements)
}
