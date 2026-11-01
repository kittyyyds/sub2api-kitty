package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *AccountHandler) StartBPSImport(c *gin.Context) {
	if !bpsImportAdminSession(c) {
		return
	}
	if h.openaiOAuthReauth == nil {
		response.Error(c, http.StatusServiceUnavailable, "BPS authorization service unavailable")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var in service.BPSImportInput
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "请检查批量授权输入格式，每批最多 20 个账号")
		return
	}
	admin, ok := h.adminService.(service.BPSImportAdmin)
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "BPS import service unavailable")
		return
	}
	result, err := h.openaiOAuthReauth.StartBPSImport(c.Request.Context(), adminActorScope(c), in, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
func (h *AccountHandler) ListBPSImports(c *gin.Context) {
	if !bpsImportAdminSession(c) {
		return
	}
	if h.openaiOAuthReauth == nil {
		response.Error(c, http.StatusServiceUnavailable, "BPS authorization service unavailable")
		return
	}
	response.Success(c, h.openaiOAuthReauth.BPSImports(adminActorScope(c)))
}
func (h *AccountHandler) GetBPSImport(c *gin.Context)    { h.bpsImportStatus(c, false) }
func (h *AccountHandler) CancelBPSImport(c *gin.Context) { h.bpsImportStatus(c, true) }
func (h *AccountHandler) bpsImportStatus(c *gin.Context, cancel bool) {
	if !bpsImportAdminSession(c) {
		return
	}
	if h.openaiOAuthReauth == nil {
		response.Error(c, http.StatusServiceUnavailable, "BPS authorization service unavailable")
		return
	}
	result, err := h.openaiOAuthReauth.BPSImport(adminActorScope(c), c.Param("batch_id"), cancel)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func bpsImportAdminSession(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if _, observer := service.ObserverGroupIDs(c.Request.Context()); observer {
		response.Error(c, http.StatusForbidden, "批量 BPS 授权仅限完整管理员")
		return false
	}
	if adminActorScope(c) == "admin:0" {
		response.Error(c, http.StatusForbidden, "请使用管理员登录会话")
		return false
	}
	return true
}
