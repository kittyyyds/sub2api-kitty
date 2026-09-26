package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/google/uuid"
)

const (
	ApplicationSlugFreeMedia     = "free-media-studio"
	ApplicationSlugCreativeImage = "inspiration-image-refiner"
	ApplicationSlugEcommerce     = "ecommerce-visual-planner"
	ApplicationSlugGrokVideo     = "grok-video-director"

	ApplicationRunStatusPending   = "pending"
	ApplicationRunStatusRunning   = "running"
	ApplicationRunStatusCompleted = "completed"
	ApplicationRunStatusFailed    = "failed"
	ApplicationRunStatusCancelled = "cancelled"

	ApplicationStepPlanner = "planner"
	ApplicationStepMedia   = "media"

	ApplicationExecutorChatCompletions = "chat_completions"
	ApplicationExecutorImageGeneration = "image_generation"
	ApplicationExecutorImageEdit       = "image_edit"
	ApplicationExecutorVideoGeneration = "video_generation"
	ApplicationExecutorVideoEdit       = "video_edit"
	ApplicationExecutorVideoExtension  = "video_extension"

	ApplicationCredentialPlanner = "planner"
	ApplicationCredentialMedia   = "media"

	applicationInputMaxBytes         = 32 * 1024
	applicationPlannerOutputMaxBytes = 64 * 1024
	applicationMediaOutputMaxBytes   = 32 * 1024
	applicationStepRequestMaxBytes   = 64 * 1024
	applicationMaxErrorBytes         = 2000
	applicationMaxURLBytes           = 2048
	applicationMaxRequestIDBytes     = 256
)

var (
	ErrApplicationNotFound = infraerrors.NotFound(
		"APPLICATION_NOT_FOUND", "application not found",
	)
	ErrApplicationRunNotFound = infraerrors.NotFound(
		"APPLICATION_RUN_NOT_FOUND", "application run not found",
	)
	ErrApplicationStepNotFound = infraerrors.NotFound(
		"APPLICATION_STEP_NOT_FOUND", "application run step not found",
	)
	ErrApplicationInvalidInput = infraerrors.BadRequest(
		"APPLICATION_INVALID_INPUT", "application input is invalid",
	)
	ErrApplicationAPIKeyUnavailable = infraerrors.BadRequest(
		"APPLICATION_API_KEY_UNAVAILABLE", "selected API key is not active or does not belong to the current user",
	)
	ErrApplicationInvalidTransition = infraerrors.Conflict(
		"APPLICATION_INVALID_TRANSITION", "application run state has changed",
	)
	ErrApplicationStaleWrite = infraerrors.Conflict(
		"APPLICATION_STALE_WRITE", "application run was updated by another request",
	)
)

type ApplicationFormOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

type ApplicationFormField struct {
	Key         string                  `json:"key"`
	Label       string                  `json:"label"`
	Type        string                  `json:"type"`
	Required    bool                    `json:"required,omitempty"`
	Description string                  `json:"description,omitempty"`
	Placeholder string                  `json:"placeholder,omitempty"`
	Default     any                     `json:"default,omitempty"`
	Options     []ApplicationFormOption `json:"options,omitempty"`
	Min         *float64                `json:"min,omitempty"`
	Max         *float64                `json:"max,omitempty"`
	Step        *float64                `json:"step,omitempty"`
	Accept      string                  `json:"accept,omitempty"`
	Multiple    bool                    `json:"multiple,omitempty"`
	MaxItems    int                     `json:"max_items,omitempty"`
	VisibleWhen map[string]any          `json:"visible_when,omitempty"`
}

type ApplicationFormDefinition struct {
	Fields []ApplicationFormField `json:"fields"`
}

type ApplicationDefinition struct {
	Slug           string                    `json:"slug"`
	Revision       int                       `json:"revision"`
	Version        string                    `json:"version"`
	Name           string                    `json:"name"`
	Description    string                    `json:"description"`
	Category       string                    `json:"category"`
	Status         string                    `json:"status"`
	Tags           []string                  `json:"tags"`
	SupportedModes []string                  `json:"supported_modes"`
	Form           ApplicationFormDefinition `json:"form"`
	PlannerPrompt  string                    `json:"-"`
}

type ApplicationNextAction struct {
	StepKey           string         `json:"step_key"`
	Executor          string         `json:"executor"`
	CredentialRole    string         `json:"credential_role"`
	APIKeyID          int64          `json:"api_key_id"`
	Path              string         `json:"path"`
	Method            string         `json:"method"`
	ContentType       string         `json:"content_type"`
	Request           map[string]any `json:"request"`
	Body              map[string]any `json:"body"`
	RepairInstruction string         `json:"repair_instruction,omitempty"`
	SessionID         string         `json:"session_id"`
	GatewayRequestID  string         `json:"gateway_request_id,omitempty"`
}

type ApplicationRunStep struct {
	ID               int64           `json:"id"`
	RunID            string          `json:"-"`
	StepKey          string          `json:"step_key"`
	Sequence         int             `json:"sequence"`
	Executor         string          `json:"executor"`
	CredentialRole   string          `json:"credential_role"`
	Status           string          `json:"status"`
	Request          json.RawMessage `json:"-"`
	Output           json.RawMessage `json:"output,omitempty"`
	GatewayRequestID string          `json:"gateway_request_id,omitempty"`
	ErrorCode        string          `json:"error_code,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	Attempt          int             `json:"attempt"`
	Version          int64           `json:"-"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type ApplicationRun struct {
	ID              string                 `json:"id"`
	UserID          int64                  `json:"-"`
	AppSlug         string                 `json:"app_slug"`
	AppRevision     int                    `json:"app_revision"`
	Status          string                 `json:"status"`
	PlannerAPIKeyID *int64                 `json:"planner_api_key_id,omitempty"`
	MediaAPIKeyID   int64                  `json:"media_api_key_id"`
	SessionID       string                 `json:"session_id"`
	CurrentStepKey  string                 `json:"current_step_key,omitempty"`
	Input           json.RawMessage        `json:"input"`
	Output          json.RawMessage        `json:"output,omitempty"`
	ErrorCode       string                 `json:"error_code,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	RetryCount      int                    `json:"retry_count"`
	Version         int64                  `json:"-"`
	Steps           []*ApplicationRunStep  `json:"steps,omitempty"`
	NextAction      *ApplicationNextAction `json:"next_action"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	CancelledAt     *time.Time             `json:"cancelled_at,omitempty"`
}

type StartApplicationRunRequest struct {
	PlannerAPIKeyID *int64
	MediaAPIKeyID   int64
	Input           json.RawMessage
}

type CompleteApplicationStepRequest struct {
	Output           json.RawMessage
	GatewayRequestID string
}

type FailApplicationStepRequest struct {
	Error            string
	Output           json.RawMessage
	GatewayRequestID string
}

type CheckpointApplicationStepRequest struct {
	Output           json.RawMessage
	GatewayRequestID string
}

type CreateApplicationRunParams struct {
	Run         *ApplicationRun
	InitialStep *ApplicationRunStep
}

type CompleteApplicationStepParams struct {
	UserID           int64
	RunID            string
	StepKey          string
	StepOutput       json.RawMessage
	GatewayRequestID string
	NextStep         *ApplicationRunStep
	RunOutput        json.RawMessage
}

type FailApplicationStepParams struct {
	UserID           int64
	RunID            string
	StepKey          string
	StepOutput       json.RawMessage
	GatewayRequestID string
	ErrorCode        string
	ErrorMessage     string
}

type CheckpointApplicationStepParams struct {
	UserID           int64
	RunID            string
	StepKey          string
	StepOutput       json.RawMessage
	GatewayRequestID string
}

type ApplicationRunRepository interface {
	Create(ctx context.Context, params CreateApplicationRunParams) error
	GetByID(ctx context.Context, userID int64, runID string) (*ApplicationRun, error)
	List(ctx context.Context, userID int64, params pagination.PaginationParams, appSlug string) ([]*ApplicationRun, *pagination.PaginationResult, error)
	CompleteStep(ctx context.Context, params CompleteApplicationStepParams) error
	CheckpointStep(ctx context.Context, params CheckpointApplicationStepParams) error
	FailStep(ctx context.Context, params FailApplicationStepParams) error
	Cancel(ctx context.Context, userID int64, runID string) error
	Retry(ctx context.Context, userID int64, runID string) error
}

type ApplicationCenterService struct {
	runRepo    ApplicationRunRepository
	apiKeyRepo APIKeyRepository
}

func NewApplicationCenterService(runRepo ApplicationRunRepository, apiKeyRepo APIKeyRepository) *ApplicationCenterService {
	return &ApplicationCenterService{runRepo: runRepo, apiKeyRepo: apiKeyRepo}
}

func (s *ApplicationCenterService) ListApplications() []ApplicationDefinition {
	return applicationCatalog()
}

func (s *ApplicationCenterService) StartRun(
	ctx context.Context,
	userID int64,
	slug string,
	req StartApplicationRunRequest,
) (*ApplicationRun, error) {
	app, ok := findApplicationDefinition(slug)
	if !ok {
		return nil, ErrApplicationNotFound
	}
	if userID <= 0 || req.MediaAPIKeyID <= 0 || (req.PlannerAPIKeyID != nil && *req.PlannerAPIKeyID <= 0) {
		return nil, ErrApplicationInvalidInput
	}
	var plannerKey *APIKey
	var err error
	if req.PlannerAPIKeyID != nil {
		plannerKey, err = s.validateRunAPIKey(ctx, userID, *req.PlannerAPIKeyID)
		if err != nil {
			return nil, err
		}
	}
	input, inputMap, err := normalizeApplicationInput(req.Input)
	if err != nil {
		return nil, err
	}
	if err := validateApplicationInput(app, inputMap); err != nil {
		return nil, err
	}
	mediaKey, err := s.validateRunMediaAPIKey(ctx, userID, req.MediaAPIKeyID, app, inputMap)
	if err != nil {
		return nil, err
	}
	runID := uuid.NewString()
	now := time.Now()
	currentStepKey := ApplicationStepPlanner
	if plannerKey == nil {
		currentStepKey = ApplicationStepMedia
	}
	run := &ApplicationRun{
		ID:              runID,
		UserID:          userID,
		AppSlug:         app.Slug,
		AppRevision:     app.Revision,
		Status:          ApplicationRunStatusRunning,
		PlannerAPIKeyID: req.PlannerAPIKeyID,
		MediaAPIKeyID:   req.MediaAPIKeyID,
		SessionID:       "app:" + runID,
		CurrentStepKey:  currentStepKey,
		Input:           input,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	var step *ApplicationRunStep
	if plannerKey != nil {
		plannerRequest, err := buildApplicationPlannerRequest(app, input, inputMap, plannerKey)
		if err != nil {
			return nil, err
		}
		plannerRequestJSON, err := marshalApplicationJSON(plannerRequest, applicationStepRequestMaxBytes)
		if err != nil {
			return nil, ErrApplicationInvalidInput.WithCause(err)
		}
		step = &ApplicationRunStep{
			RunID:          runID,
			StepKey:        ApplicationStepPlanner,
			Sequence:       1,
			Executor:       ApplicationExecutorChatCompletions,
			CredentialRole: ApplicationCredentialPlanner,
			Status:         ApplicationRunStatusPending,
			Request:        plannerRequestJSON,
			Attempt:        1,
			Version:        1,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	} else {
		directPlan, err := buildDirectApplicationPlan(app, inputMap)
		if err != nil {
			return nil, err
		}
		step, err = buildApplicationMediaStep(run, app, inputMap, directPlan, mediaKey.Group.Platform, 1)
		if err != nil {
			return nil, err
		}
	}
	if err := s.runRepo.Create(ctx, CreateApplicationRunParams{Run: run, InitialStep: step}); err != nil {
		return nil, fmt.Errorf("create application run: %w", err)
	}
	return s.GetRun(ctx, userID, runID)
}

func (s *ApplicationCenterService) ListRuns(
	ctx context.Context,
	userID int64,
	params pagination.PaginationParams,
	appSlug string,
) ([]*ApplicationRun, *pagination.PaginationResult, error) {
	if appSlug != "" {
		app, ok := findApplicationDefinition(appSlug)
		if !ok {
			return nil, nil, ErrApplicationNotFound
		}
		appSlug = app.Slug
	}
	runs, result, err := s.runRepo.List(ctx, userID, params, appSlug)
	if err != nil {
		return nil, nil, fmt.Errorf("list application runs: %w", err)
	}
	for _, run := range runs {
		if err := attachApplicationNextAction(run); err != nil {
			return nil, nil, err
		}
	}
	return runs, result, nil
}

func (s *ApplicationCenterService) GetRun(ctx context.Context, userID int64, runID string) (*ApplicationRun, error) {
	if _, err := uuid.Parse(strings.TrimSpace(runID)); err != nil {
		return nil, ErrApplicationRunNotFound
	}
	run, err := s.runRepo.GetByID(ctx, userID, runID)
	if err != nil {
		return nil, fmt.Errorf("get application run: %w", err)
	}
	if err := attachApplicationNextAction(run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *ApplicationCenterService) CompleteStep(
	ctx context.Context,
	userID int64,
	runID string,
	stepKey string,
	req CompleteApplicationStepRequest,
) (*ApplicationRun, error) {
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	step := currentApplicationStep(run, stepKey)
	if step == nil {
		return nil, ErrApplicationStepNotFound
	}
	if run.Status != ApplicationRunStatusRunning || run.CurrentStepKey != stepKey ||
		(step.Status != ApplicationRunStatusPending && step.Status != ApplicationRunStatusRunning) {
		return nil, ErrApplicationInvalidTransition
	}

	completion := CompleteApplicationStepParams{
		UserID:           userID,
		RunID:            runID,
		StepKey:          stepKey,
		GatewayRequestID: boundedApplicationText(req.GatewayRequestID, applicationMaxRequestIDBytes),
	}
	if step.Executor == ApplicationExecutorChatCompletions {
		plannerOutput, plannerPlan, err := parseApplicationPlannerOutput(req.Output)
		if err != nil {
			return nil, err
		}
		app, ok := findApplicationDefinitionForRun(run.AppSlug, run.AppRevision)
		if !ok {
			return nil, ErrApplicationNotFound
		}
		inputMap, err := applicationJSONObject(run.Input)
		if err != nil {
			return nil, ErrApplicationInvalidInput.WithCause(err)
		}
		mediaKey, err := s.validateRunMediaAPIKey(ctx, userID, run.MediaAPIKeyID, app, inputMap)
		if err != nil {
			return nil, err
		}
		nextStep, err := buildApplicationMediaStep(run, app, inputMap, plannerPlan, mediaKey.Group.Platform, 2)
		if err != nil {
			return nil, err
		}
		completion.StepOutput = plannerOutput
		completion.NextStep = nextStep
	} else {
		mediaOutput, err := normalizeApplicationMediaOutput(step.Executor, req.Output)
		if err != nil {
			return nil, err
		}
		completion.StepOutput = mediaOutput
		completion.RunOutput = mediaOutput
	}

	if err := s.runRepo.CompleteStep(ctx, completion); err != nil {
		return nil, fmt.Errorf("complete application step: %w", err)
	}
	return s.GetRun(ctx, userID, runID)
}

func (s *ApplicationCenterService) FailStep(
	ctx context.Context,
	userID int64,
	runID string,
	stepKey string,
	req FailApplicationStepRequest,
) (*ApplicationRun, error) {
	if _, err := uuid.Parse(strings.TrimSpace(runID)); err != nil {
		return nil, ErrApplicationRunNotFound
	}
	message := boundedApplicationText(strings.TrimSpace(req.Error), applicationMaxErrorBytes)
	if message == "" {
		return nil, ErrApplicationInvalidInput
	}
	var output json.RawMessage
	var err error
	if len(bytes.TrimSpace(req.Output)) > 0 && !bytes.Equal(bytes.TrimSpace(req.Output), []byte("null")) {
		output, err = normalizeApplicationJSON(req.Output, applicationMediaOutputMaxBytes, false)
		if err != nil {
			return nil, ErrApplicationInvalidInput.WithCause(err)
		}
	}
	if err := s.runRepo.FailStep(ctx, FailApplicationStepParams{
		UserID:           userID,
		RunID:            runID,
		StepKey:          strings.TrimSpace(stepKey),
		StepOutput:       output,
		GatewayRequestID: boundedApplicationText(req.GatewayRequestID, applicationMaxRequestIDBytes),
		ErrorCode:        "STEP_EXECUTION_FAILED",
		ErrorMessage:     message,
	}); err != nil {
		return nil, fmt.Errorf("fail application step: %w", err)
	}
	return s.GetRun(ctx, userID, runID)
}

func (s *ApplicationCenterService) CheckpointStep(
	ctx context.Context,
	userID int64,
	runID string,
	stepKey string,
	req CheckpointApplicationStepRequest,
) (*ApplicationRun, error) {
	if _, err := uuid.Parse(strings.TrimSpace(runID)); err != nil {
		return nil, ErrApplicationRunNotFound
	}
	requestID := strings.TrimSpace(req.GatewayRequestID)
	if requestID == "" || len(requestID) > applicationMaxRequestIDBytes {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "gateway_request_id"})
	}
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	step := currentApplicationStep(run, strings.TrimSpace(stepKey))
	if step == nil {
		return nil, ErrApplicationStepNotFound
	}
	if run.Status != ApplicationRunStatusRunning || run.CurrentStepKey != step.StepKey ||
		(step.Status != ApplicationRunStatusPending && step.Status != ApplicationRunStatusRunning) ||
		!isApplicationMediaExecutor(step.Executor) {
		return nil, ErrApplicationInvalidTransition
	}
	if step.GatewayRequestID != "" && step.GatewayRequestID != requestID {
		return nil, ErrApplicationInvalidTransition
	}

	var output json.RawMessage
	if len(bytes.TrimSpace(req.Output)) > 0 && !bytes.Equal(bytes.TrimSpace(req.Output), []byte("null")) {
		output, err = normalizeApplicationMediaCheckpoint(step.Executor, req.Output)
		if err != nil {
			return nil, err
		}
	} else {
		checkpoint := map[string]any{"status": "processing"}
		if isApplicationImageExecutor(step.Executor) {
			checkpoint["task_id"] = requestID
		} else {
			checkpoint["request_id"] = requestID
		}
		output, err = marshalApplicationJSON(checkpoint, applicationMediaOutputMaxBytes)
		if err != nil {
			return nil, ErrApplicationInvalidInput.WithCause(err)
		}
	}
	value, decodeErr := applicationJSONObject(output)
	if decodeErr != nil {
		return nil, ErrApplicationInvalidInput.WithCause(decodeErr)
	}
	if outputRequestID := applicationMediaCheckpointID(step.Executor, value); outputRequestID != requestID {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{
			"field": applicationMediaCheckpointIDField(step.Executor),
		})
	}
	if !IsRecoverableApplicationMediaCheckpoint(step.Executor, output, requestID) {
		return nil, ErrApplicationInvalidInput.WithMetadata(map[string]string{"field": "status"})
	}
	if err := s.runRepo.CheckpointStep(ctx, CheckpointApplicationStepParams{
		UserID:           userID,
		RunID:            runID,
		StepKey:          step.StepKey,
		StepOutput:       output,
		GatewayRequestID: requestID,
	}); err != nil {
		return nil, fmt.Errorf("checkpoint application step: %w", err)
	}
	return s.GetRun(ctx, userID, runID)
}

func (s *ApplicationCenterService) CancelRun(ctx context.Context, userID int64, runID string) (*ApplicationRun, error) {
	if _, err := uuid.Parse(strings.TrimSpace(runID)); err != nil {
		return nil, ErrApplicationRunNotFound
	}
	if err := s.runRepo.Cancel(ctx, userID, runID); err != nil {
		return nil, fmt.Errorf("cancel application run: %w", err)
	}
	return s.GetRun(ctx, userID, runID)
}

func (s *ApplicationCenterService) RetryRun(ctx context.Context, userID int64, runID string) (*ApplicationRun, error) {
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	if run.Status != ApplicationRunStatusFailed {
		return nil, ErrApplicationInvalidTransition
	}
	step := currentApplicationStep(run, run.CurrentStepKey)
	if step == nil || step.Status != ApplicationRunStatusFailed {
		return nil, ErrApplicationInvalidTransition
	}
	keyID := run.MediaAPIKeyID
	if step.CredentialRole == ApplicationCredentialPlanner {
		if run.PlannerAPIKeyID == nil {
			return nil, ErrApplicationInvalidTransition
		}
		keyID = *run.PlannerAPIKeyID
	}
	if step.CredentialRole == ApplicationCredentialMedia {
		app, ok := findApplicationDefinitionForRun(run.AppSlug, run.AppRevision)
		if !ok {
			return nil, ErrApplicationNotFound
		}
		input, inputErr := applicationJSONObject(run.Input)
		if inputErr != nil {
			return nil, ErrApplicationInvalidInput.WithCause(inputErr)
		}
		if _, err := s.validateRunMediaAPIKey(ctx, userID, keyID, app, input); err != nil {
			return nil, err
		}
	} else if _, err := s.validateRunAPIKey(ctx, userID, keyID); err != nil {
		return nil, err
	}
	if err := s.runRepo.Retry(ctx, userID, runID); err != nil {
		return nil, fmt.Errorf("retry application run: %w", err)
	}
	return s.GetRun(ctx, userID, runID)
}

func (s *ApplicationCenterService) validateRunAPIKey(ctx context.Context, userID, apiKeyID int64) (*APIKey, error) {
	key, err := s.apiKeyRepo.GetByID(ctx, apiKeyID)
	if err != nil || key == nil || key.UserID != userID || key.Status != StatusAPIKeyActive || key.IsExpired() {
		return nil, ErrApplicationAPIKeyUnavailable
	}
	return key, nil
}

func (s *ApplicationCenterService) validateRunMediaAPIKey(
	ctx context.Context,
	userID, apiKeyID int64,
	app ApplicationDefinition,
	input map[string]any,
) (*APIKey, error) {
	key, err := s.validateRunAPIKey(ctx, userID, apiKeyID)
	if err != nil {
		return nil, err
	}
	if key.Group == nil || key.Group.Status != StatusActive || !key.Group.AllowImageGeneration {
		return nil, ErrApplicationAPIKeyUnavailable
	}
	platform := strings.ToLower(strings.TrimSpace(key.Group.Platform))
	if applicationModeRequiresGrokMedia(app, resolveApplicationMode(app.Slug, input)) {
		if platform != PlatformGrok && platform != PlatformVideo {
			return nil, ErrApplicationAPIKeyUnavailable
		}
	} else if platform != PlatformGrok && platform != PlatformOpenAI {
		return nil, ErrApplicationAPIKeyUnavailable
	}
	return key, nil
}

func currentApplicationStep(run *ApplicationRun, stepKey string) *ApplicationRunStep {
	if run == nil {
		return nil
	}
	for i := len(run.Steps) - 1; i >= 0; i-- {
		if run.Steps[i] != nil && run.Steps[i].StepKey == stepKey {
			return run.Steps[i]
		}
	}
	return nil
}

func attachApplicationNextAction(run *ApplicationRun) error {
	if run == nil || run.Status != ApplicationRunStatusRunning || run.CurrentStepKey == "" {
		if run != nil {
			run.NextAction = nil
		}
		return nil
	}
	step := currentApplicationStep(run, run.CurrentStepKey)
	if step == nil || (step.Status != ApplicationRunStatusPending && step.Status != ApplicationRunStatusRunning) {
		return ErrApplicationInvalidTransition
	}
	request, err := applicationJSONObject(step.Request)
	if err != nil {
		return fmt.Errorf("decode application step request: %w", err)
	}
	path := applicationExecutorPath(step.Executor)
	if path == "" {
		return ErrApplicationInvalidTransition
	}
	apiKeyID := run.MediaAPIKeyID
	if step.CredentialRole == ApplicationCredentialPlanner {
		if run.PlannerAPIKeyID == nil {
			return ErrApplicationInvalidTransition
		}
		apiKeyID = *run.PlannerAPIKeyID
	}
	action := &ApplicationNextAction{
		StepKey:          step.StepKey,
		Executor:         step.Executor,
		CredentialRole:   step.CredentialRole,
		APIKeyID:         apiKeyID,
		Path:             path,
		Method:           "POST",
		ContentType:      "application/json",
		Request:          cloneApplicationMap(request),
		Body:             cloneApplicationMap(request),
		SessionID:        run.SessionID,
		GatewayRequestID: step.GatewayRequestID,
	}
	if step.Executor == ApplicationExecutorChatCompletions {
		action.RepairInstruction = "只返回一个符合既定 JSON Schema 的 JSON 对象，不要使用 Markdown 代码块；必须包含非空 final_prompt。"
	}
	run.NextAction = action
	return nil
}

func isApplicationVideoExecutor(executor string) bool {
	switch executor {
	case ApplicationExecutorVideoGeneration, ApplicationExecutorVideoEdit, ApplicationExecutorVideoExtension:
		return true
	default:
		return false
	}
}

func isApplicationImageExecutor(executor string) bool {
	return executor == ApplicationExecutorImageGeneration || executor == ApplicationExecutorImageEdit
}

func isApplicationMediaExecutor(executor string) bool {
	return isApplicationImageExecutor(executor) || isApplicationVideoExecutor(executor)
}

// IsRecoverableApplicationMediaCheckpoint is the shared backend contract for
// deciding whether a persisted image or video task may be polled again.
// Unknown and terminal statuses deliberately fail closed so retry cannot reuse
// a finished upstream task identifier.
func IsRecoverableApplicationMediaCheckpoint(executor string, output []byte, gatewayRequestID string) bool {
	requestID := strings.TrimSpace(gatewayRequestID)
	if !isApplicationMediaExecutor(executor) || requestID == "" {
		return false
	}
	value, err := applicationJSONObject(json.RawMessage(output))
	if err != nil || applicationMediaCheckpointID(executor, value) != requestID {
		return false
	}
	if isApplicationImageExecutor(executor) {
		return strings.EqualFold(firstApplicationInputText(value, "status"), ImageTaskStatusProcessing)
	}
	return isRecoverableApplicationVideoStatus(firstApplicationInputText(value, "status"))
}

func applicationMediaCheckpointID(executor string, value map[string]any) string {
	if isApplicationImageExecutor(executor) {
		return firstApplicationInputText(value, "task_id", "request_id")
	}
	if isApplicationVideoExecutor(executor) {
		return firstApplicationInputText(value, "request_id", "id")
	}
	return ""
}

func applicationMediaCheckpointIDField(executor string) string {
	if isApplicationImageExecutor(executor) {
		return "task_id"
	}
	return "request_id"
}

func isRecoverableApplicationVideoStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued", "submitted", "processing", "in_progress":
		return true
	default:
		return false
	}
}

func applicationExecutorPath(executor string) string {
	switch executor {
	case ApplicationExecutorChatCompletions:
		return "/v1/chat/completions"
	case ApplicationExecutorImageGeneration:
		return "/v1/images/generations"
	case ApplicationExecutorImageEdit:
		return "/v1/images/edits"
	case ApplicationExecutorVideoGeneration:
		return "/v1/videos/generations"
	case ApplicationExecutorVideoEdit:
		return "/v1/videos/edits"
	case ApplicationExecutorVideoExtension:
		return "/v1/videos/extensions"
	default:
		return ""
	}
}

func normalizeApplicationInput(raw json.RawMessage) (json.RawMessage, map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = json.RawMessage(`{}`)
	}
	normalized, err := normalizeApplicationJSON(raw, applicationInputMaxBytes, true)
	if err != nil {
		return nil, nil, ErrApplicationInvalidInput.WithCause(err)
	}
	input, err := applicationJSONObject(normalized)
	if err != nil {
		return nil, nil, ErrApplicationInvalidInput.WithCause(err)
	}
	return normalized, input, nil
}

func normalizeApplicationJSON(raw json.RawMessage, maxBytes int, requireObject bool) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values are not allowed")
		}
		return nil, err
	}
	if requireObject {
		if _, ok := value.(map[string]any); !ok {
			return nil, fmt.Errorf("JSON object is required")
		}
	}
	bounded, err := boundApplicationJSONValue(value, 0)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(bounded)
	if err != nil {
		return nil, err
	}
	if len(out) > maxBytes {
		return nil, fmt.Errorf("JSON payload exceeds %d bytes", maxBytes)
	}
	return out, nil
}

func boundApplicationJSONValue(value any, depth int) (any, error) {
	if depth > 10 {
		return nil, fmt.Errorf("JSON nesting is too deep")
	}
	switch typed := value.(type) {
	case nil, bool, json.Number, float64:
		return typed, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
			return nil, fmt.Errorf("inline media data is not allowed")
		}
		return boundedApplicationText(typed, 8000), nil
	case []any:
		if len(typed) > 50 {
			typed = typed[:50]
		}
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			bounded, err := boundApplicationJSONValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, bounded)
		}
		return out, nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 80 {
			keys = keys[:80]
		}
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			if len(key) == 0 || len(key) > 128 {
				return nil, fmt.Errorf("invalid JSON object key")
			}
			bounded, err := boundApplicationJSONValue(typed[key], depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = bounded
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported JSON value %T", value)
	}
}

func applicationJSONObject(raw json.RawMessage) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func marshalApplicationJSON(value any, maxBytes int) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return normalizeApplicationJSON(raw, maxBytes, false)
}

func cloneApplicationMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func boundedApplicationText(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8ApplicationSuffixValid(value) {
		value = value[:len(value)-1]
	}
	return value
}

func utf8ApplicationSuffixValid(value string) bool {
	return strings.ToValidUTF8(value, "") == value
}

func validateApplicationURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > applicationMaxURLBytes {
		return "", fmt.Errorf("media URL is too long")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Host == "" ||
		(!strings.EqualFold(parsed.Scheme, "https") && !strings.EqualFold(parsed.Scheme, "http")) {
		return "", fmt.Errorf("media URL must be an absolute HTTP(S) URL")
	}
	return parsed.String(), nil
}
