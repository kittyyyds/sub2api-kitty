package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Preserve the customer-facing model in billing after channel alias mapping.
func clientRequestedModel(c *gin.Context, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if c == nil || c.Request == nil {
		return fallback
	}
	if model, ok := service.RequestedPublicModelFromContext(c.Request.Context()); ok {
		return model
	}
	return fallback
}

func clientRequestedUsageFields(c *gin.Context, mapping service.ChannelMappingResult, fallbackModel, upstreamModel string) service.ChannelUsageFields {
	return mapping.ToUsageFields(clientRequestedModel(c, fallbackModel), upstreamModel)
}
