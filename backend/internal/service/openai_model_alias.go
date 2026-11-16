package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

func lastOpenAIModelSegment(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if strings.Contains(model, "/") {
		parts := strings.Split(model, "/")
		model = parts[len(parts)-1]
	}
	return strings.TrimSpace(model)
}

func canonicalizeOpenAIModelAliasSpelling(model string) string {
	return openai.CanonicalizeOpenAIModelAliasSpelling(model)
}

func normalizeKnownOpenAICodexModel(model string) string {
	normalized := canonicalizeOpenAIModelAliasSpelling(model)
	if normalized == "" {
		return ""
	}
	// Compact aliases use the same exact base/effort rules below. If the base
	// is unknown, return no match so the caller preserves the complete name.
	normalized = strings.TrimSuffix(normalized, "-openai-compact")
	if mapped := getNormalizedCodexModel(normalized); mapped != "" {
		return mapped
	}

	if openai.IsGPT61TerraModelSpelling(normalized) {
		return "gpt-6.1-terra"
	}
	if openai.IsGPT61SolModelSpelling(normalized) {
		return "gpt-6.1-sol"
	}
	if openai.IsGPT6SolOrLunaModelSpelling(normalized) {
		if strings.HasPrefix(normalized, "gpt-6-sol") {
			return "gpt-6-sol"
		}
		return "gpt-6-luna"
	}

	switch normalized {
	case "gpt-6", "gpt-6-astra":
		return "gpt-6-astra"
	case "gpt-5.6":
		return "gpt-5.6-sol"
	}
	// A family substring is not a model identity. In particular, the old
	// catch-all gpt-5 match silently routed gpt-5.6sol and future/custom models
	// to gpt-5.4. Only known family/suffix combinations may change the target.
	for _, family := range codexVersionModelPrefixes {
		if normalized == family.prefix {
			return family.target
		}
		if suffix, ok := strings.CutPrefix(normalized, family.prefix+"-"); ok &&
			(suffix == "max" || isKnownCodexModelSuffix(suffix)) {
			return family.target
		}
	}
	if suffix, ok := strings.CutPrefix(normalized, "gpt-5.6-"); ok &&
		(suffix == "max" || isKnownCodexModelSuffix(suffix)) {
		return "gpt-5.6-sol"
	}
	// Kitty has historically accepted this exact Codex alias. Keep it without
	// restoring the old substring catch-all for arbitrary gpt-5 model names.
	if normalized == "gpt-5.5-codex" {
		return "gpt-5.5"
	}
	if suffix, ok := strings.CutPrefix(normalized, "gpt-5.5-codex-"); ok && isKnownCodexModelSuffix(suffix) {
		return "gpt-5.5"
	}
	// Retain documented legacy aliases, but never infer a replacement for an
	// unknown family. The caller preserves that model for explicit routing or
	// an authoritative upstream model error.
	for _, family := range []string{"gpt-5.3", "gpt-5.1", "gpt-5"} {
		if suffix, ok := strings.CutPrefix(normalized, family+"-"); ok && isKnownCodexModelSuffix(suffix) {
			return codexModelMap[family]
		}
	}
	return ""
}

// isOpenAIGPT56Model 判断是否 GPT-5.6 系列模型；入参可为原始模型名
// （含大小写/路径/后缀变体）或已归一化的基名，两者均能正确识别。
func isOpenAIGPT56Model(model string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(model)
	if normalized == "gpt-5.6" {
		return true
	}
	if suffix, ok := strings.CutPrefix(normalized, "gpt-5.6-"); ok && (suffix == "max" || isKnownCodexModelSuffix(suffix)) {
		return true
	}
	for _, prefix := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+"-") {
			return true
		}
	}
	return false
}

// isOpenAIGPT6AstraModel reports GPT-6 Astra and dated/provider-prefixed variants.
// The public "gpt-6" alias routes to Astra; unrelated GPT-6 families stay excluded.
func isOpenAIGPT6AstraModel(model string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(model)
	return normalized == "gpt-6" || normalized == "gpt-6-astra" || strings.HasPrefix(normalized, "gpt-6-astra-")
}

func appendUsageBillingModelCandidate(candidates []string, seen map[string]struct{}, model string) []string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return candidates
	}
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, candidate)
	}

	add(trimmed)
	if canonical := canonicalizeOpenAIModelAliasSpelling(trimmed); canonical != "" {
		add(canonical)
	}
	if normalized := normalizeKnownOpenAICodexModel(trimmed); normalized != "" {
		add(normalized)
	}
	return candidates
}

func usageBillingModelCandidates(primary string, alternates ...string) []string {
	seen := make(map[string]struct{}, 1+len(alternates))
	candidates := appendUsageBillingModelCandidate(nil, seen, primary)
	for _, alternate := range alternates {
		candidates = appendUsageBillingModelCandidate(candidates, seen, alternate)
	}
	return candidates
}

func firstUsageBillingModel(candidates []string) string {
	for _, candidate := range candidates {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isOpenAIGPT6Model(model string) bool {
	return openai.IsGPT61TerraModelSpelling(model) || isOpenAIGPT6AstraModel(model) || openai.IsGPT6SolOrLunaModelSpelling(model) || openai.IsGPT61SolModelSpelling(model)
}
