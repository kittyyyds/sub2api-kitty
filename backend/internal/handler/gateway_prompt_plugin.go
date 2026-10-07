package handler

import (
	"net/http"
	"strconv"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (h *OpenAIGatewayHandler) GatewayPromptMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil || h.pluginManager == nil || !h.pluginManager.HasGatewayPrompt() || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		path := strings.TrimSuffix(c.Request.URL.Path, "/")
		protocol := ""
		switch {
		case strings.HasSuffix(path, "/messages"), strings.HasSuffix(path, "/messages/count_tokens"):
			protocol = "anthropic"
		case strings.HasSuffix(path, "/chat/completions"):
			protocol = "chat_completions"
		case strings.HasSuffix(path, "/responses"), strings.HasSuffix(path, "/responses/compact"), strings.HasSuffix(path, "/responses/input_tokens"):
			protocol = "responses"
		default:
			c.Next()
			return
		}
		key, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || key == nil || key.GroupID == nil {
			c.Next()
			return
		}
		body, err := pkghttputil.ReadRequestBodyWithPreallocLimit(c.Request, service.GatewayPromptMaxBody)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "请求正文读取失败或超过限制"}})
			return
		}
		// Malformed JSON still goes through the existing protocol-specific validator.
		if !gjson.ValidBytes(body) {
			c.Request.Body = pkghttputil.NewPrereadBody(body)
			c.Next()
			return
		}
		out, err := h.pluginManager.TransformGatewayRequest(c.Request.Context(), *key.GroupID, key.ID, protocol, gjson.GetBytes(body, "model").String(), body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "api_error", "message": err.Error()}})
			return
		}
		c.Request.Body = pkghttputil.NewPrereadBody(out)
		c.Request.ContentLength = int64(len(out))
		c.Request.Header.Del("Content-Length")
		c.Next()
	}
}

func (h *GatewayHandler) AdminGroupAvailableModels(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "分组 ID 无效")
		return
	}
	group, err := h.gatewayService.GetPluginGroup(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	models := h.codexModelIDsForGroup(c.Request.Context(), group, "")
	if service.IsMultiProtocolAPIKeyProvider(group.Platform) {
		models = h.gatewayService.GetAvailableModels(c.Request.Context(), &group.ID, group.Platform)
		if group.ModelAllowlistEnabled() {
			models = group.ModelAllowlist.FilterForListing(models)
		}
	}
	if models == nil {
		models = []string{}
	}
	response.Success(c, gin.H{"models": models})
}
