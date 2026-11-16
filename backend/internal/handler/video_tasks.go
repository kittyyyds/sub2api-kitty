package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) ListMyVideoTasks(c *gin.Context)    { h.listVideoTasks(c, false) }
func (h *OpenAIGatewayHandler) ListAdminVideoTasks(c *gin.Context) { h.listVideoTasks(c, true) }
func (h *OpenAIGatewayHandler) MyVideoTaskStats(c *gin.Context)    { h.videoTaskStats(c, false) }
func (h *OpenAIGatewayHandler) AdminVideoTaskStats(c *gin.Context) { h.videoTaskStats(c, true) }

func videoTaskFilter(c *gin.Context, admin bool, now time.Time) (service.VideoTaskListFilter, bool) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "请先登录")
		return service.VideoTaskListFilter{}, false
	}
	if admin {
		role, ok := middleware.GetUserRoleFromContext(c)
		if !ok || !service.IsAdminRole(role) {
			response.Forbidden(c, "需要管理员权限")
			return service.VideoTaskListFilter{}, false
		}
	}
	f := service.VideoTaskListFilter{UserID: subject.UserID, Admin: admin, Status: c.Query("status"), Model: strings.TrimSpace(c.Query("model")), EndAt: now.UTC()}
	if admin {
		f.UserID = 0
	}
	type idField struct {
		key, label string
		target     *int64
	}
	ids := []idField{{"api_key_id", "密钥", &f.APIKeyID}, {"group_id", "分组", &f.GroupID}}
	if admin {
		ids = append(ids, idField{"user_id", "用户", &f.UserID})
	}
	for _, field := range ids {
		if raw := c.Query(field.key); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id < 1 {
				response.BadRequest(c, field.label+"编号无效")
				return f, false
			}
			*field.target = id
		}
	}
	if raw := c.Query("end_at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "结束时间须包含时区")
			return f, false
		}
		f.EndAt = parsed
	}
	f.StartAt = f.EndAt.Add(-30 * 24 * time.Hour)
	if raw := c.Query("start_at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "开始时间须包含时区")
			return f, false
		}
		f.StartAt = parsed
	}
	if err := f.ValidateScope(); err != nil {
		response.BadRequest(c, "筛选无效：日期范围应在 93 天以内，模型名最多 200 字节，状态须为有效任务状态")
		return f, false
	}
	return f, true
}

func videoTaskPagination(c *gin.Context) (page, size int, err error) {
	page, size = 1, 20
	for _, field := range []struct {
		key    string
		target *int
	}{{"page", &page}, {"page_size", &size}} {
		if raw := c.Query(field.key); raw != "" {
			value, parseErr := strconv.Atoi(raw)
			if parseErr != nil || value < 1 {
				return 0, 0, fmt.Errorf("invalid pagination")
			}
			*field.target = value
		}
	}
	if size > 100 || page > 10000/size+1 {
		return 0, 0, fmt.Errorf("pagination out of range")
	}
	return page, size, nil
}

func (h *OpenAIGatewayHandler) listVideoTasks(c *gin.Context, admin bool) {
	filter, ok := videoTaskFilter(c, admin, time.Now())
	if !ok {
		return
	}
	page, size, err := videoTaskPagination(c)
	if err != nil {
		response.BadRequest(c, "页码或每页数量无效，请缩小日期范围后重试")
		return
	}
	filter.Limit, filter.Offset = size, (page-1)*size
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	items, err := h.gatewayService.ListVideoTaskSummaries(ctx, filter)
	if err != nil {
		response.InternalError(c, "读取视频任务失败，请稍后重试")
		return
	}
	response.Success(c, gin.H{"items": items, "page": page, "page_size": size, "has_more": len(items) == size})
}

func (h *OpenAIGatewayHandler) videoTaskStats(c *gin.Context, admin bool) {
	filter, ok := videoTaskFilter(c, admin, time.Now())
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	stats, err := h.gatewayService.GetVideoTaskStats(ctx, filter)
	if err != nil {
		response.InternalError(c, "读取视频统计失败，请缩小日期范围后重试")
		return
	}
	response.Success(c, stats)
}
