package service

import "strings"

// CanvasModelCapability describes only contracts implemented by this gateway.
// A model appearing in a supplier's catalog is not proof of these capabilities.
type CanvasModelCapability struct {
	Generic             bool              `json:"generic,omitempty"`
	SecondOptions       []int             `json:"second_options,omitempty"`
	ReferenceVideos     *CanvasCountRange `json:"reference_videos,omitempty"`
	MaxReferenceSeconds float64           `json:"max_reference_seconds,omitempty"`
	Face                bool              `json:"face,omitempty"`
	GenerateAudio       bool              `json:"generate_audio,omitempty"`
	MediaType           string            `json:"media_type"`
	Supported           bool              `json:"supported"`
	Provider            string            `json:"provider,omitempty"`
	Family              string            `json:"family,omitempty"`
	Seconds             *CanvasCountRange `json:"seconds,omitempty"`
	Resolutions         []string          `json:"resolutions,omitempty"`
	AspectRatios        []string          `json:"aspect_ratios,omitempty"`
	ReferenceImages     *CanvasCountRange `json:"reference_images,omitempty"`
	ReferenceAudios     *CanvasCountRange `json:"reference_audios,omitempty"`
	InputTransport      []string          `json:"input_transport,omitempty"`
	Operations          []string          `json:"operations"`
}

type CanvasCountRange struct {
	Min   int `json:"min"`
	Max   int `json:"max"`
	Fixed int `json:"fixed,omitempty"`
}

func CanvasCapabilities(modelID, platform string) CanvasModelCapability {
	modelID = strings.TrimSpace(modelID)
	if spec, ok := suanliVideoModels[modelID]; ok && platform == PlatformVideo {
		result := CanvasModelCapability{
			MediaType: "video", Supported: spec.contractKnown, Provider: SuanliSpecialVideoProvider,
			Family: "h3", Seconds: &CanvasCountRange{Min: 5, Max: 15},
			Resolutions: []string{spec.resolution}, AspectRatios: []string{"16:9", "9:16", "1:1"},
			ReferenceImages: &CanvasCountRange{Min: 1, Max: 9}, ReferenceAudios: &CanvasCountRange{Min: 0, Max: 3},
			InputTransport: []string{"public_url"}, Operations: []string{"image_to_video"},
		}
		if spec.seedance {
			result.Family = "seedance"
			result.ReferenceImages.Max = 1
			result.ReferenceAudios.Max = 0
		}
		if spec.fixedSeconds > 0 {
			result.Family = "sd25"
			result.Seconds = &CanvasCountRange{Min: spec.fixedSeconds, Max: spec.fixedSeconds, Fixed: spec.fixedSeconds}
			result.ReferenceImages.Max = 10
			result.ReferenceAudios.Max = 0
		}
		return result
	}
	if platform == PlatformVideo {
		return CanvasModelCapability{MediaType: "video", Operations: []string{}}
	}
	name := strings.ToLower(modelID)
	if name == "dall-e-3" {
		return CanvasModelCapability{MediaType: "image", Supported: true, Operations: []string{"text_to_image"}}
	}
	if strings.HasPrefix(name, "gpt-image-") || strings.HasPrefix(name, "grok-imagine-image") {
		return CanvasModelCapability{MediaType: "image", Supported: true, Operations: []string{"text_to_image", "image_edit"}}
	}
	if strings.Contains(name, "image") || strings.Contains(name, "video") || strings.Contains(name, "audio") ||
		strings.Contains(name, "tts") || strings.Contains(name, "whisper") || strings.Contains(name, "embedding") {
		return CanvasModelCapability{MediaType: "other", Operations: []string{}}
	}
	return CanvasModelCapability{MediaType: "text", Supported: modelID != "", Operations: []string{"chat", "agent"}}
}
