package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageAuditPassesCloseGatewayEvidence(t *testing.T) {
	request := []byte(`{"model":"gpt-5.4","input":"Explain bounded queues briefly."}`)
	local, _ := estimateUsageAuditInput(UsageAuditProtocolOpenAIResponses, "gpt-5.4", request)
	require.NotNil(t, local)
	response := []byte(`{"id":"resp_1","output":[{"type":"message","content":[{"type":"output_text","text":"A bounded queue limits retained work."}]}],"usage":{"input_tokens":9,"output_tokens":8,"input_tokens_details":{"cached_tokens":0}}}`)
	output := estimateUsageAuditOutput("gpt-5.4", response, intPtr(8))
	require.NotNil(t, output)

	audit := BuildUsageAudit(UsageAuditBuildInput{
		Protocol:     UsageAuditProtocolOpenAIResponses,
		Model:        "gpt-5.4",
		RequestBody:  request,
		ResponseBody: response,
		Upstream: UsageAuditTokenSnapshot{
			InputTokens:      usageAuditIntPtr(*local),
			TotalInputTokens: usageAuditIntPtr(*local),
			OutputTokens:     usageAuditIntPtr(*output),
		},
	})

	require.Equal(t, UsageAuditStatusPass, audit.Status)
	require.Equal(t, UsageAuditEvidenceGateway, audit.EvidenceSource)
	require.Equal(t, UsageAuditConfidenceMedium, audit.Confidence)
	require.JSONEq(t, `{"input_tokens":9,"output_tokens":8,"input_tokens_details":{"cached_tokens":0}}`, string(audit.UpstreamRawUsage))
}

func TestBuildUsageAuditFlagsLargeTokenDeviation(t *testing.T) {
	request := []byte(`{"model":"gpt-5.4","input":"short request"}`)
	response := []byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"short answer"}]}],"usage":{"input_tokens":1200,"output_tokens":900}}`)

	audit := BuildUsageAudit(UsageAuditBuildInput{
		Protocol:     UsageAuditProtocolOpenAIResponses,
		Model:        "gpt-5.4",
		RequestBody:  request,
		ResponseBody: response,
		Upstream: UsageAuditTokenSnapshot{
			InputTokens:      usageAuditIntPtr(1200),
			TotalInputTokens: usageAuditIntPtr(1200),
			OutputTokens:     usageAuditIntPtr(900),
		},
	})

	require.Equal(t, UsageAuditStatusAnomaly, audit.Status)
	require.Contains(t, audit.Reasons, "input_token_deviation_high")
	require.Contains(t, audit.Reasons, "output_token_deviation_high")
}

func TestBuildUsageAuditExtractsSSEUsageWithoutPersistingOutput(t *testing.T) {
	request := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`)
	response := []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0}}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello back\"}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n")

	audit := BuildClaudeUsageAudit("/v1/messages", "claude-sonnet-4-6", request, response, false, ClaudeUsage{InputTokens: 5, OutputTokens: 3})

	require.NotEmpty(t, audit.UpstreamRawUsage)
	require.NotContains(t, string(audit.UpstreamRawUsage), "hello back")
	var evidence map[string]any
	require.NoError(t, json.Unmarshal(audit.UpstreamRawUsage, &evidence))
	require.Contains(t, evidence, "samples")
	require.NotNil(t, audit.Local.OutputTokens)
}

func TestBuildUsageAuditMarksTruncatedCaptureLowConfidence(t *testing.T) {
	audit := BuildUsageAudit(UsageAuditBuildInput{
		Protocol:         UsageAuditProtocolOpenAIChat,
		Model:            "gpt-5.4",
		RequestBody:      []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}]}`),
		ResponseBody:     []byte(`{"usage":{"prompt_tokens":8,"completion_tokens":2}}`),
		CaptureTruncated: true,
		Upstream: UsageAuditTokenSnapshot{
			InputTokens:      usageAuditIntPtr(8),
			TotalInputTokens: usageAuditIntPtr(8),
			OutputTokens:     usageAuditIntPtr(2),
		},
	})

	require.Equal(t, UsageAuditStatusWarning, audit.Status)
	require.Equal(t, UsageAuditConfidenceLow, audit.Confidence)
	require.Contains(t, audit.Reasons, "response_capture_truncated")
}

func TestUsageAuditProtocolForEndpoint(t *testing.T) {
	require.Equal(t, UsageAuditProtocolAnthropicMessages, UsageAuditProtocolForEndpoint("/v1/messages"))
	require.Equal(t, UsageAuditProtocolOpenAIResponses, UsageAuditProtocolForEndpoint("/v1/responses"))
	require.Equal(t, UsageAuditProtocolOpenAIChat, UsageAuditProtocolForEndpoint("/v1/chat/completions"))
	require.Equal(t, UsageAuditProtocolUnknown, UsageAuditProtocolForEndpoint("/v1/embeddings"))
}
