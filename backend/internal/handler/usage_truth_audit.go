package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

// Record only the usage counters already obtained during forwarding. No request
// or response body is passed into the audit path, and no response writer wrapper
// retains a second copy of streamed output.
func populateClaudeUsageAudit(result *service.ForwardResult, endpoint string) {
	if result == nil || result.UsageAudit != nil {
		return
	}
	if result.ImageCount > 0 {
		result.UsageAudit = service.NewNotApplicableUsageAudit("non_token_billing")
		return
	}
	result.UsageAudit = service.BuildClaudeUsageAudit(endpoint, result.Usage)
}

func populateOpenAIUsageAudit(result *service.OpenAIForwardResult, endpoint string) {
	if result == nil || result.UsageAudit != nil {
		return
	}
	if result.ImageCount > 0 || result.VideoCount > 0 {
		result.UsageAudit = service.NewNotApplicableUsageAudit("non_token_billing")
		return
	}
	result.UsageAudit = service.BuildOpenAIUsageAudit(endpoint, result.Usage)
}
