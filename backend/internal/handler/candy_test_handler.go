package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type candyTestGroupAuthorizer interface {
	GetAvailableGroups(ctx context.Context, userID int64) ([]service.Group, error)
}

type CandyTestHandler struct {
	svc    *service.CandyTestService
	groups candyTestGroupAuthorizer
}

func NewCandyTestHandler(svc *service.CandyTestService, keys *service.APIKeyService) *CandyTestHandler {
	return &CandyTestHandler{svc: svc, groups: keys}
}

func (h *CandyTestHandler) allowedGroups(c *gin.Context) (map[int64]bool, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "请先登录")
		return nil, false
	}
	if h.groups == nil {
		response.Error(c, http.StatusInternalServerError, "分组权限服务不可用")
		return nil, false
	}
	groups, err := h.groups.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	allowed := make(map[int64]bool, len(groups))
	for _, g := range groups {
		allowed[g.ID] = true
	}
	return allowed, true
}

func (h *CandyTestHandler) List(c *gin.Context)         { h.list(c, false) }
func (h *CandyTestHandler) AdminList(c *gin.Context)    { h.list(c, true) }
func (h *CandyTestHandler) History(c *gin.Context)      { h.history(c, false) }
func (h *CandyTestHandler) AdminHistory(c *gin.Context) { h.history(c, true) }
func (h *CandyTestHandler) history(c *gin.Context, admin bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "测试编号无效")
		return
	}
	c.Header("Cache-Control", "no-store")
	if !admin {
		allowed, ok := h.allowedGroups(c)
		if !ok {
			return
		}
		groupID, err := h.svc.PublishedGroupID(c.Request.Context(), id)
		if err != nil || !allowed[groupID] {
			response.NotFound(c, "测试不存在或未公开")
			return
		}
	}
	items, err := h.svc.History(c.Request.Context(), id, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
func (h *CandyTestHandler) list(c *gin.Context, admin bool) {
	c.Header("Cache-Control", "no-store")
	var allowed map[int64]bool
	if !admin {
		var ok bool
		allowed, ok = h.allowedGroups(c)
		if !ok {
			return
		}
	}
	cards, err := h.svc.List(c.Request.Context(), admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !admin {
		visible := make([]service.CandyTestCard, 0, len(cards))
		for _, card := range cards {
			if allowed[card.GroupID] {
				visible = append(visible, card)
			}
		}
		cards = visible
	}
	response.Success(c, gin.H{"cards": cards, "updated_at": time.Now().UTC()})
}
func (h *CandyTestHandler) Keys(c *gin.Context) {
	items, err := h.svc.KeyOptions(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
func (h *CandyTestHandler) Save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input service.CandyTestConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "配置格式无效或超过大小限制")
		return
	}
	saved, err := h.svc.Save(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, saved)
}
func (h *CandyTestHandler) Run(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "测试编号无效")
		return
	}
	runID, err := h.svc.RunNow(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"run_id": runID})
}
