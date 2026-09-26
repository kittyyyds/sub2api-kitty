package routes

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// A dedicated video key must not fall through to Anthropic/text dispatch when
// the selected route does not recognize its platform.
func videoPlatformEndpointGuard(c *gin.Context) {
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key == nil || key.Group == nil || key.Group.Platform != service.PlatformVideo {
		c.Next()
		return
	}
	path := strings.TrimPrefix(c.Request.URL.Path, "/v1")
	video := path == "/videos" || strings.HasPrefix(path, "/videos/")
	discovery := c.Request.Method == http.MethodGet && (path == "/models" || strings.HasPrefix(path, "/models/") || path == "/usage" || path == "/sub2api/billing")
	if video || discovery {
		c.Next()
		return
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found_error", "message": "This video group only supports video endpoints"}})
}
