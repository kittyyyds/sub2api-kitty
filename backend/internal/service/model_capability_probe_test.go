package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type modelCapabilityProbeRepo struct {
	AccountRepository
	account *Account
}

func (r *modelCapabilityProbeRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

type modelCapabilityProbeUpstream struct {
	status       int
	model        string
	invalidTool  bool
	timeout      bool
	responseBody string
}

type modelCapabilityProbeContextUpstream struct {
	redirectsDisabled bool
}

func (u *modelCapabilityProbeContextUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *modelCapabilityProbeContextUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.redirectsDisabled = HTTPUpstreamRedirectsDisabled(req.Context())
	return nil, context.Canceled
}

func (u *modelCapabilityProbeUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.DoWithTLS(req, "", 0, 0, nil)
}

func (u *modelCapabilityProbeUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if u.timeout {
		<-req.Context().Done()
		return nil, context.DeadlineExceeded
	}
	status := u.status
	if status == 0 {
		status = http.StatusOK
	}
	model := u.model
	if model == "" {
		model = "gpt-4o-mini"
	}
	body := `{"model":"` + model + `","choices":[{"message":{"content":"pong"}}]}`
	if strings.HasSuffix(req.URL.Path, "/responses") {
		body = `{"model":"` + model + `","output":[{"type":"message","content":[{"type":"output_text","text":"pong"}]}]}`
	}
	if strings.HasSuffix(req.URL.Path, "/responses") {
		requestBytes, _ := io.ReadAll(req.Body)
		if strings.Contains(string(requestBytes), `"responses_tools"`) {
			body = `{"model":"` + model + `","output":[{"type":"function_call","name":"probe_lookup_weather_deadbeef","arguments":"{\\"ok\\":true}"}]}`
		}
		// The production probe uses a random tool name; echo it from the request.
		if strings.Contains(string(requestBytes), `"tools"`) {
			payload := string(requestBytes)
			for _, prefix := range []string{`"name":"probe_lookup_weather_`, `"name":"paired_behavior_submit_`} {
				nameStart := strings.Index(payload, prefix)
				if nameStart < 0 {
					continue
				}
				nameStart += len(`"name":"`)
				nameEnd := strings.Index(payload[nameStart:], `"`)
				if nameEnd <= 0 {
					continue
				}
				name := payload[nameStart : nameStart+nameEnd]
				args := `{"ok":true}`
				if strings.HasPrefix(name, modelBehaviorToolNamePrefix) {
					answer := modelBehaviorCases[0].Expected
					for _, testCase := range modelBehaviorCases {
						if strings.Contains(payload, testCase.Prompt) {
							answer = testCase.Expected
							break
						}
					}
					args = `{"answer":"` + answer + `"}`
				}
				if u.invalidTool {
					args = `not-json`
				}
				body = `{"model":"` + model + `","output":[{"type":"function_call","name":"` + name + `","arguments":"` + strings.ReplaceAll(args, `"`, `\"`) + `"}]}`
				break
			}
		}
	}
	if u.responseBody != "" {
		body = u.responseBody
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func newModelCapabilityProbeService(upstream HTTPUpstream, account *Account) *AccountTestService {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	return &AccountTestService{
		accountRepo:  &modelCapabilityProbeRepo{account: account},
		httpUpstream: upstream,
		cfg:          cfg,
	}
}

func TestProbeModelCapabilitiesVerifiesTextAndToolCall(t *testing.T) {
	account := &Account{
		ID:       7,
		Name:     "probe",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "secret",
			"base_url": "http://upstream.test/v1",
		},
	}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{}, account)

	result, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{ModelID: "gpt-4o-mini"})
	require.NoError(t, err)
	require.Len(t, result.Results, 3)
	for _, item := range result.Results {
		require.Equal(t, "verified", item.Status, item.Protocol)
	}
	require.True(t, result.Results[2].ToolCallFound)
	require.True(t, result.Results[2].ToolArgumentsValid)
	require.NotEmpty(t, result.Results[2].ToolName)
}

func TestProbeModelCapabilitiesDisablesRedirects(t *testing.T) {
	account := &Account{
		ID: 13, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "secret", "base_url": "http://upstream.test/v1"},
	}
	upstream := &modelCapabilityProbeContextUpstream{}
	service := newModelCapabilityProbeService(upstream, account)
	_, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID: "gpt-5", Protocols: []string{ModelCapabilityProtocolResponses},
	})
	require.NoError(t, err)
	require.True(t, upstream.redirectsDisabled)
}

func TestProbeModelCapabilitiesClassifiesMismatchAndUnsupportedTool(t *testing.T) {
	account := &Account{
		ID:       8,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "secret",
			"base_url": "http://upstream.test/v1",
		},
	}
	upstream := &modelCapabilityProbeUpstream{model: "other-model", invalidTool: true}
	service := newModelCapabilityProbeService(upstream, account)
	result, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID:   "gpt-4o-mini",
		Protocols: []string{ModelCapabilityProtocolResponses, ModelCapabilityProtocolResponsesTools},
	})
	require.NoError(t, err)
	require.Equal(t, "model_mismatch", result.Results[0].Status)
	require.Equal(t, "unsupported", result.Results[1].Status)
	require.True(t, result.Results[1].ToolCallFound)
	require.False(t, result.Results[1].ToolArgumentsValid)

	validModelInvalidTool := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{invalidTool: true}, account)
	result, err = validModelInvalidTool.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID:   "gpt-4o-mini",
		Protocols: []string{ModelCapabilityProtocolResponsesTools},
	})
	require.NoError(t, err)
	require.Equal(t, "unsupported", result.Results[0].Status)
	require.True(t, result.Results[0].ToolCallFound)
	require.False(t, result.Results[0].ToolArgumentsValid)
}

func TestProbeModelCapabilitiesComparesResponseWithMappedModel(t *testing.T) {
	account := &Account{
		ID:       11,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "secret",
			"base_url": "http://upstream.test/v1",
			"model_mapping": map[string]any{
				"public-alias": "upstream-model",
			},
		},
	}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{model: "public-alias"}, account)

	result, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID:   "public-alias",
		Protocols: []string{ModelCapabilityProtocolResponsesTools},
	})

	require.NoError(t, err)
	require.Equal(t, "upstream-model", result.UpstreamModel)
	require.Equal(t, "model_mismatch", result.Results[0].Status)
	require.Equal(t, "responses_tools", result.Results[0].Capability)
	require.True(t, result.Results[0].ToolCallFound)
	require.True(t, result.Results[0].ToolArgumentsValid)
}

func TestProbeModelCapabilitiesClassifiesEndpointAndTimeout(t *testing.T) {
	account := &Account{
		ID:       9,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "secret",
			"base_url": "http://upstream.test/v1",
		},
	}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{status: http.StatusNotFound}, account)
	result, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{Protocols: []string{ModelCapabilityProtocolResponses}})
	require.NoError(t, err)
	require.Equal(t, "unsupported", result.Results[0].Status)
	require.Empty(t, result.Results[0].UpstreamModel)
	require.True(t, result.Results[0].Attempted)

	timeoutService := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{timeout: true}, account)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = timeoutService.ProbeModelCapabilities(ctx, account.ID, ModelCapabilityProbeInput{Protocols: []string{ModelCapabilityProtocolResponses}})
	require.NoError(t, err)
	require.Equal(t, "timeout", result.Results[0].Status)
	require.True(t, result.Results[0].Attempted)
}

func TestCapabilityProbeErrorDoesNotExposeUntrustedUpstreamText(t *testing.T) {
	body := []byte(`{"error":{"message":"Authorization: Bearer secret-value"}}`)
	message := capabilityProbeError(body)
	require.Equal(t, "upstream returned an error", message)
	require.NotContains(t, message, "secret-value")
}

func TestCapabilityProbePayloadUsesReasoningSafeOutputBudget(t *testing.T) {
	for _, protocol := range []string{
		ModelCapabilityProtocolChatCompletions,
		ModelCapabilityProtocolResponses,
		ModelCapabilityProtocolResponsesTools,
	} {
		body, err := capabilityProbePayload(protocol, "gpt-5", "probe_tool")
		require.NoError(t, err)
		if protocol == ModelCapabilityProtocolChatCompletions {
			require.Contains(t, string(body), `"max_tokens":512`)
		} else {
			require.Contains(t, string(body), `"max_output_tokens":512`)
		}
	}
}

func TestProbeModelCapabilitiesTreatsBudgetExhaustionAsInconclusive(t *testing.T) {
	account := &Account{
		ID: 15, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "secret", "base_url": "http://upstream.test/v1"},
	}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{
		responseBody: `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`,
	}, account)
	result, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID: "gpt-5", Protocols: []string{ModelCapabilityProtocolResponses, ModelCapabilityProtocolResponsesTools},
	})
	require.NoError(t, err)
	for _, item := range result.Results {
		require.Equal(t, "needs_retest", item.Status)
		require.Equal(t, "output_budget_exhausted", item.Evidence)
		require.NotEqual(t, "unsupported", item.Status)
	}
}

func TestProbeModelCapabilitiesRedactsCredentialEchoes(t *testing.T) {
	account := &Account{
		ID: 16, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "top-secret-token", "base_url": "http://upstream.test/v1"},
	}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{
		status:       http.StatusBadRequest,
		responseBody: `{"error":{"message":"Authorization: Bearer top-secret-token"}}`,
	}, account)
	result, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID: "gpt-5", Protocols: []string{ModelCapabilityProtocolResponses},
	})
	require.NoError(t, err)
	require.Equal(t, "upstream returned an error", result.Results[0].Error)
	require.NotContains(t, result.Results[0].Error, "top-secret-token")

	modelEchoService := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{
		responseBody: `{"model":"top-secret-token","output":[{"type":"message","content":[{"type":"output_text","text":"pong"}]}]}`,
	}, account)
	result, err = modelEchoService.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID: "gpt-5", Protocols: []string{ModelCapabilityProtocolResponses},
	})
	require.NoError(t, err)
	require.Equal(t, "[redacted]", result.Results[0].UpstreamModel)
}

func TestCapabilityProbeTextResponseRequiresActualText(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		body     string
		want     bool
	}{
		{name: "empty chat choice", protocol: ModelCapabilityProtocolChatCompletions, body: `{"choices":[{}]}`},
		{name: "empty chat content", protocol: ModelCapabilityProtocolChatCompletions, body: `{"choices":[{"message":{"content":"  "}}]}`},
		{name: "chat text", protocol: ModelCapabilityProtocolChatCompletions, body: `{"choices":[{"message":{"content":"pong"}}]}`, want: true},
		{name: "chat content part", protocol: ModelCapabilityProtocolChatCompletions, body: `{"choices":[{"message":{"content":[{"type":"text","text":"pong"}]}}]}`, want: true},
		{name: "chat numeric content part", protocol: ModelCapabilityProtocolChatCompletions, body: `{"choices":[{"message":{"content":[{"type":"text","text":42}]}}]}`},
		{name: "empty responses output", protocol: ModelCapabilityProtocolResponses, body: `{"output":[{}]}`},
		{name: "responses function only", protocol: ModelCapabilityProtocolResponses, body: `{"output":[{"type":"function_call","name":"tool","arguments":"{}"}]}`},
		{name: "responses output text", protocol: ModelCapabilityProtocolResponses, body: `{"output":[{"type":"message","content":[{"type":"output_text","text":"pong"}]}]}`, want: true},
		{name: "responses numeric content part", protocol: ModelCapabilityProtocolResponses, body: `{"output":[{"type":"message","content":[{"type":"output_text","text":42}]}]}`},
		{name: "responses convenience text", protocol: ModelCapabilityProtocolResponses, body: `{"output_text":"pong"}`, want: true},
		{name: "responses numeric convenience text", protocol: ModelCapabilityProtocolResponses, body: `{"output_text":42}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, capabilityProbeTextResponseValid(tt.protocol, []byte(tt.body)))
		})
	}
}

func TestCapabilityProbeBoundsUntrustedMetadataByRunes(t *testing.T) {
	longValue := strings.Repeat("界", modelCapabilityAuditTextMaxRunes+1)
	responseModel := truncateModelCapabilityAuditText(capabilityProbeResponseModel([]byte(`{"model":"` + longValue + `"}`)))
	require.True(t, utf8.ValidString(responseModel))
	require.Len(t, []rune(responseModel), modelCapabilityAuditTextMaxRunes)

	toolBody := []byte(`{"output":[{"type":"function_call","name":"` + longValue + `","arguments":"{\"ok\":true}"}]}`)
	found, valid, toolName := capabilityProbeToolCall(toolBody, "expected_tool")
	require.True(t, found)
	require.False(t, valid)
	require.True(t, utf8.ValidString(toolName))
	require.Len(t, []rune(toolName), modelCapabilityAuditTextMaxRunes)
}

func TestProbeModelCapabilitiesComparesUntruncatedResponseModel(t *testing.T) {
	mappedModel := strings.Repeat("m", modelCapabilityAuditTextMaxRunes)
	account := &Account{
		ID: 14, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "secret", "base_url": "http://upstream.test/v1",
			"model_mapping": map[string]any{"public-alias": mappedModel},
		},
	}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{model: mappedModel + "-different"}, account)
	result, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID: "public-alias", Protocols: []string{ModelCapabilityProtocolResponses},
	})
	require.NoError(t, err)
	require.Equal(t, "model_mismatch", result.Results[0].Status)
	require.Len(t, []rune(result.Results[0].UpstreamModel), modelCapabilityAuditTextMaxRunes)
}

func TestProbeModelCapabilitiesRejectsOverlongRequestedAndMappedModels(t *testing.T) {
	account := &Account{
		ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "secret", "base_url": "http://upstream.test/v1",
			"model_mapping": map[string]any{"public-alias": strings.Repeat("m", modelCapabilityAuditTextMaxRunes+1)},
		},
	}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{}, account)
	_, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{
		ModelID: strings.Repeat("m", modelCapabilityAuditTextMaxRunes+1),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MODEL_CAPABILITY_MODEL_INVALID")

	_, err = service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{ModelID: "public-alias"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MODEL_CAPABILITY_MAPPED_MODEL_INVALID")
}

func TestProbeModelCapabilitiesRejectsUnsupportedAccount(t *testing.T) {
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{}, account)
	_, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{})
	require.Error(t, err)
}
