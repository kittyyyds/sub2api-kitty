package service

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func failedOpenAINonStreamingResult(usage *OpenAIUsage, responseID string) *openaiNonStreamingResult {
	if !openAIUsageHasTokens(usage) {
		return nil
	}
	return &openaiNonStreamingResult{OpenAIUsage: usage, usage: usage, responseID: responseID}
}

// A failed turn may settle only usage actually reported upstream. It cannot
// publish response affinity or a replay checkpoint, or clear account failures.
func newOpenAIFailedHTTPResult(c *gin.Context, resp *http.Response, usage *OpenAIUsage, responseID, model, upstreamModel string, stream bool, started time.Time) *OpenAIForwardResult {
	if !openAIUsageHasTokens(usage) || GetOpsCyberPolicy(c) != nil {
		return nil
	}
	result := &OpenAIForwardResult{
		RequestID: resp.Header.Get("x-request-id"), UpstreamHeaders: resp.Header,
		ResponseID: responseID, Usage: *usage, Model: model, UpstreamModel: upstreamModel,
		UpstreamResponseModel: observedUpstreamResponseModel(c), UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c),
		Stream:                      stream, Duration: time.Since(started), upstreamFailed: true,
	}
	stampOpenAIResponsesUpstreamEndpoint(c, result)
	return result
}
