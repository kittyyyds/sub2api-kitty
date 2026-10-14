package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// A private context key cannot be supplied through an HTTP header. Only the
// owner-checking bridge below can introduce a resolved gateway credential.
type canvasResolvedKeyContext struct{}

type CanvasKeyReader interface {
	GetOwnedForCanvas(context.Context, int64, int64) (*service.APIKey, error)
}

func resolvedCanvasKey(ctx context.Context) *service.APIKey {
	key, _ := ctx.Value(canvasResolvedKeyContext{}).(*service.APIKey)
	return key
}

// CanvasGatewayRequest preserves the original client, cancellation and body;
// only the allowlisted route and private credential context are changed.
func CanvasGatewayRequest(c *gin.Context, keys CanvasKeyReader, keyID int64, target string) (*http.Request, bool) {
	subject, ok := GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return nil, false
	}
	if keyID <= 0 || !CanvasGatewayPathAllowed(c.Request.Method, target) {
		AbortWithError(c, http.StatusNotFound, "NOT_FOUND", "Canvas endpoint not found")
		return nil, false
	}
	key, err := keys.GetOwnedForCanvas(c.Request.Context(), subject.UserID, keyID)
	if err != nil || key == nil || key.UserID != subject.UserID {
		AbortWithError(c, http.StatusNotFound, "API_KEY_NOT_FOUND", "API key not found")
		return nil, false
	}
	req := c.Request.Clone(context.WithValue(c.Request.Context(), canvasResolvedKeyContext{}, key))
	req.URL.Path = target
	req.URL.RawPath = ""
	req.RequestURI = req.URL.RequestURI()
	// The main-site JWT must never be forwarded to an upstream provider.
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "X-Canvas-Key-ID", "Cookie"} {
		req.Header.Del(name)
	}
	return req, true
}

func CanvasKeyID(c *gin.Context) (int64, bool) {
	values := c.Request.Header.Values("X-Canvas-Key-ID")
	if len(values) != 1 {
		AbortWithError(c, http.StatusBadRequest, "INVALID_CANVAS_KEY_ID", "Select an API key")
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(values[0]), 10, 64)
	if err != nil || id <= 0 {
		AbortWithError(c, http.StatusBadRequest, "INVALID_CANVAS_KEY_ID", "Invalid API key ID")
		return 0, false
	}
	return id, true
}

// Keep this explicit: no arbitrary URL, admin route, delete or WebSocket relay.
func CanvasGatewayPathAllowed(method, target string) bool {
	if strings.ContainsAny(target, "%\\?#") || strings.Contains(target, "//") {
		return false
	}
	if method == http.MethodPost {
		switch target {
		case "/v1/responses", "/v1/chat/completions", "/v1/images/generations", "/v1/images/edits",
			"/v1/images/generations/async", "/v1/images/edits/async", "/v1/videos", "/v1/videos/generations", "/v1/video-references":
			return true
		}
		return false
	}
	if method != http.MethodGet {
		return false
	}
	if target == "/v1/models" {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(target, "/v1/"), "/")
	if !strings.HasPrefix(target, "/v1/") {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	if len(parts) == 3 && parts[0] == "images" && parts[1] == "tasks" {
		return true
	}
	if len(parts) >= 2 && parts[0] == "videos" {
		if parts[1] == "generations" {
			return len(parts) == 3 || len(parts) == 4 && parts[3] == "content"
		}
		if parts[1] == "edits" || parts[1] == "extensions" {
			return false
		}
		return len(parts) == 2 || len(parts) == 3 && parts[2] == "content"
	}
	return false
}
