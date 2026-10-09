package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// The browser submits pricing intent. Only server-owned catalog / previously
// persisted prices may supply an inherited price; an empty input is never zero.
func (s *adminServiceImpl) normalizeSavedGroupModelPricing(ctx context.Context, platform string, pricing, previous []ChannelModelPricing) ([]ChannelModelPricing, error) {
	if platform != PlatformVideo {
		return normalizeGroupModelPricing(platform, pricing)
	}
	needsCatalog := false
	for _, p := range pricing {
		if p.Video != nil && p.Video.PriceSource != "" {
			needsCatalog = true
		}
	}
	cfg := DefaultVideoRuntimeSettings()
	cat := VideoSupplierCatalog{}
	if needsCatalog {
		var err error
		cfg, err = s.settingService.GetVideoRuntime(ctx)
		if err != nil {
			return nil, err
		}
		cat, err = s.settingService.GetVideoCatalog(ctx)
		if err != nil {
			return nil, err
		}
	}
	out, err := prepareVideoPricingForSave(pricing, previous, cat, cfg)
	if err != nil {
		return nil, err
	}
	return normalizeGroupModelPricing(platform, out)
}

func prepareVideoPricingForSave(pricing, previous []ChannelModelPricing, cat VideoSupplierCatalog, cfg VideoRuntimeSettings) ([]ChannelModelPricing, error) {
	oldByModel := make(map[string]ChannelModelPricing, len(previous))
	for _, p := range previous {
		if len(p.Models) == 1 {
			oldByModel[strings.TrimSpace(p.Models[0])] = p
		}
	}
	out := make([]ChannelModelPricing, len(pricing))
	for i, submitted := range pricing {
		p := submitted.Clone()
		out[i] = p
		if p.Video == nil {
			continue
		}
		v := *p.Video
		p.Video = &v
		// Selling prices are administrator input; supplier metadata is not.
		// This also covers legacy callers that omit price_source entirely.
		v.Cost, v.SyncedAt, v.AccountSynced = nil, "", false
		if len(p.Models) == 1 {
			model := strings.TrimSpace(p.Models[0])
			if old, ok := oldByModel[model]; ok && old.Video != nil {
				v.Cost, v.SyncedAt, v.AccountSynced = old.Video.Cost, old.Video.SyncedAt, old.Video.AccountSynced
			}
			costModel := model
			if v.Cost != nil && v.Cost.Model != "" {
				costModel = v.Cost.Model
			}
			if cost, ok := cat.Models[costModel]; ok && cost.UnavailableReason == "" && cost.Spec != nil && (cost.Spec.Supported || v.AccountSynced) {
				if v.AccountSynced {
					cost, _ = selectedVideoCost(cost)
				}
				v.Cost, v.SyncedAt = &cost, cat.LastSuccess
			}
		}
		out[i] = p
		switch v.PriceSource {
		case "": // Existing integrations retain their pre-controls contract.
			continue
		case "manual":
			v.AutoPrice = false
			v.AppliedMarkup = VideoMarkup{}
			out[i] = p
			continue
		case "upstream":
		default:
			return nil, fmt.Errorf("未知的视频定价方式")
		}
		if len(p.Models) != 1 {
			return nil, fmt.Errorf("请选择一个确切的视频型号")
		}
		model := strings.TrimSpace(p.Models[0])
		old, exists := oldByModel[model]
		costModel := model
		if v.Cost != nil && v.Cost.Model != "" {
			costModel = v.Cost.Model
		}
		cost, available := cat.Models[costModel]
		if available && v.AccountSynced {
			cost, available = selectedVideoCost(cost)
		}
		// A failed refresh may retain the last authoritative supplier cost. Never
		// trust a cost/spec/markup snapshot sent back by a stale browser form.
		if !available && exists && old.Video != nil && old.Video.Cost != nil {
			cost, available = *old.Video.Cost, true
		}
		if !available || cost.Spec == nil || !cost.Spec.Supported || cost.UnavailableReason != "" {
			return nil, fmt.Errorf("型号 %s 没有已核实的上游价格，请先读取价格或保留原定价", model)
		}
		if err := cost.Spec.Validate(); err != nil {
			return nil, err
		}
		v.Spec = *cost.Spec
		initial := !exists || old.Video == nil || old.PerRequestPrice == nil || old.Video.PriceSource == "manual" || (old.Video.PriceSource == "" && !old.Video.AutoPrice)
		if !initial {
			p.PerRequestPrice, p.BillingMode = old.PerRequestPrice, old.BillingMode
			v.ReferencePerSecond, v.ResolutionPrices = old.Video.ReferencePerSecond, old.Video.ResolutionPrices
			v.AppliedMarkup = old.Video.AppliedMarkup
		} else {
			p.PerRequestPrice = nil
			p.BillingMode = cost.BillingMode
			v.ReferencePerSecond, v.ResolutionPrices = 0, nil
			v.AppliedMarkup = VideoMarkup{}
		}
		// Saving a rule applies it once; AutoPrice controls subsequent scheduled
		// updates. Direction switches still protect an inherited existing price.
		updated, err := repriceVideoEntry(p, cost, cfg, time.Now().UTC().Format(time.RFC3339), initial)
		if err != nil {
			return nil, fmt.Errorf("型号 %s：%w", model, err)
		}
		out[i] = updated
	}
	return out, nil
}
