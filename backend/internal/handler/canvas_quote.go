package handler

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Quote shares final video group prices. It does not reserve or deduct funds.
// Other media pricing continues to use the existing billing preview endpoint.
func (h *CanvasHandler) Quote(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, ok := h.ownedKey(c)
	if !ok {
		return
	}
	var input struct {
		Model   string `json:"model"`
		Seconds int    `json:"seconds"`
		Count   int    `json:"count"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "请输入型号、整数秒数和数量")
		return
	}
	input.Model = strings.TrimSpace(input.Model)
	models, err := h.allowedModels(c, key)
	if err != nil {
		if !c.IsAborted() {
			response.Error(c, canvasGatewayErrorStatus(err), "当前密钥不可用，请检查分组、余额或有效期")
		}
		return
	}
	if !slices.Contains(models, input.Model) {
		response.BadRequest(c, "当前密钥没有此型号的调用权限")
		return
	}
	quote, err := canvasVideoQuote(key.Group, input.Model, input.Seconds, input.Count)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, quote)
}

type canvasQuote struct {
	Model          string    `json:"model"`
	BillingMode    string    `json:"billing_mode"`
	Unit           string    `json:"unit"`
	UnitPrice      float64   `json:"unit_price"`
	Seconds        int       `json:"seconds"`
	Count          int       `json:"count"`
	EstimatedCost  float64   `json:"estimated_cost"`
	FinalUnitPrice bool      `json:"final_unit_price"`
	QuotedAt       time.Time `json:"quoted_at"`
	Note           string    `json:"note"`
}

func canvasVideoQuote(group *service.Group, model string, seconds, count int) (*canvasQuote, error) {
	price, err := service.ResolveVideoGroupPrice(group, model)
	if err != nil {
		return nil, err
	}
	capability := service.CanvasCapabilities(model, service.PlatformVideo)
	if !capability.Supported || capability.Seconds == nil {
		return nil, errors.New("该型号尚未完成画布适配")
	}
	if count == 0 {
		count = 1
	}
	if count != 1 {
		return nil, errors.New("每个视频任务生成一条，请分别预估批量任务")
	}
	if seconds == 0 && capability.Seconds.Fixed > 0 {
		seconds = capability.Seconds.Fixed
	}
	if seconds < capability.Seconds.Min || seconds > capability.Seconds.Max {
		return nil, errors.New("视频时长不符合该型号限制")
	}
	unit, units := "条", 1
	if price.BillingMode == service.BillingModeVideo {
		unit, units = "秒", seconds
	}
	cost := price.UnitPrice * float64(units)
	if math.IsInf(cost, 0) || math.IsNaN(cost) || cost >= 1e12 {
		return nil, errors.New("预估费用超出支持范围")
	}
	return &canvasQuote{Model: model, BillingMode: string(price.BillingMode), Unit: unit,
		UnitPrice: price.UnitPrice, Seconds: seconds, Count: count, EstimatedCost: math.Round(cost*1e8) / 1e8,
		FinalUnitPrice: true, QuotedAt: time.Now().UTC(), Note: "以提交任务时锁定的单价结算；下载重试不重新生成"}, nil
}
