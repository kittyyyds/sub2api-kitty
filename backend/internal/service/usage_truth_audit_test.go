package service

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireDisabledUsageAudit(t *testing.T, audit *UsageAudit) {
	t.Helper()
	require.NotNil(t, audit)
	require.Equal(t, UsageAuditStatusUnverifiable, audit.Status)
	require.Equal(t, UsageAuditConfidenceLow, audit.Confidence)
	require.Equal(t, UsageAuditEvidenceNormalized, audit.EvidenceSource)
	require.Equal(t, []string{"local_estimate_disabled"}, audit.Reasons)
	require.Equal(t, UsageAuditTokenSnapshot{}, audit.Local)
	require.Equal(t, UsageAuditTokenDelta{}, audit.Delta)
	require.Equal(t, UsageAuditDeviation{}, audit.DeviationPct)
	require.Empty(t, audit.Estimator)
	require.Empty(t, audit.UpstreamRawUsage)
	require.False(t, audit.CaptureTruncated)
}

func TestBuildUsageAuditDoesNotParseOrEstimateBodies(t *testing.T) {
	for _, protocol := range []string{UsageAuditProtocolAnthropicMessages, UsageAuditProtocolOpenAIResponses, UsageAuditProtocolOpenAIChat, UsageAuditProtocolUnknown} {
		t.Run(protocol, func(t *testing.T) {
			upstream := UsageAuditTokenSnapshot{
				InputTokens: usageAuditIntPtr(1200), TotalInputTokens: usageAuditIntPtr(1200),
				OutputTokens: usageAuditIntPtr(900), CacheReadTokens: usageAuditIntPtr(600),
			}
			// Even valid short bodies and apparently conflicting usage must not be
			// independently tokenized, classified as an anomaly, or used for billing.
			audit := BuildUsageAudit(UsageAuditBuildInput{
				Protocol: protocol, Model: "gpt-5.4",
				RequestBody:      []byte(`{"model":"gpt-5.4","input":"short request"}`),
				ResponseBody:     []byte(`{"output_text":"short answer","usage":{"input_tokens":1,"output_tokens":1}}`),
				CaptureTruncated: true, Upstream: upstream,
			})
			requireDisabledUsageAudit(t, audit)
			require.Equal(t, protocol, audit.Protocol)
			require.Equal(t, upstream, audit.Upstream)
			require.InDelta(t, 50.0, *audit.CacheHitRate, 0.001)
		})
	}
}

func TestBuildUsageAuditLargePayloadDoesNotAddBodySizedAllocations(t *testing.T) {
	// A 25 MiB request previously allocated over 1 GiB of tokenizer work.
	// Allocation count must remain bounded independently of either body size.
	body := bytes.Repeat([]byte("large request and response "), 1024*1024)
	input := UsageAuditBuildInput{Protocol: UsageAuditProtocolOpenAIResponses, RequestBody: body, ResponseBody: body}
	allocations := testing.AllocsPerRun(10, func() {
		usageAuditPerformanceSink = BuildUsageAudit(input)
	})
	require.LessOrEqual(t, allocations, 4.0)
	requireDisabledUsageAudit(t, usageAuditPerformanceSink)
}

func TestBuildClaudeUsageAuditPreservesUpstreamCacheAccounting(t *testing.T) {
	usage := ClaudeUsage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 300, CacheReadInputTokens: 600}
	audit := BuildClaudeUsageAudit("/v1/messages", usage)
	requireDisabledUsageAudit(t, audit)
	require.Equal(t, 100, *audit.Upstream.InputTokens)
	require.Equal(t, 1000, *audit.Upstream.TotalInputTokens)
	require.Equal(t, 20, *audit.Upstream.OutputTokens)
	require.Equal(t, 300, *audit.Upstream.CacheCreationTokens)
	require.Equal(t, 600, *audit.Upstream.CacheReadTokens)
	require.InDelta(t, 60.0, *audit.CacheHitRate, 0.001)
}

func TestBuildOpenAIUsageAuditPreservesUpstreamCacheAccounting(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 20, CacheCreationInputTokens: 300, CacheReadInputTokens: 600}
	audit := BuildOpenAIUsageAudit("/v1/responses", usage)
	requireDisabledUsageAudit(t, audit)
	// OpenAI input already includes cached input; do not add it a second time.
	require.Equal(t, 1000, *audit.Upstream.InputTokens)
	require.Equal(t, 1000, *audit.Upstream.TotalInputTokens)
	require.Equal(t, 20, *audit.Upstream.OutputTokens)
	require.Equal(t, 300, *audit.Upstream.CacheCreationTokens)
	require.Equal(t, 600, *audit.Upstream.CacheReadTokens)
	require.InDelta(t, 60.0, *audit.CacheHitRate, 0.001)
}

func TestBuildUsageAuditZeroOrMissingUsageIsNotVerified(t *testing.T) {
	for _, upstream := range []UsageAuditTokenSnapshot{{}, {TotalInputTokens: usageAuditIntPtr(0), CacheReadTokens: usageAuditIntPtr(0)}} {
		audit := BuildUsageAudit(UsageAuditBuildInput{Upstream: upstream})
		requireDisabledUsageAudit(t, audit)
		require.Nil(t, audit.CacheHitRate)
		require.Equal(t, UsageAuditProtocolUnknown, audit.Protocol)
	}
}

func TestUsageAuditHistoricalEvidenceRoundTrips(t *testing.T) {
	stored := `{"version":1,"status":"anomaly","confidence":"medium","evidence_source":"gateway_response","protocol":"openai_responses","estimator":"responses_tiktoken_v1","upstream":{"input_tokens":1000,"total_input_tokens":1000,"output_tokens":100},"local":{"input_tokens":20,"total_input_tokens":20,"output_tokens":10},"delta":{"input_tokens":980,"output_tokens":90},"deviation_pct":{"input_tokens":4900,"output_tokens":900},"cache_hit_rate":50,"reasons":["input_token_deviation_high"],"upstream_raw_usage":{"input_tokens":1000,"output_tokens":100},"capture_truncated":false}`
	var historical UsageAudit
	require.NoError(t, json.Unmarshal([]byte(stored), &historical))
	reencoded, err := json.Marshal(historical)
	require.NoError(t, err)
	require.JSONEq(t, stored, string(reencoded))
}

func TestEnsureUsageAuditPreservesBillingUsageAndMedia(t *testing.T) {
	openAIUsage := OpenAIUsage{InputTokens: 1000, OutputTokens: 20, CacheReadInputTokens: 600}
	for _, websocket := range []bool{false, true} {
		result := &OpenAIForwardResult{Usage: openAIUsage, OpenAIWSMode: websocket}
		ensureOpenAIUsageAudit(result, "/v1/responses")
		requireDisabledUsageAudit(t, result.UsageAudit)
		require.Equal(t, openAIUsage, result.Usage)
	}
	claudeUsage := ClaudeUsage{InputTokens: 100, OutputTokens: 20, CacheReadInputTokens: 600}
	result := &ForwardResult{Usage: claudeUsage}
	ensureGatewayUsageAudit(result, "/v1/messages")
	requireDisabledUsageAudit(t, result.UsageAudit)
	require.Equal(t, claudeUsage, result.Usage)
	for _, media := range []*OpenAIForwardResult{{ImageCount: 1}, {VideoCount: 1}} {
		ensureOpenAIUsageAudit(media, "/v1/images/generations")
		require.Equal(t, UsageAuditStatusNotApplicable, media.UsageAudit.Status)
	}
	image := &ForwardResult{ImageCount: 1}
	ensureGatewayUsageAudit(image, "/v1/images/generations")
	require.Equal(t, UsageAuditStatusNotApplicable, image.UsageAudit.Status)
}

func TestEnsureUsageAuditPreservesExistingAudit(t *testing.T) {
	existing := &UsageAudit{Version: 1, Status: UsageAuditStatusPass}
	openAI := &OpenAIForwardResult{UsageAudit: existing}
	claude := &ForwardResult{UsageAudit: existing}
	ensureOpenAIUsageAudit(openAI, "/v1/responses")
	ensureGatewayUsageAudit(claude, "/v1/messages")
	require.Same(t, existing, openAI.UsageAudit)
	require.Same(t, existing, claude.UsageAudit)
	ensureOpenAIUsageAudit(nil, "")
	ensureGatewayUsageAudit(nil, "")
}

func TestUsageAuditProtocolForEndpoint(t *testing.T) {
	require.Equal(t, UsageAuditProtocolAnthropicMessages, UsageAuditProtocolForEndpoint("/v1/messages"))
	require.Equal(t, UsageAuditProtocolOpenAIResponses, UsageAuditProtocolForEndpoint("/v1/responses"))
	require.Equal(t, UsageAuditProtocolOpenAIChat, UsageAuditProtocolForEndpoint("/v1/chat/completions"))
	require.Equal(t, UsageAuditProtocolUnknown, UsageAuditProtocolForEndpoint("/v1/embeddings"))
}
