package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// All routes inherit authentication, hierarchy checks and the administrative audit middleware.
func registerRelayManagementRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	cleanup := admin.Group("/users/cleanup")
	cleanup.GET("/summary", h.Admin.UserCleanup.Summary)
	cleanup.POST("/preview", h.Admin.UserCleanup.Preview)
	cleanup.POST("/execute", h.Admin.UserCleanup.Execute)
	admin.GET("/users/:id/cleanup-guard", h.Admin.UserCleanup.GetGuard)
	admin.PUT("/users/:id/cleanup-guard", h.Admin.UserCleanup.UpdateGuard)
}
