package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *QQBotHandler) CommunityGuide(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	guide, err := h.bot.CommunityGuide(ctx)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "暂时无法读取群福利，请稍后重试")
		return
	}
	response.Success(c, guide)
}
