package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (h *OpenAIGatewayHandler) GatewayPromptMiddleware() gin.HandlerFunc {
	if h == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return gatewayPromptMiddleware(h.pluginManager)
}

type gatewayPromptTransformer interface {
	HasGatewayPrompt() bool
	TransformGatewayRequest(context.Context, int64, int64, string, string, []byte) ([]byte, error)
}

func gatewayPromptMiddleware(manager gatewayPromptTransformer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil || !manager.HasGatewayPrompt() || c.Request.Method != http.MethodPost {
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
			abortGatewayPromptError(c, err, true)
			return
		}
		// Malformed JSON still goes through the existing protocol-specific validator.
		if !gjson.ValidBytes(body) {
			c.Request.Body = pkghttputil.NewPrereadBody(body)
			c.Next()
			return
		}
		out, err := manager.TransformGatewayRequest(c.Request.Context(), *key.GroupID, key.ID, protocol, gjson.GetBytes(body, "model").String(), body)
		if err != nil {
			abortGatewayPromptError(c, err, false)
			return
		}
		c.Request.Body = pkghttputil.NewPrereadBody(out)
		c.Request.ContentLength = int64(len(out))
		c.Request.Header.Del("Content-Length")
		c.Next()
	}
}

// Input errors must never ask clients to retry an unchanged request as a 503.
func abortGatewayPromptError(c *gin.Context, err error, readingBody bool) {
	status, kind, code, message := http.StatusServiceUnavailable, "api_error", "prompt_plugin_unavailable", err.Error()
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		status, kind, code = http.StatusRequestEntityTooLarge, "invalid_request_error", "request_body_too_large"
		message = fmt.Sprintf("请求正文超过提示词处理上限（%d 字节）；请压缩会话、减少附件或新建会话后重试，重复发送相同请求不会恢复。", maxErr.Limit)
	case errors.Is(err, service.ErrGatewayPromptInvalidRequest):
		status, kind, code, message = http.StatusBadRequest, "invalid_request_error", "invalid_request_body", service.ErrGatewayPromptInvalidRequest.Error()
	case readingBody:
		status, kind, code, message = http.StatusBadRequest, "invalid_request_error", "invalid_request_body", "请求正文读取或解压失败；请检查 Content-Encoding 和正文格式后重试。"
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"type": kind, "code": code, "message": message}})
}
