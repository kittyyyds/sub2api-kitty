package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const mediaPricingRefreshThrottle = 2 * time.Second

var mediaPricingRefreshAt sync.Map // key: image/video + group ID, value: time.Time

func allowMediaPricingRefresh(kind string, groupID int64) bool {
	if groupID <= 0 {
		return false
	}
	now := time.Now()
	key := kind + ":" + strconv.FormatInt(groupID, 10)
	if previous, ok := mediaPricingRefreshAt.Load(key); ok {
		if refreshedAt, ok := previous.(time.Time); ok && now.Sub(refreshedAt) < mediaPricingRefreshThrottle {
			return false
		}
	}
	mediaPricingRefreshAt.Store(key, now)
	return true
}

// refreshGroupMediaPricing loads the authoritative group projection when the
// API-key snapshot may be missing media pricing fields. Authentication
// snapshots normally contain the complete projection, but keeping this
// request-level fallback prevents a stale/partial snapshot from silently
// falling through to USD defaults and rate multipliers.
func refreshGroupMediaPricing(ctx context.Context, repo GroupRepository, apiKey *APIKey) *APIKey {
	if apiKey == nil || apiKey.GroupID == nil || *apiKey.GroupID <= 0 || repo == nil {
		return apiKey
	}
	group, err := repo.GetByIDLite(ctx, *apiKey.GroupID)
	if err != nil || group == nil {
		return apiKey
	}
	clone := *apiKey
	clone.Group = group
	return &clone
}

func knownImageModelPriceCandidate(model string) bool {
	model = CanonicalImagePriceModel(model)
	if model == "" {
		return false
	}
	if _, ok := getDefaultGrokImagineImagePrice(model, ImageBillingSize1K); ok {
		return true
	}
	return strings.HasPrefix(model, "gpt-image-")
}

func knownVideoModelPriceCandidate(model string) bool {
	_, ok := getDefaultGrokImagineVideoPrice(model, VideoBillingResolution480P)
	return ok
}

// imageModelPriceMayBeMissing reports whether the request may be using a
// stale auth snapshot for a model-level final image price. A configured
// model/size hit is authoritative and must not trigger a database refresh.
func imageModelPriceMayBeMissing(apiKey *APIKey, imageSize string, candidates ...string) bool {
	if apiKey == nil || apiKey.Group == nil {
		return true
	}
	if _, ok := configuredImagePriceModel(apiKey, imageSize, candidates...); ok {
		return false
	}
	for _, candidate := range candidates {
		if knownImageModelPriceCandidate(candidate) {
			return true
		}
	}
	return false
}

func videoModelPriceMayBeMissing(apiKey *APIKey, resolution string, candidates ...string) bool {
	if apiKey == nil || apiKey.Group == nil {
		return true
	}
	if _, ok := configuredVideoPriceModel(apiKey, resolution, candidates...); ok {
		return false
	}
	for _, candidate := range candidates {
		if knownVideoModelPriceCandidate(candidate) {
			return true
		}
	}
	return false
}

// imageCountBillingEnabled 表示图片端点兜底按实际出图张数计费；显式分组/渠道 token 价仍优先。
func imageCountBillingEnabled(apiKey *APIKey) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.ImageCountBilling
}

// imageCountBillingApplies 判断分组按张计费开关是否作用于本次请求。
// 开关只影响专用图片端点；混合的 Responses/Chat/WS 请求即使带图片工具，
// 也必须保留 token 计费，避免整条请求被按图片单价结算而丢失文本 token 费用。
func imageCountBillingApplies(apiKey *APIKey, imageEndpoint bool) bool {
	return imageEndpoint && imageCountBillingEnabled(apiKey)
}

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
