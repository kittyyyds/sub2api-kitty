package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *ChannelHandler) SupplierBalances(c *gin.Context) {
	p, e1 := operationsInt(c, "page", 1)
	s, e2 := operationsInt(c, "page_size", 20)
	if e1 != nil || e2 != nil || p < 1 || p > 1000000 || s < 1 || s > 100 {
		response.ErrorFrom(c, service.ErrUpstreamOperationsValidation)
		return
	}
	out, err := h.channelService.SupplierBalances(c.Request.Context(), int(p), int(s))
	if err != nil {
		response.InternalError(c, "Supplier balance snapshot unavailable")
		return
	}
	response.Success(c, out)
}
func (h *ChannelHandler) SaveSupplierBalanceThresholds(c *gin.Context) {
	var v service.SupplierBalanceThresholds
	if c.ShouldBindJSON(&v) != nil || service.ValidateSupplierBalanceThresholds(v) != nil {
		response.ErrorFrom(c, service.ErrUpstreamOperationsValidation)
		return
	}
	if err := h.channelService.SaveSupplierBalanceThresholds(c.Request.Context(), v); err != nil {
		response.InternalError(c, "Supplier balance threshold save failed")
		return
	}
	response.Success(c, v)
}
