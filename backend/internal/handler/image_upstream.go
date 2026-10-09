package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"time"
)

func (h *OpenAIGatewayHandler) ImageUpstreamSettings(c *gin.Context) {
	state, err := h.gatewayService.ImageAdminSettings(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "读取图片跟价配置失败")
		return
	}
	response.Success(c, state)
}
func (h *OpenAIGatewayHandler) UpdateImageUpstreamSettings(c *gin.Context) {
	var cfg service.ImageRuntimeSettings
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "图片跟价配置格式无效")
		return
	}
	if err := h.gatewayService.SaveImageAdminSettings(c.Request.Context(), cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, cfg)
}
func (h *OpenAIGatewayHandler) SyncImageUpstreamPrices(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	cat, err := h.gatewayService.SyncImagePrices(ctx, true, c.Query("apply") == "true")
	if err != nil {
		response.Error(c, 502, "图片价格同步失败，保留最后有效售价")
		return
	}
	response.Success(c, cat)
}
func (h *OpenAIGatewayHandler) SaveImageUpstreamModelPrice(c *gin.Context) {
	var input struct {
		GroupID int64    `json:"group_id"`
		Model   string   `json:"model"`
		Price   *float64 `json:"price"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.GroupID <= 0 || input.Model == "" {
		response.BadRequest(c, "请选择图片分组和型号")
		return
	}
	if err := h.gatewayService.SaveImageModelPrice(c.Request.Context(), input.GroupID, input.Model, input.Price); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"saved": true})
}
