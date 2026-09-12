package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountClientProfileManager interface {
	UpdateOpenAIClientProfile(context.Context, int64, map[string]any, *string) (*service.Account, error)
	GetOpenAIClientProfileEgress(context.Context, int64) (*service.OpenAIClientProfileEgress, error)
	ProbeOpenAIClientProfileEgress(context.Context, int64, *int64) (*service.OpenAIClientProfileEgress, error)
}

func (h *AccountHandler) clientProfileManager(c *gin.Context) (accountClientProfileManager, int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return nil, 0, false
	}
	m, ok := h.adminService.(accountClientProfileManager)
	if !ok {
		response.InternalError(c, "Client profile service unavailable")
		return nil, 0, false
	}
	return m, id, true
}

func (h *AccountHandler) UpdateClientProfile(c *gin.Context) {
	m, id, ok := h.clientProfileManager(c)
	if !ok {
		return
	}
	var req struct {
		Profile         map[string]any `json:"profile" binding:"required"`
		FingerprintMode *string        `json:"fingerprint_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid client profile")
		return
	}
	a, err := m.UpdateOpenAIClientProfile(c.Request.Context(), id, req.Profile, req.FingerprintMode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), a))
}

func (h *AccountHandler) GetClientProfileEgress(c *gin.Context) {
	m, id, ok := h.clientProfileManager(c)
	if !ok {
		return
	}
	result, err := m.GetOpenAIClientProfileEgress(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AccountHandler) ProbeClientProfileEgress(c *gin.Context) {
	m, id, ok := h.clientProfileManager(c)
	if !ok {
		return
	}
	var req struct {
		ProxyID *int64 `json:"proxy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid proxy selection")
		return
	}
	result, err := m.ProbeOpenAIClientProfileEgress(c.Request.Context(), id, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
