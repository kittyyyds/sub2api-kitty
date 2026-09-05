package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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
	status      int
	model       string
	invalidTool bool
	timeout     bool
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
			nameStart := strings.Index(payload, `"name":"probe_lookup_weather_`)
			if nameStart >= 0 {
				nameStart += len(`"name":"`)
				nameEnd := strings.Index(payload[nameStart:], `"`)
				if nameEnd > 0 {
					name := payload[nameStart : nameStart+nameEnd]
					args := `{"ok":true}`
					if u.invalidTool {
						args = `not-json`
					}
					body = `{"model":"` + model + `","output":[{"type":"function_call","name":"` + name + `","arguments":"` + strings.ReplaceAll(args, `"`, `\"`) + `"}]}`
				}
			}
		}
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
	require.Equal(t, "model_mismatch", result.Results[1].Status)

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

	timeoutService := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{timeout: true}, account)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = timeoutService.ProbeModelCapabilities(ctx, account.ID, ModelCapabilityProbeInput{Protocols: []string{ModelCapabilityProtocolResponses}})
	require.NoError(t, err)
	require.Equal(t, "timeout", result.Results[0].Status)
}

func TestProbeModelCapabilitiesRejectsUnsupportedAccount(t *testing.T) {
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	service := newModelCapabilityProbeService(&modelCapabilityProbeUpstream{}, account)
	_, err := service.ProbeModelCapabilities(context.Background(), account.ID, ModelCapabilityProbeInput{})
	require.Error(t, err)
}
