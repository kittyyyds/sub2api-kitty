package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const (
	UsageAuditVersion = 1

	UsageAuditProtocolAnthropicMessages  = "anthropic_messages"
	UsageAuditProtocolOpenAIResponses    = "openai_responses"
	UsageAuditProtocolOpenAIChat         = "openai_chat_completions"
	UsageAuditProtocolUnknown            = "unknown"
	UsageAuditEstimatorAnthropicTiktoken = "anthropic_to_responses_tiktoken_v1"
	UsageAuditEstimatorResponsesTiktoken = "responses_tiktoken_v1"
	UsageAuditEstimatorSemanticJSON      = "semantic_json_tiktoken_v1"

	usageAuditRawEvidenceMaxBytes = 32 * 1024
)

type UsageAuditStatus string

const (
	UsageAuditStatusPass          UsageAuditStatus = "pass"
	UsageAuditStatusWarning       UsageAuditStatus = "warning"
	UsageAuditStatusAnomaly       UsageAuditStatus = "anomaly"
	UsageAuditStatusUnverifiable  UsageAuditStatus = "unverifiable"
	UsageAuditStatusNotApplicable UsageAuditStatus = "not_applicable"
	UsageAuditFilterMissing                        = "missing"
)

type UsageAuditConfidence string

const (
	UsageAuditConfidenceHigh   UsageAuditConfidence = "high"
	UsageAuditConfidenceMedium UsageAuditConfidence = "medium"
	UsageAuditConfidenceLow    UsageAuditConfidence = "low"
)

type UsageAuditEvidenceSource string

const (
	UsageAuditEvidenceUpstreamRaw UsageAuditEvidenceSource = "upstream_raw"
	UsageAuditEvidenceGateway     UsageAuditEvidenceSource = "gateway_response"
	UsageAuditEvidenceNormalized  UsageAuditEvidenceSource = "normalized_only"
)

type UsageAuditTokenSnapshot struct {
	InputTokens         *int `json:"input_tokens,omitempty"`
	TotalInputTokens    *int `json:"total_input_tokens,omitempty"`
	OutputTokens        *int `json:"output_tokens,omitempty"`
	CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     *int `json:"cache_read_tokens,omitempty"`
}

type UsageAuditTokenDelta struct {
	InputTokens  *int `json:"input_tokens,omitempty"`
	OutputTokens *int `json:"output_tokens,omitempty"`
}

type UsageAuditDeviation struct {
	InputTokens  *float64 `json:"input_tokens,omitempty"`
	OutputTokens *float64 `json:"output_tokens,omitempty"`
}

// UsageAudit contains only token evidence and derived metadata. Request and
// response content are intentionally excluded from the persisted structure.
type UsageAudit struct {
	Version          int                      `json:"version"`
	Status           UsageAuditStatus         `json:"status"`
	Confidence       UsageAuditConfidence     `json:"confidence"`
	EvidenceSource   UsageAuditEvidenceSource `json:"evidence_source"`
	Protocol         string                   `json:"protocol"`
	Estimator        string                   `json:"estimator,omitempty"`
	Upstream         UsageAuditTokenSnapshot  `json:"upstream"`
	Local            UsageAuditTokenSnapshot  `json:"local"`
	Delta            UsageAuditTokenDelta     `json:"delta"`
	DeviationPct     UsageAuditDeviation      `json:"deviation_pct"`
	CacheHitRate     *float64                 `json:"cache_hit_rate,omitempty"`
	Reasons          []string                 `json:"reasons"`
	UpstreamRawUsage json.RawMessage          `json:"upstream_raw_usage,omitempty"`
	CaptureTruncated bool                     `json:"capture_truncated"`
}

type UsageAuditBuildInput struct {
	Protocol         string
	Model            string
	RequestBody      []byte
	ResponseBody     []byte
	CaptureTruncated bool
	Upstream         UsageAuditTokenSnapshot
}

type UsageAuditStatusCounts struct {
	Pass          int64 `json:"pass"`
	Warning       int64 `json:"warning"`
	Anomaly       int64 `json:"anomaly"`
	Unverifiable  int64 `json:"unverifiable"`
	NotApplicable int64 `json:"not_applicable"`
	Unknown       int64 `json:"unknown"`
	Missing       int64 `json:"missing"`
}

type UsageAuditDimensionStats struct {
	Key                       string                 `json:"key"`
	Name                      string                 `json:"name"`
	Total                     int64                  `json:"total"`
	Counts                    UsageAuditStatusCounts `json:"counts"`
	AverageInputDeviationPct  *float64               `json:"average_input_deviation_pct,omitempty"`
	MaximumInputDeviationPct  *float64               `json:"maximum_input_deviation_pct,omitempty"`
	AverageOutputDeviationPct *float64               `json:"average_output_deviation_pct,omitempty"`
	MaximumOutputDeviationPct *float64               `json:"maximum_output_deviation_pct,omitempty"`
}

type UsageAuditStats struct {
	TotalRecords              int64                      `json:"total_records"`
	AuditedRecords            int64                      `json:"audited_records"`
	CoveragePct               float64                    `json:"coverage_pct"`
	Counts                    UsageAuditStatusCounts     `json:"counts"`
	AverageInputDeviationPct  *float64                   `json:"average_input_deviation_pct,omitempty"`
	MaximumInputDeviationPct  *float64                   `json:"maximum_input_deviation_pct,omitempty"`
	AverageOutputDeviationPct *float64                   `json:"average_output_deviation_pct,omitempty"`
	MaximumOutputDeviationPct *float64                   `json:"maximum_output_deviation_pct,omitempty"`
	ByPlatform                []UsageAuditDimensionStats `json:"by_platform"`
	ByAccount                 []UsageAuditDimensionStats `json:"by_account"`
	ByModel                   []UsageAuditDimensionStats `json:"by_model"`
}

// UsageAuditRepository is deliberately narrow so existing billing test doubles
// do not need to implement audit-only reporting methods.
type UsageAuditRepository interface {
	GetUsageAuditStats(ctx context.Context, filters usagestats.UsageLogFilters) (*UsageAuditStats, error)
}

func BuildClaudeUsageAudit(endpoint, model string, requestBody, responseBody []byte, captureTruncated bool, usage ClaudeUsage) *UsageAudit {
	inputTokens := usage.InputTokens
	outputTokens := usage.OutputTokens
	cacheCreationTokens := usage.CacheCreationInputTokens
	cacheReadTokens := usage.CacheReadInputTokens
	totalInputTokens := inputTokens + cacheCreationTokens + cacheReadTokens
	return BuildUsageAudit(UsageAuditBuildInput{
		Protocol:         UsageAuditProtocolForEndpoint(endpoint),
		Model:            model,
		RequestBody:      requestBody,
		ResponseBody:     responseBody,
		CaptureTruncated: captureTruncated,
		Upstream: UsageAuditTokenSnapshot{
			InputTokens:         usageAuditIntPtr(inputTokens),
			TotalInputTokens:    usageAuditIntPtr(totalInputTokens),
			OutputTokens:        usageAuditIntPtr(outputTokens),
			CacheCreationTokens: usageAuditIntPtr(cacheCreationTokens),
			CacheReadTokens:     usageAuditIntPtr(cacheReadTokens),
		},
	})
}

func BuildOpenAIUsageAudit(endpoint, model string, requestBody, responseBody []byte, captureTruncated bool, usage OpenAIUsage) *UsageAudit {
	inputTokens := usage.InputTokens
	outputTokens := usage.OutputTokens
	cacheCreationTokens := usage.CacheCreationInputTokens
	cacheReadTokens := usage.CacheReadInputTokens
	return BuildUsageAudit(UsageAuditBuildInput{
		Protocol:         UsageAuditProtocolForEndpoint(endpoint),
		Model:            model,
		RequestBody:      requestBody,
		ResponseBody:     responseBody,
		CaptureTruncated: captureTruncated,
		Upstream: UsageAuditTokenSnapshot{
			InputTokens:         usageAuditIntPtr(inputTokens),
			TotalInputTokens:    usageAuditIntPtr(inputTokens),
			OutputTokens:        usageAuditIntPtr(outputTokens),
			CacheCreationTokens: usageAuditIntPtr(cacheCreationTokens),
			CacheReadTokens:     usageAuditIntPtr(cacheReadTokens),
		},
	})
}

func NewUnverifiableUsageAudit(protocol string, upstream UsageAuditTokenSnapshot, reason string) *UsageAudit {
	if strings.TrimSpace(protocol) == "" {
		protocol = UsageAuditProtocolUnknown
	}
	reasons := []string{reason}
	if strings.TrimSpace(reason) == "" {
		reasons = []string{"capture_unavailable"}
	}
	return &UsageAudit{
		Version:        UsageAuditVersion,
		Status:         UsageAuditStatusUnverifiable,
		Confidence:     UsageAuditConfidenceLow,
		EvidenceSource: UsageAuditEvidenceNormalized,
		Protocol:       protocol,
		Upstream:       upstream,
		Reasons:        reasons,
	}
}

func NewNotApplicableUsageAudit(reason string) *UsageAudit {
	if strings.TrimSpace(reason) == "" {
		reason = "non_token_billing"
	}
	return &UsageAudit{
		Version:        UsageAuditVersion,
		Status:         UsageAuditStatusNotApplicable,
		Confidence:     UsageAuditConfidenceHigh,
		EvidenceSource: UsageAuditEvidenceNormalized,
		Protocol:       UsageAuditProtocolUnknown,
		Reasons:        []string{reason},
	}
}

func UsageAuditProtocolForEndpoint(endpoint string) string {
	normalized := strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case strings.Contains(normalized, "chat/completions"):
		return UsageAuditProtocolOpenAIChat
	case strings.Contains(normalized, "responses"):
		return UsageAuditProtocolOpenAIResponses
	case strings.Contains(normalized, "messages"):
		return UsageAuditProtocolAnthropicMessages
	default:
		return UsageAuditProtocolUnknown
	}
}

func BuildUsageAudit(input UsageAuditBuildInput) *UsageAudit {
	protocol := strings.TrimSpace(input.Protocol)
	if protocol == "" {
		protocol = UsageAuditProtocolUnknown
	}

	rawUsage := extractUsageAuditEvidence(input.ResponseBody)
	evidenceSource := UsageAuditEvidenceNormalized
	if len(rawUsage) > 0 {
		evidenceSource = UsageAuditEvidenceGateway
	}

	localInput, estimator := estimateUsageAuditInput(protocol, input.Model, input.RequestBody)
	localOutput := estimateUsageAuditOutput(input.Model, input.ResponseBody, input.Upstream.OutputTokens)
	local := UsageAuditTokenSnapshot{
		InputTokens:      localInput,
		TotalInputTokens: localInput,
		OutputTokens:     localOutput,
	}

	audit := &UsageAudit{
		Version:          UsageAuditVersion,
		Confidence:       UsageAuditConfidenceMedium,
		EvidenceSource:   evidenceSource,
		Protocol:         protocol,
		Estimator:        estimator,
		Upstream:         input.Upstream,
		Local:            local,
		Reasons:          make([]string, 0, 4),
		UpstreamRawUsage: rawUsage,
		CaptureTruncated: input.CaptureTruncated,
	}

	audit.Delta.InputTokens, audit.DeviationPct.InputTokens = usageAuditDifference(input.Upstream.TotalInputTokens, localInput)
	audit.Delta.OutputTokens, audit.DeviationPct.OutputTokens = usageAuditDifference(input.Upstream.OutputTokens, localOutput)
	if input.Upstream.TotalInputTokens != nil && *input.Upstream.TotalInputTokens > 0 && input.Upstream.CacheReadTokens != nil {
		hitRate := float64(*input.Upstream.CacheReadTokens) / float64(*input.Upstream.TotalInputTokens) * 100
		audit.CacheHitRate = roundedFloatPtr(hitRate)
	}

	audit.classify()
	return audit
}

func (a *UsageAudit) classify() {
	if a == nil {
		return
	}
	if len(a.UpstreamRawUsage) == 0 {
		a.Reasons = append(a.Reasons, "raw_usage_not_captured")
	}
	if a.CaptureTruncated {
		a.Reasons = append(a.Reasons, "response_capture_truncated")
	}
	if a.Local.TotalInputTokens == nil {
		a.Reasons = append(a.Reasons, "local_input_unavailable")
	}
	if a.Local.OutputTokens == nil {
		a.Reasons = append(a.Reasons, "local_output_unavailable")
	}

	inputAnomaly := usageAuditExceeds(a.Delta.InputTokens, a.DeviationPct.InputTokens, 128, 35)
	outputAnomaly := usageAuditExceeds(a.Delta.OutputTokens, a.DeviationPct.OutputTokens, 64, 35)
	inputWarning := usageAuditExceeds(a.Delta.InputTokens, a.DeviationPct.InputTokens, 64, 15)
	outputWarning := usageAuditExceeds(a.Delta.OutputTokens, a.DeviationPct.OutputTokens, 32, 20)

	switch {
	case inputAnomaly || outputAnomaly:
		a.Status = UsageAuditStatusAnomaly
		if inputAnomaly {
			a.Reasons = append(a.Reasons, "input_token_deviation_high")
		}
		if outputAnomaly {
			a.Reasons = append(a.Reasons, "output_token_deviation_high")
		}
	case a.Local.TotalInputTokens == nil && a.Local.OutputTokens == nil:
		a.Status = UsageAuditStatusUnverifiable
	case inputWarning || outputWarning || a.CaptureTruncated || len(a.UpstreamRawUsage) == 0 || a.Local.TotalInputTokens == nil || a.Local.OutputTokens == nil:
		a.Status = UsageAuditStatusWarning
		if inputWarning {
			a.Reasons = append(a.Reasons, "input_token_deviation")
		}
		if outputWarning {
			a.Reasons = append(a.Reasons, "output_token_deviation")
		}
	default:
		a.Status = UsageAuditStatusPass
	}

	if a.Status == UsageAuditStatusUnverifiable || len(a.UpstreamRawUsage) == 0 || a.CaptureTruncated {
		a.Confidence = UsageAuditConfidenceLow
	} else if a.EvidenceSource == UsageAuditEvidenceUpstreamRaw && a.Local.TotalInputTokens != nil && a.Local.OutputTokens != nil {
		a.Confidence = UsageAuditConfidenceHigh
	} else {
		a.Confidence = UsageAuditConfidenceMedium
	}
}

func usageAuditDifference(upstream, local *int) (*int, *float64) {
	if upstream == nil || local == nil {
		return nil, nil
	}
	delta := *upstream - *local
	if *local <= 0 {
		if delta == 0 {
			zero := 0.0
			return &delta, &zero
		}
		return &delta, nil
	}
	deviation := float64(delta) / float64(*local) * 100
	return &delta, roundedFloatPtr(deviation)
}

func usageAuditExceeds(delta *int, deviation *float64, minimumDelta int, minimumPct float64) bool {
	return delta != nil && deviation != nil && absInt(*delta) >= minimumDelta && math.Abs(*deviation) >= minimumPct
}

func estimateUsageAuditInput(protocol, model string, body []byte) (*int, string) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, ""
	}
	switch protocol {
	case UsageAuditProtocolAnthropicMessages:
		if count, err := estimateAnthropicCountTokensLocally(body); err == nil {
			return usageAuditIntPtr(count), UsageAuditEstimatorAnthropicTiktoken
		}
	case UsageAuditProtocolOpenAIResponses:
		var request openAIInputTokensCountRequest
		if err := json.Unmarshal(body, &request); err == nil {
			if strings.TrimSpace(request.Model) == "" {
				request.Model = model
			}
			if count, err := estimateOpenAIInputTokens(request); err == nil {
				return usageAuditIntPtr(count), UsageAuditEstimatorResponsesTiktoken
			}
		}
	}
	count, err := estimateUsageAuditSemanticJSONTokens(model, body)
	if err != nil {
		return nil, ""
	}
	return usageAuditIntPtr(count), UsageAuditEstimatorSemanticJSON
}

func estimateUsageAuditOutput(model string, response []byte, upstreamOutput *int) *int {
	text := extractUsageAuditOutputText(response)
	if strings.TrimSpace(text) == "" {
		if upstreamOutput != nil && *upstreamOutput == 0 && len(bytes.TrimSpace(response)) > 0 {
			return usageAuditIntPtr(0)
		}
		return nil
	}
	codec, err := openAIInputTokensCodecForModel(model)
	if err != nil {
		return nil
	}
	count, err := codec.Count(text)
	if err != nil {
		return nil
	}
	return usageAuditIntPtr(count)
}

func estimateUsageAuditSemanticJSONTokens(model string, body []byte) (int, error) {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return 0, err
	}
	value = sanitizeUsageAuditSemanticValue(value, "")
	canonical, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	codec, err := openAIInputTokensCodecForModel(model)
	if err != nil {
		return 0, err
	}
	return codec.Count(string(canonical))
}

func sanitizeUsageAuditSemanticValue(value any, key string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			switch strings.ToLower(childKey) {
			case "model", "stream", "temperature", "top_p", "top_k", "seed", "max_tokens", "max_output_tokens", "timeout", "n":
				continue
			}
			out[childKey] = sanitizeUsageAuditSemanticValue(childValue, childKey)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = sanitizeUsageAuditSemanticValue(typed[i], key)
		}
		return out
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(strings.ToLower(trimmed), "data:") || (len(trimmed) > 2048 && looksLikeUsageAuditBase64(trimmed)) {
			return "[binary payload]"
		}
		return typed
	default:
		return value
	}
}

func looksLikeUsageAuditBase64(value string) bool {
	if len(value)%4 != 0 {
		return false
	}
	for _, char := range value[:minInt(len(value), 512)] {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '+' || char == '/' || char == '=' || char == '\n' || char == '\r') {
			return false
		}
	}
	return true
}

func extractUsageAuditOutputText(response []byte) string {
	if len(bytes.TrimSpace(response)) == 0 {
		return ""
	}
	if bytes.Contains(response, []byte("data:")) {
		if output := extractUsageAuditSSEOutput(response); output != "" {
			return output
		}
	}
	var root any
	if err := json.Unmarshal(response, &root); err != nil {
		return ""
	}
	var builder strings.Builder
	appendUsageAuditJSONOutput(&builder, root)
	return builder.String()
}

func extractUsageAuditSSEOutput(response []byte) string {
	var deltas strings.Builder
	var terminal strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(response))
	scanner.Buffer(make([]byte, 16*1024), 2*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var event any
		if json.Unmarshal(payload, &event) != nil {
			continue
		}
		if eventMap, ok := event.(map[string]any); ok && appendUsageAuditDelta(&deltas, eventMap) {
			continue
		}
		appendUsageAuditJSONOutput(&terminal, event)
	}
	if deltas.Len() > 0 {
		return deltas.String()
	}
	return terminal.String()
}

func appendUsageAuditDelta(builder *strings.Builder, event map[string]any) bool {
	eventType, _ := event["type"].(string)
	if delta, ok := event["delta"].(string); ok && strings.Contains(eventType, "delta") {
		appendUsageAuditText(builder, delta)
		return delta != ""
	}
	if delta, ok := event["delta"].(map[string]any); ok {
		before := builder.Len()
		for _, key := range []string{"text", "thinking", "partial_json", "content", "reasoning_content", "arguments"} {
			if text, ok := delta[key].(string); ok {
				appendUsageAuditText(builder, text)
			}
		}
		return builder.Len() > before
	}
	if choices, ok := event["choices"].([]any); ok {
		before := builder.Len()
		for _, choice := range choices {
			choiceMap, _ := choice.(map[string]any)
			delta, _ := choiceMap["delta"].(map[string]any)
			appendUsageAuditJSONOutput(builder, delta)
		}
		return builder.Len() > before
	}
	return false
}

func appendUsageAuditJSONOutput(builder *strings.Builder, value any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			appendUsageAuditJSONOutput(builder, item)
		}
	case map[string]any:
		if output, exists := typed["output"]; exists {
			appendUsageAuditJSONOutput(builder, output)
			return
		}
		if choices, exists := typed["choices"]; exists {
			appendUsageAuditJSONOutput(builder, choices)
			return
		}
		if candidates, exists := typed["candidates"]; exists {
			appendUsageAuditJSONOutput(builder, candidates)
			return
		}
		if response, exists := typed["response"]; exists {
			appendUsageAuditJSONOutput(builder, response)
			return
		}
		if message, exists := typed["message"]; exists {
			appendUsageAuditJSONOutput(builder, message)
		}
		if content, exists := typed["content"]; exists {
			switch contentTyped := content.(type) {
			case string:
				appendUsageAuditText(builder, contentTyped)
			case []any, map[string]any:
				appendUsageAuditJSONOutput(builder, contentTyped)
			}
		}
		for _, key := range []string{"output_text", "text", "thinking", "reasoning_content", "refusal", "arguments", "output", "input"} {
			if text, ok := typed[key].(string); ok {
				appendUsageAuditText(builder, text)
			}
		}
		for _, key := range []string{"parts", "tool_calls", "function_call", "delta", "data"} {
			if nested, ok := typed[key]; ok {
				appendUsageAuditJSONOutput(builder, nested)
			}
		}
	}
}

func appendUsageAuditText(builder *strings.Builder, text string) {
	if text == "" {
		return
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(text)
}

func extractUsageAuditEvidence(response []byte) json.RawMessage {
	if len(bytes.TrimSpace(response)) == 0 {
		return nil
	}
	samples := make([]any, 0, 4)
	appendSample := func(root any) {
		for _, sample := range usageAuditSamplesFromRoot(root) {
			if len(samples) >= 16 {
				break
			}
			samples = append(samples, sample)
		}
	}

	var root any
	if json.Unmarshal(response, &root) == nil {
		appendSample(root)
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(response))
		scanner.Buffer(make([]byte, 16*1024), 2*1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
				continue
			}
			var event any
			if json.Unmarshal(payload, &event) == nil {
				appendSample(event)
			}
		}
	}
	if len(samples) == 0 {
		return nil
	}
	var evidence any = samples[0]
	if len(samples) > 1 {
		evidence = map[string]any{"samples": samples}
	}
	raw, err := json.Marshal(evidence)
	if err != nil || len(raw) > usageAuditRawEvidenceMaxBytes {
		return nil
	}
	return raw
}

func usageAuditSamplesFromRoot(root any) []any {
	value, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	samples := make([]any, 0, 2)
	appendUsage := func(candidate any) {
		if object, ok := candidate.(map[string]any); ok && len(object) > 0 {
			samples = append(samples, object)
		}
	}
	appendUsage(value["usage"])
	appendUsage(value["usageMetadata"])
	for _, key := range []string{"response", "message", "data"} {
		if nested, ok := value[key].(map[string]any); ok {
			appendUsage(nested["usage"])
			appendUsage(nested["usageMetadata"])
			if response, ok := nested["response"].(map[string]any); ok {
				appendUsage(response["usage"])
			}
		}
	}
	return samples
}

func roundedFloatPtr(value float64) *float64 {
	rounded := math.Round(value*100) / 100
	return &rounded
}

func usageAuditIntPtr(value int) *int {
	return &value
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
