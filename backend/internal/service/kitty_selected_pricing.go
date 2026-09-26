package service

import (
	"encoding/json"
	"strings"
)

func applyKittySelectedLunaPricing(data map[string]json.RawMessage) {
	for model, raw := range data {
		if normalizeCodexModel(strings.TrimPrefix(strings.ToLower(model), "openai/")) != "gpt-5.6-luna" {
			continue
		}
		var entry map[string]json.RawMessage
		if json.Unmarshal(raw, &entry) != nil || entry == nil {
			continue
		}
		for field, price := range map[string]float64{
			"input_cost_per_token": 0.4e-6, "output_cost_per_token": 2.4e-6,
			"cache_creation_input_token_cost": 0.5e-6, "cache_read_input_token_cost": 0.04e-6,
		} {
			for suffix, ratio := range map[string]float64{"": 1, "_priority": 2, "_flex": 0.5, "_batches": 0.5} {
				encoded, _ := json.Marshal(price * ratio)
				entry[field+suffix] = encoded
			}
			ladder := 2.0
			if field == "output_cost_per_token" {
				ladder = 1.5
			}
			for suffix, tier := range map[string]float64{"": 1, "_priority": 2, "_flex": 0.5, "_batches": 0.5} {
				above := field + "_above_272k_tokens" + suffix
				if _, exists := entry[above]; exists {
					encoded, _ := json.Marshal(price * ladder * tier)
					entry[above] = encoded
				}
			}
		}
		encoded, err := json.Marshal(entry)
		if err == nil {
			data[model] = encoded
		}
	}
}
