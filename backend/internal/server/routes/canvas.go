package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RegisterCanvasRoutes uses the main site's session; canvas has no account or
// token issuer of its own. The gateway handler only relays explicit endpoints.
func RegisterCanvasRoutes(r *gin.Engine, v1 *gin.RouterGroup, jwtAuth middleware.JWTAuthMiddleware, keys *service.APIKeyService, users *service.UserService, settings *service.SettingService, limiter *middleware.PanelRateLimiter) {
	h := handler.NewCanvasHandler(keys, users, r)
	panel := v1.Group("/canvas", gin.HandlerFunc(jwtAuth), middleware.BackendModeUserGuard(settings), limiter.Global())
	panel.GET("/bootstrap", h.Bootstrap)
	panel.GET("/keys/:id/models", h.Models)
	panel.POST("/keys/:id/quote", h.Quote)
	gateway := r.Group("/canvas-api/v1", gin.HandlerFunc(jwtAuth), middleware.BackendModeUserGuard(settings))
	gateway.Any("/*path", h.Gateway)
}
