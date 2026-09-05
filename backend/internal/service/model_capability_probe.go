package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/tidwall/gjson"
)

const (
	ModelCapabilityProtocolChatCompletions = "chat_completions"
	ModelCapabilityProtocolResponses       = "responses"
	ModelCapabilityProtocolResponsesTools  = "responses_tools"
	modelCapabilityProbeTimeout            = 20 * time.Second
	modelCapabilityProbeMaxBodyBytes       = 256 * 1024
)

// ModelCapabilityProbeInput describes one administrator-triggered probe.
// Protocols may be omitted to run all supported OpenAI-compatible probes.
type ModelCapabilityProbeInput struct {
	ModelID   string
	Protocols []string
}

type ModelCapabilityProbeResult struct {
	Protocol           string `json:"protocol"`
	Status             string `json:"status"`
	Capability         string `json:"capability"`
	RequestedModel     string `json:"requested_model"`
	UpstreamModel      string `json:"upstream_model,omitempty"`
	HTTPStatus         int    `json:"http_status"`
	LatencyMS          int64  `json:"latency_ms"`
	ToolName           string `json:"tool_name,omitempty"`
	ToolCallFound      bool   `json:"tool_call_found"`
	ToolArgumentsValid bool   `json:"tool_arguments_valid"`
	Error              string `json:"error,omitempty"`
	Evidence           string `json:"evidence"`
}

type ModelCapabilityProbeResponse struct {
	AccountID      int64                        `json:"account_id"`
	AccountName    string                       `json:"account_name"`
	Platform       string                       `json:"platform"`
	RequestedModel string                       `json:"requested_model"`
	UpstreamModel  string                       `json:"upstream_model,omitempty"`
	Results        []ModelCapabilityProbeResult `json:"results"`
	Disclaimer     string                       `json:"disclaimer"`
}

func normalizeModelCapabilityProtocols(protocols []string) ([]string, error) {
	if len(protocols) == 0 {
		return []string{
			ModelCapabilityProtocolChatCompletions,
			ModelCapabilityProtocolResponses,
			ModelCapabilityProtocolResponsesTools,
		}, nil
	}
	seen := make(map[string]struct{}, len(protocols))
	result := make([]string, 0, len(protocols))
	for _, raw := range protocols {
		protocol := strings.ToLower(strings.TrimSpace(raw))
		switch protocol {
		case ModelCapabilityProtocolChatCompletions, ModelCapabilityProtocolResponses, ModelCapabilityProtocolResponsesTools:
		default:
			return nil, infraerrors.BadRequest("MODEL_CAPABILITY_PROTOCOL_INVALID", "unsupported capability probe protocol")
		}
		if _, ok := seen[protocol]; ok {
			continue
		}
		seen[protocol] = struct{}{}
		result = append(result, protocol)
	}
	if len(result) == 0 {
		return nil, infraerrors.BadRequest("MODEL_CAPABILITY_PROTOCOL_EMPTY", "at least one probe protocol is required")
	}
	return result, nil
}

func capabilityProbeNonce() string {
	var raw [5]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func capabilityProbePayload(protocol, modelID, toolName string) ([]byte, error) {
	switch protocol {
	case ModelCapabilityProtocolChatCompletions:
		return json.Marshal(map[string]any{
			"model":      modelID,
			"messages":   []map[string]any{{"role": "user", "content": "Reply with the single word pong."}},
			"max_tokens": 8,
			"stream":     false,
		})
	case ModelCapabilityProtocolResponses:
		return json.Marshal(map[string]any{
			"model":             modelID,
			"input":             "Reply with the single word pong.",
			"max_output_tokens": 8,
			"stream":            false,
		})
	case ModelCapabilityProtocolResponsesTools:
		return json.Marshal(map[string]any{
			"model": modelID,
			"input": []map[string]any{{
				"role": "user",
				"content": []map[string]any{{
					"type": "input_text",
					"text": "Call the supplied function with ok=true. You must use the tool.",
				}},
			}},
			"tools": []map[string]any{{
				"type":        "function",
				"name":        toolName,
				"description": "Capability probe function.",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
					"required":   []string{"ok"},
				},
			}},
			"tool_choice":       "required",
			"max_output_tokens": 64,
			"stream":            false,
		})
	default:
		return nil, errors.New("unsupported capability probe protocol")
	}
}

func capabilityProbeError(body []byte) string {
	message := strings.TrimSpace(gjson.GetBytes(body, "error.message").String())
	if message == "" {
		message = strings.TrimSpace(gjson.GetBytes(body, "message").String())
	}
	if message == "" {
		return "upstream returned an error"
	}
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}

func capabilityProbeResponseModel(body []byte) string {
	return strings.TrimSpace(gjson.GetBytes(body, "model").String())
}

func capabilityProbeJSONBody(body []byte) bool {
	var value any
	return json.Unmarshal(body, &value) == nil
}

func capabilityProbeTextResponseValid(protocol string, body []byte) bool {
	switch protocol {
	case ModelCapabilityProtocolChatCompletions:
		return len(gjson.GetBytes(body, "choices").Array()) > 0
	case ModelCapabilityProtocolResponses:
		if strings.TrimSpace(gjson.GetBytes(body, "output_text").String()) != "" {
			return true
		}
		return len(gjson.GetBytes(body, "output").Array()) > 0
	default:
		return true
	}
}

func capabilityProbeToolCall(body []byte, expectedName string) (found, valid bool, name string) {
	var response struct {
		Output []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"output"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false, false, ""
	}
	for _, item := range response.Output {
		if item.Type != "function_call" {
			continue
		}
		found = true
		name = item.Name
		if name != expectedName {
			return true, false, name
		}
		var args map[string]any
		if json.Unmarshal([]byte(item.Arguments), &args) != nil {
			return true, false, name
		}
		ok, exists := args["ok"].(bool)
		return true, exists && ok, name
	}
	return false, false, ""
}

func capabilityProbeTLSProfile(s *AccountTestService, account *Account) *tlsfingerprint.Profile {
	if s == nil || s.tlsFPProfileService == nil || account == nil {
		return nil
	}
	return s.tlsFPProfileService.ResolveTLSProfile(account)
}

// ProbeModelCapabilities sends bounded, non-streaming requests through the same
// upstream transport used by normal forwarding. It deliberately does not persist
// request bodies, response bodies, or credentials.
func (s *AccountTestService) ProbeModelCapabilities(ctx context.Context, accountID int64, input ModelCapabilityProbeInput) (*ModelCapabilityProbeResponse, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, errors.New("capability probe service unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if account.Type != AccountTypeAPIKey || (!account.IsOpenAI() && !account.IsCNProvider()) {
		return nil, infraerrors.BadRequest("MODEL_CAPABILITY_ACCOUNT_UNSUPPORTED", "only OpenAI-compatible API key accounts can be probed")
	}
	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return nil, infraerrors.BadRequest("MODEL_CAPABILITY_CREDENTIAL_MISSING", "account API key is missing")
	}

	protocols, err := normalizeModelCapabilityProtocols(input.Protocols)
	if err != nil {
		return nil, err
	}
	requestedModel := strings.TrimSpace(input.ModelID)
	if requestedModel == "" {
		requestedModel = openai.DefaultTestModel
	}
	upstreamModel := strings.TrimSpace(account.GetMappedModel(requestedModel))
	if upstreamModel == "" {
		upstreamModel = requestedModel
	}
	response := &ModelCapabilityProbeResponse{
		AccountID:      account.ID,
		AccountName:    account.Name,
		Platform:       account.Platform,
		RequestedModel: requestedModel,
		UpstreamModel:  upstreamModel,
		Disclaimer:     "工具调用可以验证上游能力和返回结构，但不能百分之百证明底层模型身份。建议更换随机探测后复测。",
		Results:        make([]ModelCapabilityProbeResult, 0, len(protocols)),
	}

	for _, protocol := range protocols {
		result := s.probeModelCapabilityProtocol(ctx, account, requestedModel, upstreamModel, authToken, protocol)
		response.Results = append(response.Results, result)
	}
	return response, nil
}

func (s *AccountTestService) probeModelCapabilityProtocol(parent context.Context, account *Account, requestedModel, upstreamModel, authToken, protocol string) ModelCapabilityProbeResult {
	result := ModelCapabilityProbeResult{
		Protocol:       protocol,
		Status:         "error",
		Capability:     "unknown",
		RequestedModel: requestedModel,
		HTTPStatus:     0,
		Evidence:       "no_response",
	}
	toolName := ""
	if protocol == ModelCapabilityProtocolResponsesTools {
		toolName = "probe_lookup_weather_" + capabilityProbeNonce()
		result.ToolName = toolName
	}
	body, err := capabilityProbePayload(protocol, upstreamModel, toolName)
	if err != nil {
		result.Error = "failed to build probe request"
		return result
	}

	baseURL := account.GetOpenAIBaseURL()
	if account.IsCNProvider() && account.IsAdaptiveAPIProtocol() {
		if protocol == ModelCapabilityProtocolResponses || protocol == ModelCapabilityProtocolResponsesTools {
			baseURL = account.GetCNProtocolBaseURL(APIProtocolResponses)
		} else {
			baseURL = account.GetCNProtocolBaseURL(APIProtocolChatCompletions)
		}
	}
	if strings.TrimSpace(baseURL) == "" {
		result.Error = "account base URL is missing"
		result.Evidence = "configuration_error"
		return result
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		result.Error = "invalid account base URL"
		result.Evidence = "configuration_error"
		return result
	}
	probeURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)
	if protocol == ModelCapabilityProtocolResponses || protocol == ModelCapabilityProtocolResponsesTools {
		probeURL = buildOpenAIResponsesURLForPlatform(account.Platform, normalizedBaseURL)
	}

	probeCtx, cancel := context.WithTimeout(parent, modelCapabilityProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodPost, probeURL, bytes.NewReader(body))
	if err != nil {
		result.Error = "failed to create probe request"
		return result
	}
	request = request.WithContext(WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+authToken)
	if protocol == ModelCapabilityProtocolChatCompletions {
		applyOpenAICodexProbeHeaders(request.Header)
	}
	account.ApplyHeaderOverrides(request.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	started := time.Now()
	resp, err := s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, capabilityProbeTLSProfile(s, account))
	result.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			result.Status = "timeout"
			result.Evidence = "request_timeout"
			result.Error = "upstream request timed out"
			return result
		}
		result.Evidence = "transport_error"
		result.Error = "upstream request failed"
		return result
	}
	if resp == nil {
		result.Evidence = "transport_error"
		result.Error = "upstream returned no response"
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	result.HTTPStatus = resp.StatusCode
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, modelCapabilityProbeMaxBodyBytes))
	if readErr != nil {
		result.Evidence = "response_read_error"
		result.Error = "failed to read upstream response"
		return result
	}
	result.UpstreamModel = capabilityProbeResponseModel(responseBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			result.Status = "unsupported"
			result.Capability = "unsupported"
			result.Evidence = "endpoint_status"
			result.Error = fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode)
			return result
		}
		result.Evidence = "upstream_status"
		result.Error = capabilityProbeError(responseBody)
		return result
	}
	if !capabilityProbeJSONBody(responseBody) {
		result.Status = "needs_retest"
		result.Evidence = "invalid_json"
		result.Error = "upstream returned invalid JSON"
		return result
	}
	if protocol != ModelCapabilityProtocolResponsesTools && !capabilityProbeTextResponseValid(protocol, responseBody) {
		result.Status = "unsupported"
		if protocol == ModelCapabilityProtocolResponses {
			result.Capability = "responses_text"
		} else {
			result.Capability = "chat_completions_text"
		}
		result.Evidence = "invalid_response_shape"
		result.Error = "upstream returned no text output"
		return result
	}

	if result.UpstreamModel != "" && !strings.EqualFold(result.UpstreamModel, requestedModel) && !strings.EqualFold(result.UpstreamModel, upstreamModel) {
		result.Status = "model_mismatch"
		result.Capability = "verified"
		result.Evidence = "response_model_mismatch"
		result.Error = "upstream response model differs from requested model"
		return result
	}
	if protocol == ModelCapabilityProtocolResponsesTools {
		result.ToolCallFound, result.ToolArgumentsValid, result.ToolName = capabilityProbeToolCall(responseBody, toolName)
		if result.ToolCallFound && result.ToolArgumentsValid {
			result.Status = "verified"
			result.Capability = "responses_tools"
			result.Evidence = "verified_tool_call"
			return result
		}
		result.Status = "unsupported"
		result.Capability = "responses_tools"
		result.Evidence = "missing_or_invalid_tool_call"
		result.Error = "response did not contain the expected tool call and valid JSON arguments"
		return result
	}

	result.Status = "verified"
	if protocol == ModelCapabilityProtocolResponses {
		result.Capability = "responses_text"
	} else {
		result.Capability = "chat_completions_text"
	}
	result.Evidence = "valid_json_response"
	return result
}
