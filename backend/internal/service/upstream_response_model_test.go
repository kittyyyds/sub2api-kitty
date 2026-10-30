package service

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamResponseModelObserverTerminalWinsAndRecordsConflict(t *testing.T) {
	observer := &upstreamResponseModelObserver{}

	observer.ObserveOpenAI([]byte(`{"type":"response.created","response":{"model":"gpt-5.5"}}`), "response.created")
	observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-5.4"}}`), "response.completed")

	require.Equal(t, "gpt-5.4", observer.Model())
	require.True(t, observer.Conflict())
}

func TestUpstreamResponseModelObserverSupportsAnthropicAndGeminiShapes(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveAnthropic([]byte(`{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`))
		require.Equal(t, "claude-sonnet-4-20250514", observer.Model())
	})

	t.Run("gemini outer and nested", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveGemini([]byte(`{"response":{"modelVersion":"gemini-2.5-pro"}}`))
		observer.ObserveGemini([]byte(`{"modelVersion":"gemini-2.5-pro-latest"}`))
		require.Equal(t, "gemini-2.5-pro-latest", observer.Model())
		require.True(t, observer.Conflict())
	})
}

func TestUpstreamResponseModelObservationAttemptReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	first := beginUpstreamResponseModelObservation(c)
	first.Observe("failed-attempt-model", false)
	second := beginUpstreamResponseModelObservation(c)
	second.Observe("successful-attempt-model", false)

	require.Equal(t, "successful-attempt-model", observedUpstreamResponseModel(c))
	require.False(t, observedUpstreamResponseModelConflict(c))
}

func TestUpstreamModelMismatchThreeStateAndCaseInsensitiveComparison(t *testing.T) {
	require.Nil(t, upstreamModelMismatch("gpt-5.5", ""))
	require.Nil(t, upstreamModelMismatch("grok-4.7", ""))

	matched := upstreamModelMismatch("gpt-5.5", "GPT-5.5")
	require.NotNil(t, matched)
	require.False(t, *matched)

	mismatched := upstreamModelMismatch("gpt-5.5", "gpt-5.4")
	require.NotNil(t, mismatched)
	require.True(t, *mismatched)
}

func TestUpstreamModelMismatchTreatsGrokBuildRuntimeIDsAsAliases(t *testing.T) {
	tests := []struct {
		name          string
		sentModel     string
		responseModel string
	}{
		{
			name:          "issue 5634 grok 4.6",
			sentModel:     "grok-4.6",
			responseModel: "grok-4.6-build",
		},
		{
			name:          "grok 4.6 latest",
			sentModel:     "grok-4.6-latest",
			responseModel: "grok-4.6-build",
		},
		{
			name:          "issue 5647 grok 4.5 latest",
			sentModel:     "grok-4.5-latest",
			responseModel: "grok-4.5-build",
		},
		{
			name:          "grok 4.5 canonical",
			sentModel:     "grok-4.5",
			responseModel: "GROK-4.5-BUILD",
		},
		{
			name:          "grok 4.7 build alias",
			sentModel:     "grok-4.7",
			responseModel: "grok-4.7-build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mismatch := upstreamModelMismatch(tt.sentModel, tt.responseModel)

			require.NotNil(t, mismatch)
			require.False(t, *mismatch)
		})
	}
}

func TestUpstreamModelMismatchTreatsOfficialOpenAISnapshotsAsAliases(t *testing.T) {
	for _, tt := range []struct {
		sent, response string
	}{
		{"gpt-4o", "gpt-4o-2024-08-06"},
		{"gpt-4o-latest", "gpt-4o-2024-08-06"},
		{"o3-mini", "o3-mini-2025-01-31"},
		{"gpt-4", "gpt-4-0613"},
	} {
		mismatch := upstreamModelMismatch(tt.sent, tt.response)
		require.NotNil(t, mismatch)
		require.False(t, *mismatch, "%s should match %s", tt.sent, tt.response)
	}
}

func TestUpstreamModelMismatchRejectsNonSnapshotSuffixes(t *testing.T) {
	for _, response := range []string{"gpt-5-mini", "gpt-5-build", "gpt-5-2025-99-99"} {
		mismatch := upstreamModelMismatch("gpt-5", response)
		require.NotNil(t, mismatch)
		require.True(t, *mismatch, "gpt-5 must not match %s", response)
	}
}

func TestUpstreamModelMismatchDoesNotCollapseDifferentModels(t *testing.T) {
	tests := []struct {
		name          string
		sentModel     string
		responseModel string
	}{
		{
			name:          "different grok versions",
			sentModel:     "grok-4.5",
			responseModel: "grok-4.6-build",
		},
		{
			name:          "unrelated build suffix",
			sentModel:     "gpt-5.5",
			responseModel: "gpt-5.5-build",
		},
		{
			name:          "different grok runtime",
			sentModel:     "grok-build-0.1",
			responseModel: "grok-4.5-build",
		},
		{
			name:          "composer fast is not a 4.5 build alias",
			sentModel:     "grok-composer-2.5-fast",
			responseModel: "grok-4.5-build",
		},
		{
			name:          "fast build is not the plain build alias",
			sentModel:     "grok-4.7-build-fast",
			responseModel: "grok-4.7-build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mismatch := upstreamModelMismatch(tt.sentModel, tt.responseModel)

			require.NotNil(t, mismatch)
			require.True(t, *mismatch)
		})
	}
}

func TestObserveOpenAISSEBodyIgnoresMalformedPayload(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observeOpenAISSEBody(observer, "data: not-json\n\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.4\"}}\n\n")

	require.Equal(t, "gpt-5.4", observer.Model())
	require.False(t, observer.Conflict())
}

func TestObserveAntigravityGeminiSSELineReadsWrapperModelWithoutUnwrap(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "top-level sibling",
			payload: `{"modelVersion":"gemini-3-pro","response":{"candidates":[]}}`,
			want:    "gemini-3-pro",
		},
		{
			name:    "single wrapper",
			payload: `{"response":{"modelVersion":"gemini-3-pro","candidates":[]}}`,
			want:    "gemini-3-pro",
		},
		{
			name:    "nested response after one wrapper",
			payload: `{"response":{"response":{"modelVersion":"gemini-3-pro","candidates":[]}}}`,
			want:    "gemini-3-pro",
		},
		{
			name:    "outer declaration takes precedence",
			payload: `{"modelVersion":"gemini-outer","response":{"modelVersion":"gemini-inner","candidates":[]}}`,
			want:    "gemini-outer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(nil)
			beginUpstreamResponseModelObservation(c)

			svc := &AntigravityGatewayService{}
			svc.observeAntigravityGeminiSSELine(c, "data: "+tt.payload)

			require.Equal(t, tt.want, observedUpstreamResponseModel(c))
			require.False(t, observedUpstreamResponseModelConflict(c))
		})
	}
}

func TestUpstreamResponseModelObserverRejectsMalformedJSONWithModelField(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observer.ObserveOpenAI([]byte(`{"response":{"model":"gpt-5.4"}`), "response.completed")

	require.Empty(t, observer.Model())
}

func TestUpstreamResponseModelObserverBoundsUntrustedModelName(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observer.Observe("  "+strings.Repeat("模", upstreamResponseModelMaxLength+1)+"  ", false)

	require.Len(t, []rune(observer.Model()), upstreamResponseModelMaxLength)
}

func TestRedactUpstreamResponseModelSecrets(t *testing.T) {
	longOverride := strings.Repeat("L", maxHeaderOverrideValueLength)
	longOverridePrefix := string([]rune(longOverride)[:upstreamResponseModelMaxLength])
	prefixedTruncatedLeak := "relay:" + longOverridePrefix[:upstreamResponseModelMaxLength-len("relay:")]
	longCredential := strings.Repeat("C", maxHeaderOverrideValueLength)
	longCredentialPrefix := string([]rune(longCredential)[:upstreamResponseModelMaxLength])
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":                    "api-key-secret-canary",
			"access_token":               "access-token-secret-canary",
			"refresh_token":              "Bearer refresh-token-secret-canary",
			"id_token":                   longCredential,
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides: map[string]any{
				"x-relay-key":     "relay-secret-canary",
				"x-bearer-secret": "Bearer bearer-secret-canary",
				"x-long-secret":   longOverride,
			},
		},
	}

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "exact override", model: "relay-secret-canary", want: upstreamResponseModelRedacted},
		{name: "embedded override", model: "model-relay-secret-canary-runtime", want: upstreamResponseModelRedacted},
		{name: "bearer token without scheme", model: "bearer-secret-canary", want: upstreamResponseModelRedacted},
		{name: "long override truncated by observer", model: longOverridePrefix, want: upstreamResponseModelRedacted},
		{name: "prefixed long override truncated by observer", model: prefixedTruncatedLeak, want: upstreamResponseModelRedacted},
		{name: "API key", model: "runtime-api-key-secret-canary", want: upstreamResponseModelRedacted},
		{name: "access token", model: "access-token-secret-canary", want: upstreamResponseModelRedacted},
		{name: "refresh token without scheme", model: "refresh-token-secret-canary", want: upstreamResponseModelRedacted},
		{name: "long credential truncated by observer", model: longCredentialPrefix, want: upstreamResponseModelRedacted},
		{name: "ordinary model", model: "gpt-5.5", want: "gpt-5.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, redactUpstreamResponseModelSecrets(account, tt.model))
		})
	}
}

func TestRedactUpstreamResponseModelCoversSensitiveCredentialTable(t *testing.T) {
	for _, key := range SensitiveCredentialKeys {
		t.Run(key, func(t *testing.T) {
			secret := "credential-secret-canary-" + key
			account := &Account{Credentials: map[string]any{key: secret}}
			require.Equal(t, upstreamResponseModelRedacted, redactUpstreamResponseModelSecrets(account, "relay:"+secret))
		})
	}
}

func TestUpstreamResponseModelObserverServiceTierTerminalEventWins(t *testing.T) {
	observer := &upstreamResponseModelObserver{}

	// response.created echoes the requested tier and must not count as a declaration.
	observer.ObserveOpenAI([]byte(`{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol","service_tier":"priority"}}`), "response.created")
	require.Equal(t, "", observer.ServiceTier())

	observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.6-sol","service_tier":"default","usage":{"input_tokens":1,"output_tokens":2}}}`), "response.completed")
	require.Equal(t, "default", observer.ServiceTier())
	require.False(t, observer.Conflict(), "tier echo must not be reported as a model conflict")
}

func TestUpstreamResponseModelObserverServiceTierUntypedPayloads(t *testing.T) {
	t.Run("chat completions chunks agree", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveOpenAI([]byte(`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-5.4","service_tier":"default","choices":[]}`), "")
		observer.ObserveOpenAI([]byte(`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-5.4","service_tier":"default","choices":[]}`), "")
		require.Equal(t, "default", observer.ServiceTier())
	})

	t.Run("disagreeing chunks are not trusted", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveOpenAI([]byte(`{"model":"gpt-5.4","service_tier":"priority"}`), "")
		observer.ObserveOpenAI([]byte(`{"model":"gpt-5.4","service_tier":"default"}`), "")
		require.Equal(t, "", observer.ServiceTier())
	})

	t.Run("non-stream body normalises fast and drops auto", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveOpenAI([]byte(`{"id":"resp_1","object":"response","model":"gpt-5.6-sol","service_tier":"fast"}`), "")
		require.Equal(t, "priority", observer.ServiceTier())

		auto := &upstreamResponseModelObserver{}
		auto.ObserveOpenAI([]byte(`{"id":"resp_2","object":"response","model":"gpt-5.6-sol","service_tier":"auto"}`), "")
		require.Equal(t, "", auto.ServiceTier())
	})

	t.Run("model-free deltas never declare a tier", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveOpenAI([]byte(`{"type":"response.output_text.delta","delta":"hi","service_tier":"default"}`), "response.output_text.delta")
		require.Equal(t, "", observer.ServiceTier())
	})
}

func TestUpstreamResponseModelObserverServiceTierAnthropicSpeed(t *testing.T) {
	t.Run("message_start reports standard", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveAnthropic([]byte(`{"type":"message_start","message":{"id":"msg_1","model":"claude-opus-5","usage":{"input_tokens":8,"output_tokens":1,"speed":"standard"}}}`))
		observer.ObserveAnthropic([]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`))
		require.Equal(t, "standard", observer.ServiceTier())
		require.Equal(t, "claude-opus-5", observer.Model())
	})

	t.Run("non-stream body reports fast", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveAnthropic([]byte(`{"id":"msg_1","type":"message","model":"claude-opus-5","usage":{"input_tokens":8,"output_tokens":12,"speed":"fast"}}`))
		require.Equal(t, "fast", observer.ServiceTier())
	})

	t.Run("missing speed declares nothing", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{}
		observer.ObserveAnthropic([]byte(`{"id":"msg_1","type":"message","model":"claude-opus-5","usage":{"input_tokens":8,"output_tokens":12}}`))
		require.Equal(t, "", observer.ServiceTier())
	})
}

func TestObservedUpstreamResponseServiceTierFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	require.Equal(t, "", observedUpstreamResponseServiceTier(c))

	observer := beginUpstreamResponseModelObservation(c)
	observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.6-sol","service_tier":"default"}}`), "response.completed")
	require.Equal(t, "default", observedUpstreamResponseServiceTier(c))
}

func TestAstraGatewayCompletionUsesTextDelta(t *testing.T) {
	o := &upstreamResponseModelObserver{}
	o.ObserveOpenAI([]byte(`{"type":"response.output_text.delta","delta":"323"}`), "response.output_text.delta")
	o.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-6-astra","status":"completed","output":[]}}`), "response.completed")
	require.True(t, o.astraCompleted)
}
