package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (h *OpenAIGatewayHandler) VideoUpstreamSettings(c *gin.Context) {
	cfg, catalog, err := h.gatewayService.VideoAdminSettings(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "读取视频配置失败")
		return
	}
	response.Success(c, gin.H{"settings": cfg, "catalog": catalog})
}
func (h *OpenAIGatewayHandler) UpdateVideoUpstreamSettings(c *gin.Context) {
	var cfg service.VideoRuntimeSettings
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "视频配置格式无效")
		return
	}
	if err := h.gatewayService.SaveVideoAdminSettings(c.Request.Context(), cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, cfg)
}
func (h *OpenAIGatewayHandler) SyncVideoUpstreamPrices(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	cat, err := h.gatewayService.SyncVideoPrices(ctx, true)
	if err != nil {
		response.Error(c, 502, "同步失败，已有型号继续使用最后有效价格")
		return
	}
	response.Success(c, cat)
}
func (h *OpenAIGatewayHandler) ProbeVideoUpstreamPrices(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	cat, err := h.gatewayService.ProbeVideoPrices(ctx)
	if err != nil {
		response.Error(c, 502, "读取上游价格失败，保留最后有效目录和分组售价")
		return
	}
	response.Success(c, cat)
}
func (h *OpenAIGatewayHandler) ImportVideoUpstreamModels(c *gin.Context) {
	var input struct {
		GroupID int64    `json:"group_id"`
		Models  []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "请选择视频分组和型号")
		return
	}
	if err := h.gatewayService.ImportVideoPricing(c.Request.Context(), input.GroupID, input.Models); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"imported": len(input.Models)})
}

func (h *OpenAIGatewayHandler) unifiedVideoLookup(c *gin.Context, key *service.APIKey, endpoint service.GrokMediaEndpoint, id string) bool {
	if key.Group == nil || key.Group.Platform != service.PlatformVideo || !endpoint.IsVideoLookupRequest() {
		return false
	}
	task, err := h.gatewayService.FindVideoTask(c.Request.Context(), id, key.UserID, key.ID)
	if err != nil {
		h.errorResponse(c, 503, "video_task_unavailable", "视频任务暂时无法查询，请稍后重试")
		return true
	}
	if task == nil || !task.Pending.Unified {
		return false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()
	body, err := h.gatewayService.UnifiedVideoStatus(ctx, task, h.apiKeyService)
	if err != nil {
		h.errorResponse(c, 502, "video_status_unavailable", err.Error())
		return true
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	if endpoint == service.GrokMediaEndpointVideoContent {
		if gjson.GetBytes(body, "delivery_status").String() == "expired" {
			h.errorResponse(c, 410, "video_expired", "上游视频文件已过期")
			return true
		}
		url := gjson.GetBytes(body, "url").String()
		if gjson.GetBytes(body, "status").String() != "completed" || url == "" {
			h.errorResponse(c, 409, "video_not_ready", "视频尚未交付，请继续查询原任务")
			return true
		}
		if err := h.gatewayService.StreamUnifiedVideoContent(c, url); err != nil {
			if !service.IsResponseCommitted(c) {
				h.errorResponse(c, http.StatusBadGateway, "video_content_unavailable", err.Error())
			}
			c.Abort()
		}
		return true
	}
	c.Data(http.StatusOK, "application/json", body)
	return true
}
