package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// ProbeOpenAIProxyTimezone reads a configured proxy by ID. It never accepts an
// arbitrary URL from the browser, changes an account, or returns credentials.
func (h *AccountHandler) ProbeOpenAIProxyTimezone(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("proxy_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}
	var input struct {
		Force bool `json:"force"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid detection options")
		return
	}
	proxy, err := h.adminService.GetProxy(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Proxy not found")
		return
	}
	response.Success(c, h.openAIGatewayService.ProbeOpenAIProxyTimezone(c.Request.Context(), proxy, input.Force))
}
