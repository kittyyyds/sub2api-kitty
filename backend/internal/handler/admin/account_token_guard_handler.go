package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AccountTokenGuardHandler 提供智能运维 → 凭证守护的状态、配置、日志与手动操作接口。
type AccountTokenGuardHandler struct {
	svc    *service.AccountTokenGuardService
	reauth *service.OpenAIOAuthReauthService
	prober service.ProxyExitInfoProber
}

func NewAccountTokenGuardHandler(svc *service.AccountTokenGuardService, reauth *service.OpenAIOAuthReauthService, prober service.ProxyExitInfoProber) *AccountTokenGuardHandler {
	return &AccountTokenGuardHandler{svc: svc, reauth: reauth, prober: prober}
}

func (h *AccountTokenGuardHandler) Status(c *gin.Context) {
	status, err := h.svc.Status(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "凭证守护状态读取失败: "+err.Error())
		return
	}
	response.Success(c, status)
}

func (h *AccountTokenGuardHandler) SaveConfig(c *gin.Context) {
	var cfg service.AccountTokenGuardConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "配置格式不正确")
		return
	}
	if err := service.ValidateAccountTokenGuardConfig(cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	saved, err := h.svc.SaveConfig(c.Request.Context(), cfg)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "配置保存失败: "+err.Error())
		return
	}
	response.Success(c, saved)
}

func (h *AccountTokenGuardHandler) Run(c *gin.Context) {
	stats, err := h.svc.RunCycle(c.Request.Context(), true)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "巡检未完成: "+err.Error())
		return
	}
	response.Success(c, stats)
}

// StartRun 接受后台巡检并立即返回。保留 Run 以兼容旧客户端。
func (h *AccountTokenGuardHandler) StartRun(c *gin.Context) {
	job, err := h.svc.StartRun(true)
	if err != nil && job == nil {
		response.Error(c, http.StatusServiceUnavailable, "巡检任务创建失败: "+err.Error())
		return
	}
	response.Accepted(c, job)
}

func (h *AccountTokenGuardHandler) Job(c *gin.Context) {
	job, ok := h.svc.Job(c.Param("id"))
	if !ok {
		response.NotFound(c, "巡检任务不存在")
		return
	}
	response.Success(c, job)
}

func (h *AccountTokenGuardHandler) Cancel(c *gin.Context) {
	job, err := h.svc.CancelRun(c.Param("id"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, job)
}

func (h *AccountTokenGuardHandler) Events(c *gin.Context) {
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		response.BadRequest(c, "offset 不合法")
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit < 1 || limit > 200 {
		response.BadRequest(c, "limit 不合法")
		return
	}
	status, err := h.svc.Status(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "日志读取失败")
		return
	}
	events := status.Events
	if offset >= len(events) {
		events = nil
	} else {
		events = events[offset:]
	}
	if len(events) > limit {
		events = events[:limit]
	}
	response.Success(c, gin.H{"items": events, "has_more": len(status.Events) > offset+len(events)})
}

func (h *AccountTokenGuardHandler) Relogin(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "账号 ID 不合法")
		return
	}
	action, err := h.svc.ReloginAccount(c.Request.Context(), accountID)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "重登失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"account_id": accountID, "action": action})
}

func (h *AccountTokenGuardHandler) StartTwoFALogin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input struct {
		service.AccountTokenGuardReloginAccount
		CredentialTarget string                         `json:"credential_target"`
		ProxyID          *int64                         `json:"proxy_id"`
		LoginEnvironment *service.TwoFALoginEnvironment `json:"login_environment"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "登录凭据格式不正确")
		return
	}
	entry := input.AccountTokenGuardReloginAccount
	if err := service.ValidateOpenAITwoFALogin(entry); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var job *service.OpenAITwoFALoginJob
	var err error
	switch input.CredentialTarget {
	case "operations":
		status, statusErr := h.reauth.CredentialEncryptionStatus()
		if statusErr != nil {
			response.ErrorFrom(c, statusErr)
			return
		}
		if !status.Configured {
			response.BadRequest(c, "请先在凭证运营中启用凭据加密，再进行 2FA 导入")
			return
		}
		var ticket *service.AccountImportTicket
		if input.CredentialTarget == "operations" {
			var prepareErr error
			ticket, prepareErr = h.svc.PrepareTwoFAImport(c.Request.Context(), entry, input.ProxyID)
			if prepareErr != nil {
				response.BadRequest(c, prepareErr.Error())
				return
			}
			input.ProxyID = ticket.ProxyID
			c.Request = c.Request.WithContext(service.WithAccountImportTicket(c.Request.Context(), ticket))
			defer func() {
				if c.Writer.Status() >= 400 {
					h.svc.CancelImportTicket(ticket.ID)
				}
			}()
		}
		if input.ProxyID != nil {
			login, prepareErr := h.reauth.TwoFAImportLogin(c.Request.Context(), entry, *input.ProxyID, input.LoginEnvironment, h.prober)
			if prepareErr != nil {
				response.BadRequest(c, prepareErr.Error())
				return
			}
			job, err = h.svc.StartTwoFAProxyLogin(c.Request.Context(), entry, login)
		} else {
			job, err = h.svc.StartTwoFALoginForOperations(c.Request.Context(), entry)
		}
	case "", "guard": // Preserve the contract for older clients.
		if input.ProxyID != nil {
			response.BadRequest(c, "代理导入必须使用加密凭证运营模式")
			return
		}
		job, err = h.svc.StartTwoFALogin(c.Request.Context(), entry)
	default:
		response.BadRequest(c, "登录凭据目标不合法")
		return
	}
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	response.Accepted(c, job)
}

func (h *AccountTokenGuardHandler) TwoFALogin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	job, ok := h.svc.TwoFALogin(c.Param("id"))
	if !ok {
		response.NotFound(c, "登录任务不存在或已过期")
		return
	}
	response.Success(c, job)
}

func (h *AccountTokenGuardHandler) DeleteTwoFALogin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	h.svc.DeleteTwoFALogin(c.Param("id"))
	response.Success(c, gin.H{"deleted": true})
}

func (h *AccountTokenGuardHandler) TwoFABrowserFrame(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")
	c.Header("X-Content-Type-Options", "nosniff")
	raw, err := h.svc.TwoFABrowserAction(c.Request.Context(), c.Param("id"), "frame", nil)
	if err != nil {
		response.Error(c, http.StatusConflict, "登录窗口尚未就绪或已结束")
		return
	}
	c.Data(http.StatusOK, "image/jpeg", raw)
}
func (h *AccountTokenGuardHandler) TwoFABrowserInput(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var input struct {
		Type  string  `json:"type"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		Text  string  `json:"text"`
		Key   string  `json:"key"`
		Delta float64 `json:"delta"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "操作参数不正确")
		return
	}
	if _, err := h.svc.TwoFABrowserAction(c.Request.Context(), c.Param("id"), "input", input); err != nil {
		response.Error(c, http.StatusConflict, "登录窗口操作未完成，请刷新窗口")
		return
	}
	response.Success(c, map[string]bool{"accepted": true})
}
func (h *AccountTokenGuardHandler) TwoFABrowserResume(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, err := h.svc.TwoFABrowserAction(c.Request.Context(), c.Param("id"), "resume", nil); err != nil {
		response.Error(c, http.StatusConflict, "登录窗口尚未就绪或已结束")
		return
	}
	response.Success(c, map[string]bool{"accepted": true})
}
