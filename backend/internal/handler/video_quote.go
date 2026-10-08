package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"slices"
	"time"
)

type videoQuoteInput struct {
	service.UnifiedVideoInput
	Count int `json:"count"`
}

func quoteVideoInput(ctx context.Context, key *service.APIKey, input videoQuoteInput, override ...*service.VideoGroupPrice) (*canvasQuote, error) {
	var price *service.VideoGroupPrice
	var err error
	if len(override) > 0 && override[0] != nil {
		price = override[0]
	} else {
		price, err = service.ResolveVideoGroupPrice(key.Group, input.Model)
		if err != nil {
			return nil, err
		}
	}
	if price.Video == nil {
		return canvasVideoQuote(key.Group, input.Model, input.Seconds, input.Count)
	}
	if input.Count != 0 && input.Count != 1 {
		return nil, errors.New("每次只报价一个视频任务")
	}
	if input.Prompt == "" {
		input.Prompt = "报价预估"
	}
	raw, err := json.Marshal(input.UnifiedVideoInput)
	if err != nil {
		return nil, err
	}
	spec := price.Video.Spec
	// Settings may quote output alone; submission still validates all references.
	if len(input.Images)+len(input.Videos)+len(input.Audios) == 0 {
		spec.MinImages = 0
		spec.RequiresVisualOrAudio = false
	}
	prepared, err := service.PrepareUnifiedVideoInput(raw, "application/json", spec)
	if err != nil {
		return nil, err
	}
	seconds, _, err := service.MeasureVideoReferences(ctx, key.UserID, &prepared, price.Video.Spec)
	if err != nil {
		return nil, err
	}
	unitPrice := price.UnitPrice
	if len(price.Video.ResolutionPrices) > 0 {
		var ok bool
		unitPrice, ok = price.Video.ResolutionPrices[prepared.Resolution]
		if !ok {
			return nil, errors.New("该分辨率暂无有效价格")
		}
	}
	units := float64(prepared.Seconds)
	unit := "秒"
	if price.BillingMode == service.BillingModePerRequest {
		units = 1
		unit = "条"
	}
	cost := service.QuantizeUsageBillingAmount(unitPrice*units + price.Video.ReferencePerSecond*seconds)
	return &canvasQuote{Model: input.Model, BillingMode: string(price.BillingMode), Unit: unit, UnitPrice: unitPrice, Seconds: prepared.Seconds, Count: 1, EstimatedCost: cost, FinalUnitPrice: true, QuotedAt: time.Now().UTC(), ReferenceSeconds: seconds, ReferencePerSecond: price.Video.ReferencePerSecond, OutputCost: service.QuantizeUsageBillingAmount(unitPrice * units), ReferenceCost: service.QuantizeUsageBillingAmount(price.Video.ReferencePerSecond * seconds), ReferenceVideos: prepared.Videos, Note: "提交预扣，成功结算，明确失败退款；以提交时锁价为准，重试下载不重复计费"}, nil
}

func (h *OpenAIGatewayHandler) VideoQuote(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key.Group == nil || key.Group.Platform != service.PlatformVideo {
		response.Forbidden(c, "请选择视频分组密钥")
		return
	}
	var input videoQuoteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "视频报价参数无效")
		return
	}
	if slices.Contains(key.DeniedModelsInGroup(), input.Model) || (key.Group.ModelAllowlistEnabled() && !slices.Contains(key.Group.ModelAllowlist.FilterForListing([]string{input.Model}), input.Model)) {
		response.Forbidden(c, "当前密钥没有此型号权限")
		return
	}
	// Resolve from the repository to avoid quoting an old auth-cache group price.
	price, err := h.gatewayService.ResolveVideoGroupPricing(c.Request.Context(), key, input.Model)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 55*time.Second)
	defer cancel()
	result, err := quoteVideoInput(ctx, key, input, price)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}
