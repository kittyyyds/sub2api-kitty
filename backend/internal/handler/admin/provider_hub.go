package admin

import (
	"errors"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func hubResponse(c *gin.Context, out any, err error) {
	if err != nil {
		if errors.Is(err, service.ErrProviderHubInput) || errors.Is(err, service.ErrProviderHubConflict) || errors.Is(err, service.ErrProviderRemote) || errors.Is(err, service.ErrNewAPIEncryptionKey) {
			response.ErrorFrom(c, err)
		} else {
			response.ErrorFrom(c, service.ErrProviderHubUnavailable)
		}
		return
	}
	response.Success(c, out)
}
func (h *ChannelHandler) ProviderHubState(c *gin.Context) {
	out, e := h.channelService.ReadProviderHub(c.Request.Context())
	hubResponse(c, out, e)
}
func (h *ChannelHandler) ProviderHubPurchases(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	page, e1 := operationsInt(c, "page", 1)
	size, e2 := operationsInt(c, "page_size", 20)
	if e != nil || e1 != nil || e2 != nil {
		hubResponse(c, nil, service.ErrProviderHubInput)
		return
	}
	out, e := h.channelService.ProviderHubPurchases(c.Request.Context(), id, int(page), int(size))
	hubResponse(c, out, e)
}
func (h *ChannelHandler) ProviderHubMutation(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cmd service.ProviderHubCommand
		if c.ShouldBindJSON(&cmd) != nil {
			hubResponse(c, nil, service.ErrProviderHubInput)
			return
		}
		cmd.Action = action
		if raw := c.Param("id"); raw != "" {
			id, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || id <= 0 {
				hubResponse(c, nil, service.ErrProviderHubInput)
				return
			}
			cmd.ID = id
		}
		out, e := h.channelService.MutateProviderHub(c.Request.Context(), cmd)
		hubResponse(c, out, e)
	}
}
func (h *ChannelHandler) ProviderHubSync(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		hubResponse(c, nil, service.ErrProviderHubInput)
		return
	}
	e = h.channelService.SyncProviderHubWallet(c.Request.Context(), id)
	hubResponse(c, map[string]bool{"synced": e == nil}, e)
}
func (h *ChannelHandler) ProviderOperations(c *gin.Context) {
	from, e1 := time.Parse(time.RFC3339, c.Query("from"))
	until, e2 := time.Parse(time.RFC3339, c.Query("until"))
	supplier, e3 := operationsInt(c, "supplier_id", 0)
	wallet, e4 := operationsInt(c, "wallet_id", 0)
	account, e5 := operationsInt(c, "account_id", 0)
	group, e6 := operationsInt(c, "group_id", 0)
	page, e7 := operationsInt(c, "page", 1)
	size, e8 := operationsInt(c, "page_size", 20)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil || e7 != nil || e8 != nil {
		hubResponse(c, nil, service.ErrProviderHubInput)
		return
	}
	out, e := h.channelService.ProviderOperations(c.Request.Context(), service.ProviderOperationsFilter{From: from, Until: until, SupplierID: supplier, WalletID: wallet, AccountID: account, GroupID: group, Page: int(page), PageSize: int(size), Model: c.Query("model"), Dimension: c.DefaultQuery("dimension", "supplier"), Timezone: c.DefaultQuery("timezone", "UTC")})
	hubResponse(c, out, e)
}
func (h *ChannelHandler) ProviderHubKeys(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		hubResponse(c, nil, service.ErrProviderHubInput)
		return
	}
	out, e := h.channelService.ProviderHubRemoteKeys(c.Request.Context(), id)
	hubResponse(c, out, e)
}
func (h *ChannelHandler) ProviderHubGroups(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		hubResponse(c, nil, service.ErrProviderHubInput)
		return
	}
	out, e := h.channelService.ProviderHubRemoteGroups(c.Request.Context(), id)
	hubResponse(c, out, e)
}
func (h *ChannelHandler) ProviderHubCreateKey(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Name        string `json:"name"`
		Group       string `json:"group"`
		OperationID string `json:"operation_id"`
	}
	if e != nil || id <= 0 || c.ShouldBindJSON(&req) != nil {
		hubResponse(c, nil, service.ErrProviderHubInput)
		return
	}
	e = h.channelService.CreateProviderHubRemoteKey(c.Request.Context(), id, req.Name, req.Group, req.OperationID)
	hubResponse(c, map[string]bool{"created": e == nil}, e)
}
func (h *ChannelHandler) ProviderHubImportKeys(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var req service.ProviderHubImport
	if e != nil || id <= 0 || c.ShouldBindJSON(&req) != nil {
		hubResponse(c, nil, service.ErrProviderHubInput)
		return
	}
	out, e := h.channelService.ImportProviderHubKeys(c.Request.Context(), id, req, h.providerAdmin)
	hubResponse(c, map[string]any{"account_ids": out}, e)
}
