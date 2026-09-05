package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelCapabilityProbeRequest struct {
	ModelID   string   `json:"model_id"`
	Protocols []string `json:"protocols"`
}

// ProbeModelCapabilities runs bounded active probes against an OpenAI-compatible account.
// POST /api/v1/admin/accounts/:id/capability-probe
func (h *AccountHandler) ProbeModelCapabilities(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.accountTestService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Account test service unavailable")
		return
	}
	var req ModelCapabilityProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.accountTestService.ProbeModelCapabilities(c.Request.Context(), accountID, service.ModelCapabilityProbeInput{
		ModelID:   req.ModelID,
		Protocols: req.Protocols,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
