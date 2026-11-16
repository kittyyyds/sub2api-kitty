package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *GatewayHandler) AdminGroupAvailableModels(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "分组 ID 无效")
		return
	}
	group, err := h.gatewayService.GetGroup(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	models := h.codexModelIDsForGroup(c.Request.Context(), group, "")
	if service.IsMultiProtocolAPIKeyProvider(group.Platform) {
		models = h.gatewayService.GetAvailableModels(c.Request.Context(), &group.ID, group.Platform)
		if group.ModelAllowlistEnabled() {
			models = group.ModelAllowlist.FilterForListing(models)
		}
	}
	if models == nil {
		models = []string{}
	}
	response.Success(c, gin.H{"models": models})
}
