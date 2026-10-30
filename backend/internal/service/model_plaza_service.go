package service

import (
	"context"
	"fmt"
	"maps"
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
	Name                 string
	Platform             string
	Pricing              *ChannelModelPricing
	OfficialPricing      *PlazaOfficialPricing
	FinalImagePriceTiers []string
	FinalVideoPriceTiers []string
	// LongContextBasis 多档时的计价基准（整单 / 仅超出部分），单档为空。
	LongContextBasis ContextPricingBasis
	// TimePricing 计费会生效的分时倍率时段；无分时为 nil。
	TimePricing *TimePricingSchedule
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 合并该分组的
// 网关账号目录与关联渠道模型，普通分组按平台隔离，Composite 分组保留具体平台。
type PlazaGroup struct {
	ID                 int64
	Name               string
	Description        string
	I18n               GroupI18n
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
	// 视频独立倍率与图片独立倍率分别配置，开启时覆盖非最终价档位的分组/用户专属倍率。
	VideoRateIndependent bool
	VideoRateMultiplier  float64
	// LongContextPricingEnabled 分组是否按上下文长度应用阶梯价；关闭时模型展示的是最低档。
	LongContextPricingEnabled bool
	Models                    []PlazaModel
}

// ModelPlazaService 聚合模型广场数据。
//
// 模型枚举来自网关账号目录及渠道配置；token 模型的展示单价与阶梯由 BillingService 的阶梯表
// 查询给出（与扣费走同一条解析链与计费函数），图片/按次模型沿用渠道/分组档位价。
type ModelPlazaService struct {
	channelRepo    ChannelRepository
	groupRepo      GroupRepository
	pricingService *PricingService
	billingService *BillingService
	resolver       *ModelPricingResolver
	gatewayService *GatewayService
}

// NewModelPlazaService 创建模型广场服务。
func NewModelPlazaService(
	channelRepo ChannelRepository,
	groupRepo GroupRepository,
	pricingService *PricingService,
	billingService *BillingService,
	resolver *ModelPricingResolver,
	gatewayService *GatewayService,
) *ModelPlazaService {
	return &ModelPlazaService{
		channelRepo:    channelRepo,
		groupRepo:      groupRepo,
		pricingService: pricingService,
		billingService: billingService,
		resolver:       resolver,
		gatewayService: gatewayService,
	}
}

// ListGroups 返回模型广场数据：每个活跃分组附带其可用模型与定价。
//
// 合并网关模型目录与 Active 渠道专用模型，按分组平台和模型白名单隔离：
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

	byGroup := make(map[int64]*PlazaGroup, len(groups))
	groupEnt := make(map[int64]*Group, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		byGroup[g.ID] = &PlazaGroup{
			ID:                        g.ID,
			Name:                      g.Name,
			Description:               g.Description,
			I18n:                      g.I18n,
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
		groupEnt[g.ID] = g
		order = append(order, g.ID)
	}

	type modelKey struct {
		platform string
		name     string
	}
	// modelIdx[groupID][platform+modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[modelKey]int, len(groups))
	for i := range groups {
		g := &groups[i]
		pg := byGroup[g.ID]
		idx := make(map[modelKey]int)
		modelIdx[g.ID] = idx
		configuredModels := plazaConfiguredModelNames(g)
		if g.Platform != PlatformVideo && (s.gatewayService == nil || s.gatewayService.accountRepo == nil) {
			// Preserve standalone catalog callers; an account-aware gateway uses
			// accountModels below to enforce account-group model restrictions.
			configuredModels = append(configuredModels, g.ModelAllowlist.Models...)
		}
		for _, configuredModel := range configuredModels {
			name := strings.TrimSpace(configuredModel)
			if name == "" {
				continue
			}
			platform := g.Platform
			indexName, displayName := plazaModelIndexName(name), plazaModelDisplayName(name)
			if platform == PlatformVideo {
				indexName, displayName = name, name
			}
			if platform == PlatformComposite {
				detected, ok := DetectModelPlatform(name)
				if !ok || !isConcreteRequestPlatform(detected) {
					continue
				}
				platform = detected
			}
			key := modelKey{
				platform: strings.ToLower(strings.TrimSpace(platform)),
				name:     indexName,
			}
			if _, exists := idx[key]; exists {
				continue
			}
			idx[key] = len(pg.Models)
			pg.Models = append(pg.Models, PlazaModel{
				Name:                 displayName,
				Platform:             platform,
				Pricing:              matchGroupModelPricing(g, name),
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
			if g.Platform == PlatformVideo {
				continue // This group advertises only its exact authoritative price cards.
			}
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
				key := modelKey{
					platform: strings.ToLower(strings.TrimSpace(m.Platform)),
					name:     plazaModelIndexName(m.Name),
				}
				if at, seen := idx[key]; seen {
					if groupPricing != nil || (pg.Models[at].Pricing == nil && pricing != nil) {
						pg.Models[at].Pricing = pricing
					}
					pg.Models[at].FinalImagePriceTiers = plazaFinalImagePriceTiers(g, m.Name)
					pg.Models[at].FinalVideoPriceTiers = plazaFinalVideoPriceTiers(g, m.Name)
					continue
				}
				idx[key] = len(pg.Models)
				pg.Models = append(pg.Models, PlazaModel{
					Name:                 plazaModelDisplayName(m.Name),
					Platform:             m.Platform,
					Pricing:              pricing,
					FinalImagePriceTiers: plazaFinalImagePriceTiers(g, m.Name),
					FinalVideoPriceTiers: plazaFinalVideoPriceTiers(g, m.Name),
				})
			}
		}
	}

	officialMemo := make(map[string]*PlazaOfficialPricing)
	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		g := groupEnt[gid]
		// 渠道提供专用模型与定价；账号模型目录补齐未配置渠道的普通分组。
		idx := modelIdx[gid]
		if idx == nil {
			idx = make(map[modelKey]int)
		}
		accountModels, err := s.accountModels(ctx, g)
		if err != nil {
			return nil, fmt.Errorf("list account models for group %d: %w", gid, err)
		}
		for _, m := range accountModels {
			key := modelKey{platform: m.Platform, name: m.Name}
			if _, seen := idx[key]; seen {
				continue
			}
			idx[key] = len(pg.Models)
			pg.Models = append(pg.Models, PlazaModel{Name: m.Name, Platform: m.Platform})
		}
		// 渠道模型同样遵守分组准入限制；通配模式不能作为可调用模型卡片展示。
		models := make([]PlazaModel, 0, len(pg.Models))
		for _, m := range pg.Models {
			if strings.TrimSpace(m.Name) != "" && !strings.Contains(m.Name, "*") && g.ModelAllowlist.Allows(m.Name) {
				models = append(models, m)
			}
		}
		pg.Models = models
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool {
			if pg.Models[i].Name != pg.Models[j].Name {
				return pg.Models[i].Name < pg.Models[j].Name
			}
			return pg.Models[i].Platform < pg.Models[j].Platform
		})
		for j := range pg.Models {
			s.fillDisplayPricing(ctx, &pg.Models[j], g)
			if g.Platform != PlatformVideo && ImageGroupModelConfig(g, pg.Models[j].Name) == nil {
				pg.Models[j].OfficialPricing = s.lookupOfficialPricing(ctx, pg.Models[j].Name, officialMemo)
			}
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

// accountModels 使用网关的账号枚举和平台目录，不向上游发起模型发现请求。
// 无可调度账号的平台不生成目录；现有渠道条目仍独立展示。
func (s *ModelPlazaService) accountModels(ctx context.Context, g *Group) ([]SupportedModel, error) {
	if s.gatewayService == nil || s.gatewayService.accountRepo == nil {
		return nil, nil
	}
	accounts, err := s.gatewayService.accountRepo.ListSchedulableByGroupID(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	platforms := make(map[string]struct{})
	for i := range accounts {
		platform := accounts[i].Platform
		if g.Platform != PlatformComposite {
			if platform != g.Platform && !mixedListingAccountAllowed(g.Platform, &accounts[i]) {
				continue
			}
			platform = g.Platform
		}
		platforms[platform] = struct{}{}
	}
	var out []SupportedModel
	for platform := range platforms {
		ids := s.gatewayService.GetAvailableModels(ctx, &g.ID, platform)
		// 复用 /v1/models 对未映射账号的平台目录；CN 供应商仅枚举账号映射。
		if len(ids) == 0 && (platform != PlatformOpenAI || ids == nil) && !IsMultiProtocolAPIKeyProvider(platform) {
			ids = DefaultModelIDsForPlatform(platform)
		}
		if g.ModelAllowlistEnabled() {
			source := ids
			if !IsMultiProtocolAPIKeyProvider(platform) {
				source = ModelListingSource(platform, ids, DefaultModelIDsForPlatform(platform))
			}
			ids = g.ModelAllowlist.FilterForListing(source)
		}
		for _, id := range ids {
			// 默认目录及白名单展开仍须有账号能够在本分组服务该模型，
			// 不能把空枚举或平台默认值当成账号分组限制的豁免。
			for i := range accounts {
				account := &accounts[i]
				if account.Platform != platform && (!mixedListingAccountAllowed(platform, account) || !mixedListingModelAllowed(platform, id)) {
					continue
				}
				if account.IsModelSupportedInGroup(&g.ID, id) {
					out = append(out, SupportedModel{Name: id, Platform: platform})
					break
				}
			}
		}
	}
	return out, nil
}

// fillDisplayPricing 把模型的展示定价换成实收口径：
// token 模型取计费阶梯表（单价与档位均由真实计费函数得出），
// 图片/按次模型（或阶梯表不可用时）沿用渠道定价与分组图片档位价。
func (s *ModelPlazaService) fillDisplayPricing(ctx context.Context, m *PlazaModel, g *Group) {
	if groupPricing := matchGroupModelPricing(g, m.Name); groupPricing != nil {
		m.Pricing = groupPricing
		// 分组模型级固定价就是实收价，不能再被分组图片兜底档位覆盖。
		// 否则模型广场会显示 image_price_1k/2k/4k，而实际按 model_pricing 扣费。
		if groupPricing.BillingMode == BillingModeImage || groupPricing.BillingMode == BillingModePerRequest {
			return
		}
	}
	mediaPricing := plazaMediaDisplayPricing(m.Pricing, g, m.Name)
	if mediaPricing != nil && mediaPricing.BillingMode != BillingModeToken {
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

// plazaConfiguredModelNames combines configured models with per-model media
// prices so a deliberately priced media model remains visible before a channel
// has advertised it in its supported-model list.
func plazaConfiguredModelNames(g *Group) []string {
	if g == nil {
		return nil
	}
	if g.Platform == PlatformVideo {
		return VideoGroupModelIDs(g)
	}
	names := []string{}
	// Keep supplier display IDs intact; billing lookup may canonicalize, but
	// customers must copy the exact model name accepted by the upstream.
	for _, p := range g.ModelPricing {
		if p.Image != nil && p.Image.Provider == "suanli" {
			names = append(names, p.Models...)
		}
	}
	// An allowlist limits eligible models; it does not create an upstream
	// capability or override account-group restrictions. Media prices above
	// and below still publish explicitly configured media models.

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

func plazaModelDisplayName(model string) string {
	name := strings.TrimSpace(model)
	if family := CanonicalGrokImagineVideoPriceFamily(name); family != "" {
		return family
	}
	return name
}

func plazaMediaDisplayPricing(p *ChannelModelPricing, g *Group, model string) *ChannelModelPricing {
	if g != nil && g.Platform == PlatformVideo {
		price, err := ResolveVideoGroupPrice(g, model)
		if err != nil {
			return nil
		}
		// Exact video model cells already contain the final customer unit price.
		// Clients must not apply the group's token or legacy media multipliers.
		display := &ChannelModelPricing{Platform: PlatformVideo, Models: []string{price.Model}, BillingMode: price.BillingMode, PerRequestPrice: &price.UnitPrice}
		if price.Video != nil {
			// Carry only the effective selling prices and public input contract.
			// Supplier costs and pricing controls never enter the plaza projection.
			display.Video = &VideoModelConfig{
				Spec:               price.Video.Spec,
				ReferencePerSecond: price.Video.ReferencePerSecond,
				ResolutionPrices:   maps.Clone(price.Video.ResolutionPrices),
			}
		}
		return display
	}
	return plazaVideoDisplayPricing(plazaImageDisplayPricing(p, g, model), g, model)
}

// plazaImageDisplayPricing preserves the site's final per-model USD prices;
// those tiers bypass rate multipliers in both billing and the catalog.
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
	if !hasModelPrice && ImageGroupModelConfig(g, model) != nil {
		// Supplier image billing requires exact model cells. Never advertise a
		// stale channel fallback while a cleared override awaits synchronization.
		clone := ChannelModelPricing{BillingMode: BillingModeImage}
		if p != nil {
			clone = *p
		}
		clone.PerRequestPrice = nil
		clone.Intervals = nil
		return &clone
	}
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
