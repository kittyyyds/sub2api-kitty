package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *OpenAIOAuthHandler) savedLoginAccount(c *gin.Context) (*service.Account, int64, bool) {
	c.Header("Cache-Control", "no-store")
	owner, ok := passwordLoginOwner(c)
	if !ok {
		return nil, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return nil, 0, false
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, 0, false
	}
	return account, owner, true
}
func (h *OpenAIOAuthHandler) SavedPasswordLogin(c *gin.Context) {
	account, _, ok := h.savedLoginAccount(c)
	if !ok {
		return
	}
	summary, err := h.openaiOAuthService.SavedPasswordLogin(account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"saved": summary})
}
func (h *OpenAIOAuthHandler) RememberPasswordLogin(c *gin.Context) {
	account, owner, ok := h.savedLoginAccount(c)
	if !ok {
		return
	}
	var input struct {
		JobID string `json:"job_id"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "Invalid login task")
		return
	}
	if err := h.openaiOAuthService.RememberPasswordLogin(owner, input.JobID, account); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}
func (h *OpenAIOAuthHandler) ForgetPasswordLogin(c *gin.Context) {
	account, _, ok := h.savedLoginAccount(c)
	if !ok {
		return
	}
	if err := h.openaiOAuthService.ForgetPasswordLogin(account); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"saved": false})
}
func (h *OpenAIOAuthHandler) StartSavedPasswordLogin(c *gin.Context) {
	account, owner, ok := h.savedLoginAccount(c)
	if !ok {
		return
	}
	job, err := h.openaiOAuthService.StartSavedPasswordLogin(c.Request.Context(), owner, account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, job)
}
