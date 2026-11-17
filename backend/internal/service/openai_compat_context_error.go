package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const OpenAIContextLengthExceededCode = "context_length_exceeded"

func WriteChatCompletionsContextLengthExceeded(c *gin.Context, message string) {
	if strings.TrimSpace(message) == "" {
		message = OpenAIContextLengthExceededMessage
	}
	MarkResponseCommitted(c)
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "code": OpenAIContextLengthExceededCode, "message": message}})
}

func AnthropicPromptTooLongMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		message = OpenAIContextLengthExceededMessage
	}
	if strings.HasPrefix(strings.ToLower(message), "prompt is too long") {
		return message
	}
	return "prompt is too long: " + message
}
