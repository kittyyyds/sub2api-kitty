package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPopulateOpenAIUsageAuditPreservesUsageAndDoesNotClaimVerification(t *testing.T) {
	usage := service.OpenAIUsage{InputTokens: 1000, OutputTokens: 20, CacheReadInputTokens: 600}
	for _, endpoint := range []string{"/v1/responses", "/v1/chat/completions", "/v1/messages"} {
		result := &service.OpenAIForwardResult{Usage: usage}
		populateOpenAIUsageAudit(result, endpoint)
		require.Equal(t, usage, result.Usage)
		require.Equal(t, service.UsageAuditProtocolForEndpoint(endpoint), result.UsageAudit.Protocol)
		require.Equal(t, service.UsageAuditStatusUnverifiable, result.UsageAudit.Status)
		require.Equal(t, []string{"local_estimate_disabled"}, result.UsageAudit.Reasons)
		require.Equal(t, service.UsageAuditTokenSnapshot{}, result.UsageAudit.Local)
		require.Equal(t, 600, *result.UsageAudit.Upstream.CacheReadTokens)
	}
}

func TestPopulateClaudeUsageAuditPreservesUsageAndDoesNotClaimVerification(t *testing.T) {
	usage := service.ClaudeUsage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 300, CacheReadInputTokens: 600}
	for _, endpoint := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		result := &service.ForwardResult{Usage: usage}
		populateClaudeUsageAudit(result, endpoint)
		require.Equal(t, usage, result.Usage)
		require.Equal(t, service.UsageAuditProtocolForEndpoint(endpoint), result.UsageAudit.Protocol)
		require.Equal(t, service.UsageAuditStatusUnverifiable, result.UsageAudit.Status)
		require.Equal(t, []string{"local_estimate_disabled"}, result.UsageAudit.Reasons)
		require.Equal(t, 1000, *result.UsageAudit.Upstream.TotalInputTokens)
	}
}

func TestPopulateUsageAuditPreservesExistingAndNonTokenBilling(t *testing.T) {
	existing := &service.UsageAudit{Version: 1, Status: service.UsageAuditStatusPass}
	openAI := &service.OpenAIForwardResult{UsageAudit: existing}
	claude := &service.ForwardResult{UsageAudit: existing}
	populateOpenAIUsageAudit(openAI, "/v1/responses")
	populateClaudeUsageAudit(claude, "/v1/messages")
	require.Same(t, existing, openAI.UsageAudit)
	require.Same(t, existing, claude.UsageAudit)
	populateOpenAIUsageAudit(nil, "")
	populateClaudeUsageAudit(nil, "")
	for _, media := range []*service.OpenAIForwardResult{{ImageCount: 1}, {VideoCount: 1}} {
		populateOpenAIUsageAudit(media, "/v1/images/generations")
		require.Equal(t, service.UsageAuditStatusNotApplicable, media.UsageAudit.Status)
	}
	image := &service.ForwardResult{ImageCount: 1}
	populateClaudeUsageAudit(image, "/v1/images/generations")
	require.Equal(t, service.UsageAuditStatusNotApplicable, image.UsageAudit.Status)
}
