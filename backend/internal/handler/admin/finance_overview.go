package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"time"
)

func (h *ChannelHandler) FinanceOverview(c *gin.Context) {
	from, e1 := time.Parse(time.RFC3339, c.Query("from"))
	until, e2 := time.Parse(time.RFC3339, c.Query("until"))
	if e1 != nil || e2 != nil || !service.ValidateFinanceRange(from, until) {
		response.ErrorFrom(c, service.ErrUpstreamOperationsValidation)
		return
	}
	out, err := h.channelService.FinanceOverview(c.Request.Context(), from, until)
	if err != nil {
		response.InternalError(c, "Finance overview unavailable")
		return
	}
	response.Success(c, out)
}
