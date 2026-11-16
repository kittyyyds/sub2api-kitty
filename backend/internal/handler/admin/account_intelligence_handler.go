package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) StartIntelligenceTest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var in service.AccountIntelligenceInput
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "Invalid test input")
		return
	}
	result, err := h.accountTestService.StartAccountIntelligence(c.Request.Context(), id, in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, result)
}
func (h *AccountHandler) IntelligenceHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if _, err = h.adminService.GetAccount(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	result, err := h.accountTestService.AccountIntelligenceHistory(c.Request.Context(), []int64{id}, limit, offset)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}
func (h *AccountHandler) IntelligenceRecent(c *gin.Context) {
	parts := strings.Split(c.Query("ids"), ",")
	if len(parts) > 100 {
		response.BadRequest(c, "Too many accounts")
		return
	}
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, e := strconv.ParseInt(part, 10, 64)
		if e != nil || id <= 0 {
			response.BadRequest(c, "Invalid account ID")
			return
		}
		ids = append(ids, id)
	}
	result, err := h.accountTestService.AccountIntelligenceHistory(c.Request.Context(), ids, 20, 0)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}

func (h *AccountHandler) GetIntelligenceSchedule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	result, err := h.accountTestService.GetIntelligenceSchedule(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}
func (h *AccountHandler) SaveIntelligenceSchedule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var in service.AccountIntelligenceSchedule
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "Invalid schedule input")
		return
	}
	result, err := h.accountTestService.SaveIntelligenceSchedule(c.Request.Context(), id, in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}
