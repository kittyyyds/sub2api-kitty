package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type canvasKeys interface {
	middleware.CanvasKeyReader
	List(context.Context, int64, pagination.PaginationParams, service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error)
}

type canvasUsers interface {
	GetByID(context.Context, int64) (*service.User, error)
}

type CanvasHandler struct {
	apiKeyService canvasKeys
	userService   canvasUsers
	gateway       http.Handler
}

func NewCanvasHandler(keys *service.APIKeyService, users *service.UserService, gateway http.Handler) *CanvasHandler {
	return &CanvasHandler{apiKeyService: keys, userService: users, gateway: gateway}
}

type canvasGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type canvasKey struct {
	ID         int64        `json:"id"`
	Name       string       `json:"name"`
	KeyDisplay string       `json:"key_display"`
	Status     string       `json:"status"`
	GroupID    *int64       `json:"group_id"`
	Group      *canvasGroup `json:"group"`
	ExpiresAt  *time.Time   `json:"expires_at"`
}

func (h *CanvasHandler) Bootstrap(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	keys := make([]canvasKey, 0)
	for page := 1; ; page++ {
		items, result, err := h.apiKeyService.List(c.Request.Context(), subject.UserID, pagination.PaginationParams{Page: page, PageSize: 1000, SortBy: "id", SortOrder: "asc"}, service.APIKeyListFilters{})
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		for _, key := range items {
			if key.UserID != subject.UserID {
				continue
			}
			entry := canvasKey{ID: key.ID, Name: key.Name, KeyDisplay: canvasKeyDisplay(key), Status: key.Status, GroupID: key.GroupID, ExpiresAt: key.ExpiresAt}
			if key.IsExpired() {
				entry.Status = service.StatusAPIKeyExpired
			}
			if key.Group != nil {
				entry.Group = &canvasGroup{ID: key.Group.ID, Name: key.Group.Name, Platform: key.Group.Platform}
			}
			keys = append(keys, entry)
		}
		if len(items) == 0 || result == nil || int64(page*1000) >= result.Total {
			break
		}
	}
	response.Success(c, gin.H{
		"user": gin.H{"id": user.ID, "username": user.Username, "email": user.Email, "balance": user.Balance},
		"keys": keys, "storage_mode": "local", "capabilities_version": "1",
	})
}

func canvasKeyDisplay(key service.APIKey) string {
	prefix := key.KeyPrefix
	if prefix == "" && len(key.Key) > 8 {
		prefix = key.Key[:8]
	}
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return prefix + "********"
}

func (h *CanvasHandler) ownedKey(c *gin.Context) (*service.APIKey, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return nil, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid API key ID")
		return nil, false
	}
	key, err := h.apiKeyService.GetOwnedForCanvas(c.Request.Context(), subject.UserID, id)
	if err != nil || key == nil || key.UserID != subject.UserID {
		response.NotFound(c, "API key not found")
		return nil, false
	}
	return key, true
}

func (h *CanvasHandler) Models(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, ok := h.ownedKey(c)
	if !ok {
		return
	}
	models, err := h.allowedModels(c, key)
	if err != nil {
		if !c.IsAborted() {
			response.Error(c, canvasGatewayErrorStatus(err), err.Error())
		}
		return
	}
	platform := ""
	if key.Group != nil {
		platform = key.Group.Platform
	}
	items := make([]gin.H, 0, len(models))
	for _, id := range models {
		items = append(items, gin.H{"id": id, "capabilities": canvasModelCapabilities(id, platform)})
	}
	response.Success(c, gin.H{"api_key_id": key.ID, "models": items, "capabilities_version": "1"})
}

// The catalog is collected through the very same gateway auth/model allowlist
// chain as generation. No provider URL or credential is returned to the client.
func (h *CanvasHandler) allowedModels(c *gin.Context, key *service.APIKey) ([]string, error) {
	requestContext := c.Copy()
	requestContext.Writer = c.Writer
	requestContext.Request = c.Request.Clone(c.Request.Context())
	requestContext.Request.Method = http.MethodGet
	requestContext.Request.Body = nil
	requestContext.Request.ContentLength = 0
	requestContext.Request.URL.RawQuery = ""
	req, ok := middleware.CanvasGatewayRequest(requestContext, h.apiKeyService, key.ID, "/v1/models")
	if !ok {
		c.Abort()
		return nil, &canvasGatewayError{status: http.StatusForbidden, message: "API key is unavailable"}
	}
	recorder := &canvasModelRecorder{header: make(http.Header), status: http.StatusOK}
	h.gateway.ServeHTTP(recorder, req)
	if recorder.status < 200 || recorder.status >= 300 {
		return nil, &canvasGatewayError{status: recorder.status, message: "API key models are unavailable"}
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.body.Bytes(), &payload); err != nil {
		return nil, &canvasGatewayError{status: http.StatusBadGateway, message: "Invalid model catalog response"}
	}
	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]bool)
	for _, item := range payload.Data {
		if item.ID != "" && !seen[item.ID] {
			models = append(models, item.ID)
			seen[item.ID] = true
		}
	}
	return models, nil
}

func (h *CanvasHandler) Gateway(c *gin.Context) {
	target := strings.TrimPrefix(c.Request.URL.Path, "/canvas-api")
	if !middleware.CanvasGatewayPathAllowed(c.Request.Method, target) {
		response.NotFound(c, "Canvas endpoint not found")
		return
	}
	id, ok := middleware.CanvasKeyID(c)
	if !ok {
		return
	}
	req, ok := middleware.CanvasGatewayRequest(c, h.apiKeyService, id, target)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	h.gateway.ServeHTTP(c.Writer, req)
}

type canvasGatewayError struct {
	status  int
	message string
}

func (e *canvasGatewayError) Error() string { return e.message }
func canvasGatewayErrorStatus(err error) int {
	var gatewayErr *canvasGatewayError
	if errors.As(err, &gatewayErr) {
		return gatewayErr.status
	}
	return http.StatusBadGateway
}

// Model lists are bounded; generation uses the original ResponseWriter directly.
type canvasModelRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *canvasModelRecorder) Header() http.Header    { return r.header }
func (r *canvasModelRecorder) WriteHeader(status int) { r.status = status }
func (r *canvasModelRecorder) Write(data []byte) (int, error) {
	if r.body.Len()+len(data) > 4<<20 {
		return 0, fmt.Errorf("model catalog exceeds limit")
	}
	return r.body.Write(data)
}
