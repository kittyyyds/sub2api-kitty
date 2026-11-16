package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *ChannelHandler) PricingAudit(c *gin.Context) {
	report, err := h.channelService.AuditPricing(c.Request.Context(), h.billingService)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, report)
}
