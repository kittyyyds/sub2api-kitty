package service

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// PricingAuditInventory is deliberately limited to model names. No account
// credentials, keys or tokens are returned to this administrative feature.
type PricingAuditInventory struct {
	GroupID   int64
	AccountID int64
	Models    map[string]string
}
type PricingAuditRepository interface {
	ReadPricingAuditInventory(context.Context, int) ([]PricingAuditInventory, error)
}
type PricingAuditRow struct {
	GroupID   int64       `json:"group_id"`
	GroupName string      `json:"group_name"`
	Model     string      `json:"model"`
	Origins   []string    `json:"origins"`
	Status    string      `json:"status"`
	Source    string      `json:"source"`
	Mode      BillingMode `json:"mode"`
	Reason    string      `json:"reason"`
}
type PricingAuditReport struct {
	QueriedAt time.Time         `json:"queried_at"`
	ReadOnly  bool              `json:"read_only"`
	Truncated bool              `json:"truncated"`
	Scope     string            `json:"scope"`
	Rows      []PricingAuditRow `json:"rows"`
}

func pricingAuditStatus(resolved *ResolvedPricing, configured *ChannelModelPricing, baseExists bool) (string, string) {
	if configured != nil && (configured.Image != nil || configured.Video != nil) {
		return "review", "request_dependent_tiers"
	}
	if resolved == nil {
		return "missing", "pricing_not_resolved"
	}
	if resolved.Mode != BillingModeToken {
		// Media tiers depend on duration/size/label and need request-specific checks.
		if len(resolved.RequestTiers) > 0 {
			return "review", "request_dependent_tiers"
		}
		if configured != nil && configured.PerRequestPrice == nil {
			return "missing", "default_request_price_missing"
		}
		v := resolved.DefaultPerRequestPrice
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return "invalid", "invalid_price"
		}
		if v == 0 {
			return "free", "explicit_zero_price"
		}
		return "priced", "resolved"
	}
	if resolved.BasePricing == nil {
		return "missing", "base_price_missing"
	}
	// The resolver allocates an all-zero base for configured models. Nil rates
	// must not silently become an intentional free model in the audit.
	if !baseExists && (configured == nil || configured.InputPrice == nil || configured.OutputPrice == nil) {
		return "missing", "input_or_output_price_missing"
	}
	p := resolved.BasePricing
	allZero := true
	for _, v := range []float64{p.InputPricePerToken, p.OutputPricePerToken, p.CacheReadPricePerToken, p.CacheCreationPricePerToken,
		p.InputPricePerTokenPriority, p.OutputPricePerTokenPriority, p.CacheReadPricePerTokenPriority, p.CacheCreationPricePerTokenPriority,
		p.ImageInputPricePerToken, p.ImageOutputPricePerToken, p.ImageCacheReadPricePerToken, p.CacheCreation5mPrice, p.CacheCreation1hPrice} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return "invalid", "invalid_price"
		}
		allZero = allZero && v == 0
	}
	if len(resolved.Intervals) > 0 {
		return "review", "context_dependent_intervals"
	}
	if allZero {
		return "free", "resolved_zero_price"
	}
	return "priced", "resolved"
}

// AuditPricing only observes configuration. It never executes an upstream
// request, adjusts a rate, or adds a gate to the gateway's hot path.
func (s *ChannelService) AuditPricing(ctx context.Context, billing *BillingService) (*PricingAuditReport, error) {
	if s.groupRepo == nil || billing == nil {
		return nil, infraerrors.ServiceUnavailable("PRICING_AUDIT_UNAVAILABLE", "Pricing audit unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repo.(PricingAuditRepository)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("PRICING_AUDIT_UNAVAILABLE", "Pricing inventory unavailable")
	}
	inventory, err := repo.ReadPricingAuditInventory(ctx, 1001)
	if err != nil {
		return nil, err
	}
	report := &PricingAuditReport{QueriedAt: time.Now().UTC(), ReadOnly: true, Scope: "configured_candidates_not_live_request_or_exhaustive_wildcard_validation", Rows: []PricingAuditRow{}}
	if len(inventory) > 1000 {
		inventory = inventory[:1000]
		report.Truncated = true
	}
	resolver := NewModelPricingResolver(s, billing)
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		models := map[string]map[string]bool{}
		add := func(model, origin string) {
			model = strings.TrimSpace(model)
			if model == "" {
				return
			}
			if len(model) > 200 {
				report.Truncated = true
				return
			}
			if models[model] == nil {
				models[model] = map[string]bool{}
			}
			models[model][origin] = true
		}
		for _, p := range group.ModelPricing {
			for _, model := range p.Models {
				add(model, "group_pricing")
			}
		}
		add(group.DefaultMappedModel, "group_default_mapping")
		channel := cache.channelByGroupID[group.ID]
		if channel != nil {
			for _, p := range channel.ModelPricing {
				if p.Platform != "" && p.Platform != group.Platform {
					continue
				}
				for _, model := range p.Models {
					add(model, "channel_pricing")
				}
			}
			for from, to := range channel.ModelMapping[group.Platform] {
				add(from, "channel_mapping_from")
				add(to, "channel_mapping_to")
			}
		}
		for _, a := range inventory {
			if a.GroupID != group.ID {
				continue
			}
			if len(a.Models) == 0 {
				add("*", "account_models_not_enumerated")
			}
			for from, to := range a.Models {
				add(from, "account_mapping_from")
				add(to, "account_mapping_to")
			}
		}
		if len(models) == 0 {
			add("*", "models_not_enumerated")
		}
		names := make([]string, 0, len(models))
		for name := range models {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, model := range names {
			if len(report.Rows) >= 5000 {
				report.Truncated = true
				return report, nil
			}
			row := PricingAuditRow{GroupID: group.ID, GroupName: group.Name, Model: model, Origins: []string{}}
			for origin := range models[model] {
				row.Origins = append(row.Origins, origin)
			}
			sort.Strings(row.Origins)
			if strings.ContainsAny(model, "*?") {
				row.Status = "review"
				row.Reason = "wildcard_requires_concrete_request_model"
			} else {
				base, _ := billing.GetModelPricing(model)
				configured := matchGroupModelPricing(&group, model)
				if configured == nil {
					configured = resolver.lookupChannelPricingNormalized(ctx, group.ID, model)
				}
				resolved := resolver.Resolve(ctx, PricingInput{Model: model, GroupID: &group.ID, Group: &group})
				row.Status, row.Reason = pricingAuditStatus(resolved, configured, base != nil)
				if resolved != nil {
					row.Source = resolved.Source
					row.Mode = resolved.Mode
				}
			}
			report.Rows = append(report.Rows, row)
		}
	}
	return report, nil
}
