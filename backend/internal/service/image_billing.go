package service

import (
	"log/slog"
	"sort"
	"strings"
)

// CanonicalImagePriceModel normalizes provider prefixes and casing while
// keeping distinct model IDs independent. In particular, grok-imagine must not
// inherit the price configured for grok-imagine-image-quality.
func CanonicalImagePriceModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	for {
		matched := false
		for _, prefix := range []string{"xai/", "x-ai/", "grok/", "openai/", "models/"} {
			if strings.HasPrefix(model, prefix) {
				model = strings.TrimPrefix(model, prefix)
				matched = true
				break
			}
		}
		if !matched {
			return model
		}
	}
}

// NormalizeImageModelPrices cleans a model x resolution map. Unknown
// resolutions and negative prices are dropped; an explicit zero remains valid.
func NormalizeImageModelPrices(in map[string]map[string]float64) map[string]map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	modelKeys := make([]string, 0, len(in))
	for modelKey := range in {
		modelKeys = append(modelKeys, modelKey)
	}
	sort.Strings(modelKeys)

	out := make(map[string]map[string]float64)
	for _, modelKey := range modelKeys {
		model := CanonicalImagePriceModel(modelKey)
		if model == "" || len(in[modelKey]) == 0 {
			continue
		}
		tierKeys := make([]string, 0, len(in[modelKey]))
		for tierKey := range in[modelKey] {
			tierKeys = append(tierKeys, tierKey)
		}
		sort.Strings(tierKeys)

		tiers := out[model]
		if tiers == nil {
			tiers = make(map[string]float64)
		}
		for _, tierKey := range tierKeys {
			price := in[modelKey][tierKey]
			if price < 0 {
				continue
			}
			tier, ok := ClassifyImageBillingTier(tierKey)
			if !ok {
				slog.Warn("image_model_prices_unknown_resolution_dropped",
					"model", modelKey,
					"resolution", tierKey)
				continue
			}
			if previous, exists := tiers[tier]; exists && previous != price {
				slog.Warn("image_model_prices_conflicting_tier_price",
					"model", model,
					"resolution", tier,
					"previous_price", previous,
					"price", price)
			}
			tiers[tier] = price
		}
		if len(tiers) > 0 {
			out[model] = tiers
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// LookupImageModelPrice returns a configured final USD-per-image price, or nil when
// the model or resolution has no override.
func LookupImageModelPrice(prices map[string]map[string]float64, model, imageSize string) *float64 {
	if len(prices) == 0 {
		return nil
	}
	model = CanonicalImagePriceModel(model)
	if model == "" {
		return nil
	}
	tiers, ok := prices[model]
	if !ok || len(tiers) == 0 {
		return nil
	}
	tier := NormalizeImageBillingTierOrDefault(imageSize)
	price, ok := tiers[tier]
	if !ok {
		return nil
	}
	return &price
}
