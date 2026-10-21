package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *ProxyHandler) BindingPolicy(c *gin.Context) {
	svc, ok := h.adminService.(service.ProxyBindingAdmin)
	if !ok {
		response.BadRequest(c, "IP 规则未就绪")
		return
	}
	p, e := svc.GetProxyBindingPolicy(c.Request.Context())
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, p)
}
func (h *ProxyHandler) SaveBindingPolicy(c *gin.Context) {
	svc, ok := h.adminService.(service.ProxyBindingAdmin)
	if !ok {
		response.BadRequest(c, "IP 规则未就绪")
		return
	}
	var p service.ProxyBindingPolicy
	if c.ShouldBindJSON(&p) != nil {
		response.BadRequest(c, "配置格式无效")
		return
	}
	if e := svc.SaveProxyBindingPolicy(c.Request.Context(), p); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	response.Success(c, p)
}
func (h *ProxyHandler) BindingSummaries(c *gin.Context) {
	svc, ok := h.adminService.(service.ProxyBindingAdmin)
	if !ok {
		response.BadRequest(c, "IP 规则未就绪")
		return
	}
	rows, e := svc.ProxyBindingSummaries(c.Request.Context())
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, rows)
}
func (h *ProxyHandler) AllowBinding(c *gin.Context) {
	svc, ok := h.adminService.(service.ProxyBindingAdmin)
	if !ok {
		response.BadRequest(c, "IP 规则未就绪")
		return
	}
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		response.BadRequest(c, "代理 ID 无效")
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if c.ShouldBindJSON(&req) != nil || !req.Confirm {
		response.BadRequest(c, "请确认历史已达上限，仍额外放行一个新账号")
		return
	}
	if e = svc.AllowOneProxyBinding(c.Request.Context(), id); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	response.Success(c, gin.H{"allowed": 1})
}

func (h *ProxyHandler) ReserveOAuthImport(c *gin.Context) {
	svc, ok := h.adminService.(service.AccountImportCoordinator)
	if !ok {
		response.BadRequest(c, "导入协调服务未就绪")
		return
	}
	var req struct {
		Platform string `json:"platform"`
		ProxyID  *int64 `json:"proxy_id"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "参数无效")
		return
	}
	switch req.Platform {
	case service.PlatformOpenAI, service.PlatformAnthropic, service.PlatformGemini, service.PlatformAntigravity, service.PlatformGrok:
	default:
		response.BadRequest(c, "不支持的 OAuth 平台")
		return
	}
	ticket, e := svc.PrepareAccountImport(c.Request.Context(), req.Platform, "oauth", req.ProxyID, "")
	if e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	response.Success(c, ticket)
}
func (h *ProxyHandler) ReleaseOAuthImport(c *gin.Context) {
	svc, ok := h.adminService.(service.AccountImportCoordinator)
	if !ok {
		response.BadRequest(c, "导入协调服务未就绪")
		return
	}
	if e := svc.CancelAccountImport(c.Request.Context(), c.Param("ticket")); e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, gin.H{"released": true})
}
