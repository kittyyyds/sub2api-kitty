package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Keep these user-facing APIs separate from the administrator's test plans,
// results, decisions and scheduler. Hiding a report never stops its execution.
func registerChannelQualityPublicRoutes(user *gin.RouterGroup, h *handler.UserHandler, settings *service.SettingService) {
	guard := channelQualityPublicGuard(settings)
	user.GET("/test-results", guard, h.ListTestResults)
	user.GET("/test-results/:id/history", guard, h.ListTestResultHistory)
	user.GET("/test-votes", guard, h.ListTestVotes)
	user.POST("/test-results/:id/vote", guard, h.VoteTestResult)
}

func channelQualityPublicGuard(settings *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settings.IsChannelQualityPublicEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		response.ErrorWithDetails(c, http.StatusForbidden, "Channel quality results are not publicly available", "CHANNEL_QUALITY_PUBLIC_DISABLED", nil)
		c.Abort()
	}
}
