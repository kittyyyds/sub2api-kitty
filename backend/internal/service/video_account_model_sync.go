package service

import (
	"context"
	"net/url"
	"sort"
	"strings"
)

// Only these non-secret account fields are needed. Never load API credentials
// into the catalog, pricing JSON or diagnostic messages.
type VideoAccountModelBinding struct {
	BaseURL       string
	Models        map[string]string
	AllowedModels []string
}

type VideoAccountModelRepository interface {
	ListVideoAccountModelBindings(context.Context, int64) ([]VideoAccountModelBinding, error)
}

func videoAccountSelectedModels(bindings []VideoAccountModelBinding) (map[string]string, map[string]bool) {
	selected, conflicts := map[string]string{}, map[string]bool{}
	for _, b := range bindings {
		u, err := url.Parse(strings.TrimSpace(b.BaseURL))
		validProvider := err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), "suanliai.top") &&
			(u.Port() == "" || u.Port() == "443") && u.User == nil
		a := &Account{AccountGroups: []AccountGroup{{GroupID: 1, AllowedModels: b.AllowedModels}}}
		id := int64(1)
		for model, upstream := range b.Models {
			if model == "" || strings.TrimSpace(model) != model || strings.ContainsAny(model, "*?") || !a.IsModelAllowedInGroup(&id, model) {
				continue
			}
			if upstream == "" {
				upstream = model
			}
			if !validProvider || strings.ContainsAny(upstream, "*?") {
				conflicts[model] = true
				continue
			}
			if old, ok := selected[model]; ok && old != upstream {
				conflicts[model] = true
			}
			selected[model] = upstream
		}
	}
	return selected, conflicts
}

// A configured account ID is the administrator's availability decision.
// Preserve documented parameter bounds; a static review flag is not a second
// allowlist. Invalid pricing/protocol metadata still cannot create a free task.
func selectedVideoCost(cost VideoSupplierPrice) (VideoSupplierPrice, bool) {
	if cost.UnavailableReason != "" || cost.Spec == nil {
		return cost, false
	}
	spec := *cost.Spec
	spec.Supported = true
	if !spec.Generic {
		spec.Reason = ""
	}
	if spec.Validate() != nil {
		// An incomplete static parameter row is not another availability veto.
		// Use the same bounded generic transport as a newly added supplier ID.
		spec = genericVideoSpecification()
		if !cost.ReferencePriceKnown {
			spec.MaxVideos, spec.MaxReferenceSeconds, spec.ReferencePricePending = 0, 0, true
		}
	}
	cost.Spec = &spec
	return cost, true
}

func reconcileVideoAccountPricing(previous []ChannelModelPricing, bindings []VideoAccountModelBinding, models map[string]VideoSupplierPrice, cfg VideoRuntimeSettings, at string, reprice bool) ([]ChannelModelPricing, bool) {
	selected, conflicts := videoAccountSelectedModels(bindings)
	next := make([]ChannelModelPricing, 0, len(previous)+len(selected))
	existing := map[string]bool{}
	warning := len(conflicts) > 0
	for _, entry := range previous {
		for _, m := range entry.Models {
			existing[m] = true
		}
		if entry.Video == nil || len(entry.Models) != 1 || entry.Video.PriceSource == "manual" || !entry.Video.AutoPrice {
			next = append(next, entry)
			continue
		}
		model := entry.Models[0]
		upstream, bound := selected[model]
		if cfg.AutoImportAccountModels && entry.Video.AccountSynced && !bound && !conflicts[model] {
			continue // Remove only auto-managed entries, never a manual price.
		}
		if cfg.AutoImportAccountModels && conflicts[model] {
			next = append(next, entry)
			continue
		}
		if !bound || !cfg.AutoImportAccountModels {
			upstream = model
			if entry.Video.Cost != nil {
				upstream = entry.Video.Cost.Model
			}
		}
		cost, ok := models[upstream]
		managed := cfg.AutoImportAccountModels && bound
		if managed && ok {
			cost, ok = selectedVideoCost(cost)
		}
		if !ok || cost.UnavailableReason != "" {
			warning = true
			next = append(next, entry)
			continue
		}
		// A remapped ID is a different product, not a price fluctuation of the
		// previous product. Recompute its initial price using existing controls.
		remapped := managed && entry.Video.Cost != nil && entry.Video.Cost.Model != upstream
		candidate := entry.Clone()
		v := *entry.Video
		candidate.Video = &v
		if managed {
			v.AccountSynced, v.Spec = true, *cost.Spec
		}
		if reprice || remapped {
			updated, err := repriceVideoEntry(candidate, cost, cfg, at, remapped)
			if err != nil {
				warning = true
				next = append(next, entry)
				continue
			}
			candidate = updated
		}
		if validateVideoGroupPricing([]ChannelModelPricing{candidate}) != nil {
			warning = true
			next = append(next, entry)
			continue
		}
		next = append(next, candidate)
	}
	if cfg.AutoImportAccountModels {
		keys := make([]string, 0, len(selected))
		for model := range selected {
			keys = append(keys, model)
		}
		sort.Strings(keys)
		for _, model := range keys {
			if existing[model] || conflicts[model] {
				continue
			}
			cost, ok := models[selected[model]]
			if ok {
				cost, ok = selectedVideoCost(cost)
			}
			if !ok {
				warning = true
				continue
			}
			entry := ChannelModelPricing{Platform: PlatformVideo, Models: []string{model}, BillingMode: cost.BillingMode,
				Video: &VideoModelConfig{PriceSource: "upstream", AutoPrice: true, AccountSynced: true, Spec: *cost.Spec}}
			entry, err := repriceVideoEntry(entry, cost, cfg, at, true)
			if err != nil || validateVideoGroupPricing([]ChannelModelPricing{entry}) != nil {
				warning = true
				continue
			}
			next = append(next, entry)
		}
	}
	return next, warning
}
