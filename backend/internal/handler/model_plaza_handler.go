package handler

import (
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelPlazaHandler 处理「模型广场」查询。
//
// 广场路由挂 OptionalJWT 中间件：匿名可访问（除非 require_auth 开启），带 token 则
// 识别用户。可见性规则（橱窗语义，与「可用渠道」的可绑定语义不同）：
//   - 匿名：仅非专属分组（订阅型照常展示）；
//   - 登录：非专属分组 + user_allowed_groups 授权的专属分组（不检查订阅有效性）；
//     若该用户开启了公开分组限制，则公开分组同样需要落在授权集合内。
type ModelPlazaHandler struct {
	plazaService   *service.ModelPlazaService
	apiKeyService  *service.APIKeyService
	settingService *service.SettingService
}

// NewModelPlazaHandler 创建模型广场 handler。
func NewModelPlazaHandler(
	plazaService *service.ModelPlazaService,
	apiKeyService *service.APIKeyService,
	settingService *service.SettingService,
) *ModelPlazaHandler {
	return &ModelPlazaHandler{
		plazaService:   plazaService,
		apiKeyService:  apiKeyService,
		settingService: settingService,
	}
}

// modelPlazaOfficialPricing 官方参考价（USD per token，与计费目录同源）。
type modelPlazaOfficialPricing struct {
	InputPrice        *float64 `json:"input_price"`
	OutputPrice       *float64 `json:"output_price"`
	CacheWritePrice   *float64 `json:"cache_write_price"`
	CacheWrite1hPrice *float64 `json:"cache_write_1h_price,omitempty"`
	CacheReadPrice    *float64 `json:"cache_read_price"`
	// Intervals 官方长上下文阶梯，仅多档模型给出。
	Intervals []userPricingIntervalDTO `json:"intervals,omitempty"`
}

// modelPlazaTimePricingPeriod 分时倍率时段（配置时区当天 [start, end)）。
type modelPlazaTimePricingPeriod struct {
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Multiplier float64 `json:"multiplier"`
}

// modelPlazaTimePricing 计费会生效的分时倍率（仅倍率 ≠ 1 的时段）。
// WeekdaysOnly 为 true 时时段仅周一至周五生效，周末整天按标准价计费。
type modelPlazaTimePricing struct {
	Timezone     string                        `json:"timezone"`
	WeekdaysOnly bool                          `json:"weekdays_only,omitempty"`
	Periods      []modelPlazaTimePricingPeriod `json:"periods"`
}

// modelPlazaModel 广场模型条目：实收口径展示定价（白名单形态）+ 官方参考价。
type modelPlazaModel struct {
	Name                 string                     `json:"name"`
	Platform             string                     `json:"platform"`
	Pricing              *userSupportedModelPricing `json:"pricing"`
	OfficialPricing      *modelPlazaOfficialPricing `json:"official_pricing"`
	// LongContextBasis 多档时的计价基准："whole_request"（整单按档）| "marginal"（仅超出部分）。
	LongContextBasis     string                     `json:"long_context_basis,omitempty"`
	// TimePricing 分时倍率时段，落在时段内的请求整单乘倍率；无分时省略。
	TimePricing          *modelPlazaTimePricing     `json:"time_pricing,omitempty"`
	FinalImagePriceTiers []string                   `json:"final_image_price_tiers,omitempty"`
	FinalVideoPriceTiers []string                   `json:"final_video_price_tiers,omitempty"`
}

// modelPlazaGroup 广场分组条目（白名单字段）。
type modelPlazaGroup struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Platform           string   `json:"platform"`
	SubscriptionType   string   `json:"subscription_type"`
	RateMultiplier     float64  `json:"rate_multiplier"`
	UserRateMultiplier *float64 `json:"user_rate_multiplier,omitempty"`
	PeakRateEnabled    bool     `json:"peak_rate_enabled"`
	PeakStart          string   `json:"peak_start"`
	PeakEnd            string   `json:"peak_end"`
	PeakRateMultiplier float64  `json:"peak_rate_multiplier"`
	IsExclusive        bool     `json:"is_exclusive"`
	// 生图独立倍率：为 true 时图片计费模型的实付倍率取 ImageRateMultiplier，
	// 不取分组/用户专属倍率。
	ImageRateIndependent bool    `json:"image_rate_independent"`
	ImageRateMultiplier  float64 `json:"image_rate_multiplier"`
	VideoRateIndependent bool    `json:"video_rate_independent"`
	VideoRateMultiplier  float64 `json:"video_rate_multiplier"`
	// 分组是否启用长上下文阶梯计费；关闭时模型实付列只展示最低档/基础价。
	LongContextPricingEnabled bool              `json:"long_context_pricing_enabled"`
	Models                    []modelPlazaModel `json:"models"`
}

// modelPlazaResponse 广场页响应。
type modelPlazaResponse struct {
	Description string            `json:"description"`
	Groups      []modelPlazaGroup `json:"groups"`
}

// Get 返回模型广场数据。
// GET /api/v1/model-plaza
func (h *ModelPlazaHandler) Get(c *gin.Context) {
	if h.settingService == nil {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	if !rt.Enabled {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}

	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if rt.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return
	}

	groups, err := h.plazaService.ListGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// allowedGroups == nil 表示匿名；登录用户恒为非 nil（可能为空集合）。
	var allowedGroups map[int64]struct{}
	var restrictPublicGroups bool
	var userRates map[int64]float64
	if authed {
		allowedGroups, restrictPublicGroups, err = h.apiKeyService.GetUserGroupVisibility(c.Request.Context(), subject.UserID)
		if err != nil {
			// 可见性数据拿不到时不能静默降级成匿名视图（会错漏专属分组），直接报错。
			response.ErrorFrom(c, err)
			return
		}
		userRates, err = h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
		if err != nil {
			// 专属倍率仅是展示增强，失败降级为分组默认倍率。
			slog.Warn("model_plaza_user_rates_failed", "error", err, "user_id", subject.UserID)
			userRates = nil
		}
	}

	visible := filterPlazaVisibleGroups(groups, allowedGroups, restrictPublicGroups)

	out := make([]modelPlazaGroup, 0, len(visible))
	for i := range visible {
		dto := toModelPlazaGroupDTO(&visible[i], userRates)
		if len(dto.Models) == 0 {
			continue
		}
		out = append(out, dto)
	}
	response.Success(c, modelPlazaResponse{
		Description: rt.Description,
		Groups:      out,
	})
}

// filterPlazaVisibleGroups 按登录态裁剪分组可见性。
// allowedGroups == nil 表示匿名（仅非专属）；非 nil 表示登录（非专属 + 授权专属）。
// restrictPublicGroups 为 true 时，公开分组也必须落在 allowedGroups 内，否则用户会
// 在广场看到自己实际绑定不了的分组。
func filterPlazaVisibleGroups(
	groups []service.PlazaGroup,
	allowedGroups map[int64]struct{},
	restrictPublicGroups bool,
) []service.PlazaGroup {
	visible := make([]service.PlazaGroup, 0, len(groups))
	for _, g := range groups {
		if g.IsExclusive || (restrictPublicGroups && allowedGroups != nil) {
			if allowedGroups == nil {
				continue
			}
			if _, ok := allowedGroups[g.ID]; !ok {
				continue
			}
		}
		visible = append(visible, g)
	}
	return visible
}

// toModelPlazaGroupDTO 将 service 层广场分组映射为白名单 DTO,并合并用户专属倍率。
func toModelPlazaGroupDTO(g *service.PlazaGroup, userRates map[int64]float64) modelPlazaGroup {
	visibleModels := selectModelPlazaModels(g)
	models := make([]modelPlazaModel, 0, len(visibleModels))
	for i := range visibleModels {
		m := &visibleModels[i]
		models = append(models, modelPlazaModel{
			Name:                 m.Name,
			Platform:             m.Platform,
			Pricing:              toUserPricing(m.Pricing),
			OfficialPricing:      toModelPlazaOfficialPricing(m.OfficialPricing),
			LongContextBasis:     string(m.LongContextBasis),
			TimePricing:          toModelPlazaTimePricing(m.TimePricing),
			FinalImagePriceTiers: m.FinalImagePriceTiers,
			FinalVideoPriceTiers: m.FinalVideoPriceTiers,
		})
	}
	dto := modelPlazaGroup{
		ID:                        g.ID,
		Name:                      g.Name,
		Description:               g.Description,
		Platform:                  g.Platform,
		SubscriptionType:          g.SubscriptionType,
		RateMultiplier:            g.RateMultiplier,
		PeakRateEnabled:           g.PeakRateEnabled,
		PeakStart:                 g.PeakStart,
		PeakEnd:                   g.PeakEnd,
		PeakRateMultiplier:        g.PeakRateMultiplier,
		IsExclusive:               g.IsExclusive,
		ImageRateIndependent:      g.ImageRateIndependent,
		ImageRateMultiplier:       g.ImageRateMultiplier,
		VideoRateIndependent:      g.VideoRateIndependent,
		VideoRateMultiplier:       g.VideoRateMultiplier,
		LongContextPricingEnabled: g.LongContextPricingEnabled,
		Models:                    models,
	}
	if rate, ok := userRates[g.ID]; ok {
		dto.UserRateMultiplier = &rate
	}
	return dto
}

// selectModelPlazaModels 只裁剪客户价格页的展示目录，不改变分组真实可调用模型。
// OpenAI 生图专属组按组名分出 1K 与 2K/4K；其他 OpenAI 组仅展示文本。
// Claude 保持原目录；Grok 则使用明确的文本、图片和视频白名单。
func selectModelPlazaModels(g *service.PlazaGroup) []service.PlazaModel {
	if g == nil {
		return nil
	}
	if tiers, ok := openAIImageCatalogTiers(g); ok {
		models := make([]service.PlazaModel, 0, 1)
		for i := range g.Models {
			if !strings.EqualFold(strings.TrimSpace(g.Models[i].Name), "gpt-image-2") {
				continue
			}
			if model, keep := trimFinalMediaModel(g.Models[i], service.BillingModeImage, tiers); keep {
				models = append(models, model)
			}
		}
		return models
	}

	models := make([]service.PlazaModel, 0, len(g.Models))
	for i := range g.Models {
		model := g.Models[i]
		platform := strings.ToLower(strings.TrimSpace(model.Platform))
		if platform == "" {
			platform = strings.ToLower(strings.TrimSpace(g.Platform))
		}
		switch platform {
		case service.PlatformOpenAI:
			if isModelPlazaTextModel(&model) {
				models = append(models, model)
			}
		case service.PlatformGrok:
			name := strings.ToLower(strings.TrimSpace(model.Name))
			switch name {
			case "grok-4.5", "grok-4.6":
				if isModelPlazaTextModel(&model) {
					models = append(models, model)
				}
			case "grok-imagine-image", "grok-imagine-image-2.0", "grok-imagine-image-quality":
				if selected, keep := trimFinalMediaModel(model, service.BillingModeImage, []string{
					service.ImageBillingSize1K,
					service.ImageBillingSize2K,
				}); keep {
					models = append(models, selected)
				}
			case service.VideoPriceFamilyGrokImagineVideo, service.VideoPriceFamilyGrokImagineVideo15:
				if selected, keep := trimFinalMediaModel(model, service.BillingModeVideo, []string{
					service.VideoBillingResolution480P,
					service.VideoBillingResolution720P,
					service.VideoBillingResolution1080P,
				}); keep {
					models = append(models, selected)
				}
			}
		default:
			models = append(models, model)
		}
	}
	return models
}

func openAIImageCatalogTiers(g *service.PlazaGroup) ([]string, bool) {
	if g == nil || !strings.EqualFold(strings.TrimSpace(g.Platform), service.PlatformOpenAI) {
		return nil, false
	}
	name := strings.ToLower(strings.TrimSpace(g.Name))
	if !strings.Contains(name, "image") || !strings.Contains(name, "生图") {
		return nil, false
	}
	if strings.Contains(name, "1k") {
		return []string{service.ImageBillingSize1K}, true
	}
	return []string{service.ImageBillingSize2K, service.ImageBillingSize4K}, true
}

func isModelPlazaTextModel(model *service.PlazaModel) bool {
	if model == nil {
		return false
	}
	mode := service.BillingModeToken
	if model.Pricing != nil && model.Pricing.BillingMode != "" {
		mode = model.Pricing.BillingMode
	}
	if mode != service.BillingModeToken {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(model.Name))
	return !strings.Contains(name, "image") && !strings.Contains(name, "video")
}

// trimFinalMediaModel 只保留后台明确配置为最终客户价的媒体档位。
// 价格与档位切片均在克隆上修改，避免污染 service 层的共享结果。
func trimFinalMediaModel(
	model service.PlazaModel,
	mode service.BillingMode,
	allowedTiers []string,
) (service.PlazaModel, bool) {
	if model.Pricing == nil || model.Pricing.BillingMode != mode {
		return service.PlazaModel{}, false
	}
	finalTiers := model.FinalImagePriceTiers
	if mode == service.BillingModeVideo {
		finalTiers = model.FinalVideoPriceTiers
	}
	if len(finalTiers) == 0 {
		return service.PlazaModel{}, false
	}

	pricing := *model.Pricing
	pricing.Intervals = make([]service.PricingInterval, 0, len(allowedTiers))
	selectedTiers := make([]string, 0, len(allowedTiers))
	for _, allowed := range allowedTiers {
		if !containsFold(finalTiers, allowed) {
			continue
		}
		for i := range model.Pricing.Intervals {
			if strings.EqualFold(strings.TrimSpace(model.Pricing.Intervals[i].TierLabel), allowed) {
				pricing.Intervals = append(pricing.Intervals, model.Pricing.Intervals[i])
				selectedTiers = append(selectedTiers, allowed)
				break
			}
		}
	}
	if len(pricing.Intervals) == 0 {
		return service.PlazaModel{}, false
	}

	model.Pricing = &pricing
	if mode == service.BillingModeVideo {
		model.FinalVideoPriceTiers = selectedTiers
	} else {
		model.FinalImagePriceTiers = selectedTiers
	}
	return model, true
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

// toModelPlazaTimePricing 转换分时倍率；nil 透传（JSON 省略）。
func toModelPlazaTimePricing(p *service.TimePricingSchedule) *modelPlazaTimePricing {
	if p == nil || len(p.Periods) == 0 {
		return nil
	}
	periods := make([]modelPlazaTimePricingPeriod, 0, len(p.Periods))
	for _, period := range p.Periods {
		periods = append(periods, modelPlazaTimePricingPeriod{
			StartTime:  period.StartTime,
			EndTime:    period.EndTime,
			Multiplier: period.Multiplier,
		})
	}
	return &modelPlazaTimePricing{Timezone: p.Timezone, WeekdaysOnly: p.WeekdaysOnly, Periods: periods}
}

// toModelPlazaOfficialPricing 转换官方参考价；nil 透传（前端显示 "-"）。
func toModelPlazaOfficialPricing(p *service.PlazaOfficialPricing) *modelPlazaOfficialPricing {
	if p == nil {
		return nil
	}
	return &modelPlazaOfficialPricing{
		InputPrice:        p.InputPrice,
		OutputPrice:       p.OutputPrice,
		CacheWritePrice:   p.CacheWritePrice,
		CacheWrite1hPrice: p.CacheWrite1hPrice,
		CacheReadPrice:    p.CacheReadPrice,
		Intervals:         toUserPricingIntervals(p.Intervals),
	}
}
