package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type applicationCenterAPIKeyRepoStub struct {
	APIKeyRepository
	keys map[int64]*APIKey
}

func applicationKeyID(id int64) *int64 {
	return &id
}

func (s *applicationCenterAPIKeyRepoStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	key, ok := s.keys[id]
	if !ok {
		return nil, ErrAPIKeyNotFound
	}
	copy := *key
	return &copy, nil
}

type applicationCenterRunRepoStub struct {
	ApplicationRunRepository
	created CreateApplicationRunParams
	run     *ApplicationRun
}

func (s *applicationCenterRunRepoStub) Create(_ context.Context, params CreateApplicationRunParams) error {
	s.created = params
	runCopy := *params.Run
	stepCopy := *params.InitialStep
	runCopy.Steps = []*ApplicationRunStep{&stepCopy}
	s.run = &runCopy
	return nil
}

func (s *applicationCenterRunRepoStub) GetByID(_ context.Context, userID int64, runID string) (*ApplicationRun, error) {
	if s.run == nil || s.run.ID != runID || s.run.UserID != userID {
		return nil, ErrApplicationRunNotFound
	}
	runCopy := *s.run
	runCopy.Steps = append([]*ApplicationRunStep(nil), s.run.Steps...)
	return &runCopy, nil
}

func (s *applicationCenterRunRepoStub) CheckpointStep(_ context.Context, params CheckpointApplicationStepParams) error {
	if s.run == nil || s.run.ID != params.RunID || s.run.UserID != params.UserID || s.run.CurrentStepKey != params.StepKey {
		return ErrApplicationInvalidTransition
	}
	for _, step := range s.run.Steps {
		if step.StepKey == params.StepKey {
			step.Status = ApplicationRunStatusRunning
			step.Output = append(json.RawMessage(nil), params.StepOutput...)
			step.GatewayRequestID = params.GatewayRequestID
			return nil
		}
	}
	return ErrApplicationStepNotFound
}

func (s *applicationCenterRunRepoStub) CompleteStep(_ context.Context, params CompleteApplicationStepParams) error {
	if s.run == nil || s.run.ID != params.RunID || s.run.UserID != params.UserID || s.run.CurrentStepKey != params.StepKey {
		return ErrApplicationInvalidTransition
	}
	for _, step := range s.run.Steps {
		if step.StepKey != params.StepKey {
			continue
		}
		step.Status = ApplicationRunStatusCompleted
		step.Output = append(json.RawMessage(nil), params.StepOutput...)
		step.GatewayRequestID = params.GatewayRequestID
		if params.NextStep != nil {
			s.run.Steps = append(s.run.Steps, params.NextStep)
			s.run.CurrentStepKey = params.NextStep.StepKey
		} else {
			s.run.Status = ApplicationRunStatusCompleted
			s.run.CurrentStepKey = ""
			s.run.Output = append(json.RawMessage(nil), params.RunOutput...)
		}
		return nil
	}
	return ErrApplicationStepNotFound
}

func (s *applicationCenterRunRepoStub) List(context.Context, int64, pagination.PaginationParams, string) ([]*ApplicationRun, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func TestApplicationCatalogIsVersionedAndFinite(t *testing.T) {
	apps := applicationCatalog()
	require.Len(t, apps, 4)
	require.Equal(t, []string{
		ApplicationSlugFreeMedia,
		ApplicationSlugCreativeImage,
		ApplicationSlugEcommerce,
		ApplicationSlugGrokVideo,
	}, []string{apps[0].Slug, apps[1].Slug, apps[2].Slug, apps[3].Slug})
	for _, app := range apps {
		require.Positive(t, app.Revision)
		require.NotEmpty(t, app.Name)
		require.NotEmpty(t, app.Description)
		require.NotEmpty(t, app.SupportedModes)
		require.NotEmpty(t, app.Form.Fields)
	}
	require.Equal(t, []int{3, 4, 4, 2}, []int{apps[0].Revision, apps[1].Revision, apps[2].Revision, apps[3].Revision})
	require.Equal(t, []string{"1.2", "1.2", "1.2", "1.0"}, []string{apps[0].Version, apps[1].Version, apps[2].Version, apps[3].Version})

	creativeAlias, ok := findApplicationDefinition("creative-image")
	require.True(t, ok)
	require.Equal(t, ApplicationSlugCreativeImage, creativeAlias.Slug)
	ecommerceAlias, ok := findApplicationDefinition("ecommerce-visual")
	require.True(t, ok)
	require.Equal(t, ApplicationSlugEcommerce, ecommerceAlias.Slug)
}

func TestApplicationCatalogDoesNotExposeLegacyClientManagedFlag(t *testing.T) {
	payload, err := json.Marshal(applicationCatalog())
	require.NoError(t, err)
	require.NotContains(t, string(payload), "client_managed")
}

func TestApplicationCenterFreeMediaUsesDirectMediaStepWithoutPlanner(t *testing.T) {
	runRepo := &applicationCenterRunRepoStub{}
	keyRepo := &applicationCenterAPIKeyRepoStub{keys: map[int64]*APIKey{
		12: {ID: 12, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{
			Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true,
		}},
	}}
	svc := NewApplicationCenterService(runRepo, keyRepo)

	run, err := svc.StartRun(context.Background(), 7, ApplicationSlugFreeMedia, StartApplicationRunRequest{
		MediaAPIKeyID: 12,
		Input:         json.RawMessage(`{"mode":"text_to_image","prompt":"preserve the supplied subject"}`),
	})

	require.NoError(t, err)
	require.Nil(t, run.PlannerAPIKeyID)
	require.Equal(t, ApplicationStepMedia, run.CurrentStepKey)
	require.Len(t, run.Steps, 1)
	require.Equal(t, ApplicationExecutorImageGeneration, run.NextAction.Executor)
	require.Equal(t, "gpt-image-2", run.NextAction.Request["model"])
	require.Equal(t, "preserve the supplied subject", run.NextAction.Request["prompt"])
}

func TestApplicationCenterFreeMediaVideoRequiresGrokAndBuildsVideoStep(t *testing.T) {
	runRepo := &applicationCenterRunRepoStub{}
	keyRepo := &applicationCenterAPIKeyRepoStub{keys: map[int64]*APIKey{
		12: {ID: 12, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{
			Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true,
		}},
		13: {ID: 13, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{
			Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true,
		}},
	}}
	svc := NewApplicationCenterService(runRepo, keyRepo)

	run, err := svc.StartRun(context.Background(), 7, ApplicationSlugFreeMedia, StartApplicationRunRequest{
		MediaAPIKeyID: 12,
		Input:         json.RawMessage(`{"mode":"text_to_video","prompt":"animate the supplied subject","model":"grok-imagine-video"}`),
	})
	require.NoError(t, err)
	require.Equal(t, ApplicationExecutorVideoGeneration, run.NextAction.Executor)
	require.Equal(t, xai.ModelGrokImagineVideo, run.NextAction.Request["model"])

	_, err = svc.StartRun(context.Background(), 7, ApplicationSlugFreeMedia, StartApplicationRunRequest{
		MediaAPIKeyID: 13,
		Input:         json.RawMessage(`{"mode":"text_to_video","prompt":"animate the supplied subject"}`),
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_API_KEY_UNAVAILABLE", infraerrors.Reason(err))
}

func TestApplicationCenterFreeMediaCompletesThroughUnifiedMediaStep(t *testing.T) {
	runRepo := &applicationCenterRunRepoStub{}
	keyRepo := &applicationCenterAPIKeyRepoStub{keys: map[int64]*APIKey{
		12: {ID: 12, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{
			Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true,
		}},
	}}
	svc := NewApplicationCenterService(runRepo, keyRepo)
	run, err := svc.StartRun(context.Background(), 7, ApplicationSlugFreeMedia, StartApplicationRunRequest{
		MediaAPIKeyID: 12,
		Input:         json.RawMessage(`{"mode":"text_to_image","prompt":"a clean product shot"}`),
	})
	require.NoError(t, err)

	completed, err := svc.CompleteStep(context.Background(), 7, run.ID, ApplicationStepMedia, CompleteApplicationStepRequest{
		Output: json.RawMessage(`{"count":1,"images":[{"url":"https://cdn.example/product.png"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, ApplicationRunStatusCompleted, completed.Status)
	require.Empty(t, completed.CurrentStepKey)
	require.Len(t, completed.Steps, 1)
	require.Equal(t, ApplicationRunStatusCompleted, completed.Steps[0].Status)
	require.JSONEq(t, `{"count":1,"images":[{"url":"https://cdn.example/product.png"}]}`, string(completed.Output))
}

func TestApplicationCenterStartRunValidatesKeysAndBuildsStrictPlannerAction(t *testing.T) {
	runRepo := &applicationCenterRunRepoStub{}
	keyRepo := &applicationCenterAPIKeyRepoStub{keys: map[int64]*APIKey{
		11: {ID: 11, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{Platform: PlatformOpenAI, Status: StatusActive}},
		12: {ID: 12, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true}},
	}}
	svc := NewApplicationCenterService(runRepo, keyRepo)

	run, err := svc.StartRun(context.Background(), 7, ApplicationSlugCreativeImage, StartApplicationRunRequest{
		PlannerAPIKeyID: applicationKeyID(11),
		MediaAPIKeyID:   12,
		Input:           json.RawMessage(`{"idea":"雨夜里的霓虹书店","operation":"image_edit","reference_count":1}`),
	})
	require.NoError(t, err)
	require.Equal(t, ApplicationRunStatusRunning, run.Status)
	require.Equal(t, "app:"+run.ID, run.SessionID)
	require.NotNil(t, run.NextAction)
	require.Equal(t, ApplicationExecutorChatCompletions, run.NextAction.Executor)
	require.Equal(t, int64(11), run.NextAction.APIKeyID)
	require.Equal(t, "/v1/chat/completions", run.NextAction.Path)
	require.Equal(t, false, run.NextAction.Request["stream"])
	responseFormat := run.NextAction.Request["response_format"].(map[string]any)
	jsonSchema := responseFormat["json_schema"].(map[string]any)
	require.Equal(t, true, jsonSchema["strict"])

	keyRepo.keys[11].Group.Platform = PlatformGrok
	run, err = svc.StartRun(context.Background(), 7, ApplicationSlugCreativeImage, StartApplicationRunRequest{
		PlannerAPIKeyID: applicationKeyID(11), MediaAPIKeyID: 12, Input: json.RawMessage(`{"idea":"grok planner"}`),
	})
	require.NoError(t, err)
	require.Equal(t, "json_object", run.NextAction.Request["response_format"].(map[string]any)["type"])

	keyRepo.keys[11].UserID = 99
	_, err = svc.StartRun(context.Background(), 7, ApplicationSlugCreativeImage, StartApplicationRunRequest{
		PlannerAPIKeyID: applicationKeyID(11), MediaAPIKeyID: 12, Input: json.RawMessage(`{"idea":"test idea"}`),
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_API_KEY_UNAVAILABLE", infraerrors.Reason(err))

	keyRepo.keys[11].UserID = 7
	keyRepo.keys[12].Group.Platform = PlatformOpenAI
	_, err = svc.StartRun(context.Background(), 7, ApplicationSlugCreativeImage, StartApplicationRunRequest{
		PlannerAPIKeyID: applicationKeyID(11), MediaAPIKeyID: 12, Input: json.RawMessage(`{"idea":"test idea"}`),
	})
	require.NoError(t, err)

	keyRepo.keys[12].Group.Platform = PlatformGrok
	keyRepo.keys[12].Group.AllowImageGeneration = false
	_, err = svc.StartRun(context.Background(), 7, ApplicationSlugCreativeImage, StartApplicationRunRequest{
		PlannerAPIKeyID: applicationKeyID(11), MediaAPIKeyID: 12, Input: json.RawMessage(`{"idea":"test idea"}`),
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_API_KEY_UNAVAILABLE", infraerrors.Reason(err))
}

func TestApplicationCenterStartRunWithoutPlannerUsesOriginalPromptAndOpenAIImageContract(t *testing.T) {
	runRepo := &applicationCenterRunRepoStub{}
	keyRepo := &applicationCenterAPIKeyRepoStub{keys: map[int64]*APIKey{
		12: {ID: 12, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{
			Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true,
		}},
	}}
	svc := NewApplicationCenterService(runRepo, keyRepo)

	run, err := svc.StartRun(context.Background(), 7, ApplicationSlugCreativeImage, StartApplicationRunRequest{
		MediaAPIKeyID: 12,
		Input: json.RawMessage(`{
			"brief":"professional prompt, preserve every supplied detail",
			"mode":"text_to_image",
			"model":"gpt-image-2",
			"size":"1536x1024",
			"quality":"high",
			"n":2
		}`),
	})
	require.NoError(t, err)
	require.Nil(t, run.PlannerAPIKeyID)
	require.Equal(t, ApplicationStepMedia, run.CurrentStepKey)
	require.Len(t, run.Steps, 1)
	require.Equal(t, 1, run.Steps[0].Sequence)
	require.NotNil(t, run.NextAction)
	require.Equal(t, ApplicationExecutorImageGeneration, run.NextAction.Executor)
	require.Equal(t, "professional prompt, preserve every supplied detail", run.NextAction.Request["prompt"])
	require.Equal(t, "gpt-image-2", run.NextAction.Request["model"])
	require.Equal(t, "1536x1024", run.NextAction.Request["size"])
	require.Equal(t, "high", run.NextAction.Request["quality"])
	require.NotContains(t, run.NextAction.Request, "resolution")
	require.NotContains(t, run.NextAction.Request, "aspect_ratio")
}

func TestApplicationDirectPromptResolverPreservesStructuredFieldsInStableOrder(t *testing.T) {
	tests := []struct {
		name   string
		slug   string
		input  map[string]any
		prompt string
	}{
		{
			name: "creative image",
			slug: ApplicationSlugCreativeImage,
			input: map[string]any{
				"direct_prompt": "保持专业摄影师写好的主体描述",
				"purpose":       "品牌视觉",
				"style":         "商业棚拍",
				"composition":   "主体居中，右侧留白",
				"lighting":      "柔和轮廓光",
				"palette":       "冷白与品牌蓝",
				"must_keep":     "包装文字和 Logo",
			},
			prompt: "保持专业摄影师写好的主体描述\n" +
				"使用场景：品牌视觉\n" +
				"视觉风格：商业棚拍\n" +
				"构图：主体居中，右侧留白\n" +
				"光线：柔和轮廓光\n" +
				"配色：冷白与品牌蓝\n" +
				"必须保留：包装文字和 Logo",
		},
		{
			name: "video director",
			slug: ApplicationSlugGrokVideo,
			input: map[string]any{
				"brief":           "一镜到底展示手表",
				"style":           "商业广告",
				"subject_action":  "手表缓慢旋转",
				"camera_movement": "稳定环绕",
				"pacing":          "前慢后快",
				"continuity":      "表盘刻度保持一致",
				"must_keep":       "品牌标识",
			},
			prompt: "一镜到底展示手表\n" +
				"视觉风格：商业广告\n" +
				"主体动作：手表缓慢旋转\n" +
				"镜头运动：稳定环绕\n" +
				"节奏：前慢后快\n" +
				"连续性：表盘刻度保持一致\n" +
				"禁止改动：品牌标识",
		},
		{
			name: "ecommerce placement",
			slug: ApplicationSlugEcommerce,
			input: map[string]any{
				"product_name":      "机械键盘",
				"selling_points":    "低延迟和热插拔",
				"audience":          "电竞玩家",
				"placement":         "hero",
				"marketing_goal":    "突出响应速度",
				"composition":       "键盘占画面三分之二",
				"lighting":          "轮廓光",
				"palette":           "黑色与荧光绿",
				"copy_requirements": "右上角保留标题区",
				"must_keep":         "键帽字符准确",
			},
			prompt: "商品：机械键盘\n" +
				"核心卖点：低延迟和热插拔\n" +
				"目标人群：电竞玩家\n" +
				"投放场景：hero\n" +
				"营销目标：突出响应速度\n" +
				"构图：键盘占画面三分之二\n" +
				"光线：轮廓光\n" +
				"配色：黑色与荧光绿\n" +
				"文案要求：右上角保留标题区\n" +
				"必须保留：键帽字符准确",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, ok := findApplicationDefinition(tt.slug)
			require.True(t, ok)
			require.NoError(t, validateApplicationInput(app, tt.input))
			plan, err := buildDirectApplicationPlan(app, tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.prompt, plan["final_prompt"])
		})
	}

}

func TestApplicationCenterRejectsOpenAIKeyForVideoApplication(t *testing.T) {
	keyRepo := &applicationCenterAPIKeyRepoStub{keys: map[int64]*APIKey{
		12: {ID: 12, UserID: 7, Status: StatusAPIKeyActive, Group: &Group{
			Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true,
		}},
	}}
	svc := NewApplicationCenterService(&applicationCenterRunRepoStub{}, keyRepo)

	_, err := svc.StartRun(context.Background(), 7, ApplicationSlugGrokVideo, StartApplicationRunRequest{
		MediaAPIKeyID: 12,
		Input:         json.RawMessage(`{"brief":"animate this scene","mode":"text_to_video"}`),
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_API_KEY_UNAVAILABLE", infraerrors.Reason(err))
}

func TestApplicationMediaRequestsUseThirdPartyGrokCapabilities(t *testing.T) {
	options := grokImageAspectRatioOptions()
	optionValues := make([]string, 0, len(options))
	for _, option := range options {
		value, ok := option.Value.(string)
		require.True(t, ok)
		optionValues = append(optionValues, value)
	}
	require.Equal(t, grokImageAspectRatios, optionValues)

	for _, ratio := range grokImageAspectRatios {
		image, executor, err := buildApplicationImageRequest(map[string]any{
			"model": xai.ModelGrokImagineImageQuality, "aspect_ratio": ratio, "resolution": "2k",
		}, "text_to_image", "draw a product")
		require.NoError(t, err, ratio)
		require.Equal(t, ApplicationExecutorImageGeneration, executor)
		require.Equal(t, "2k", image["resolution"])
		require.Equal(t, ratio, image["aspect_ratio"])
	}

	singleEdit, executor, err := buildApplicationImageRequest(map[string]any{
		"model": xai.ModelGrokImagineImageQuality, "aspect_ratio": "16:9", "resolution": "2k",
		"reference_image_urls": []any{"https://cdn.example/source.png"},
	}, "image_edit", "edit a product")
	require.NoError(t, err)
	require.Equal(t, ApplicationExecutorImageEdit, executor)
	require.NotContains(t, singleEdit, "aspect_ratio")

	multiEdit, executor, err := buildApplicationImageRequest(map[string]any{
		"model": xai.ModelGrokImagineImageQuality, "aspect_ratio": "4:3", "resolution": "2k",
		"reference_image_urls": []any{
			"https://cdn.example/source-1.png",
			"https://cdn.example/source-2.png",
		},
	}, "image_edit", "combine two products")
	require.NoError(t, err)
	require.Equal(t, ApplicationExecutorImageEdit, executor)
	require.Equal(t, "4:3", multiEdit["aspect_ratio"])
	require.Len(t, multiEdit["images"], 2)

	for _, invalid := range []map[string]any{
		{"aspect_ratio": "5:4", "resolution": "2k"},
		{"aspect_ratio": "1:1", "resolution": "4k"},
	} {
		_, _, err = buildApplicationImageRequest(invalid, "text_to_image", "draw a product")
		require.Error(t, err)
	}

	for _, ratio := range []string{"16:9", "1:1", "9:16", "3:2", "2:3"} {
		video, executor, err := buildApplicationVideoRequest(map[string]any{"aspect_ratio": ratio}, "text_to_video", "animate")
		require.NoError(t, err)
		require.Equal(t, ApplicationExecutorVideoGeneration, executor)
		require.Equal(t, ratio, video["aspect_ratio"])
	}
	_, _, err = buildApplicationVideoRequest(map[string]any{"aspect_ratio": "4:3"}, "text_to_video", "animate")
	require.Error(t, err)
}

func TestApplicationOpenAIImageRequestUsesSizeAndQuality(t *testing.T) {
	body, executor, err := buildApplicationImageRequestForPlatform(map[string]any{
		"model": applicationGPTImageModel,
		"size":  "1024x1536", "quality": "medium", "n": json.Number("4"),
	}, "image_edit", "keep the exact product details", PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, ApplicationExecutorImageEdit, executor)
	require.Equal(t, applicationGPTImageModel, body["model"])
	require.Equal(t, "1024x1536", body["size"])
	require.Equal(t, "medium", body["quality"])
	require.NotContains(t, body, "resolution")
	require.NotContains(t, body, "aspect_ratio")

	_, _, err = buildApplicationImageRequestForPlatform(map[string]any{
		"model": xai.ModelGrokImagineImageQuality,
	}, "text_to_image", "wrong platform", PlatformOpenAI)
	require.Error(t, err)
}

func TestApplicationImageRequestEnforcesPlatformCountLimits(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		model    string
		accepted int
		rejected int
	}{
		{
			name: "grok accepts four and rejects five", platform: PlatformGrok,
			model: xai.ModelGrokImagineImageQuality, accepted: 4, rejected: 5,
		},
		{
			name: "openai accepts eight and rejects nine", platform: PlatformOpenAI,
			model: applicationGPTImageModel, accepted: 8, rejected: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _, err := buildApplicationImageRequestForPlatform(map[string]any{
				"model": tt.model,
				"n":     json.Number(strconv.Itoa(tt.accepted)),
			}, "text_to_image", "draw", tt.platform)
			require.NoError(t, err)
			require.Equal(t, tt.accepted, body["n"])

			_, _, err = buildApplicationImageRequestForPlatform(map[string]any{
				"model": tt.model,
				"n":     json.Number(strconv.Itoa(tt.rejected)),
			}, "text_to_image", "draw", tt.platform)
			require.Error(t, err)
			require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))
		})
	}

	_, _, err := buildApplicationImageRequestForPlatform(map[string]any{
		"model": xai.ModelGrokImagineImageQuality,
		"n":     json.Number("4.5"),
	}, "text_to_image", "draw", PlatformGrok)
	require.Error(t, err)
	require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))
}

func TestApplicationPlannerOutputAcceptsFencedJSONAndRequiresFinalPrompt(t *testing.T) {
	raw, err := json.Marshal("```json\n{\"final_prompt\":\"cinematic cat\",\"negative_prompt\":null,\"rationale\":\"clear subject\"}\n```")
	require.NoError(t, err)
	normalized, plan, err := parseApplicationPlannerOutput(raw)
	require.NoError(t, err)
	require.JSONEq(t, `{"final_prompt":"cinematic cat","negative_prompt":null,"rationale":"clear subject"}`, string(normalized))
	require.Equal(t, "cinematic cat", plan["final_prompt"])

	_, _, err = parseApplicationPlannerOutput(json.RawMessage(`{"rationale":"missing"}`))
	require.Error(t, err)
	require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))

	for _, invalid := range []json.RawMessage{
		json.RawMessage(`{"final_prompt":12,"negative_prompt":null,"rationale":"reason"}`),
		json.RawMessage(`{"final_prompt":"prompt","negative_prompt":false,"rationale":"reason"}`),
		json.RawMessage(`{"final_prompt":"prompt","negative_prompt":null,"rationale":12}`),
		json.RawMessage(`{"final_prompt":"prompt","negative_prompt":null,"rationale":"reason","extra":true}`),
	} {
		_, _, err = parseApplicationPlannerOutput(invalid)
		require.Error(t, err)
		require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))
	}
}

func TestApplicationPlannerDefaultsAndMediaPrompt(t *testing.T) {
	require.Equal(t, "gpt-5.5", defaultApplicationPlannerModel(&APIKey{Group: &Group{Platform: PlatformOpenAI}}))

	prompt, err := applicationMediaPrompt(map[string]any{
		"final_prompt":    "cinematic product shot",
		"negative_prompt": "blur, distorted logo",
	})
	require.NoError(t, err)
	require.Equal(t, "cinematic product shot\n\nNegative prompt (avoid): blur, distorted logo", prompt)

	prompt, err = applicationMediaPrompt(map[string]any{"final_prompt": "cinematic product shot", "negative_prompt": nil})
	require.NoError(t, err)
	require.Equal(t, "cinematic product shot", prompt)
}

func TestApplicationDefinitionForRunAllowsOlderRevision(t *testing.T) {
	current, ok := findApplicationDefinition(ApplicationSlugCreativeImage)
	require.True(t, ok)
	require.Greater(t, current.Revision, 1)

	resolved, ok := findApplicationDefinitionForRun(ApplicationSlugCreativeImage, current.Revision-1)
	require.True(t, ok)
	require.Equal(t, current.Revision, resolved.Revision)

	_, ok = findApplicationDefinitionForRun(ApplicationSlugCreativeImage, current.Revision+1)
	require.False(t, ok)
}

func TestApplicationFrontendOperationCompatibilityAndVideoModelRules(t *testing.T) {
	require.Equal(t, "image_edit", resolveApplicationMode(ApplicationSlugCreativeImage, map[string]any{"operation": "image_edit"}))
	require.Equal(t, "image_to_video", resolveApplicationMode(ApplicationSlugGrokVideo, map[string]any{"operation": "image_video_generation"}))
	require.Equal(t, "video_extend", resolveApplicationMode(ApplicationSlugGrokVideo, map[string]any{"operation": "video_extension"}))

	body, executor, err := buildApplicationVideoRequest(map[string]any{
		"operation": "image_video_generation", "resolution": "1080p", "duration": json.Number("8"),
	}, "image_to_video", "animate the product")
	require.NoError(t, err)
	require.Equal(t, ApplicationExecutorVideoGeneration, executor)
	require.Equal(t, xai.ModelGrokImagineVideo15, body["model"])

	body, executor, err = buildApplicationVideoRequest(map[string]any{
		"operation": "video_edit", "resolution": "720p",
	}, "video_edit", "change the lighting")
	require.NoError(t, err)
	require.Equal(t, ApplicationExecutorVideoEdit, executor)
	require.Equal(t, xai.ModelGrokImagineVideo, body["model"])

	_, _, err = buildApplicationVideoRequest(map[string]any{
		"model": xai.ModelGrokImagineVideo15, "resolution": "720p",
	}, "reference_to_video", "use these references")
	require.Error(t, err)

	body, executor, err = buildApplicationVideoRequest(map[string]any{
		"model": xai.ModelGrokImagineVideo15, "resolution": "1080p",
	}, "text_to_video", "ocean waves")
	require.NoError(t, err)
	require.Equal(t, ApplicationExecutorVideoGeneration, executor)
	require.Equal(t, xai.ModelGrokImagineVideo15, body["model"])
	require.Equal(t, "1080p", body["resolution"])
}

func TestApplicationMediaOutputPersistsOnlyBoundedReferences(t *testing.T) {
	output, err := normalizeApplicationMediaOutput(ApplicationExecutorImageGeneration, json.RawMessage(`{
		"count":2,
		"images":[
			{"url":"https://cdn.example/a.png","revised_prompt":"first"},
			{"url":"https://cdn.example/b.png","revised_prompt":"second","binary":"ignored"}
		],
		"base64":"ignored"
	}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"count":2,"images":[{"url":"https://cdn.example/a.png","revised_prompt":"first"},{"url":"https://cdn.example/b.png","revised_prompt":"second"}]}`, string(output))

	_, err = normalizeApplicationMediaOutput(ApplicationExecutorImageGeneration, json.RawMessage(`{
		"count":1,"images":[{"url":"data:image/png;base64,AAAA"}]
	}`))
	require.Error(t, err)

	video, err := normalizeApplicationMediaOutput(ApplicationExecutorVideoGeneration, json.RawMessage(`{
		"request_id":"req-1","status":"completed","video_url":"https://cdn.example/video.mp4","extra":"ignored"
	}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"request_id":"req-1","status":"completed","video_url":"https://cdn.example/video.mp4"}`, string(video))
}

func TestApplicationVideoCheckpointPersistsRequestIDWithoutCompletingRun(t *testing.T) {
	runID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	runRepo := &applicationCenterRunRepoStub{run: &ApplicationRun{
		ID: runID, UserID: 7, AppSlug: ApplicationSlugGrokVideo, AppRevision: 2,
		Status: ApplicationRunStatusRunning, CurrentStepKey: ApplicationStepMedia,
		PlannerAPIKeyID: applicationKeyID(11), MediaAPIKeyID: 12, SessionID: "app:" + runID,
		Input: json.RawMessage(`{"brief":"animate","mode":"text_to_video"}`),
		Steps: []*ApplicationRunStep{{
			StepKey: ApplicationStepMedia, Executor: ApplicationExecutorVideoGeneration,
			CredentialRole: ApplicationCredentialMedia, Status: ApplicationRunStatusPending,
			Request: json.RawMessage(`{"model":"grok-imagine-video-1.5","prompt":"animate"}`),
		}},
	}}
	svc := NewApplicationCenterService(runRepo, &applicationCenterAPIKeyRepoStub{})

	run, err := svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output:           json.RawMessage(`{"request_id":"video-1","status":"processing"}`),
		GatewayRequestID: "video-1",
	})
	require.NoError(t, err)
	require.Equal(t, ApplicationRunStatusRunning, run.Status)
	require.Equal(t, ApplicationRunStatusRunning, run.Steps[0].Status)
	require.Equal(t, "video-1", run.Steps[0].GatewayRequestID)
	require.NotNil(t, run.NextAction)
	require.Equal(t, "video-1", run.NextAction.GatewayRequestID)

	_, err = svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output: json.RawMessage(`{"request_id":"different"}`), GatewayRequestID: "video-1",
	})
	require.Error(t, err)

	_, err = svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output: json.RawMessage(`{"request_id":"video-2","status":"processing"}`), GatewayRequestID: "video-2",
	})
	require.ErrorIs(t, err, ErrApplicationInvalidTransition)

	_, err = svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output: json.RawMessage(`{"request_id":"video-1","status":"failed"}`), GatewayRequestID: "video-1",
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))
}

func TestApplicationImageCheckpointUsesAsyncTaskContract(t *testing.T) {
	runID := "6ba7b810-9dad-11d1-80b4-00c04fd430c9"
	runRepo := &applicationCenterRunRepoStub{run: &ApplicationRun{
		ID: runID, UserID: 7, AppSlug: ApplicationSlugCreativeImage, AppRevision: 3,
		Status: ApplicationRunStatusRunning, CurrentStepKey: ApplicationStepMedia,
		MediaAPIKeyID: 12, SessionID: "app:" + runID,
		Input: json.RawMessage(`{"brief":"draw","mode":"text_to_image"}`),
		Steps: []*ApplicationRunStep{{
			StepKey: ApplicationStepMedia, Executor: ApplicationExecutorImageGeneration,
			CredentialRole: ApplicationCredentialMedia, Status: ApplicationRunStatusPending,
			Request: json.RawMessage(`{"model":"grok-imagine-image","prompt":"draw"}`),
		}},
	}}
	svc := NewApplicationCenterService(runRepo, &applicationCenterAPIKeyRepoStub{})

	run, err := svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output:           json.RawMessage(`{"task_id":"imgtask-1","status":"processing","images":[{"url":"ignored"}]}`),
		GatewayRequestID: "imgtask-1",
	})
	require.NoError(t, err)
	require.Equal(t, ApplicationRunStatusRunning, run.Status)
	require.Equal(t, ApplicationRunStatusRunning, run.Steps[0].Status)
	require.Equal(t, "imgtask-1", run.Steps[0].GatewayRequestID)
	require.JSONEq(t, `{"task_id":"imgtask-1","status":"processing"}`, string(run.Steps[0].Output))
	require.Equal(t, "imgtask-1", run.NextAction.GatewayRequestID)

	run, err = svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output:           json.RawMessage(`{"request_id":"imgtask-1","status":"PROCESSING"}`),
		GatewayRequestID: "imgtask-1",
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"task_id":"imgtask-1","status":"processing"}`, string(run.Steps[0].Output))

	_, err = svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output: json.RawMessage(`{"task_id":"imgtask-1","status":"completed"}`), GatewayRequestID: "imgtask-1",
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))

	_, err = svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output: json.RawMessage(`{"task_id":"other-task","status":"processing"}`), GatewayRequestID: "imgtask-1",
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))

	_, err = svc.CheckpointStep(context.Background(), 7, runID, ApplicationStepMedia, CheckpointApplicationStepRequest{
		Output: json.RawMessage(`{"count":1,"images":[{"url":"https://cdn.example/image.png"}]}`), GatewayRequestID: "imgtask-1",
	})
	require.Error(t, err)
	require.Equal(t, "APPLICATION_INVALID_INPUT", infraerrors.Reason(err))
}

func TestRecoverableApplicationMediaCheckpointRejectsTerminalAndUnknownStatuses(t *testing.T) {
	for _, idField := range []string{"task_id", "request_id"} {
		output := []byte(`{"` + idField + `":"image-1","status":"processing"}`)
		require.True(t, IsRecoverableApplicationMediaCheckpoint(
			ApplicationExecutorImageGeneration,
			output,
			"image-1",
		), idField)
	}
	for _, status := range []string{"completed", "failed", "expired", "unknown", ""} {
		output := []byte(`{"task_id":"image-1","status":"` + status + `"}`)
		require.False(t, IsRecoverableApplicationMediaCheckpoint(
			ApplicationExecutorImageGeneration,
			output,
			"image-1",
		), status)
	}
	for _, status := range []string{"pending", "queued", "submitted", "processing", "in_progress"} {
		output := []byte(`{"request_id":"video-1","status":"` + status + `"}`)
		require.True(t, IsRecoverableApplicationMediaCheckpoint(
			ApplicationExecutorVideoGeneration,
			output,
			"video-1",
		), status)
	}
	for _, status := range []string{"failed", "expired", "cancelled", "done", "completed", "succeeded", "unknown", ""} {
		output := []byte(`{"request_id":"video-1","status":"` + status + `"}`)
		require.False(t, IsRecoverableApplicationMediaCheckpoint(
			ApplicationExecutorVideoGeneration,
			output,
			"video-1",
		), status)
	}
	require.False(t, IsRecoverableApplicationMediaCheckpoint(
		ApplicationExecutorVideoGeneration,
		[]byte(`{"request_id":"video-2","status":"processing"}`),
		"video-1",
	))
	require.False(t, IsRecoverableApplicationMediaCheckpoint(
		ApplicationExecutorImageGeneration,
		[]byte(`{"task_id":"image-1","status":"completed"}`),
		"image-1",
	))
}
