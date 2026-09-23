package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// PlazaOfficialPricing 模型广场展示用的官方参考价（USD per token），与计费同源：
// LiteLLM → 内置兜底价卡 → 模型策略。字段为 nil 表示该项缺失（0 视为未配置）。
type PlazaOfficialPricing struct {
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64 // 5m 缓存写入（= LiteLLM cache_creation）
	CacheWrite1hPrice *float64 // 1h 缓存写入，仅计费会区分 5m/1h 时给出
	CacheReadPrice    *float64
	// Intervals 官方长上下文阶梯（多档时给出），不受分组开关影响。
	Intervals []PricingInterval
}

// PlazaModel 模型广场中单个模型条目：按实收口径合成的展示定价 + 官方参考价。
type PlazaModel struct {
	Name            string
	Platform        string
	Pricing         *ChannelModelPricing
	OfficialPricing *PlazaOfficialPricing
	// LongContextBasis 多档时的计价基准（整单 / 仅超出部分），单档为空。
	LongContextBasis ContextPricingBasis
	// TimePricing 计费会生效的分时倍率时段；无分时为 nil。
	TimePricing *TimePricingSchedule
	// FinalImagePriceTiers and FinalVideoPriceTiers identify tiers whose group
	// media price is already a customer-facing final price. The frontend must
	// not apply a group multiplier to them again.
	FinalImagePriceTiers []string
	FinalVideoPriceTiers []string
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 来自该分组关联渠道的
// 支持模型（普通分组按分组平台隔离，Composite 分组展开关联渠道已配置的
// 具体平台），与「可用渠道」页口径一致。
type PlazaGroup struct {
	ID                 int64
	Name               string
	Description        string
	Platform           string
	SubscriptionType   string
	RateMultiplier     float64
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsExclusive        bool
	// 图片按次实付倍率：ImageRateIndependent 为 true 时，图片计费模型的实付
	// = 档位价 × ImageRateMultiplier，不乘分组/用户专属倍率（与计费口径一致）。
	ImageRateIndependent bool
	ImageRateMultiplier  float64
	// VideoRateIndependent follows the same effective-rate rule as image
	// pricing, but applies to the per-second video price table.
	VideoRateIndependent bool
	VideoRateMultiplier  float64
	// LongContextPricingEnabled 分组是否按上下文长度应用阶梯价；关闭时模型展示的是最低档。
	LongContextPricingEnabled bool
	Models                    []PlazaModel
}

// ModelPlazaService 聚合模型广场数据。
//
// 模型枚举来自渠道配置；token 模型的展示单价与阶梯由 BillingService 的阶梯表
// 查询给出（与扣费走同一条解析链与计费函数），图片/按次模型沿用渠道/分组档位价。
type ModelPlazaService struct {
	channelRepo    ChannelRepository
	groupRepo      GroupRepository
	pricingService *PricingService
	billingService *BillingService
	resolver       *ModelPricingResolver
}

// NewModelPlazaService 创建模型广场服务。
func NewModelPlazaService(
	channelRepo ChannelRepository,
	groupRepo GroupRepository,
	pricingService *PricingService,
	billingService *BillingService,
	resolver *ModelPricingResolver,
) *ModelPlazaService {
	return &ModelPlazaService{
		channelRepo:    channelRepo,
		groupRepo:      groupRepo,
		pricingService: pricingService,
		billingService: billingService,
		resolver:       resolver,
	}
}

// ListGroups 返回模型广场数据：每个活跃分组附带其可用模型与定价。
//
// 模型枚举口径与 ListAvailable 一致（Active 渠道、SupportedModels ∪ 全局定价回落、
// 平台隔离），仅把顶层从渠道换成分组：
//   - 渠道按 lower(name) 排序后遍历，保证同名模型去重结果确定；
//   - 同分组同名模型「先见者胜」，仅当已存条目无定价而新条目有定价时升级替换；
//   - token 模型的单价与阶梯按实收口径合成（见 ResolveContextPricingSchedule），
//     图片计费模型的档位价按实收口径合成（见 plazaImageDisplayPricing）；
//   - 每个模型附带官方参考价（查不到为 nil）；
//   - 只返回 Models 非空的分组；分组按 RateMultiplier 升序（同倍率按名称），
//     组内模型按名称排序。
//
// 可见性过滤（专属分组）不在此层做，由 handler 按登录态裁剪。
func (s *ModelPlazaService) ListGroups(ctx context.Context) ([]PlazaGroup, error) {
	channels, err := s.channelRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	sort.SliceStable(channels, func(i, j int) bool {
		return strings.ToLower(channels[i].Name) < strings.ToLower(channels[j].Name)
	})

	type modelKey struct {
		platform string
		name     string
	}

	byGroup := make(map[int64]*PlazaGroup, len(groups))
	groupEnt := make(map[int64]*Group, len(groups))
	// modelIdx[groupID][platform+modelName] = index into byGroup[groupID].Models.
	// It is initialized from group configuration so a configured media price is
	// visible even before a channel explicitly advertises the model.
	modelIdx := make(map[int64]map[modelKey]int, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		pg := &PlazaGroup{
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
		}
		byGroup[g.ID] = pg
		groupEnt[g.ID] = g
		order = append(order, g.ID)

		configuredModels := plazaConfiguredModelNames(g)
		idx := make(map[modelKey]int, len(configuredModels))
		modelIdx[g.ID] = idx
		for _, configuredModel := range configuredModels {
			name := strings.TrimSpace(configuredModel)
			if name == "" {
				continue
			}
			platform := g.Platform
			if platform == PlatformComposite {
				detected, ok := DetectModelPlatform(name)
				if !ok || !isConcreteRequestPlatform(detected) {
					continue
				}
				platform = detected
			}
			key := modelKey{
				platform: strings.ToLower(strings.TrimSpace(platform)),
				name:     plazaModelIndexName(name),
			}
			if _, exists := idx[key]; exists {
				continue
			}
			idx[key] = len(pg.Models)
			pg.Models = append(pg.Models, PlazaModel{
				Name:                 plazaModelDisplayName(name),
				Platform:             platform,
				Pricing:              plazaMediaDisplayPricing(matchGroupModelPricing(g, name), g, name),
				FinalImagePriceTiers: plazaFinalImagePriceTiers(g, name),
				FinalVideoPriceTiers: plazaFinalVideoPriceTiers(g, name),
			})
		}
	}
	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		ch.normalizeBillingModelSource()
		supported := ch.SupportedModels()
		fillGlobalPricingFallback(s.pricingService, supported)

		for _, gid := range ch.GroupIDs {
			pg, ok := byGroup[gid]
			if !ok {
				continue
			}
			g := groupEnt[gid]
			idx := modelIdx[gid]
			for j := range supported {
				m := supported[j]
				if pg.Platform == PlatformComposite {
					if !isConcreteRequestPlatform(m.Platform) {
						continue
					}
				} else if m.Platform != pg.Platform {
					continue
				}
				groupPricing := matchGroupModelPricing(g, m.Name)
				pricing := groupPricing
				if pricing == nil {
					pricing = m.Pricing
				}
				pricing = plazaMediaDisplayPricing(pricing, g, m.Name)
				finalImagePriceTiers := plazaFinalImagePriceTiers(g, m.Name)
				finalVideoPriceTiers := plazaFinalVideoPriceTiers(g, m.Name)
				key := modelKey{
					platform: strings.ToLower(strings.TrimSpace(m.Platform)),
					name:     plazaModelIndexName(m.Name),
				}
				if at, seen := idx[key]; seen {
					// A group price always overrides. Otherwise retain the first
					// priced entry after the configured-model placeholder.
					if groupPricing != nil || (pg.Models[at].Pricing == nil && pricing != nil) {
						pg.Models[at].Pricing = pricing
					}
					pg.Models[at].FinalImagePriceTiers = finalImagePriceTiers
					pg.Models[at].FinalVideoPriceTiers = finalVideoPriceTiers
					continue
				}
				idx[key] = len(pg.Models)
				pg.Models = append(pg.Models, PlazaModel{
					Name:                 plazaModelDisplayName(m.Name),
					Platform:             m.Platform,
					Pricing:              pricing,
					FinalImagePriceTiers: finalImagePriceTiers,
					FinalVideoPriceTiers: finalVideoPriceTiers,
				})
			}
		}
	}

	officialMemo := make(map[string]*PlazaOfficialPricing)
	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool {
			if pg.Models[i].Name != pg.Models[j].Name {
				return pg.Models[i].Name < pg.Models[j].Name
			}
			return pg.Models[i].Platform < pg.Models[j].Platform
		})
		g := groupEnt[gid]
		for j := range pg.Models {
			s.fillDisplayPricing(ctx, &pg.Models[j], g)
			pg.Models[j].OfficialPricing = s.lookupOfficialPricing(ctx, pg.Models[j].Name, officialMemo)
		}
		out = append(out, *pg)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RateMultiplier != out[j].RateMultiplier {
			return out[i].RateMultiplier < out[j].RateMultiplier
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ListPlazaGroups exposes the model-plaza catalog for services that should not
// depend on handler-facing method names.
func (s *ModelPlazaService) ListPlazaGroups(ctx context.Context) ([]PlazaGroup, error) {
	return s.ListGroups(ctx)
}

// fillDisplayPricing 把模型的展示定价换成实收口径：
// token 模型取计费阶梯表（单价与档位均由真实计费函数得出），
// 图片/按次模型（或阶梯表不可用时）沿用渠道定价与分组图片档位价。
func (s *ModelPlazaService) fillDisplayPricing(ctx context.Context, m *PlazaModel, g *Group) {
	// Media prices are request prices, not token prices. Resolve them before the
	// token ladder so a per-model final media price cannot be overwritten by an
	// otherwise valid LiteLLM token schedule.
	if groupPricing := matchGroupModelPricing(g, m.Name); groupPricing != nil {
		m.Pricing = groupPricing
	}
	mediaPricing := plazaMediaDisplayPricing(m.Pricing, g, m.Name)
	if mediaPricing != nil && (mediaPricing.BillingMode == BillingModeImage || mediaPricing.BillingMode == BillingModeVideo) {
		m.Pricing = mediaPricing
		return
	}
	if s.billingService != nil && s.resolver != nil {
		sched, err := s.billingService.ResolveContextPricingSchedule(ctx, s.resolver, ContextPricingScheduleInput{
			Model:    m.Name,
			Group:    g,
			Platform: m.Platform,
		})
		if err == nil && sched != nil && len(sched.Tiers) > 0 {
			m.Pricing = plazaPricingFromSchedule(m.Pricing, sched)
			if len(sched.Tiers) > 1 {
				m.LongContextBasis = sched.Basis
			}
			m.TimePricing = sched.TimePricing
			return
		}
	}
	m.Pricing = mediaPricing
}

// plazaPricingFromSchedule 把阶梯表压成展示用的 ChannelModelPricing：
// 平价取首档单价，多档时 Intervals 逐档给出绝对单价；图片/按次字段沿用原始定价。
func plazaPricingFromSchedule(raw *ChannelModelPricing, sched *ContextPricingSchedule) *ChannelModelPricing {
	out := &ChannelModelPricing{BillingMode: BillingModeToken}
	if raw != nil {
		out.ImageInputPrice = raw.ImageInputPrice
		out.ImageOutputPrice = raw.ImageOutputPrice
		out.PerRequestPrice = raw.PerRequestPrice
		out.ReasoningEffortMultipliers = reasoningEffortMultipliersFromPricing(raw)
	}
	first := sched.Tiers[0]
	out.InputPrice = first.Input
	out.OutputPrice = first.Output
	out.CacheWritePrice = first.CacheWrite
	out.CacheWrite1hPrice = first.CacheWrite1h
	out.CacheReadPrice = first.CacheRead
	if len(sched.Tiers) > 1 {
		out.Intervals = plazaIntervalsFromTiers(sched.Tiers)
	}
	return out
}

func plazaIntervalsFromTiers(tiers []ContextPricingTier) []PricingInterval {
	intervals := make([]PricingInterval, 0, len(tiers))
	for i, t := range tiers {
		intervals = append(intervals, PricingInterval{
			MinTokens:         t.MinTokens,
			MaxTokens:         t.MaxTokens,
			TierLabel:         t.Label,
			InputPrice:        t.Input,
			OutputPrice:       t.Output,
			CacheWritePrice:   t.CacheWrite,
			CacheWrite1hPrice: t.CacheWrite1h,
			CacheReadPrice:    t.CacheRead,
			SortOrder:         i,
		})
	}
	return intervals
}

// plazaConfiguredModelNames combines configured models with model-specific
// media price keys. A final media price is meaningful by itself and should be
// present in the public catalog even while a channel's model list is catching up.
func plazaConfiguredModelNames(g *Group) []string {
	if g == nil {
		return nil
	}
	names := append([]string(nil), g.ModelAllowlist.Models...)

	imageNames := make([]string, 0, len(g.ImageModelPrices))
	for model, tiers := range g.ImageModelPrices {
		if len(tiers) == 0 {
			continue
		}
		if canonical := CanonicalImagePriceModel(model); canonical != "" {
			imageNames = append(imageNames, canonical)
		}
	}
	sort.Strings(imageNames)
	names = append(names, imageNames...)

	videoNames := make([]string, 0, len(g.VideoModelPrices))
	for model, tiers := range g.VideoModelPrices {
		if len(tiers) == 0 {
			continue
		}
		canonical := CanonicalGrokImagineVideoPriceFamily(model)
		if canonical == "" {
			canonical = strings.ToLower(strings.TrimSpace(model))
		}
		if canonical != "" {
			videoNames = append(videoNames, canonical)
		}
	}
	sort.Strings(videoNames)
	return append(names, videoNames...)
}

func plazaModelIndexName(model string) string {
	if family := CanonicalGrokImagineVideoPriceFamily(model); family != "" {
		return family
	}
	normalized := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(normalized, "image") {
		if canonical := CanonicalImagePriceModel(normalized); canonical != "" {
			return canonical
		}
	}
	return normalized
}

// plazaModelDisplayName keeps supported Grok video aliases callable while the
// catalog exposes one stable row per billing family.
func plazaModelDisplayName(model string) string {
	name := strings.TrimSpace(model)
	if family := CanonicalGrokImagineVideoPriceFamily(name); family != "" {
		return family
	}
	return name
}

func plazaMediaDisplayPricing(p *ChannelModelPricing, g *Group, model string) *ChannelModelPricing {
	return plazaVideoDisplayPricing(plazaImageDisplayPricing(p, g, model), g, model)
}

// plazaImageDisplayPricing builds effective image tiers. Per-model prices are
// already final USD/image values and therefore must not be multiplied again by
// the group or user rate. Flat group and channel prices retain their usual
// billing semantics.
func plazaImageDisplayPricing(p *ChannelModelPricing, g *Group, model string) *ChannelModelPricing {
	if g == nil {
		return p
	}
	modelPrices := map[string]*float64{
		ImageBillingSize1K: LookupImageModelPrice(g.ImageModelPrices, model, ImageBillingSize1K),
		ImageBillingSize2K: LookupImageModelPrice(g.ImageModelPrices, model, ImageBillingSize2K),
		ImageBillingSize4K: LookupImageModelPrice(g.ImageModelPrices, model, ImageBillingSize4K),
	}
	hasModelPrice := modelPrices[ImageBillingSize1K] != nil ||
		modelPrices[ImageBillingSize2K] != nil || modelPrices[ImageBillingSize4K] != nil
	hasFlatPrice := g.ImagePrice1K != nil || g.ImagePrice2K != nil || g.ImagePrice4K != nil
	if !hasModelPrice && (p == nil || p.BillingMode != BillingModeImage || !hasFlatPrice) {
		return p
	}
	channelTierPrice := func(label string) *float64 {
		if p == nil {
			return nil
		}
		for i := range p.Intervals {
			if p.Intervals[i].TierLabel == label && p.Intervals[i].PerRequestPrice != nil {
				return p.Intervals[i].PerRequestPrice
			}
		}
		return p.PerRequestPrice
	}
	tiers := []struct {
		label      string
		groupPrice *float64
	}{
		{ImageBillingSize1K, g.ImagePrice1K},
		{ImageBillingSize2K, g.ImagePrice2K},
		{ImageBillingSize4K, g.ImagePrice4K},
	}
	clone := ChannelModelPricing{BillingMode: BillingModeImage}
	if p != nil {
		clone = *p
	}
	if hasModelPrice {
		clone.BillingMode = BillingModeImage
	}
	clone.Intervals = make([]PricingInterval, 0, len(tiers))
	for i, tier := range tiers {
		price := modelPrices[tier.label]
		if price == nil {
			price = tier.groupPrice
		}
		// A model-specific final price makes the channel card irrelevant for
		// missing tiers; billing falls back to the group flat/default price.
		if price == nil && !hasModelPrice {
			price = channelTierPrice(tier.label)
		}
		if price == nil && hasModelPrice {
			if fallback, ok := getDefaultGrokImagineImagePrice(model, tier.label); ok {
				price = &fallback
			}
		}
		if price == nil {
			continue
		}
		value := *price
		clone.Intervals = append(clone.Intervals, PricingInterval{
			TierLabel:       tier.label,
			PerRequestPrice: &value,
			SortOrder:       i,
		})
	}
	return &clone
}

func plazaFinalImagePriceTiers(g *Group, model string) []string {
	if g == nil {
		return nil
	}
	tiers := make([]string, 0, 3)
	for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
		if LookupImageModelPrice(g.ImageModelPrices, model, tier) != nil {
			tiers = append(tiers, tier)
		}
	}
	if len(tiers) == 0 {
		return nil
	}
	return tiers
}

// plazaVideoDisplayPricing follows the image precedence for USD/second video
// tiers. A final per-model price wins, then group flat and channel prices, then
// the trusted Grok defaults needed for a partially configured family.
func plazaVideoDisplayPricing(p *ChannelModelPricing, g *Group, model string) *ChannelModelPricing {
	if g == nil {
		return p
	}
	modelPrices := map[string]*float64{
		VideoBillingResolution480P:  LookupVideoModelPrice(g.VideoModelPrices, model, VideoBillingResolution480P),
		VideoBillingResolution720P:  LookupVideoModelPrice(g.VideoModelPrices, model, VideoBillingResolution720P),
		VideoBillingResolution1080P: LookupVideoModelPrice(g.VideoModelPrices, model, VideoBillingResolution1080P),
	}
	hasModelPrice := modelPrices[VideoBillingResolution480P] != nil ||
		modelPrices[VideoBillingResolution720P] != nil || modelPrices[VideoBillingResolution1080P] != nil
	hasFlatPrice := g.VideoPrice480P != nil || g.VideoPrice720P != nil || g.VideoPrice1080P != nil
	if !hasModelPrice && (p == nil || p.BillingMode != BillingModeVideo || !hasFlatPrice) {
		return p
	}
	channelTierPrice := func(label string) *float64 {
		if p == nil {
			return nil
		}
		for i := range p.Intervals {
			if strings.EqualFold(strings.TrimSpace(p.Intervals[i].TierLabel), label) &&
				p.Intervals[i].PerRequestPrice != nil {
				return p.Intervals[i].PerRequestPrice
			}
		}
		return p.PerRequestPrice
	}
	tiers := []struct {
		label      string
		groupPrice *float64
	}{
		{VideoBillingResolution480P, g.VideoPrice480P},
		{VideoBillingResolution720P, g.VideoPrice720P},
		{VideoBillingResolution1080P, g.VideoPrice1080P},
	}
	clone := ChannelModelPricing{BillingMode: BillingModeVideo}
	if p != nil {
		clone = *p
	}
	if hasModelPrice {
		clone.BillingMode = BillingModeVideo
	}
	clone.Intervals = make([]PricingInterval, 0, len(tiers))
	for i, tier := range tiers {
		price := modelPrices[tier.label]
		if price == nil {
			price = tier.groupPrice
		}
		// Keep the catalog aligned with getVideoUnitPrice: once any model
		// specific final tier exists, never borrow another channel tier.
		if price == nil && !hasModelPrice {
			price = channelTierPrice(tier.label)
		}
		if price == nil && hasModelPrice {
			if fallback, ok := getDefaultGrokImagineVideoPrice(model, tier.label); ok {
				price = &fallback
			}
		}
		if price == nil {
			continue
		}
		value := *price
		clone.Intervals = append(clone.Intervals, PricingInterval{
			TierLabel:       tier.label,
			PerRequestPrice: &value,
			SortOrder:       i,
		})
	}
	return &clone
}

func plazaFinalVideoPriceTiers(g *Group, model string) []string {
	if g == nil {
		return nil
	}
	tiers := make([]string, 0, 3)
	for _, tier := range []string{
		VideoBillingResolution480P,
		VideoBillingResolution720P,
		VideoBillingResolution1080P,
	} {
		if LookupVideoModelPrice(g.VideoModelPrices, model, tier) != nil {
			tiers = append(tiers, tier)
		}
	}
	if len(tiers) == 0 {
		return nil
	}
	return tiers
}

// lookupOfficialPricing 查询模型的官方参考价（与计费同源：LiteLLM → 内置兜底 → 模型策略），
// 带 memo 避免同名模型重复解析。官方阶梯按无分组、无渠道的口径查阶梯表。
// 生产路径由 BillingService 保证展示和实际计费使用同一份模型策略；仅在
// 轻量目录场景没有 BillingService 时，退化为直接读取 PricingService 的目录价格。
func (s *ModelPlazaService) lookupOfficialPricing(ctx context.Context, modelName string, memo map[string]*PlazaOfficialPricing) *PlazaOfficialPricing {
	if cached, ok := memo[modelName]; ok {
		return cached
	}
	var result *PlazaOfficialPricing
	if s.billingService != nil {
		result = s.lookupOfficialPricingWithBilling(ctx, modelName)
	} else if s.pricingService != nil {
		result = plazaOfficialPricingFromCatalog(s.pricingService.GetModelPricing(modelName))
	}
	memo[modelName] = result
	return result
}

func (s *ModelPlazaService) lookupOfficialPricingWithBilling(ctx context.Context, modelName string) *PlazaOfficialPricing {
	var result *PlazaOfficialPricing
	if mp, err := s.billingService.GetModelPricing(modelName); err == nil && mp != nil {
		result = &PlazaOfficialPricing{
			InputPrice:      nonZeroPtr(mp.InputPricePerToken),
			OutputPrice:     nonZeroPtr(mp.OutputPricePerToken),
			CacheWritePrice: nonZeroPtr(mp.CacheCreationPricePerToken),
			CacheReadPrice:  nonZeroPtr(mp.CacheReadPricePerToken),
		}
		// 计费只在支持 5m/1h 分档时使用 1h 价，其余情况 1h 价对用户无意义。
		if mp.SupportsCacheBreakdown {
			result.CacheWrite1hPrice = nonZeroPtr(mp.CacheCreation1hPrice)
		}
		if s.resolver != nil {
			sched, schedErr := s.billingService.ResolveContextPricingSchedule(ctx, s.resolver, ContextPricingScheduleInput{Model: modelName})
			if schedErr == nil && sched != nil && len(sched.Tiers) > 1 {
				result.Intervals = plazaIntervalsFromTiers(sched.Tiers)
			}
		}
		if result.InputPrice == nil && result.OutputPrice == nil && result.CacheWritePrice == nil &&
			result.CacheWrite1hPrice == nil && result.CacheReadPrice == nil && len(result.Intervals) == 0 {
			result = nil
		}
	}
	return result
}

func plazaOfficialPricingFromCatalog(mp *LiteLLMModelPricing) *PlazaOfficialPricing {
	if mp == nil || mp.TokenPricingAbsent {
		return nil
	}
	result := &PlazaOfficialPricing{
		InputPrice:      nonZeroPtr(mp.InputCostPerToken),
		OutputPrice:     nonZeroPtr(mp.OutputCostPerToken),
		CacheWritePrice: nonZeroPtr(mp.CacheCreationInputTokenCost),
		CacheReadPrice:  nonZeroPtr(mp.CacheReadInputTokenCost),
	}
	// Keep the display consistent with unified billing: the 1h cache tier is
	// only meaningful when it is explicitly more expensive than the 5m tier.
	if mp.CacheCreationInputTokenCostAbove1hr > mp.CacheCreationInputTokenCost {
		result.CacheWrite1hPrice = nonZeroPtr(mp.CacheCreationInputTokenCostAbove1hr)
	}
	if result.InputPrice == nil && result.OutputPrice == nil && result.CacheWritePrice == nil &&
		result.CacheWrite1hPrice == nil && result.CacheReadPrice == nil {
		return nil
	}
	return result
}
