package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func passwordLoginOwner(c *gin.Context) (int64, bool) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Administrator authentication required")
		return 0, false
	}
	return subject.UserID, true
}

func (h *OpenAIOAuthHandler) PasswordLoginCapabilities(c *gin.Context) {
	if _, ok := passwordLoginOwner(c); !ok {
		return
	}
	response.Success(c, gin.H{"available": h.openaiOAuthService.PasswordLoginAvailable(c.Request.Context())})
}
func (h *OpenAIOAuthHandler) StartPasswordLogin(c *gin.Context) {
	owner, ok := passwordLoginOwner(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var input service.OpenAIPasswordLoginInput
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "Invalid login input")
		return
	}
	job, err := h.openaiOAuthService.StartPasswordLogin(c.Request.Context(), owner, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, job)
}
func (h *OpenAIOAuthHandler) PasswordLoginStatus(c *gin.Context) { h.passwordLoginAction(c, "status") }
func (h *OpenAIOAuthHandler) CancelPasswordLogin(c *gin.Context) { h.passwordLoginAction(c, "cancel") }
func (h *OpenAIOAuthHandler) SubmitPasswordLoginCode(c *gin.Context) {
	h.passwordLoginAction(c, "code")
}
func (h *OpenAIOAuthHandler) passwordLoginAction(c *gin.Context, action string) {
	owner, ok := passwordLoginOwner(c)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if action == "code" {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		if c.ShouldBindJSON(&input) != nil {
			response.BadRequest(c, "Invalid verification code")
			return
		}
	}
	job, err := h.openaiOAuthService.PasswordLoginAction(c.Request.Context(), owner, c.Param("jobID"), action, input.Code)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}
func (h *OpenAIOAuthHandler) ExchangePasswordLogin(c *gin.Context) {
	owner, ok := passwordLoginOwner(c)
	if !ok {
		return
	}
	result, err := h.openaiOAuthService.TakePasswordLoginResult(c.Request.Context(), owner, c.Param("jobID"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
