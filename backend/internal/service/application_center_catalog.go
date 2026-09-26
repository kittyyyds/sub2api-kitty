package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	applicationGPTImageModel         = "gpt-image-2"
	applicationGrokImageMaxCount     = 4
	applicationOpenAIImageMaxCount   = 8
	applicationDirectPromptMaxBytes  = 8000
	applicationNegativePromptMaxSize = 2000
)

var applicationDirectPromptAliases = []string{
	"direct_prompt",
	"prompt",
	"brief",
	"description",
	"idea",
	"concept",
}

var grokImageAspectRatios = []string{
	"1:1",
	"16:9",
	"9:16",
	"4:3",
	"3:4",
	"3:2",
	"2:3",
	"2:1",
	"1:2",
	"19.5:9",
	"9:19.5",
	"20:9",
	"9:20",
	"auto",
}

type applicationDirectPromptField struct {
	label   string
	aliases []string
}

type applicationDirectPromptResolution struct {
	finalPrompt    string
	negativePrompt string
}

func applicationCatalog() []ApplicationDefinition {
	minOne := 1.0
	maxFour := 4.0
	maxFifteen := 15.0
	return []ApplicationDefinition{
		{
			Slug:           ApplicationSlugFreeMedia,
			Revision:       3,
			Version:        "1.2",
			Name:           "自由媒体工作台",
			Description:    "直接使用原始提示词，自由选择 GPT Image、Grok 图片或视频模型。",
			Category:       "media",
			Status:         "available",
			Tags:           []string{"GPT Image", "Grok", "图片", "视频"},
			SupportedModes: []string{"text_to_image", "image_edit", "text_to_video", "image_to_video"},
			Form: ApplicationFormDefinition{Fields: []ApplicationFormField{
				{Key: "mode", Label: "生成方式", Type: "select", Required: true, Options: []ApplicationFormOption{
					{Label: "文生图", Value: "text_to_image"},
					{Label: "图生图", Value: "image_edit"},
					{Label: "文生视频", Value: "text_to_video"},
					{Label: "图生视频", Value: "image_to_video"},
				}},
				{Key: "prompt", Label: "原始提示词", Type: "textarea", Required: true},
			}},
		},
		{
			Slug:           ApplicationSlugCreativeImage,
			Revision:       4,
			Version:        "1.2",
			Name:           "灵感精修生图",
			Description:    "把零散灵感整理成可执行的视觉方案，再生成或精修图片。",
			Category:       "image",
			Status:         "available",
			Tags:           []string{"GPT Image", "Grok", "生图", "图生图"},
			SupportedModes: []string{"text_to_image", "image_edit"},
			PlannerPrompt:  "你是一名视觉创意总监。将用户的零散灵感整理成清晰、可直接用于图像模型的提示词，保留主体、构图、光线、材质与风格约束，不虚构用户未要求的品牌事实。",
			Form: ApplicationFormDefinition{Fields: []ApplicationFormField{
				{Key: "brief", Label: "创作需求", Type: "textarea", Required: true, Placeholder: "描述主体、场景、风格和用途"},
				{Key: "mode", Label: "生成方式", Type: "select", Required: true, Default: "text_to_image", Options: []ApplicationFormOption{
					{Label: "文生图", Value: "text_to_image"},
					{Label: "参考图精修", Value: "image_edit"},
				}},
				{Key: "model", Label: "媒体模型", Type: "select", Required: true, Default: xai.ModelGrokImagineImageQuality, Options: imageModelOptions()},
				{Key: "aspect_ratio", Label: "画面比例", Type: "select", Default: "1:1", Options: grokImageAspectRatioOptions()},
				{Key: "resolution", Label: "清晰度", Type: "select", Default: "2k", Options: imageResolutionOptions()},
				{Key: "size", Label: "图片尺寸", Type: "select", Default: "1024x1024", Options: openAIImageSizeOptions()},
				{Key: "quality", Label: "图片质量", Type: "select", Default: "auto", Options: imageQualityOptions()},
				{Key: "n", Label: "生成数量", Type: "number", Default: 1, Min: &minOne, Max: &maxFour, Step: &minOne},
				{Key: "reference_files", Label: "参考图片", Type: "file", Accept: "image/png,image/jpeg,image/webp", Multiple: true, MaxItems: 5, VisibleWhen: map[string]any{"mode": "image_edit"}},
				{Key: "planner_model", Label: "策划模型", Type: "text", Description: "留空时按策划密钥所属平台选择默认模型"},
			}},
		},
		{
			Slug:           ApplicationSlugEcommerce,
			Revision:       4,
			Version:        "1.2",
			Name:           "电商视觉策划",
			Description:    "分析商品卖点、受众与投放场景，输出适合转化的商品视觉。",
			Category:       "commerce",
			Status:         "available",
			Tags:           []string{"GPT Image", "Grok", "电商", "商品图", "图生图"},
			SupportedModes: []string{"image_edit", "text_to_image"},
			PlannerPrompt:  "你是一名电商视觉策划总监。根据商品、卖点、受众和投放场景，生成突出真实商品特征且适合转化的视觉提示词。不得改动商标、包装文字、商品数量或用户明确要求保留的结构。",
			Form: ApplicationFormDefinition{Fields: []ApplicationFormField{
				{Key: "product_name", Label: "商品名称", Type: "text", Required: true},
				{Key: "selling_points", Label: "核心卖点", Type: "textarea", Required: true, Placeholder: "填写材质、功能、目标受众与必须保留的信息"},
				{Key: "placement", Label: "投放场景", Type: "select", Default: "product_detail", Options: []ApplicationFormOption{
					{Label: "商品详情页", Value: "product_detail"},
					{Label: "主图", Value: "hero"},
					{Label: "社交媒体", Value: "social"},
					{Label: "广告素材", Value: "advertisement"},
				}},
				{Key: "mode", Label: "生成方式", Type: "select", Required: true, Default: "image_edit", Options: []ApplicationFormOption{
					{Label: "参考商品图生成", Value: "image_edit"},
					{Label: "纯文字生成", Value: "text_to_image"},
				}},
				{Key: "model", Label: "媒体模型", Type: "select", Required: true, Default: xai.ModelGrokImagineImageQuality, Options: imageModelOptions()},
				{Key: "aspect_ratio", Label: "画面比例", Type: "select", Default: "1:1", Options: grokImageAspectRatioOptions()},
				{Key: "resolution", Label: "清晰度", Type: "select", Default: "2k", Options: imageResolutionOptions()},
				{Key: "size", Label: "图片尺寸", Type: "select", Default: "1024x1024", Options: openAIImageSizeOptions()},
				{Key: "quality", Label: "图片质量", Type: "select", Default: "auto", Options: imageQualityOptions()},
				{Key: "n", Label: "生成数量", Type: "number", Default: 1, Min: &minOne, Max: &maxFour, Step: &minOne},
				{Key: "reference_files", Label: "商品参考图", Type: "file", Required: true, Accept: "image/png,image/jpeg,image/webp", Multiple: true, MaxItems: 5, VisibleWhen: map[string]any{"mode": "image_edit"}},
				{Key: "planner_model", Label: "策划模型", Type: "text", Description: "留空时按策划密钥所属平台选择默认模型"},
			}},
		},
		{
			Slug:           ApplicationSlugGrokVideo,
			Revision:       2,
			Version:        "1.0",
			Name:           "Grok 视频导演",
			Description:    "先完成镜头与节奏规划，再执行文生视频、图生视频、参考生视频、编辑或续写。",
			Category:       "video",
			Status:         "available",
			Tags:           []string{"Grok", "视频", "导演"},
			SupportedModes: []string{"text_to_video", "image_to_video", "reference_to_video", "video_edit", "video_extend"},
			PlannerPrompt:  "你是一名短视频导演。把用户需求整理成可以直接交给 Grok Imagine Video 的镜头提示词，明确主体动作、镜头运动、光线、节奏、连续性与禁止变化的元素；不要添加无法从素材确认的品牌事实。",
			Form: ApplicationFormDefinition{Fields: []ApplicationFormField{
				{Key: "brief", Label: "视频需求", Type: "textarea", Required: true, Placeholder: "描述主体动作、镜头、节奏和用途"},
				{Key: "mode", Label: "视频方式", Type: "select", Required: true, Default: "text_to_video", Options: []ApplicationFormOption{
					{Label: "文生视频", Value: "text_to_video"},
					{Label: "图生视频", Value: "image_to_video"},
					{Label: "多图参考生视频", Value: "reference_to_video"},
					{Label: "视频编辑", Value: "video_edit"},
					{Label: "视频续写", Value: "video_extend"},
				}},
				{Key: "model", Label: "视频模型", Type: "select", Required: true, Default: xai.ModelGrokImagineVideo15, Options: []ApplicationFormOption{
					{Label: "Grok Imagine Video 1.5", Value: xai.ModelGrokImagineVideo15},
					{Label: "Grok Imagine Video", Value: xai.ModelGrokImagineVideo},
				}},
				{Key: "resolution", Label: "清晰度", Type: "select", Default: "720p", Options: []ApplicationFormOption{
					{Label: "480p", Value: "480p"},
					{Label: "720p", Value: "720p"},
					{Label: "1080p（仅 1.5 文生/图生视频）", Value: "1080p"},
				}},
				{Key: "duration", Label: "时长（秒）", Type: "number", Default: 6, Min: &minOne, Max: &maxFifteen, Step: &minOne},
				{Key: "aspect_ratio", Label: "画面比例", Type: "select", Default: "16:9", Options: videoAspectRatioOptions()},
				{Key: "reference_files", Label: "参考素材", Type: "file", Accept: "image/png,image/jpeg,image/webp,video/mp4,video/webm", Multiple: true, MaxItems: 5},
				{Key: "planner_model", Label: "策划模型", Type: "text", Description: "留空时按策划密钥所属平台选择默认模型"},
			}},
		},
	}
}

func imageModelOptions() []ApplicationFormOption {
	return []ApplicationFormOption{
		{Label: "GPT Image 2", Value: applicationGPTImageModel},
		{Label: "Grok Imagine Image Quality", Value: xai.ModelGrokImagineImageQuality},
		{Label: "Grok Imagine Image", Value: xai.ModelGrokImagineImage},
	}
}

func openAIImageSizeOptions() []ApplicationFormOption {
	return []ApplicationFormOption{
		{Label: "1:1", Value: "1024x1024"},
		{Label: "5:4", Value: "1280x1024"},
		{Label: "9:16", Value: "864x1536"},
		{Label: "21:9", Value: "1792x768"},
		{Label: "16:9", Value: "1536x864"},
		{Label: "3:2", Value: "1536x1024"},
		{Label: "4:3", Value: "1024x768"},
		{Label: "4:5", Value: "1024x1280"},
		{Label: "3:4", Value: "768x1024"},
		{Label: "2:3", Value: "1024x1536"},
	}
}

func imageQualityOptions() []ApplicationFormOption {
	return []ApplicationFormOption{
		{Label: "自动", Value: "auto"},
		{Label: "草稿", Value: "low"},
		{Label: "标准", Value: "medium"},
		{Label: "高清", Value: "high"},
	}
}

func grokImageAspectRatioOptions() []ApplicationFormOption {
	options := make([]ApplicationFormOption, 0, len(grokImageAspectRatios))
	for _, ratio := range grokImageAspectRatios {
		label := ratio
		if ratio == "auto" {
			label = "自动"
		}
		options = append(options, ApplicationFormOption{Label: label, Value: ratio})
	}
	return options
}

func normalizeGrokImageAspectRatio(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized, applicationStringAllowed(normalized, grokImageAspectRatios)
}

func videoAspectRatioOptions() []ApplicationFormOption {
	return []ApplicationFormOption{
		{Label: "16:9", Value: "16:9"},
		{Label: "1:1", Value: "1:1"},
		{Label: "9:16", Value: "9:16"},
		{Label: "3:2", Value: "3:2"},
		{Label: "2:3", Value: "2:3"},
	}
}

func imageResolutionOptions() []ApplicationFormOption {
	return []ApplicationFormOption{
		{Label: "1K", Value: "1k"},
		{Label: "2K", Value: "2k"},
	}
}

func findApplicationDefinition(slug string) (ApplicationDefinition, bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	switch slug {
	case "creative-image":
		slug = ApplicationSlugCreativeImage
	case "ecommerce-visual":
		slug = ApplicationSlugEcommerce
	}
	for _, app := range applicationCatalog() {
		if app.Slug == slug {
			return app, true
		}
	}
	return ApplicationDefinition{}, false
}

func findApplicationDefinitionForRun(slug string, revision int) (ApplicationDefinition, bool) {
	app, ok := findApplicationDefinition(slug)
	if !ok || revision <= 0 || revision > app.Revision {
		return ApplicationDefinition{}, false
	}
	return app, true
}

func validateApplicationInput(app ApplicationDefinition, input map[string]any) error {
	if _, err := resolveApplicationDirectPrompt(app, input); err != nil {
		return err
	}

	mode := resolveApplicationMode(app.Slug, input)
	if !applicationStringAllowed(mode, app.SupportedModes) {
		return ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "mode"})
	}
	if _, err := applicationReferenceURLs(input); err != nil {
		return ErrApplicationInvalidInput.WithCause(err).WithMetadata(map[string]string{"field": "reference_image_urls"})
	}
	if videoURL := applicationInputString(input, "video_url", ""); videoURL != "" {
		if _, err := validateApplicationURL(videoURL); err != nil {
			return ErrApplicationInvalidInput.WithCause(err).WithMetadata(map[string]string{"field": "video_url"})
		}
	}
	return nil
}

func defaultApplicationMode(slug string) string {
	switch slug {
	case ApplicationSlugEcommerce:
		return "image_edit"
	case ApplicationSlugGrokVideo:
		return "text_to_video"
	default:
		return "text_to_image"
	}
}

func resolveApplicationMode(slug string, input map[string]any) string {
	if mode := applicationInputString(input, "mode", ""); mode != "" {
		return mode
	}
	operation := applicationInputString(input, "operation", "")
	if slug == ApplicationSlugGrokVideo {
		switch operation {
		case "video_generation", "text_to_video":
			return "text_to_video"
		case "image_video_generation", "image_to_video":
			return "image_to_video"
		case "reference_video_generation", "reference_to_video":
			return "reference_to_video"
		case "video_edit":
			return "video_edit"
		case "video_extension", "video_extend":
			return "video_extend"
		}
	} else {
		switch operation {
		case "image_generation", "text_to_image":
			return "text_to_image"
		case "image_edit":
			return "image_edit"
		}
	}
	return defaultApplicationMode(slug)
}

func buildApplicationPlannerRequest(
	app ApplicationDefinition,
	input json.RawMessage,
	inputMap map[string]any,
	plannerKey *APIKey,
) (map[string]any, error) {
	plannerModel := applicationInputString(inputMap, "planner_model", "")
	if plannerModel == "" {
		plannerModel = defaultApplicationPlannerModel(plannerKey)
	}
	if len(plannerModel) > 128 {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "planner_model"})
	}
	return map[string]any{
		"model":  plannerModel,
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": app.PlannerPrompt + " 你必须严格输出 JSON，不得输出 Markdown。"},
			{"role": "user", "content": "请为以下输入生成最终媒体提示词：\n" + string(input)},
		},
		"response_format": applicationPlannerResponseFormat(plannerKey),
	}, nil
}

func buildDirectApplicationPlan(app ApplicationDefinition, input map[string]any) (map[string]any, error) {
	resolved, err := resolveApplicationDirectPrompt(app, input)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"final_prompt":    resolved.finalPrompt,
		"negative_prompt": resolved.negativePrompt,
		"rationale":       "",
	}, nil
}

func resolveApplicationDirectPrompt(
	app ApplicationDefinition,
	input map[string]any,
) (applicationDirectPromptResolution, error) {
	parts := make([]string, 0, 12)
	if prompt := firstApplicationInputText(input, applicationDirectPromptAliases...); prompt != "" {
		parts = append(parts, prompt)
	}
	for _, field := range applicationDirectPromptFields(app.Slug) {
		if value := firstApplicationInputText(input, field.aliases...); value != "" {
			parts = append(parts, field.label+"："+value)
		}
	}

	finalPrompt := strings.TrimSpace(strings.Join(parts, "\n"))
	if finalPrompt == "" || len(finalPrompt) > applicationDirectPromptMaxBytes {
		return applicationDirectPromptResolution{}, ErrApplicationInvalidInput.WithMetadata(
			map[string]string{"field": "direct_prompt"},
		)
	}
	negativePrompt := firstApplicationInputText(input, "negative_prompt", "avoid")
	if len(negativePrompt) > applicationNegativePromptMaxSize {
		return applicationDirectPromptResolution{}, ErrApplicationInvalidInput.WithMetadata(
			map[string]string{"field": "negative_prompt"},
		)
	}
	return applicationDirectPromptResolution{
		finalPrompt:    finalPrompt,
		negativePrompt: negativePrompt,
	}, nil
}

func applicationDirectPromptFields(slug string) []applicationDirectPromptField {
	switch slug {
	case ApplicationSlugCreativeImage:
		return []applicationDirectPromptField{
			{label: "使用场景", aliases: []string{"purpose"}},
			{label: "视觉风格", aliases: []string{"style"}},
			{label: "构图", aliases: []string{"composition"}},
			{label: "光线", aliases: []string{"lighting"}},
			{label: "配色", aliases: []string{"palette"}},
			{label: "必须保留", aliases: []string{"must_keep"}},
		}
	case ApplicationSlugGrokVideo:
		return []applicationDirectPromptField{
			{label: "视觉风格", aliases: []string{"style"}},
			{label: "主体动作", aliases: []string{"subject_action"}},
			{label: "镜头运动", aliases: []string{"camera_movement"}},
			{label: "节奏", aliases: []string{"pacing"}},
			{label: "连续性", aliases: []string{"continuity"}},
			{label: "禁止改动", aliases: []string{"must_keep"}},
		}
	case ApplicationSlugEcommerce:
		return []applicationDirectPromptField{
			{label: "商品", aliases: []string{"product_name", "product", "subject"}},
			{label: "核心卖点", aliases: []string{"selling_points"}},
			{label: "目标人群", aliases: []string{"audience"}},
			{label: "投放场景", aliases: []string{"placement"}},
			{label: "营销目标", aliases: []string{"marketing_goal"}},
			{label: "构图", aliases: []string{"composition"}},
			{label: "光线", aliases: []string{"lighting"}},
			{label: "配色", aliases: []string{"palette"}},
			{label: "文案要求", aliases: []string{"copy_requirements"}},
			{label: "必须保留", aliases: []string{"must_keep"}},
		}
	default:
		return nil
	}
}

func applicationPlannerResponseFormat(key *APIKey) map[string]any {
	if key != nil && key.Group != nil &&
		strings.EqualFold(strings.TrimSpace(key.Group.Platform), PlatformGrok) {
		return map[string]any{"type": "json_object"}
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "application_media_plan",
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"final_prompt":    map[string]any{"type": "string", "minLength": 1, "maxLength": 8000},
					"negative_prompt": map[string]any{"type": []string{"string", "null"}, "maxLength": 2000},
					"rationale":       map[string]any{"type": "string", "maxLength": 2000},
				},
				"required": []string{"final_prompt", "negative_prompt", "rationale"},
			},
		},
	}
}

func defaultApplicationPlannerModel(key *APIKey) string {
	platform := ""
	if key != nil && key.Group != nil {
		platform = strings.ToLower(strings.TrimSpace(key.Group.Platform))
	}
	switch platform {
	case PlatformAnthropic:
		return "claude-sonnet-4-5"
	case PlatformGrok:
		return "grok-4.5"
	case PlatformGemini, PlatformAntigravity:
		return "gemini-2.5-pro"
	default:
		return "gpt-5.5"
	}
}

func parseApplicationPlannerOutput(raw json.RawMessage) (json.RawMessage, map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "output"})
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, nil, ErrApplicationInvalidInput.WithCause(err)
	}
	if content, ok := decoded.(string); ok {
		candidate, err := extractApplicationPlannerJSON(content)
		if err != nil {
			return nil, nil, ErrApplicationInvalidInput.WithCause(err)
		}
		trimmed = []byte(candidate)
	}
	normalized, err := normalizeApplicationJSON(trimmed, applicationPlannerOutputMaxBytes, true)
	if err != nil {
		return nil, nil, ErrApplicationInvalidInput.WithCause(err)
	}
	plan, err := applicationJSONObject(normalized)
	if err != nil {
		return nil, nil, ErrApplicationInvalidInput.WithCause(err)
	}
	if len(plan) != 3 {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "output"})
	}
	finalPromptValue, ok := plan["final_prompt"]
	if !ok {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "final_prompt"})
	}
	finalPrompt, ok := finalPromptValue.(string)
	if !ok {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "final_prompt"})
	}
	finalPrompt = strings.TrimSpace(finalPrompt)
	if finalPrompt == "" {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "final_prompt"})
	}
	if len(finalPrompt) > 8000 {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "final_prompt"})
	}
	negativePromptValue, ok := plan["negative_prompt"]
	if !ok {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "negative_prompt"})
	}
	switch negativePrompt := negativePromptValue.(type) {
	case nil:
	case string:
		negativePrompt = strings.TrimSpace(negativePrompt)
		if len(negativePrompt) > 2000 {
			return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "negative_prompt"})
		}
		plan["negative_prompt"] = negativePrompt
	default:
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "negative_prompt"})
	}
	rationaleValue, ok := plan["rationale"]
	if !ok {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "rationale"})
	}
	rationale, ok := rationaleValue.(string)
	if !ok {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "rationale"})
	}
	rationale = strings.TrimSpace(rationale)
	if len(rationale) > 2000 {
		return nil, nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "rationale"})
	}
	plan["final_prompt"] = finalPrompt
	plan["rationale"] = rationale
	normalized, err = marshalApplicationJSON(plan, applicationPlannerOutputMaxBytes)
	if err != nil {
		return nil, nil, ErrApplicationInvalidInput.WithCause(err)
	}
	return normalized, plan, nil
}

func extractApplicationPlannerJSON(content string) (string, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		if newline := strings.IndexByte(content, '\n'); newline >= 0 {
			content = content[newline+1:]
		}
		content = strings.TrimSpace(content)
		if strings.HasSuffix(content, "```") {
			content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
		}
	}
	if json.Valid([]byte(content)) {
		return content, nil
	}
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start >= 0 && end > start {
		candidate := content[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("planner output is not valid JSON")
}

func buildApplicationMediaStep(
	run *ApplicationRun,
	app ApplicationDefinition,
	input map[string]any,
	plan map[string]any,
	mediaPlatform string,
	sequence int,
) (*ApplicationRunStep, error) {
	body, executor, err := buildApplicationMediaRequest(app, input, plan, mediaPlatform)
	if err != nil {
		return nil, err
	}
	request, err := marshalApplicationJSON(body, applicationStepRequestMaxBytes)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	now := time.Now()
	return &ApplicationRunStep{
		RunID:          run.ID,
		StepKey:        ApplicationStepMedia,
		Sequence:       sequence,
		Executor:       executor,
		CredentialRole: ApplicationCredentialMedia,
		Status:         ApplicationRunStatusPending,
		Request:        request,
		Attempt:        1,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func buildApplicationMediaRequest(
	app ApplicationDefinition,
	input map[string]any,
	plan map[string]any,
	mediaPlatform string,
) (map[string]any, string, error) {
	prompt, err := applicationMediaPrompt(plan)
	if err != nil {
		return nil, "", err
	}
	mode := resolveApplicationMode(app.Slug, input)
	if applicationModeRequiresGrokMedia(app, mode) {
		if !strings.EqualFold(strings.TrimSpace(mediaPlatform), PlatformGrok) && !strings.EqualFold(strings.TrimSpace(mediaPlatform), PlatformVideo) {
			return nil, "", ErrApplicationAPIKeyUnavailable
		}
		return buildApplicationVideoRequest(input, mode, prompt)
	}
	return buildApplicationImageRequestForPlatform(input, mode, prompt, mediaPlatform)
}

func applicationModeRequiresGrokMedia(app ApplicationDefinition, mode string) bool {
	if app.Slug == ApplicationSlugGrokVideo {
		return true
	}
	if app.Slug == ApplicationSlugFreeMedia {
		return mode == "text_to_video" || mode == "image_to_video"
	}
	return false
}

func applicationMediaPrompt(plan map[string]any) (string, error) {
	prompt, ok := plan["final_prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if !ok || prompt == "" {
		return "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "final_prompt"})
	}
	negativePrompt, ok := plan["negative_prompt"].(string)
	negativePrompt = strings.TrimSpace(negativePrompt)
	if ok && negativePrompt != "" {
		prompt += "\n\nNegative prompt (avoid): " + negativePrompt
	}
	return prompt, nil
}

func buildApplicationImageRequest(input map[string]any, mode, prompt string) (map[string]any, string, error) {
	return buildApplicationImageRequestForPlatform(input, mode, prompt, PlatformGrok)
}

func buildApplicationImageRequestForPlatform(
	input map[string]any,
	mode, prompt, mediaPlatform string,
) (map[string]any, string, error) {
	switch strings.ToLower(strings.TrimSpace(mediaPlatform)) {
	case PlatformOpenAI:
		return buildOpenAIApplicationImageRequest(input, mode, prompt)
	case PlatformGrok:
		return buildGrokApplicationImageRequest(input, mode, prompt)
	default:
		return nil, "", ErrApplicationAPIKeyUnavailable
	}
}

func buildGrokApplicationImageRequest(input map[string]any, mode, prompt string) (map[string]any, string, error) {
	model := applicationInputString(input, "model", xai.ModelGrokImagineImageQuality)
	if model == "grok-imagine" || model == "grok-imagine-edit" {
		model = xai.ModelGrokImagineImageQuality
	} else if canonical, ok := xai.CanonicalImageGenerationModel(model); ok {
		model = canonical
	} else {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "model"})
	}
	if mode != "text_to_image" && mode != "image_edit" {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "mode"})
	}
	body := map[string]any{"model": model, "prompt": prompt}
	ratio, ok := normalizeGrokImageAspectRatio(applicationInputString(input, "aspect_ratio", "1:1"))
	if !ok {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "aspect_ratio"})
	}
	resolution := strings.ToLower(applicationInputString(input, "resolution", "2k"))
	if !applicationStringAllowed(resolution, []string{"1k", "2k"}) {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "resolution"})
	}
	body["resolution"] = resolution
	n, err := resolveApplicationImageCount(input, applicationGrokImageMaxCount)
	if err != nil {
		return nil, "", err
	}
	body["n"] = n
	if mode == "image_edit" {
		references, err := applicationReferenceURLs(input)
		if err != nil {
			return nil, "", ErrApplicationInvalidInput.WithCause(err)
		}
		if len(references) > 0 {
			images := make([]map[string]string, 0, len(references))
			for _, reference := range references {
				images = append(images, map[string]string{"url": reference})
			}
			body["image"] = images[0]
			if len(images) > 1 {
				body["images"] = images
			}
		}
		if len(references) > 1 {
			body["aspect_ratio"] = ratio
		}
		return body, ApplicationExecutorImageEdit, nil
	}
	body["aspect_ratio"] = ratio
	return body, ApplicationExecutorImageGeneration, nil
}

func buildOpenAIApplicationImageRequest(input map[string]any, mode, prompt string) (map[string]any, string, error) {
	model := applicationInputString(input, "model", applicationGPTImageModel)
	if model != applicationGPTImageModel {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "model"})
	}
	if mode != "text_to_image" && mode != "image_edit" {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "mode"})
	}
	size := applicationInputString(input, "size", "")
	if size == "" {
		size = openAIImageSizeForRatio(applicationInputString(input, "aspect_ratio", "1:1"))
	}
	if !validOpenAIApplicationImageSize(size) {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "size"})
	}
	quality := strings.ToLower(applicationInputString(input, "quality", "auto"))
	if !applicationStringAllowed(quality, []string{"auto", "low", "medium", "high"}) {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "quality"})
	}
	n, err := resolveApplicationImageCount(input, applicationOpenAIImageMaxCount)
	if err != nil {
		return nil, "", err
	}
	body := map[string]any{
		"model": model, "prompt": prompt, "size": size, "quality": quality, "n": n,
	}
	if mode == "image_edit" {
		references, err := applicationReferenceURLs(input)
		if err != nil {
			return nil, "", ErrApplicationInvalidInput.WithCause(err)
		}
		if len(references) > 0 {
			images := make([]map[string]string, 0, len(references))
			for _, reference := range references {
				images = append(images, map[string]string{"url": reference})
			}
			body["image"] = images[0]
			if len(images) > 1 {
				body["images"] = images
			}
		}
		return body, ApplicationExecutorImageEdit, nil
	}
	return body, ApplicationExecutorImageGeneration, nil
}

func resolveApplicationImageCount(input map[string]any, maxCount int) (int, error) {
	value, ok := input["n"]
	if !ok {
		value, ok = input["output_count"]
	}
	if !ok {
		return 1, nil
	}

	count := 0
	valid := false
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		if err == nil {
			count, valid = parsed, true
		}
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) && math.Trunc(typed) == typed &&
			typed >= 1 && typed <= float64(maxCount) {
			count, valid = int(typed), true
		}
	case int:
		count, valid = typed, true
	case int64:
		if typed >= 1 && typed <= int64(maxCount) {
			count, valid = int(typed), true
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			count, valid = parsed, true
		}
	}
	if !valid || count < 1 || count > maxCount {
		return 0, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "n"})
	}
	return count, nil
}

func openAIImageSizeForRatio(ratio string) string {
	sizes := map[string]string{
		"1:1": "1024x1024", "5:4": "1280x1024", "9:16": "864x1536",
		"21:9": "1792x768", "16:9": "1536x864", "3:2": "1536x1024",
		"4:3": "1024x768", "4:5": "1024x1280", "3:4": "768x1024", "2:3": "1024x1536",
	}
	if size := sizes[strings.TrimSpace(ratio)]; size != "" {
		return size
	}
	return "1024x1024"
}

func validOpenAIApplicationImageSize(size string) bool {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return false
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 || width > 3840 || height > 3840 {
		return false
	}
	if width%16 != 0 || height%16 != 0 {
		return false
	}
	longEdge, shortEdge := max(width, height), min(width, height)
	pixels := width * height
	return longEdge <= shortEdge*3 && pixels >= 655360 && pixels <= 8294400
}

func buildApplicationVideoRequest(input map[string]any, mode, prompt string) (map[string]any, string, error) {
	defaultModel := xai.ModelGrokImagineVideo15
	if mode == "reference_to_video" || mode == "video_edit" || mode == "video_extend" {
		defaultModel = xai.ModelGrokImagineVideo
	}
	model := applicationInputString(input, "model", defaultModel)
	canonical, ok := xai.CanonicalVideoGenerationModel(model)
	if !ok {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "model"})
	}
	model = canonical
	if !applicationStringAllowed(mode, []string{"text_to_video", "image_to_video", "reference_to_video", "video_edit", "video_extend"}) {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "mode"})
	}
	if (mode == "reference_to_video" || mode == "video_edit" || mode == "video_extend") && model != xai.ModelGrokImagineVideo {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{
			"field": "model", "required_model": xai.ModelGrokImagineVideo,
		})
	}

	resolution := strings.ToLower(applicationInputString(input, "resolution", "720p"))
	if !applicationStringAllowed(resolution, []string{"480p", "720p", "1080p"}) {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "resolution"})
	}
	if resolution == "1080p" && (model != xai.ModelGrokImagineVideo15 || (mode != "text_to_video" && mode != "image_to_video")) {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{
			"field": "resolution", "required_modes": "text_to_video,image_to_video", "required_model": xai.ModelGrokImagineVideo15,
		})
	}
	duration := applicationInputInt(input, "duration", 6)
	if duration < 1 || duration > 15 {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "duration"})
	}
	ratio := applicationInputString(input, "aspect_ratio", "16:9")
	if !applicationStringAllowed(ratio, []string{"16:9", "1:1", "9:16", "3:2", "2:3"}) {
		return nil, "", ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "aspect_ratio"})
	}
	body := map[string]any{
		"model": model, "prompt": prompt, "resolution": resolution,
		"duration": duration, "aspect_ratio": ratio,
	}
	references, err := applicationReferenceURLs(input)
	if err != nil {
		return nil, "", ErrApplicationInvalidInput.WithCause(err)
	}
	switch mode {
	case "text_to_video":
		return body, ApplicationExecutorVideoGeneration, nil
	case "image_to_video":
		if len(references) > 0 {
			body["image"] = map[string]string{"url": references[0]}
		}
		return body, ApplicationExecutorVideoGeneration, nil
	case "reference_to_video":
		if len(references) > 0 {
			items := make([]map[string]string, 0, len(references))
			for _, reference := range references {
				items = append(items, map[string]string{"url": reference})
			}
			body["reference_images"] = items
		}
		return body, ApplicationExecutorVideoGeneration, nil
	case "video_edit", "video_extend":
		if rawVideoURL := applicationInputString(input, "video_url", ""); rawVideoURL != "" {
			videoURL, err := validateApplicationURL(rawVideoURL)
			if err != nil {
				return nil, "", ErrApplicationInvalidInput.WithCause(err)
			}
			body["video"] = map[string]string{"url": videoURL}
		}
		if mode == "video_edit" {
			return body, ApplicationExecutorVideoEdit, nil
		}
		return body, ApplicationExecutorVideoExtension, nil
	default:
		return nil, "", ErrApplicationInvalidInput
	}
}

func normalizeApplicationMediaOutput(executor string, raw json.RawMessage) (json.RawMessage, error) {
	normalized, err := normalizeApplicationJSON(raw, applicationMediaOutputMaxBytes, true)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	value, err := applicationJSONObject(normalized)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	if executor == ApplicationExecutorImageGeneration || executor == ApplicationExecutorImageEdit {
		return normalizeApplicationImageOutput(value)
	}
	return normalizeApplicationVideoOutput(value)
}

func normalizeApplicationMediaCheckpoint(executor string, raw json.RawMessage) (json.RawMessage, error) {
	normalized, err := normalizeApplicationJSON(raw, applicationMediaOutputMaxBytes, true)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	value, err := applicationJSONObject(normalized)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	if isApplicationImageExecutor(executor) {
		return normalizeApplicationImageCheckpoint(value)
	}
	if isApplicationVideoExecutor(executor) {
		return normalizeApplicationVideoOutput(value)
	}
	return nil, ErrApplicationInvalidTransition
}

func normalizeApplicationImageCheckpoint(value map[string]any) (json.RawMessage, error) {
	taskID := firstApplicationInputText(value, "task_id", "request_id")
	if taskID == "" || len(taskID) > applicationMaxRequestIDBytes {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "task_id"})
	}
	status := strings.ToLower(firstApplicationInputText(value, "status"))
	if status == "" {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "status"})
	}
	result, err := marshalApplicationJSON(map[string]any{
		"task_id": taskID,
		"status":  boundedApplicationText(status, 64),
	}, applicationMediaOutputMaxBytes)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	return result, nil
}

func normalizeApplicationImageOutput(value map[string]any) (json.RawMessage, error) {
	out := map[string]any{}
	imagesValue, _ := value["images"].([]any)
	images := make([]map[string]any, 0, min(len(imagesValue), 8))
	for _, item := range imagesValue {
		image, ok := item.(map[string]any)
		if !ok || len(images) >= 8 {
			continue
		}
		entry := map[string]any{}
		if rawURL := strings.TrimSpace(fmt.Sprint(image["url"])); rawURL != "" && rawURL != "<nil>" {
			imageURL, err := validateApplicationURL(rawURL)
			if err != nil {
				return nil, ErrApplicationInvalidInput.WithCause(err)
			}
			entry["url"] = imageURL
		}
		if prompt := strings.TrimSpace(fmt.Sprint(image["revised_prompt"])); prompt != "" && prompt != "<nil>" {
			entry["revised_prompt"] = boundedApplicationText(prompt, 600)
		}
		if len(entry) > 0 {
			images = append(images, entry)
		}
	}
	count := applicationInputInt(value, "count", len(images))
	if count < 0 || count > 100 {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "count"})
	}
	out["count"] = count
	out["images"] = images
	result, err := marshalApplicationJSON(out, applicationMediaOutputMaxBytes)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	return result, nil
}

func normalizeApplicationVideoOutput(value map[string]any) (json.RawMessage, error) {
	requestID := strings.TrimSpace(firstApplicationInputText(value, "request_id", "id"))
	if requestID == "" || len(requestID) > applicationMaxRequestIDBytes {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "request_id"})
	}
	out := map[string]any{"request_id": requestID}
	if status := strings.ToLower(strings.TrimSpace(firstApplicationInputText(value, "status"))); status != "" {
		out["status"] = boundedApplicationText(status, 64)
	}
	if rawURL := strings.TrimSpace(firstApplicationInputText(value, "video_url", "url")); rawURL != "" {
		videoURL, err := validateApplicationURL(rawURL)
		if err != nil {
			return nil, ErrApplicationInvalidInput.WithCause(err)
		}
		out["video_url"] = videoURL
	}
	result, err := marshalApplicationJSON(out, applicationMediaOutputMaxBytes)
	if err != nil {
		return nil, ErrApplicationInvalidInput.WithCause(err)
	}
	return result, nil
}

func applicationReferenceURLs(input map[string]any) ([]string, error) {
	rawItems := make([]any, 0, 8)
	for _, key := range []string{"reference_image_urls", "reference_urls", "image_urls"} {
		if values, ok := input[key].([]any); ok {
			rawItems = append(rawItems, values...)
		}
	}
	for _, key := range []string{"source_image_url", "image_url"} {
		if value := strings.TrimSpace(fmt.Sprint(input[key])); value != "" && value != "<nil>" {
			rawItems = append(rawItems, value)
		}
	}
	if len(rawItems) > 8 {
		return nil, fmt.Errorf("at most 8 media URLs are allowed")
	}
	out := make([]string, 0, len(rawItems))
	seen := make(map[string]struct{}, len(rawItems))
	for _, item := range rawItems {
		rawURL := strings.TrimSpace(fmt.Sprint(item))
		if rawURL == "" || rawURL == "<nil>" {
			continue
		}
		mediaURL, err := validateApplicationURL(rawURL)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[mediaURL]; ok {
			continue
		}
		seen[mediaURL] = struct{}{}
		out = append(out, mediaURL)
	}
	return out, nil
}

func firstApplicationInputText(input map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key]; ok {
			text, ok := value.(string)
			if ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func applicationInputString(input map[string]any, key, fallback string) string {
	if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func applicationInputInt(input map[string]any, key string, fallback int) int {
	value, ok := input[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		if err == nil {
			return parsed
		}
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func applicationStringAllowed(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
