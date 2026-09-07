package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) CursorBridgeStatus(c *gin.Context) {
	response.Success(c, service.GetCursorBridgeStatus(c.Request.Context()))
}

func (h *AccountHandler) VerifyCursorCredential(c *gin.Context) {
	var req struct {
		APIKey    string `json:"api_key" binding:"max=600"`
		AccountID int64  `json:"account_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请填写 Cursor 用户 API Key")
		return
	}
	if req.APIKey == "" && req.AccountID > 0 {
		account, err := h.adminService.GetAccount(c.Request.Context(), req.AccountID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if !account.IsCursor() {
			response.BadRequest(c, "请选择 Cursor 账号")
			return
		}
		req.APIKey = account.GetCredential("api_key")
	}
	if err := service.VerifyCursorUserKey(c.Request.Context(), req.APIKey); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"authenticated": true, "message": "Cursor 认证通过；模型权限、额度和实际生成能力仍需在保存后测试"})
}
