package service

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"strconv"
	"strings"
	"time"
)

func writeOpenAIResponsesErrorJSON(c *gin.Context, statusCode int, envelope gin.H) {
	// Every caller returns a terminal (non-failover) error after this write;
	// mark it so the handler does not append a fallback response.failed.
	MarkResponseCommitted(c)
	if StopOpenAICompactSSEKeepaliveCommitted(c) {
		errorBody, _ := envelope["error"].(gin.H)
		writeOpenAIResponsesSSEFailure(c, statusCode, errorBody)
		return
	}
	c.JSON(statusCode, envelope)
}

func writeOpenAIResponsesSSEFailure(c *gin.Context, statusCode int, errorBody gin.H) {
	if c == nil {
		return
	}
	if errorBody == nil {
		errorBody = gin.H{}
	}
	code, _ := errorBody["code"].(string)
	if code == "" {
		code, _ = errorBody["type"].(string)
		if code == "" {
			code = "upstream_error"
		}
		errorBody["code"] = code
	}
	message, _ := errorBody["message"].(string)
	if message == "" {
		message = "Upstream request failed with HTTP " + strconv.Itoa(statusCode)
		errorBody["message"] = message
	}
	MarkResponseCommitted(c)
	MarkOpsStreamError(c, code, message, statusCode)
	payload, err := json.Marshal(map[string]any{
		"type":            "response.failed",
		"sequence_number": 0,
		"response": map[string]any{
			"id":     "resp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"object": "response",
			// 严格客户端把 created_at 当必填字段，缺失会反序列化失败，
			// 终止事件就白发了（退化成盲重连）。与 writeResponsesFailedSSE 对齐。
			"created_at": time.Now().Unix(),
			"status":     "failed",
			"output":     []any{},
			"error":      errorBody,
		},
	})
	if err != nil {
		return
	}
	_, _ = c.Writer.Write([]byte("event: response.failed\ndata: "))
	_, _ = c.Writer.Write(payload)
	_, _ = c.Writer.Write([]byte("\n\n"))
	c.Writer.Flush()
}
