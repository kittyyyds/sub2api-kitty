package service

import "strings"

func imagePriceConfigFromAPIKey(apiKey *APIKey) *ImagePriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return apiKey.Group.ImagePriceConfig()
}

func apiKeyHasConfiguredImageModelPrice(apiKey *APIKey, model, imageSize string) bool {
	return apiKey != nil && apiKey.Group != nil &&
		LookupImageModelPrice(apiKey.Group.ImageModelPrices, model, imageSize) != nil
}

func configuredImagePriceModel(apiKey *APIKey, imageSize string, candidates ...string) (string, bool) {
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		model := CanonicalImagePriceModel(candidate)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		if apiKeyHasConfiguredImageModelPrice(apiKey, model, imageSize) {
			return model, true
		}
	}
	return "", false
}

func apiKeyHasConfiguredImagePrice(apiKey *APIKey, imageSize string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetImagePrice(imageSize) != nil
}

func videoPriceConfigFromAPIKey(apiKey *APIKey) *VideoPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &VideoPriceConfig{
		Price480P:   apiKey.Group.VideoPrice480P,
		Price720P:   apiKey.Group.VideoPrice720P,
		Price1080P:  apiKey.Group.VideoPrice1080P,
		ModelPrices: apiKey.Group.VideoModelPrices,
	}
}

func apiKeyHasConfiguredVideoModelPrice(apiKey *APIKey, model, resolution string) bool {
	return apiKey != nil && apiKey.Group != nil &&
		LookupVideoModelPrice(apiKey.Group.VideoModelPrices, model, resolution) != nil
}

func configuredVideoPriceModel(apiKey *APIKey, resolution string, candidates ...string) (string, bool) {
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		model := CanonicalGrokImagineVideoPriceFamily(candidate)
		if model == "" {
			model = strings.ToLower(strings.TrimSpace(candidate))
		}
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		if apiKeyHasConfiguredVideoModelPrice(apiKey, model, resolution) {
			return model, true
		}
	}
	return "", false
}

func apiKeyHasConfiguredVideoPrice(apiKey *APIKey, model, resolution string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetVideoPriceForModel(model, resolution) != nil
}

func webSearchPricePerCallFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return apiKey.Group.WebSearchPricePerCall
}

func groupSearchPricePer1kFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return apiKey.Group.GetSearchPricePer1k()
}

func groupAudioPriceConfigFromAPIKey(apiKey *APIKey) *audioPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	g := apiKey.Group
	return &audioPriceConfig{
		RealtimePerMin: g.AudioRealtimePricePerMin,
		TTSPerMChars:   g.AudioTTSPricePerMillionChars,
		STTPerHour:     g.AudioSTTPricePerHour,
	}
}
