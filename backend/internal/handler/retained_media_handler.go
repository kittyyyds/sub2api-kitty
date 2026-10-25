package handler

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"mime"
	"net/http"
	"strconv"
	"time"
)

type RetainedMediaHandler struct {
	storage *service.RetainedMediaService
	billing *service.BillingCacheService
	auth    service.APIKeyAuthCacheInvalidator
}

func NewRetainedMediaHandler(storage *service.RetainedMediaService, billing *service.BillingCacheService, auth service.APIKeyAuthCacheInvalidator) *RetainedMediaHandler {
	return &RetainedMediaHandler{storage, billing, auth}
}
func storageUser(c *gin.Context) (int64, bool) {
	c.Header("Cache-Control", "private, no-store")
	u, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "请先登录 SUB")
		return 0, false
	}
	return u.UserID, true
}
func storageError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, sql.ErrNoRows):
		response.Error(c, 404, "素材不存在")
	case errors.Is(e, service.ErrStorageExpired):
		response.Error(c, 410, e.Error())
	case errors.Is(e, service.ErrStorageChanged):
		response.Error(c, 409, e.Error())
	case errors.Is(e, service.ErrStorageInvalid):
		response.Error(c, 400, e.Error())
	case errors.Is(e, service.ErrBatchImageInsufficientBalance):
		response.Error(c, 402, "SUB 余额不足，未扣费也未修改保存期限")
	case errors.Is(e, service.ErrStorageUnavailable):
		response.Error(c, 503, e.Error())
	default:
		response.Error(c, 500, "保存或延期未完成，请重试；重复提交同一订单不会重复扣费")
	}
}
func (h *RetainedMediaHandler) Config(c *gin.Context) {
	if _, ok := storageUser(c); !ok {
		return
	}
	response.Success(c, h.storage.Config())
}
func storageOffset(c *gin.Context) int {
	o, _ := strconv.Atoi(c.Query("offset"))
	if o < 0 {
		o = 0
	}
	if o > 100000 {
		o = 100000
	}
	return o
}
func (h *RetainedMediaHandler) List(c *gin.Context) {
	u, ok := storageUser(c)
	if !ok {
		return
	}
	rows, e := h.storage.List(c.Request.Context(), u, storageOffset(c))
	if e != nil {
		storageError(c, e)
		return
	}
	response.Success(c, map[string]any{"items": rows, "server_time": time.Now().UTC(), "has_more": len(rows) == 100})
}
func (h *RetainedMediaHandler) Orders(c *gin.Context) {
	u, ok := storageUser(c)
	if !ok {
		return
	}
	rows, e := h.storage.Orders(c.Request.Context(), u, storageOffset(c))
	if e != nil {
		storageError(c, e)
		return
	}
	response.Success(c, map[string]any{"items": rows, "has_more": len(rows) == 100})
}
func (h *RetainedMediaHandler) Upload(c *gin.Context) {
	u, ok := storageUser(c)
	if !ok {
		return
	}
	if !h.storage.Enabled() {
		storageError(c, service.ErrStorageUnavailable)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.RetainedMediaMaxBytes+65536)
	reader, e := c.Request.MultipartReader()
	if e != nil {
		response.BadRequest(c, "请选择图片或视频文件")
		return
	}
	part, e := reader.NextPart()
	if e != nil || part.FormName() != "file" {
		response.BadRequest(c, "缺少素材文件")
		return
	}
	defer part.Close()
	a, e := h.storage.Upload(c.Request.Context(), u, part.FileName(), part)
	if e != nil {
		storageError(c, e)
		return
	}
	response.Success(c, a)
}
func (h *RetainedMediaHandler) Content(c *gin.Context) {
	u, ok := storageUser(c)
	if !ok {
		return
	}
	a, f, e := h.storage.Open(c.Request.Context(), u, c.Param("id"))
	if e != nil {
		storageError(c, e)
		return
	}
	defer f.Close()
	c.Header("Content-Type", a.MimeType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Name}))
	http.ServeContent(c.Writer, c.Request, a.Name, a.CreatedAt, f)
}
func (h *RetainedMediaHandler) Extend(c *gin.Context) {
	u, ok := storageUser(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var cmd service.StorageExtension
	if c.ShouldBindJSON(&cmd) != nil {
		response.BadRequest(c, "延期参数不正确")
		return
	}
	o, e := h.storage.Extend(c.Request.Context(), u, c.Param("id"), cmd)
	if e != nil {
		storageError(c, e)
		return
	}
	// Replays also retry cache invalidation after a response/network failure.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if h.billing != nil {
		_ = h.billing.InvalidateUserBalance(ctx, u)
	}
	if h.auth != nil {
		h.auth.InvalidateAuthCacheByUserID(ctx, u)
	}
	response.Success(c, o)
}
