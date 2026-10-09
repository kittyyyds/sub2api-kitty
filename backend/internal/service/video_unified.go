package service

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"slices"
	"strconv"
	"strings"
)

//go:embed video_specs.json
var unifiedVideoSpecData []byte

type UnifiedVideoSpec struct {
	Generic               bool     `json:"generic,omitempty"`
	ReferencePricePending bool     `json:"reference_price_pending,omitempty"`
	Supported             bool     `json:"supported"`
	Reason                string   `json:"reason,omitempty"`
	MinSeconds            int      `json:"min_seconds"`
	MaxSeconds            int      `json:"max_seconds"`
	Seconds               []int    `json:"seconds,omitempty"`
	Resolutions           []string `json:"resolutions"`
	Ratios                []string `json:"ratios"`
	MinImages             int      `json:"min_images"`
	MaxImages             int      `json:"max_images"`
	MaxVideos             int      `json:"max_videos"`
	MaxAudios             int      `json:"max_audios"`
	MaxReferenceSeconds   float64  `json:"max_reference_seconds"`
	MaxCombinedSeconds    float64  `json:"max_combined_seconds,omitempty"`
	RequiresVisualOrAudio bool     `json:"requires_visual_or_audio,omitempty"`
	AllowAudioOnly        bool     `json:"allow_audio_only,omitempty"`
	// Legacy evidence for the per_second reference/output same-rate fallback.
	// Do not set this merely because video_multi supplies a reference price;
	// that price must be read independently from the current supplier response.
	ReferenceBilled bool `json:"reference_billed,omitempty"`
	Face            bool `json:"face"`
	GenerateAudio   bool `json:"generate_audio"`
}

var unifiedVideoSpecs = func() map[string]UnifiedVideoSpec {
	var specs map[string]UnifiedVideoSpec
	if err := json.Unmarshal(unifiedVideoSpecData, &specs); err != nil {
		panic("invalid embedded video capability catalog")
	}
	return specs
}()

func UnifiedVideoSpecification(model string) (UnifiedVideoSpec, bool) {
	v, ok := unifiedVideoSpecs[model]
	return v, ok
}

// Transport bounds for an administrator-selected ID absent from the static
// catalog. These are not a claim about that model's provider-specific limits.
func genericVideoSpecification() UnifiedVideoSpec {
	return UnifiedVideoSpec{Supported: true, Generic: true,
		Reason: "使用通用视频接口，具体参数范围由上游校验", MinSeconds: 1, MaxSeconds: 120,
		Resolutions: []string{"480p", "720p", "768p", "1080p", "2k", "4k"},
		Ratios:      []string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"},
		MaxImages:   30, MaxVideos: 10, MaxAudios: 10, MaxReferenceSeconds: 120,
		AllowAudioOnly: true, Face: true, GenerateAudio: true}
}

func (s UnifiedVideoSpec) Validate() error {
	if !s.Supported || s.MinSeconds < 1 || s.MaxSeconds < s.MinSeconds || s.MaxSeconds > 120 || len(s.Resolutions) == 0 || len(s.Ratios) == 0 ||
		s.MinImages < 0 || s.MaxImages < s.MinImages || s.MaxImages > 30 || s.MaxVideos < 0 || s.MaxVideos > 10 || s.MaxAudios < 0 || s.MaxAudios > 10 ||
		s.MaxReferenceSeconds < 0 || s.MaxReferenceSeconds > 120 || s.MaxCombinedSeconds < 0 || s.MaxCombinedSeconds > 240 {
		return errors.New("视频型号的参数限制无效或尚未核实")
	}
	for _, v := range s.Seconds {
		if v < s.MinSeconds || v > s.MaxSeconds {
			return errors.New("可选时长超出范围")
		}
	}
	for _, v := range s.Resolutions {
		if !slices.Contains([]string{"480p", "720p", "768p", "1080p", "2k", "4k"}, strings.ToLower(v)) {
			return errors.New("不支持的分辨率")
		}
	}
	for _, v := range s.Ratios {
		if !suanliValidRatio(v) && v != "adaptive" {
			return errors.New("不支持的画面比例")
		}
	}
	return nil
}

type UnifiedVideoInput struct {
	Model         string   `json:"model"`
	Prompt        string   `json:"prompt"`
	Seconds       int      `json:"seconds"`
	AspectRatio   string   `json:"aspect_ratio"`
	Resolution    string   `json:"resolution"`
	Images        []string `json:"images,omitempty"`
	Videos        []string `json:"videos,omitempty"`
	Audios        []string `json:"audios,omitempty"`
	GenerateAudio *bool    `json:"generate_audio,omitempty"`
	Face          *bool    `json:"face,omitempty"`
}

// Multipart accepts the same named fields and URL arrays as JSON; actual local
// files use the authenticated temporary-reference upload endpoint first.
func decodeUnifiedVideoBody(body []byte, contentType string) (map[string]any, error) {
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errors.New("请输入有效的Content-Type")
	}
	if media == "application/json" {
		return decodeGrokMediaJSONObject(body)
	}
	if media != "multipart/form-data" {
		return nil, errors.New("视频请求支持JSON或multipart/form-data")
	}
	r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	out := map[string]any{}
	for n := 0; ; n++ {
		part, e := r.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		if n > 70 {
			return nil, errors.New("表单字段过多")
		}
		if part.FileName() != "" {
			return nil, errors.New("请先通过/v1/video-references上传文件，再提交返回的链接")
		}
		b, e := io.ReadAll(io.LimitReader(part, 1<<20))
		if e != nil {
			return nil, e
		}
		name := strings.TrimSuffix(part.FormName(), "[]")
		text := strings.TrimSpace(string(b))
		if slices.Contains([]string{"images", "videos", "audios"}, name) {
			a, _ := out[name].([]any)
			out[name] = append(a, text)
		} else {
			if _, exists := out[name]; exists {
				return nil, errors.New("不能重复提交同名字段")
			}
			switch name {
			case "face", "generate_audio":
				v, e := strconv.ParseBool(text)
				if e != nil {
					return nil, e
				}
				out[name] = v
			default:
				out[name] = text
			}
		}
	}
	return out, nil
}

func PrepareUnifiedVideoInput(body []byte, contentType string, spec UnifiedVideoSpec) (UnifiedVideoInput, error) {
	var input UnifiedVideoInput
	if err := spec.Validate(); err != nil {
		return input, err
	}
	p, err := decodeUnifiedVideoBody(body, contentType)
	if err != nil {
		return input, err
	}
	allowed := []string{"model", "prompt", "seconds", "duration", "aspect_ratio", "resolution", "images", "videos", "audios", "generate_audio", "face", "n"}
	for k := range p {
		if !slices.Contains(allowed, k) {
			return input, fmt.Errorf("不支持的视频参数 %s", k)
		}
	}
	input.Model, _ = p["model"].(string)
	input.Prompt, _ = p["prompt"].(string)
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 32000 {
		return input, errors.New("请输入模型和有效提示词")
	}
	seconds, exists := p["seconds"]
	if !exists {
		seconds = p["duration"]
	}
	if seconds == nil && spec.MinSeconds == spec.MaxSeconds {
		input.Seconds = spec.MinSeconds
	} else {
		input.Seconds, err = suanliPositiveInteger(seconds)
		if err != nil {
			return input, errors.New("请明确填写整数视频时长")
		}
	}
	if d, ok := p["duration"]; ok {
		v, e := suanliPositiveInteger(d)
		if e != nil || v != input.Seconds {
			return input, errors.New("duration与seconds必须相同")
		}
	}
	if input.Seconds < spec.MinSeconds || input.Seconds > spec.MaxSeconds || (len(spec.Seconds) > 0 && !slices.Contains(spec.Seconds, input.Seconds)) {
		return input, errors.New("视频时长不符合当前型号限制")
	}
	if n, ok := p["n"]; ok {
		v, e := suanliPositiveInteger(n)
		if e != nil || v != 1 {
			return input, errors.New("每次只创建一个视频任务")
		}
	}
	input.Resolution, _ = p["resolution"].(string)
	input.Resolution = strings.ToLower(strings.TrimSpace(input.Resolution))
	if input.Resolution == "" && len(spec.Resolutions) == 1 {
		input.Resolution = strings.ToLower(spec.Resolutions[0])
	}
	if !slices.ContainsFunc(spec.Resolutions, func(resolution string) bool {
		return strings.EqualFold(resolution, input.Resolution)
	}) {
		return input, errors.New("请选择当前型号支持的分辨率")
	}
	input.AspectRatio, _ = p["aspect_ratio"].(string)
	if input.AspectRatio == "" {
		input.AspectRatio = "16:9"
	}
	if !slices.Contains(spec.Ratios, input.AspectRatio) {
		return input, errors.New("当前型号不支持该画面比例")
	}
	for _, item := range []struct {
		name string
		max  int
		dst  *[]string
	}{{"images", spec.MaxImages, &input.Images}, {"videos", spec.MaxVideos, &input.Videos}, {"audios", spec.MaxAudios, &input.Audios}} {
		v, ok := p[item.name]
		if !ok {
			continue
		}
		a, ok := v.([]any)
		if !ok || len(a) > item.max {
			return input, fmt.Errorf("%s素材数量超出限制", item.name)
		}
		seen := map[string]bool{}
		for _, x := range a {
			url, ok := x.(string)
			if !ok {
				return input, errors.New("素材需要使用公开链接")
			}
			url = strings.TrimSpace(url)
			if !suanliPublicMediaURL(url, false) && !(item.name == "images" && suanliImageDataURL(url)) {
				return input, errors.New("素材链接无效，请先上传本地文件")
			}
			if seen[url] {
				return input, errors.New("同一素材不能重复提交，以免参考秒数重复计费")
			}
			seen[url] = true
			*item.dst = append(*item.dst, url)
		}
	}
	if len(input.Images) < spec.MinImages {
		return input, fmt.Errorf("该型号至少需要%d张参考图", spec.MinImages)
	}
	if spec.RequiresVisualOrAudio && len(input.Images)+len(input.Videos)+len(input.Audios) == 0 {
		return input, errors.New("该型号需要提供参考素材")
	}
	if !spec.AllowAudioOnly && len(input.Audios) > 0 && len(input.Images)+len(input.Videos) == 0 {
		return input, errors.New("使用参考音频时请同时提供图片或视频")
	}
	for _, item := range []struct {
		name    string
		allowed bool
		dst     **bool
	}{{"face", spec.Face, &input.Face}, {"generate_audio", spec.GenerateAudio, &input.GenerateAudio}} {
		if value, ok := p[item.name]; ok {
			b, valid := value.(bool)
			if !valid || !item.allowed {
				return input, fmt.Errorf("当前型号不支持参数%s或参数不是布尔值", item.name)
			}
			*item.dst = &b
		}
	}
	return input, nil
}

func (v *VideoModelConfig) Capability() CanvasModelCapability {
	s := v.Spec
	return CanvasModelCapability{MediaType: "video", Supported: s.Supported, Generic: s.Generic, Provider: SuanliSpecialVideoProvider, Family: "unified",
		Seconds: &CanvasCountRange{Min: s.MinSeconds, Max: s.MaxSeconds}, SecondOptions: s.Seconds, Resolutions: s.Resolutions, AspectRatios: s.Ratios,
		ReferenceImages: &CanvasCountRange{Min: s.MinImages, Max: s.MaxImages}, ReferenceVideos: &CanvasCountRange{Max: s.MaxVideos}, ReferenceAudios: &CanvasCountRange{Max: s.MaxAudios},
		MaxReferenceSeconds: s.MaxReferenceSeconds, InputTransport: []string{"public_url", "temporary_upload"}, Operations: []string{"text_to_video", "image_to_video", "reference_to_video"},
		Face: s.Face, GenerateAudio: s.GenerateAudio}
}

func VideoGroupCapability(group *Group, model string) CanvasModelCapability {
	if group != nil {
		for _, entry := range group.ModelPricing {
			if entry.Video != nil && slices.Contains(entry.Models, model) {
				return entry.Video.Capability()
			}
		}
	}
	return CanvasCapabilities(model, PlatformVideo)
}
