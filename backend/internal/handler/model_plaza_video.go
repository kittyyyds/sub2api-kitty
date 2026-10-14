package handler

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// This extension is specific to the group plaza. A channel's pricing card is
// not necessarily a final group selling price, so toUserPricing stays unchanged.
type modelPlazaPricing struct {
	*userSupportedModelPricing
	FinalUnitPrice     bool               `json:"final_unit_price,omitempty"`
	ReferencePerSecond *float64           `json:"reference_per_second,omitempty"`
	ResolutionPrices   map[string]float64 `json:"resolution_prices,omitempty"`
}

// The same public contract as /v1/models, plus reference combination limits.
// Never serialize VideoModelConfig: it also contains costs and private controls.
type modelPlazaVideoCapabilities struct {
	service.CanvasModelCapability
	MaxCombinedSeconds    float64 `json:"max_combined_seconds,omitempty"`
	AllowAudioOnly        bool    `json:"allow_audio_only"`
	RequiresVisualOrAudio bool    `json:"requires_visual_or_audio"`
}

func toModelPlazaPricing(p *service.ChannelModelPricing) *modelPlazaPricing {
	if p == nil {
		return nil
	}
	out := &modelPlazaPricing{userSupportedModelPricing: toUserPricing(p)}
	if p.Platform != service.PlatformVideo || p.PerRequestPrice == nil ||
		(p.BillingMode != service.BillingModeVideo && p.BillingMode != service.BillingModePerRequest) {
		return out
	}
	out.FinalUnitPrice = true
	if p.Video != nil {
		referencePrice := p.Video.ReferencePerSecond
		out.ReferencePerSecond = &referencePrice // An explicit zero must survive JSON.
		out.ResolutionPrices = maps.Clone(p.Video.ResolutionPrices)
	}
	return out
}

// Describe only the implemented input contract. Spec.Reason can be edited by an
// operator; do not publish it or infer speed, reliability or real-run acceptance.
func modelPlazaVideoDescription(spec service.UnifiedVideoSpec) string {
	if spec.Generic {
		text := "通用视频接口；型号由管理员开放，具体时长、分辨率和素材限制以上游为准"
		if spec.ReferencePricePending {
			text += "；参考视频价格未提供，暂不接受参考视频"
		}
		return text
	}
	duration := fmt.Sprintf("时长 %d–%d 秒", spec.MinSeconds, spec.MaxSeconds)
	if len(spec.Seconds) > 0 {
		options := make([]string, len(spec.Seconds))
		for i, seconds := range spec.Seconds {
			options[i] = strconv.Itoa(seconds)
		}
		duration = "可选时长 " + strings.Join(options, "/") + " 秒"
	} else if spec.MinSeconds == spec.MaxSeconds {
		duration = fmt.Sprintf("固定时长 %d 秒", spec.MinSeconds)
	}
	parts := []string{duration, "分辨率 " + strings.Join(spec.Resolutions, "/"),
		fmt.Sprintf("参考图片 %d–%d 张，视频最多 %d 个，音频最多 %d 个", spec.MinImages, spec.MaxImages, spec.MaxVideos, spec.MaxAudios)}
	if spec.MaxVideos > 0 {
		parts = append(parts, fmt.Sprintf("参考视频总时长不超过 %g 秒，按测得时长和参考秒价计费", spec.MaxReferenceSeconds))
	} else if spec.ReferencePricePending {
		parts = append(parts, "参考视频价格未提供，暂不接受参考视频")
	} else {
		parts = append(parts, "不接受参考视频")
	}
	if spec.MaxCombinedSeconds > 0 {
		parts = append(parts, fmt.Sprintf("参考视频与输出合计不超过 %g 秒", spec.MaxCombinedSeconds))
	}
	if spec.RequiresVisualOrAudio {
		parts = append(parts, "需提供参考素材")
	}
	if spec.MaxAudios > 0 && !spec.AllowAudioOnly {
		requirement := "参考音频须同时提供图片或视频"
		if spec.MaxVideos == 0 {
			requirement = "参考音频须同时提供图片"
		}
		parts = append(parts, requirement)
	}
	return strings.Join(parts, "；")
}
