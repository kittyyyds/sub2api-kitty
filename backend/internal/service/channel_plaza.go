package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// PlazaOfficialPricing 模型广场展示用的 LiteLLM 官方参考价（USD per token）。
// 字段为 nil 表示官方数据中该项缺失（0 视为未配置）。
type PlazaOfficialPricing struct {
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64 // 5m 缓存写入（= LiteLLM cache_creation）
	CacheWrite1hPrice *float64 // 1h 缓存写入（LiteLLM cache_creation_above_1hr）
	CacheReadPrice    *float64
}

// PlazaModel 模型广场中单个模型条目：渠道定价 + 官方参考价。
type PlazaModel struct {
	Name                 string
	Platform             string
	Pricing              *ChannelModelPricing
	OfficialPricing      *PlazaOfficialPricing
	FinalImagePriceTiers []string
	FinalVideoPriceTiers []string
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 以分组模型清单为基线，
// 再合并该分组关联渠道的支持模型。普通分组按分组平台隔离，Composite 分组展开为
// 可确定识别的具体平台。
type PlazaGroup struct {
	ID                   int64
	Name                 string
	Description          string
	Platform             string
	SubscriptionType     string
	RateMultiplier       float64
	ImageRateIndependent bool
	ImageRateMultiplier  float64
	VideoRateIndependent bool
	VideoRateMultiplier  float64
	PeakRateEnabled      bool
	PeakStart            string
	PeakEnd              string
	PeakRateMultiplier   float64
	IsExclusive          bool
	Models               []PlazaModel
}

// ListPlazaGroups 返回模型广场数据：每个活跃分组附带其可用模型与定价。
//
// 聚合口径以分组配置为基线，再合并 ListAvailable 的 Active 渠道模型：
//   - ModelsListConfig.Models 与已配置独立媒体价的模型都参与目录生成；
//   - 模型展示定价优先级为分组定价 > 渠道定价 > LiteLLM 官方参考价；
//   - 渠道按 lower(name) 排序后遍历，保证同名模型去重结果确定；
//   - 同分组同平台同名模型按大小写不敏感去重；
//   - 图片和视频计费模型的档位价按实收口径合成；按模型价格是美元最终单张/每秒价，
//     通用视频价与渠道视频价仍是按秒价（见 plazaMediaDisplayPricing）；
//   - 每个模型附带 LiteLLM 官方参考价（查不到为 nil）；
//   - 只返回 Models 非空的分组；分组按 RateMultiplier 升序（同倍率按名称），
//     组内模型按名称排序。
//
// 可见性过滤（专属分组）不在此层做，由 handler 按登录态裁剪。
func (s *ChannelService) ListPlazaGroups(ctx context.Context) ([]PlazaGroup, error) {
	channels, err := s.repo.ListAll(ctx)
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
	// modelIdx[groupID][platform+modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[modelKey]int, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		pg := &PlazaGroup{
			ID:                   g.ID,
			Name:                 g.Name,
			Description:          g.Description,
			Platform:             g.Platform,
			SubscriptionType:     g.SubscriptionType,
			RateMultiplier:       g.RateMultiplier,
			ImageRateIndependent: g.ImageRateIndependent,
			ImageRateMultiplier:  g.ImageRateMultiplier,
			VideoRateIndependent: g.VideoRateIndependent,
			VideoRateMultiplier:  g.VideoRateMultiplier,
			PeakRateEnabled:      g.PeakRateEnabled,
			PeakStart:            g.PeakStart,
			PeakEnd:              g.PeakEnd,
			PeakRateMultiplier:   g.PeakRateMultiplier,
			IsExclusive:          g.IsExclusive,
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
			displayName := plazaModelDisplayName(name)
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
				Name:                 displayName,
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
		s.fillGlobalPricingFallback(supported)

		for _, gid := range ch.GroupIDs {
			pg, ok := byGroup[gid]
			if !ok {
				continue
			}
			g := groupEnt[gid]
			idx := modelIdx[gid]
			for j := range supported {
				m := supported[j]
				displayName := plazaModelDisplayName(m.Name)
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
					// 分组定价始终覆盖；否则只用渠道价格补齐无价条目。
					if groupPricing != nil || (pg.Models[at].Pricing == nil && pricing != nil) {
						pg.Models[at].Pricing = pricing
					}
					pg.Models[at].FinalImagePriceTiers = finalImagePriceTiers
					pg.Models[at].FinalVideoPriceTiers = finalVideoPriceTiers
					continue
				}
				idx[key] = len(pg.Models)
				pg.Models = append(pg.Models, PlazaModel{
					Name:                 displayName,
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
		for j := range pg.Models {
			pg.Models[j].OfficialPricing = s.lookupOfficialPricing(pg.Models[j].Name, officialMemo)
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

// plazaConfiguredModelNames 把分组模型清单和独立媒体价格键合并为广场候选模型。
// 媒体价格已经是客户最终价，即使渠道的模型列表暂未写入该模型，
// 价格页也应能展示。返回顺序确定，最终去重仍由 ListPlazaGroups 完成。
func plazaConfiguredModelNames(g *Group) []string {
	if g == nil {
		return nil
	}
	names := append([]string(nil), g.ModelsListConfig.Models...)

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

// plazaModelDisplayName keeps accepted Grok video aliases callable while the
// customer-facing catalog exposes one stable model name per billing family.
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

// plazaImageDisplayPricing 为图片计费模型合成展示定价，使档位价与实收口径一致：
// 每档（1K/2K/4K）单价 = 按模型美元最终价 > 分组图片价 > 渠道同档位价 >
// 渠道默认按次价 > Grok 模型默认价，无价的档不展示。显式模型价可把原本
// 为 token 模式的目录条目提升为图片模式。返回克隆，不修改缓存共享入参。
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
		{"1K", g.ImagePrice1K},
		{"2K", g.ImagePrice2K},
		{"4K", g.ImagePrice4K},
	}
	clone := ChannelModelPricing{BillingMode: BillingModeImage}
	if p != nil {
		clone = *p
	}
	if hasModelPrice {
		clone.BillingMode = BillingModeImage
	}
	clone.Intervals = make([]PricingInterval, 0, len(tiers))
	for i, t := range tiers {
		price := modelPrices[t.label]
		if price == nil {
			price = t.groupPrice
		}
		if price == nil {
			price = channelTierPrice(t.label)
		}
		if price == nil && hasModelPrice {
			if fallback, ok := getDefaultGrokImagineImagePrice(model, t.label); ok {
				price = &fallback
			}
		}
		if price == nil {
			continue
		}
		v := *price
		clone.Intervals = append(clone.Intervals, PricingInterval{
			TierLabel:       t.label,
			PerRequestPrice: &v,
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

// plazaVideoDisplayPricing 为视频模型合成客户实际看到的每秒档位价格：按模型配置是
// 美元最终每秒价；分组平面价、渠道价和内置默认价仍需应用有效倍率。FinalVideoPriceTiers
// 让前端避免对模型最终每秒价再次叠加倍率，视频时长始终参与总价计算。
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
		if price == nil {
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

// lookupOfficialPricing 查询模型的 LiteLLM 官方参考价，带 memo 避免同名模型重复转换。
// pricingService 为 nil（测试场景）或查不到时返回 nil。
func (s *ChannelService) lookupOfficialPricing(modelName string, memo map[string]*PlazaOfficialPricing) *PlazaOfficialPricing {
	if s.pricingService == nil {
		return nil
	}
	if cached, ok := memo[modelName]; ok {
		return cached
	}
	var result *PlazaOfficialPricing
	if lp := s.pricingService.GetModelPricing(modelName); lp != nil && !lp.TokenPricingAbsent {
		result = &PlazaOfficialPricing{
			InputPrice:        nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:       nonZeroPtr(lp.OutputCostPerToken),
			CacheWritePrice:   nonZeroPtr(lp.CacheCreationInputTokenCost),
			CacheWrite1hPrice: nonZeroPtr(lp.CacheCreationInputTokenCostAbove1hr),
			CacheReadPrice:    nonZeroPtr(lp.CacheReadInputTokenCost),
		}
		if result.InputPrice == nil && result.OutputPrice == nil &&
			result.CacheWritePrice == nil && result.CacheWrite1hPrice == nil && result.CacheReadPrice == nil {
			result = nil
		}
	}
	memo[modelName] = result
	return result
}
