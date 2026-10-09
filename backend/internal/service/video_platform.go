package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// VideoGroupPrice is the final customer unit price; group, user and media rate
// multipliers do not apply. The accepted task stores this value instead of
// repricing it when polling eventually discovers completion.
type VideoGroupPrice struct {
	Video       *VideoModelConfig
	Model       string
	BillingMode BillingMode
	UnitPrice   float64
}

func (a *Account) IsVideo() bool { return a != nil && a.Platform == PlatformVideo }

// IsGrokMediaAccount includes dedicated API-key video providers without making
// them Grok OAuth/text accounts or enabling token refresh/probes.
func (a *Account) IsGrokMediaAccount() bool {
	return a != nil && (a.IsGrok() || (a.IsVideo() && a.Type == AccountTypeAPIKey))
}

func ValidateVideoAccount(platform, accountType string, credentials map[string]any) error {
	if platform != PlatformVideo {
		return nil
	}
	if accountType != AccountTypeAPIKey {
		return infraerrors.BadRequest("INVALID_VIDEO_ACCOUNT_TYPE", "video accounts require API key authentication")
	}
	if raw, exists := credentials["grok_media_provider"]; exists && raw != nil {
		provider, ok := raw.(string)
		if !ok || (strings.TrimSpace(provider) != "" && strings.TrimSpace(provider) != SuanliSpecialVideoProvider) {
			return infraerrors.BadRequest("INVALID_VIDEO_PROVIDER", "unsupported video provider")
		}
	}
	return nil
}

// Reject an invalid platform assignment before any account row or account
// settings are written. The repository repeats this check under group locks.
func validateVideoAccountGroupPlatforms(ctx context.Context, repo GroupRepository, platform string, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	if repo == nil {
		return fmt.Errorf("group repository not configured")
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		group, err := repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if group == nil {
			return ErrGroupNotFound
		}
		if (platform == PlatformVideo) != (group.Platform == PlatformVideo) {
			return infraerrors.BadRequest("VIDEO_GROUP_PLATFORM_MISMATCH", "video accounts can only bind video groups, and video groups only accept video accounts")
		}
	}
	return nil
}

func validateVideoGroupPricing(pricing []ChannelModelPricing) error {
	seen := make(map[string]struct{})
	for _, entry := range pricing {
		if entry.Video != nil {
			if entry.Video.PriceSource != "" && entry.Video.PriceSource != "manual" && entry.Video.PriceSource != "upstream" {
				return fmt.Errorf("未知的视频定价方式")
			}
			if len(entry.Models) != 1 {
				return infraerrors.BadRequest("INVALID_VIDEO_CONFIG", "统一视频价格每项必须只有一个型号")
			}
			if err := entry.Video.Spec.Validate(); err != nil {
				return err
			}
			if !validVideoMoney(entry.Video.ReferencePerSecond) {
				return fmt.Errorf("参考视频秒价无效")
			}
			for _, v := range entry.Video.ResolutionPrices {
				if !validVideoMoney(v) {
					return fmt.Errorf("分辨率价格无效")
				}
			}
			if entry.Video.Markup != nil {
				if err := entry.Video.Markup.validate(); err != nil {
					return err
				}
			}
		}
		if entry.Platform != "" && entry.Platform != PlatformVideo {
			return infraerrors.BadRequest("INVALID_VIDEO_PRICING_PLATFORM", "video prices must belong to the video platform")
		}
		if entry.BillingMode != BillingModePerRequest && entry.BillingMode != BillingModeVideo {
			return infraerrors.BadRequest("INVALID_VIDEO_BILLING_MODE", "video prices must use per_request or video (per second)")
		}
		if entry.PerRequestPrice == nil || math.IsNaN(*entry.PerRequestPrice) || math.IsInf(*entry.PerRequestPrice, 0) || *entry.PerRequestPrice < 0 || *entry.PerRequestPrice >= 1e12 {
			return infraerrors.BadRequest("INVALID_VIDEO_UNIT_PRICE", "video model requires an explicit finite non-negative unit price below 1e12")
		}
		if len(entry.Intervals) > 0 || entry.TimePricing != nil {
			return infraerrors.BadRequest("INVALID_VIDEO_PRICE_TIERS", "video prices use one fixed unit price per exact model")
		}
		if len(entry.Models) == 0 {
			return infraerrors.BadRequest("VIDEO_MODEL_REQUIRED", "video price requires an exact model ID")
		}
		for _, raw := range entry.Models {
			model := strings.TrimSpace(raw)
			if model == "" || strings.ContainsAny(model, "*?") {
				return infraerrors.BadRequest("INVALID_VIDEO_MODEL", "video prices require exact model IDs without wildcards")
			}
			if _, exists := seen[model]; exists {
				return infraerrors.BadRequest("DUPLICATE_VIDEO_MODEL", "video model has more than one price")
			}
			seen[model] = struct{}{}
		}
	}
	return nil
}

// ResolveVideoGroupPrice never reads global/channel/legacy resolution prices.
// Exact IDs are case-sensitive, matching the submitted provider model ID.
func ResolveVideoGroupPrice(group *Group, model string) (*VideoGroupPrice, error) {
	if group == nil || group.Platform != PlatformVideo {
		return nil, infraerrors.BadRequest("VIDEO_GROUP_REQUIRED", "a video group is required")
	}
	if err := validateVideoGroupPricing(group.ModelPricing); err != nil {
		return nil, err
	}
	model = strings.TrimSpace(model)
	for _, entry := range group.ModelPricing {
		for _, candidate := range entry.Models {
			if strings.TrimSpace(candidate) == model && model != "" {
				return &VideoGroupPrice{Model: model, BillingMode: entry.BillingMode, UnitPrice: *entry.PerRequestPrice, Video: entry.Video}, nil
			}
		}
	}
	return nil, infraerrors.BadRequest("VIDEO_MODEL_PRICE_REQUIRED", "configure an exact video group model price before submitting")
}

// ResolveVideoGroupPricing loads the current group for a new submission. Unlike
// ordinary auth snapshots, a stale complete price card must not lock an old price.
func (s *OpenAIGatewayService) ResolveVideoGroupPricing(ctx context.Context, apiKey *APIKey, model string) (*VideoGroupPrice, error) {
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != PlatformVideo {
		return nil, infraerrors.BadRequest("VIDEO_GROUP_REQUIRED", "a video group is required")
	}
	group := apiKey.Group
	if s != nil && s.channelService != nil && s.channelService.groupRepo != nil && apiKey.GroupID != nil {
		fresh, err := s.channelService.groupRepo.GetByID(ctx, *apiKey.GroupID)
		if err != nil {
			return nil, fmt.Errorf("load authoritative video group price: %w", err)
		}
		group = fresh
	}
	return ResolveVideoGroupPrice(group, model)
}

func videoGroupModelIDs(group *Group) []string {
	models := make([]string, 0)
	if group == nil || group.Platform != PlatformVideo {
		return models
	}
	for _, entry := range group.ModelPricing {
		for _, model := range entry.Models {
			if _, err := ResolveVideoGroupPrice(group, model); err == nil {
				models = append(models, model)
			}
		}
	}
	return models
}

// VideoGroupModelIDs lists only explicitly priced video models.
func VideoGroupModelIDs(group *Group) []string { return videoGroupModelIDs(group) }

func calculateVideoPriceSnapshotCost(snapshot *VideoPriceSnapshot, videoCount int) (*CostBreakdown, error) {
	if snapshot == nil || strings.TrimSpace(snapshot.Model) == "" || videoCount < 1 ||
		math.IsNaN(snapshot.UnitPrice) || math.IsInf(snapshot.UnitPrice, 0) || snapshot.UnitPrice < 0 ||
		math.IsNaN(snapshot.RateMultiplier) || math.IsInf(snapshot.RateMultiplier, 0) || snapshot.RateMultiplier < 0 {
		return nil, fmt.Errorf("invalid immutable video price snapshot")
	}
	units := float64(videoCount)
	switch snapshot.BillingMode {
	case BillingModePerRequest:
	case BillingModeVideo:
		if snapshot.DurationSeconds <= 0 {
			return nil, fmt.Errorf("video price snapshot requires a positive duration")
		}
		units *= float64(snapshot.DurationSeconds)
	default:
		return nil, fmt.Errorf("unsupported immutable video billing mode")
	}
	if !validVideoMoney(snapshot.ReferencePerSecond) || math.IsNaN(snapshot.ReferenceSeconds) || math.IsInf(snapshot.ReferenceSeconds, 0) || snapshot.ReferenceSeconds < 0 || snapshot.ReferenceSeconds > 120 {
		return nil, fmt.Errorf("invalid reference video price snapshot")
	}
	total := snapshot.UnitPrice*units + snapshot.ReferencePerSecond*snapshot.ReferenceSeconds*float64(videoCount)
	actual := total
	if !snapshot.FinalUnitPrice {
		// Preserve snapshots submitted before final unit pricing was introduced.
		actual *= snapshot.RateMultiplier
	}
	// Cost columns use NUMERIC(20,8), so their integral part must stay below 1e12.
	if math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(actual) || math.IsInf(actual, 0) || total >= 1e12 || actual >= 1e12 {
		return nil, fmt.Errorf("video price snapshot cost exceeds supported range")
	}
	return &CostBreakdown{TotalCost: total, ActualCost: actual, BillingMode: string(snapshot.BillingMode), FinalPriceApplied: snapshot.FinalUnitPrice}, nil
}

// Only authenticated pre-migration pending tasks may use this compatibility
// path. Those tasks never held a unit-price snapshot; new tasks must have one.
func (s *OpenAIGatewayService) calculateLegacyVideoPriceCost(ctx context.Context, apiKey *APIKey, result *OpenAIForwardResult, model string, multiplier float64) (*CostBreakdown, error) {
	if result == nil || !result.VideoLegacyPricing {
		return nil, fmt.Errorf("video completion requires its immutable price snapshot")
	}
	price, err := s.ResolveVideoGroupPricing(ctx, apiKey, model)
	if err != nil {
		return nil, err
	}
	return calculateVideoPriceSnapshotCost(&VideoPriceSnapshot{
		Model: price.Model, BillingMode: price.BillingMode, UnitPrice: price.UnitPrice,
		RateMultiplier:  multiplier,
		DurationSeconds: NormalizeVideoBillingDurationForProvider(result.VideoProvider, result.VideoDurationSeconds),
	}, result.VideoCount)
}
