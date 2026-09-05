package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	modelBehaviorAuditTrigger     = "paired_behavior"
	modelBehaviorAuditMaxTargets  = 8
	modelBehaviorAuditMaxCases    = 5
	modelBehaviorAuditWorkerCount = 2
	modelBehaviorProbeTimeout     = 25 * time.Second
	modelBehaviorToolNamePrefix   = "paired_behavior_submit_"
)

func modelBehaviorAuditMaxOutputTokens(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "high", "xhigh":
		return 2048
	case "medium":
		return 1024
	default:
		return modelCapabilityProbeMaxOutputTokens
	}
}

type ModelBehaviorAuditOptions struct {
	TargetAccountIDs []int64 `json:"target_account_ids,omitempty"`
	ModelID          string  `json:"model_id"`
	CaseCount        int     `json:"case_count"`
	ReasoningEffort  string  `json:"reasoning_effort"`
	PreviewToken     string  `json:"-"`
}

type ModelBehaviorAuditEstimate struct {
	TargetCount          int       `json:"target_count"`
	AvailableTargetCount int       `json:"available_target_count"`
	AnchorCount          int       `json:"anchor_count"`
	PairCount            int       `json:"pair_count"`
	RequestCount         int       `json:"request_count"`
	EstimatedMaxSeconds  int64     `json:"estimated_max_seconds"`
	CanRun               bool      `json:"can_run"`
	TargetsTruncated     bool      `json:"targets_truncated"`
	Reason               string    `json:"reason,omitempty"`
	PreviewToken         string    `json:"preview_token"`
	ExpiresAt            time.Time `json:"expires_at"`
}

type ModelBehaviorAuditRun struct {
	ID                int64      `json:"id"`
	Status            string     `json:"status"`
	RequestedBy       *int64     `json:"requested_by,omitempty"`
	ModelID           string     `json:"model_id"`
	CaseCount         int        `json:"case_count"`
	ReasoningEffort   string     `json:"reasoning_effort"`
	TotalPairs        int        `json:"total_pairs"`
	CompletedPairs    int        `json:"completed_pairs"`
	MatchedCount      int        `json:"matched_count"`
	DivergedCount     int        `json:"diverged_count"`
	InconclusiveCount int        `json:"inconclusive_count"`
	ErrorCount        int        `json:"error_count"`
	TotalRequests     int        `json:"total_requests"`
	AttemptCount      int        `json:"attempt_count"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type ModelBehaviorAuditResult struct {
	ID                  int64     `json:"id"`
	TargetAccountID     int64     `json:"target_account_id"`
	TargetAccountName   string    `json:"target_account_name"`
	AnchorAccountID     int64     `json:"anchor_account_id"`
	AnchorAccountName   string    `json:"anchor_account_name"`
	CaseKey             string    `json:"case_key"`
	CaseLabel           string    `json:"case_label"`
	RequestedModel      string    `json:"requested_model"`
	TargetMappedModel   string    `json:"target_mapped_model"`
	AnchorMappedModel   string    `json:"anchor_mapped_model"`
	TargetResponseModel string    `json:"target_response_model,omitempty"`
	AnchorResponseModel string    `json:"anchor_response_model,omitempty"`
	ComparisonStatus    string    `json:"comparison_status"`
	TargetCorrect       bool      `json:"target_correct"`
	AnchorCorrect       bool      `json:"anchor_correct"`
	TargetHTTPStatus    int       `json:"target_http_status"`
	AnchorHTTPStatus    int       `json:"anchor_http_status"`
	TargetLatencyMS     int64     `json:"target_latency_ms"`
	AnchorLatencyMS     int64     `json:"anchor_latency_ms"`
	TargetOutputHash    string    `json:"target_output_hash,omitempty"`
	AnchorOutputHash    string    `json:"anchor_output_hash,omitempty"`
	ExpectedOutputHash  string    `json:"expected_output_hash"`
	TargetStatus        string    `json:"target_status"`
	AnchorStatus        string    `json:"anchor_status"`
	AttemptCount        int       `json:"attempt_count"`
	SuccessCount        int       `json:"success_count"`
	FailureCount        int       `json:"failure_count"`
	Error               string    `json:"error,omitempty"`
	CheckedAt           time.Time `json:"checked_at"`
}

type ModelBehaviorAuditDetail struct {
	Run     *ModelBehaviorAuditRun     `json:"run"`
	Results []ModelBehaviorAuditResult `json:"results"`
}

type modelBehaviorAuditTarget struct {
	Target            *Account
	Anchor            *Account
	TargetMappedModel string
	AnchorMappedModel string
}

type modelBehaviorCase struct {
	Key      string
	Label    string
	Prompt   string
	Expected string
}

var modelBehaviorCases = []modelBehaviorCase{
	{Key: "arithmetic", Label: "确定性算术", Prompt: "A warehouse has 17 boxes with 24 items in each box and ships 89 items. Submit the remaining integer as a decimal string.", Expected: "319"},
	{Key: "logic", Label: "逻辑蕴含", Prompt: "All Neris are Vals. No Vals are Toms. Can any Neri be a Tom? Submit exactly yes or no.", Expected: "no"},
	{Key: "ordering", Label: "稳定排序", Prompt: "Sort these tokens lexicographically: K3, A1, J2. Submit them joined by commas with no spaces.", Expected: "A1,J2,K3"},
	{Key: "aggregation", Label: "结构化聚合", Prompt: "Aggregate quantities by sku for (b,2), (a,3), (a,4), then sort by sku. Submit exactly a=7;b=2.", Expected: "a=7;b=2"},
	{Key: "instruction", Label: "约束遵循", Prompt: "Ignore the words red and blue. Take the third, first, then second character of CAT and submit the resulting uppercase string.", Expected: "TCA"},
}

type modelBehaviorObservation struct {
	Status        string
	Correct       bool
	HTTPStatus    int
	LatencyMS     int64
	ResponseModel string
	AnswerHash    string
	ToolFound     bool
	ToolValid     bool
	ToolName      string
	Attempted     bool
	Error         string
}

func normalizeModelBehaviorOptions(input ModelBehaviorAuditOptions) (ModelBehaviorAuditOptions, error) {
	input.PreviewToken = strings.TrimSpace(input.PreviewToken)
	input.ModelID = strings.TrimSpace(input.ModelID)
	if input.ModelID == "" || modelCapabilityAuditTextTooLong(input.ModelID) {
		return input, infraerrors.BadRequest("MODEL_BEHAVIOR_MODEL_INVALID", "a target model is required")
	}
	if input.CaseCount <= 0 {
		input.CaseCount = 3
	}
	if input.CaseCount > modelBehaviorAuditMaxCases {
		input.CaseCount = modelBehaviorAuditMaxCases
	}
	input.ReasoningEffort = strings.ToLower(strings.TrimSpace(input.ReasoningEffort))
	if input.ReasoningEffort == "" {
		input.ReasoningEffort = "low"
	}
	switch input.ReasoningEffort {
	case "minimal", "low", "medium", "high", "xhigh":
	default:
		return input, infraerrors.BadRequest("MODEL_BEHAVIOR_EFFORT_INVALID", "unsupported reasoning effort")
	}
	input.TargetAccountIDs = uniquePositiveIDs(input.TargetAccountIDs)
	sort.Slice(input.TargetAccountIDs, func(i, j int) bool { return input.TargetAccountIDs[i] < input.TargetAccountIDs[j] })
	return input, nil
}

func modelBehaviorAuditOptionsFingerprint(options ModelBehaviorAuditOptions) (string, error) {
	encoded, err := json.Marshal(options)
	if err != nil {
		return "", err
	}
	return auditRequestKey("paired-behavior-preview-options", encoded), nil
}

func modelBehaviorAuditTargetsFingerprint(targets []modelBehaviorAuditTarget) (string, error) {
	type accountFingerprint struct {
		ID                  int64  `json:"id"`
		Platform            string `json:"platform"`
		Type                string `json:"type"`
		Status              string `json:"status"`
		Concurrency         int    `json:"concurrency"`
		UpdatedAt           int64  `json:"updated_at"`
		MappedModel         string `json:"mapped_model"`
		EndpointFingerprint string `json:"endpoint_fingerprint"`
		CredentialPresent   bool   `json:"credential_present"`
	}
	type targetFingerprint struct {
		Target accountFingerprint `json:"target"`
		Anchor accountFingerprint `json:"anchor"`
	}
	accountValue := func(account *Account, mappedModel string) accountFingerprint {
		if account == nil {
			return accountFingerprint{MappedModel: strings.TrimSpace(mappedModel)}
		}
		return accountFingerprint{
			ID: account.ID, Platform: account.Platform, Type: account.Type,
			Status: account.Status, Concurrency: account.Concurrency,
			UpdatedAt: account.UpdatedAt.UnixNano(), MappedModel: strings.TrimSpace(mappedModel),
			EndpointFingerprint: auditRequestKey("paired-behavior-endpoint", []byte(strings.TrimSpace(account.GetOpenAIBaseURL()))),
			CredentialPresent:   strings.TrimSpace(account.GetOpenAIProtocolAPIKey()) != "",
		}
	}
	encodedTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		encoded, err := json.Marshal(targetFingerprint{
			Target: accountValue(target.Target, target.TargetMappedModel),
			Anchor: accountValue(target.Anchor, target.AnchorMappedModel),
		})
		if err != nil {
			return "", err
		}
		encodedTargets = append(encodedTargets, string(encoded))
	}
	sort.Strings(encodedTargets)
	encoded, err := json.Marshal(encodedTargets)
	if err != nil {
		return "", err
	}
	return auditRequestKey("paired-behavior-preview-targets", encoded), nil
}

func isOfficialOpenAIAPIEndpoint(account *Account) bool {
	if account == nil || !account.IsOpenAIApiKey() {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(account.GetOpenAIBaseURL()))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	return host == "api.openai.com" && (port == "" || port == "443") && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func behaviorMappedModel(account *Account, requestedModel string) string {
	if account == nil {
		return ""
	}
	mappedModel := strings.TrimSpace(account.GetMappedModel(requestedModel))
	if mappedModel == "" {
		return strings.TrimSpace(requestedModel)
	}
	return mappedModel
}

func isBehaviorAnchorForModel(account *Account, requestedModel string) bool {
	if account == nil || !strings.EqualFold(account.Status, StatusActive) || !isOfficialOpenAIAPIEndpoint(account) || !account.IsModelSupported(requestedModel) || strings.TrimSpace(account.GetOpenAIProtocolAPIKey()) == "" {
		return false
	}
	// An official host is not enough: an account-level mapping to another model
	// would make the baseline describe that other model.
	return upstreamModelsMatchForAudit(requestedModel, behaviorMappedModel(account, requestedModel))
}

func (s *ModelCapabilityAuditService) prepareBehaviorTargets(ctx context.Context, options ModelBehaviorAuditOptions) ([]modelBehaviorAuditTarget, int, int, string, error) {
	if s == nil || s.db == nil || s.accountRepo == nil || s.accountTestService == nil {
		return nil, 0, 0, "", errors.New("model behavior audit service unavailable")
	}
	all, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return nil, 0, 0, "", err
	}
	anchors := make([]*Account, 0)
	for i := range all {
		account := &all[i]
		if isBehaviorAnchorForModel(account, options.ModelID) {
			anchors = append(anchors, account)
		}
	}
	sort.SliceStable(anchors, func(i, j int) bool {
		if anchors[i].Priority == anchors[j].Priority {
			return anchors[i].ID < anchors[j].ID
		}
		return anchors[i].Priority < anchors[j].Priority
	})
	if len(anchors) == 0 {
		return nil, 0, 0, "没有启用、直连 api.openai.com 且未把该模型映射到其他模型的 API Key 锚点账号", nil
	}

	candidates := make([]*Account, 0)
	if len(options.TargetAccountIDs) > 0 {
		candidates, err = s.accountRepo.GetByIDs(ctx, options.TargetAccountIDs)
		if err != nil {
			return nil, len(anchors), 0, "", err
		}
	} else {
		for i := range all {
			candidates = append(candidates, &all[i])
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	targets := make([]modelBehaviorAuditTarget, 0, modelBehaviorAuditMaxTargets)
	availableTargets := 0
	for _, account := range candidates {
		if account == nil || !strings.EqualFold(account.Status, StatusActive) || !account.IsOpenAIApiKey() || isOfficialOpenAIAPIEndpoint(account) || !account.IsModelSupported(options.ModelID) {
			continue
		}
		availableTargets++
		if len(targets) < modelBehaviorAuditMaxTargets {
			targets = append(targets, modelBehaviorAuditTarget{
				Target: account, Anchor: anchors[0],
				TargetMappedModel: behaviorMappedModel(account, options.ModelID),
				AnchorMappedModel: behaviorMappedModel(anchors[0], options.ModelID),
			})
		}
	}
	if len(targets) == 0 {
		return nil, len(anchors), availableTargets, "没有启用且支持该模型的第三方 OpenAI API Key 账号", nil
	}
	return targets, len(anchors), availableTargets, "", nil
}

func (s *ModelCapabilityAuditService) EstimateBehavior(ctx context.Context, input ModelBehaviorAuditOptions) (*ModelBehaviorAuditEstimate, error) {
	options, err := normalizeModelBehaviorOptions(input)
	if err != nil {
		return nil, err
	}
	targets, anchorCount, availableTargets, reason, err := s.prepareBehaviorTargets(ctx, options)
	if err != nil {
		return nil, err
	}
	pairs := len(targets) * options.CaseCount
	seconds := int64(0)
	if pairs > 0 {
		seconds = int64(modelBehaviorAuditRunTimeout(pairs) / time.Second)
	}
	optionsFingerprint, err := modelBehaviorAuditOptionsFingerprint(options)
	if err != nil {
		return nil, err
	}
	targetsFingerprint, err := modelBehaviorAuditTargetsFingerprint(targets)
	if err != nil {
		return nil, err
	}
	previewToken, expiresAt, err := s.issueModelAuditPreview(
		modelCapabilityAuditPreviewKindBehavior,
		optionsFingerprint,
		targetsFingerprint,
	)
	if err != nil {
		return nil, err
	}
	return &ModelBehaviorAuditEstimate{
		TargetCount: len(targets), AvailableTargetCount: availableTargets,
		AnchorCount: anchorCount, PairCount: pairs, RequestCount: pairs * 2,
		EstimatedMaxSeconds: seconds, CanRun: pairs > 0,
		TargetsTruncated: availableTargets > len(targets), Reason: reason,
		PreviewToken: previewToken, ExpiresAt: expiresAt,
	}, nil
}

func (s *ModelCapabilityAuditService) createBehaviorRun(ctx context.Context, options ModelBehaviorAuditOptions, requestedBy *int64, pairCount int) (*ModelBehaviorAuditRun, error) {
	encoded, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	var requested sql.NullInt64
	if requestedBy != nil {
		requested = sql.NullInt64{Int64: *requestedBy, Valid: true}
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO model_capability_probe_runs
			("trigger", status, requested_by, created_by, total_count, total_items, request_key, options, requested_scope)
		VALUES ($1, 'running', $2, $2, $3, $4, $5, $6::jsonb, $6::jsonb)
		ON CONFLICT DO NOTHING
		RETURNING id, status, requested_by, total_count, completed_count, verified_count,
		          mismatch_count, unsupported_count, error_count, error_message,
		          started_at, finished_at, created_at`, modelBehaviorAuditTrigger, requested, pairCount, pairCount*2,
		auditRequestKey(modelBehaviorAuditTrigger, encoded), string(encoded))
	run := &ModelBehaviorAuditRun{
		ModelID: options.ModelID, CaseCount: options.CaseCount,
		ReasoningEffort: options.ReasoningEffort, TotalRequests: pairCount * 2,
	}
	var finished sql.NullTime
	var requestedResult sql.NullInt64
	var errorMessage sql.NullString
	if err := row.Scan(&run.ID, &run.Status, &requestedResult, &run.TotalPairs, &run.CompletedPairs,
		&run.MatchedCount, &run.DivergedCount, &run.InconclusiveCount, &run.ErrorCount,
		&errorMessage, &run.StartedAt, &finished, &run.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.Conflict("MODEL_BEHAVIOR_AUDIT_ALREADY_RUNNING", "an identical model behavior audit is already running")
		}
		return nil, err
	}
	if requestedResult.Valid {
		run.RequestedBy = &requestedResult.Int64
	}
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
	}
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	return run, nil
}

func auditRequestKey(trigger string, encodedOptions []byte) string {
	digest := sha256.Sum256(append(append([]byte(trigger), 0), encodedOptions...))
	return hex.EncodeToString(digest[:])
}

func modelBehaviorAuditRunTimeout(pairCount int) time.Duration {
	if pairCount <= 0 {
		return 2 * time.Minute
	}
	waves := (pairCount + modelBehaviorAuditWorkerCount - 1) / modelBehaviorAuditWorkerCount
	return time.Duration(waves)*2*modelBehaviorProbeTimeout + 30*time.Second
}

func (s *ModelCapabilityAuditService) prepareBehaviorRun(ctx context.Context, input ModelBehaviorAuditOptions, requestedBy *int64) (*ModelBehaviorAuditRun, []modelBehaviorAuditTarget, error) {
	options, err := normalizeModelBehaviorOptions(input)
	if err != nil {
		return nil, nil, err
	}
	targets, _, _, reason, err := s.prepareBehaviorTargets(ctx, options)
	if err != nil {
		return nil, nil, err
	}
	optionsFingerprint, err := modelBehaviorAuditOptionsFingerprint(options)
	if err != nil {
		return nil, nil, err
	}
	targetsFingerprint, err := modelBehaviorAuditTargetsFingerprint(targets)
	if err != nil {
		return nil, nil, err
	}
	if err := s.consumeModelAuditPreview(
		modelCapabilityAuditPreviewKindBehavior,
		"MODEL_BEHAVIOR_AUDIT",
		options.PreviewToken,
		optionsFingerprint,
		targetsFingerprint,
	); err != nil {
		return nil, nil, err
	}
	if len(targets) == 0 {
		return nil, nil, infraerrors.BadRequest("MODEL_BEHAVIOR_TARGETS_EMPTY", reason)
	}
	pairCount := len(targets) * options.CaseCount
	run, err := s.createBehaviorRun(ctx, options, requestedBy, pairCount)
	if err != nil {
		return nil, nil, err
	}
	return run, targets, nil
}

func (s *ModelCapabilityAuditService) StartBehavior(ctx context.Context, input ModelBehaviorAuditOptions, requestedBy *int64) (*ModelBehaviorAuditRun, error) {
	if err := s.recoverStaleAuditRuns(ctx, modelBehaviorAuditTrigger); err != nil {
		return nil, err
	}
	run, targets, err := s.prepareBehaviorRun(ctx, input, requestedBy)
	if err != nil {
		return nil, err
	}
	options, err := normalizeModelBehaviorOptions(input)
	if err != nil {
		return nil, err
	}
	pairCount := len(targets) * options.CaseCount
	backgroundRun := *run
	runCtx, cancel := context.WithTimeout(context.Background(), modelBehaviorAuditRunTimeout(pairCount))
	go func() {
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("model_behavior_audit.background_panic", "run_id", run.ID, "recover", recovered)
				s.markRunFailed(run.ID, "paired behavior audit panicked")
			}
		}()
		if err := s.executeBehaviorRun(runCtx, &backgroundRun, options, targets); err != nil {
			s.markRunFailed(run.ID, "paired behavior audit failed")
		}
	}()
	return run, nil
}

func behaviorAuditPayload(model, effort, toolName string, testCase modelBehaviorCase) ([]byte, error) {
	return json.Marshal(map[string]any{
		"model": model,
		"input": []map[string]any{{
			"role": "user", "content": []map[string]any{{"type": "input_text", "text": testCase.Prompt}},
		}},
		"reasoning": map[string]any{"effort": effort},
		"tools": []map[string]any{{
			"type": "function", "name": toolName,
			"description": "Submit the deterministic audit answer.",
			"parameters": map[string]any{
				"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}},
				"required": []string{"answer"}, "additionalProperties": false,
			},
		}},
		"tool_choice":         map[string]any{"type": "function", "name": toolName},
		"parallel_tool_calls": false, "max_output_tokens": modelBehaviorAuditMaxOutputTokens(effort), "stream": false,
	})
}

func behaviorAuditToolAnswer(body []byte, expectedName string) (string, bool, bool) {
	var response struct {
		Output []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"output"`
	}
	if json.Unmarshal(body, &response) != nil {
		return "", false, false
	}
	for _, output := range response.Output {
		if output.Type != "function_call" || output.Name != expectedName {
			continue
		}
		var arguments struct {
			Answer string `json:"answer"`
		}
		if json.Unmarshal([]byte(output.Arguments), &arguments) != nil {
			return "", true, false
		}
		answer := strings.TrimSpace(arguments.Answer)
		return answer, true, answer != ""
	}
	return "", false, false
}

func behaviorAnswerHash(answer string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(answer)))
	return hex.EncodeToString(digest[:])
}

func (s *ModelCapabilityAuditService) probeBehaviorEndpoint(ctx context.Context, account *Account, upstreamModel, effort, toolName string, testCase modelBehaviorCase) (result modelBehaviorObservation) {
	result = modelBehaviorObservation{Status: "error", ToolName: toolName}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("model_behavior_audit.endpoint_panic", "account_id", accountID, "case", testCase.Key)
			result = modelBehaviorObservation{Status: "error", ToolName: toolName, Attempted: result.Attempted, Error: "behavior probe panicked"}
		}
	}()
	if account == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		result.Error = "behavior probe service unavailable"
		return result
	}
	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		result.Error = "account API key is missing"
		return result
	}
	baseURL, err := s.accountTestService.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		result.Error = "invalid account base URL"
		return result
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		result.Error = "mapped model is empty"
		return result
	}
	if modelCapabilityAuditTextTooLong(upstreamModel) {
		result.Error = "mapped model ID exceeds 200 characters"
		return result
	}
	body, err := behaviorAuditPayload(upstreamModel, effort, toolName, testCase)
	if err != nil {
		result.Error = "failed to build behavior probe"
		return result
	}
	probeCtx, cancel := context.WithTimeout(ctx, modelBehaviorProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodPost, buildOpenAIResponsesURLForPlatform(account.Platform, baseURL), bytes.NewReader(body))
	if err != nil {
		result.Error = "failed to create behavior probe"
		return result
	}
	requestCtx := WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI)
	request = request.WithContext(WithHTTPUpstreamRedirectsDisabled(requestCtx))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+authToken)
	account.ApplyHeaderOverrides(request.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	var release func()
	if s.concurrencyService != nil && s.concurrencyService.cache != nil {
		release, err = s.acquireAuditAccountSlot(probeCtx, account.ID, account.Concurrency)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
				result.Status, result.Error = "timeout", "timed out waiting for account concurrency"
			} else {
				result.Error = "failed to acquire account concurrency"
			}
			return result
		}
	}
	if release != nil {
		defer release()
	}
	started := time.Now()
	result.Attempted = true
	response, err := s.accountTestService.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, capabilityProbeTLSProfile(s.accountTestService, account))
	result.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			result.Status, result.Error = "timeout", "upstream request timed out"
		} else {
			result.Error = "upstream request failed"
		}
		return result
	}
	if response == nil {
		result.Error = "upstream returned no response"
		return result
	}
	defer func() { _ = response.Body.Close() }()
	result.HTTPStatus = response.StatusCode
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, modelCapabilityProbeMaxBodyBytes))
	if err != nil {
		result.Error = "failed to read upstream response"
		return result
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Error = capabilityProbeError(responseBody)
		return result
	}
	if !capabilityProbeJSONBody(responseBody) {
		result.Error = "upstream returned invalid JSON"
		return result
	}
	result.ResponseModel = redactCapabilityProbeResponseModel(account, capabilityProbeResponseModel(responseBody), authToken)
	if _, message, inconclusive := capabilityProbeInconclusive(ModelCapabilityProtocolResponsesTools, responseBody); inconclusive {
		result.Status, result.Error = "inconclusive", message
		return result
	}
	answer, found, valid := behaviorAuditToolAnswer(responseBody, toolName)
	result.ToolFound, result.ToolValid = found, valid
	if !found || !valid {
		result.Status, result.Error = "invalid", "expected tool answer was not returned"
		return result
	}
	result.AnswerHash = behaviorAnswerHash(answer)
	result.Correct = answer == testCase.Expected
	if result.Correct {
		result.Status = "correct"
	} else {
		result.Status = "incorrect"
	}
	return result
}

func compareBehaviorObservations(target, anchor modelBehaviorObservation) string {
	if target.Status == "inconclusive" || anchor.Status == "inconclusive" {
		return "inconclusive"
	}
	if !behaviorObservationComparable(target) || !behaviorObservationComparable(anchor) {
		return "error"
	}
	if target.Correct && anchor.Correct {
		return "matched"
	}
	if !target.Correct && anchor.Correct {
		return "diverged"
	}
	return "inconclusive"
}

func behaviorObservationComparable(result modelBehaviorObservation) bool {
	return result.Status == "correct" || result.Status == "incorrect"
}

func behaviorObservationCounts(results ...modelBehaviorObservation) (attempts, successes, failures int) {
	for _, result := range results {
		if !result.Attempted {
			continue
		}
		attempts++
		if behaviorObservationComparable(result) {
			successes++
		} else {
			failures++
		}
	}
	return attempts, successes, failures
}

func (s *ModelCapabilityAuditService) insertBehaviorResult(ctx context.Context, runID int64, requestedModel string, target modelBehaviorAuditTarget, testCase modelBehaviorCase, targetResult, anchorResult modelBehaviorObservation, checkedAt time.Time) error {
	if target.Target == nil || target.Anchor == nil {
		return errors.New("paired behavior target is incomplete")
	}
	comparison := compareBehaviorObservations(targetResult, anchorResult)
	errorsList := make([]string, 0, 2)
	if targetResult.Error != "" {
		errorsList = append(errorsList, "target: "+targetResult.Error)
	}
	if anchorResult.Error != "" {
		errorsList = append(errorsList, "anchor: "+anchorResult.Error)
	}
	errorMessage := strings.Join(errorsList, "; ")
	attempts, successes, failures := behaviorObservationCounts(targetResult, anchorResult)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO model_behavior_audit_results
			(run_id, target_account_id, target_account_name, anchor_account_id, anchor_account_name,
			 case_key, case_label, requested_model, target_mapped_model, anchor_mapped_model,
			 target_response_model, anchor_response_model, comparison_status, target_status, anchor_status,
			 target_correct, anchor_correct, target_http_status, anchor_http_status,
			 target_latency_ms, anchor_latency_ms, target_output_hash, anchor_output_hash,
			 expected_output_hash, attempt_count, success_count, failure_count, error_message, checked_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),$13,$14,$15,
		        $16,$17,NULLIF($18,0),NULLIF($19,0),NULLIF($20,0),NULLIF($21,0),NULLIF($22,''),
		        NULLIF($23,''),$24,$25,$26,$27,NULLIF($28,''),$29)
		ON CONFLICT (run_id, target_account_id, case_key) DO UPDATE SET
			target_account_name=EXCLUDED.target_account_name,
			anchor_account_id=EXCLUDED.anchor_account_id,
			anchor_account_name=EXCLUDED.anchor_account_name,
			case_label=EXCLUDED.case_label,
			requested_model=EXCLUDED.requested_model,
			target_mapped_model=EXCLUDED.target_mapped_model,
			anchor_mapped_model=EXCLUDED.anchor_mapped_model,
			target_response_model=EXCLUDED.target_response_model,
			anchor_response_model=EXCLUDED.anchor_response_model,
			comparison_status=EXCLUDED.comparison_status,
			target_status=EXCLUDED.target_status,
			anchor_status=EXCLUDED.anchor_status,
			target_correct=EXCLUDED.target_correct,
			anchor_correct=EXCLUDED.anchor_correct,
			target_http_status=EXCLUDED.target_http_status,
			anchor_http_status=EXCLUDED.anchor_http_status,
			target_latency_ms=EXCLUDED.target_latency_ms,
			anchor_latency_ms=EXCLUDED.anchor_latency_ms,
			target_output_hash=EXCLUDED.target_output_hash,
			anchor_output_hash=EXCLUDED.anchor_output_hash,
			expected_output_hash=EXCLUDED.expected_output_hash,
			attempt_count=EXCLUDED.attempt_count,
			success_count=EXCLUDED.success_count,
			failure_count=EXCLUDED.failure_count,
			error_message=EXCLUDED.error_message,
			checked_at=EXCLUDED.checked_at`,
		runID, target.Target.ID, target.Target.Name, target.Anchor.ID, target.Anchor.Name,
		testCase.Key, testCase.Label, truncateModelCapabilityAuditText(requestedModel),
		truncateModelCapabilityAuditText(target.TargetMappedModel), truncateModelCapabilityAuditText(target.AnchorMappedModel),
		truncateModelCapabilityAuditText(targetResult.ResponseModel), truncateModelCapabilityAuditText(anchorResult.ResponseModel), comparison, targetResult.Status, anchorResult.Status,
		targetResult.Correct, anchorResult.Correct, targetResult.HTTPStatus, anchorResult.HTTPStatus,
		targetResult.LatencyMS, anchorResult.LatencyMS, targetResult.AnswerHash, anchorResult.AnswerHash,
		behaviorAnswerHash(testCase.Expected), attempts, successes, failures, nullableText(errorMessage), checkedAt)
	return err
}

func (s *ModelCapabilityAuditService) probeBehaviorPair(ctx context.Context, options ModelBehaviorAuditOptions, itemTarget modelBehaviorAuditTarget, testCase modelBehaviorCase) (targetResult, anchorResult modelBehaviorObservation) {
	toolName := modelBehaviorToolNamePrefix + capabilityProbeNonce()
	defer func() {
		if recovered := recover(); recovered != nil {
			message := "paired behavior worker panicked"
			targetResult = modelBehaviorObservation{Status: "error", ToolName: toolName, Error: message}
			anchorResult = modelBehaviorObservation{Status: "error", ToolName: toolName, Error: message}
		}
	}()
	targetResult = s.probeBehaviorEndpoint(ctx, itemTarget.Target, itemTarget.TargetMappedModel, options.ReasoningEffort, toolName, testCase)
	if !behaviorObservationComparable(targetResult) {
		anchorResult = modelBehaviorObservation{Status: "skipped", ToolName: toolName}
		return targetResult, anchorResult
	}
	anchorResult = s.probeBehaviorEndpoint(ctx, itemTarget.Anchor, itemTarget.AnchorMappedModel, options.ReasoningEffort, toolName, testCase)
	return targetResult, anchorResult
}

func (s *ModelCapabilityAuditService) executeBehaviorRun(ctx context.Context, run *ModelBehaviorAuditRun, options ModelBehaviorAuditOptions, targets []modelBehaviorAuditTarget) error {
	runCtx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	type job struct {
		target   modelBehaviorAuditTarget
		testCase modelBehaviorCase
	}
	type outcome struct {
		job          job
		targetResult modelBehaviorObservation
		anchorResult modelBehaviorObservation
		ack          chan bool
	}
	jobs := make(chan job)
	outcomes := make(chan outcome, run.TotalPairs)
	var workers sync.WaitGroup
	for i := 0; i < modelBehaviorAuditWorkerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				ack := make(chan bool, 1)
				if runCtx.Err() != nil {
					outcomes <- outcome{
						job:          item,
						targetResult: modelBehaviorObservation{Status: "error", Error: runCtx.Err().Error()},
						anchorResult: modelBehaviorObservation{Status: "error", Error: runCtx.Err().Error()},
						ack:          ack,
					}
					return
				}
				targetResult, anchorResult := s.probeBehaviorPair(runCtx, options, item.target, item.testCase)
				outcomes <- outcome{job: item, targetResult: targetResult, anchorResult: anchorResult, ack: ack}
				select {
				case proceed := <-ack:
					if !proceed {
						return
					}
				case <-runCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, target := range targets {
			for _, testCase := range modelBehaviorCases[:options.CaseCount] {
				select {
				case jobs <- job{target: target, testCase: testCase}:
				case <-runCtx.Done():
					return
				}
			}
		}
	}()
	go func() { workers.Wait(); close(outcomes) }()

	completed, matched, diverged, inconclusive, errorCount := 0, 0, 0, 0, 0
	attemptCount := 0
	persistenceFailed := false
	for item := range outcomes {
		pairAttempts, _, _ := behaviorObservationCounts(item.targetResult, item.anchorResult)
		attemptCount += pairAttempts
		comparison := compareBehaviorObservations(item.targetResult, item.anchorResult)
		persistCtx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
		err := s.insertBehaviorResult(persistCtx, run.ID, options.ModelID, item.job.target, item.job.testCase, item.targetResult, item.anchorResult, time.Now())
		cancel()
		if err != nil {
			persistenceFailed = true
			errorCount++
			stopRun()
		} else {
			completed++
			switch comparison {
			case "matched":
				matched++
			case "diverged":
				diverged++
			case "inconclusive":
				inconclusive++
			default:
				errorCount++
			}
		}
		if err := s.updateBehaviorRunProgress(run.ID, completed, attemptCount, matched, diverged, inconclusive, errorCount); err != nil {
			slog.Warn("model_behavior_audit.progress_update_failed", "run_id", run.ID, "error", err)
			persistenceFailed = true
			errorCount++
			stopRun()
		}
		item.ack <- !persistenceFailed && runCtx.Err() == nil
	}
	status := "completed"
	errorMessage := ""
	if ctx.Err() != nil || persistenceFailed || completed < run.TotalPairs {
		status = "partial"
	}
	if persistenceFailed {
		errorMessage = "one or more paired results could not be persisted"
	} else if ctx.Err() != nil {
		errorMessage = "paired behavior audit exceeded its execution deadline"
	}
	finished := time.Now()
	persistCtx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
	_, err := s.db.ExecContext(persistCtx, `
		UPDATE model_capability_probe_runs
		SET status=$2, completed_count=$3, completed_items=$4, verified_count=$5,
		    mismatch_count=$6, unsupported_count=$7, error_count=$8, heartbeat_at=NOW(), finished_at=$9,
		    error_message=NULLIF($10,''), error_summary=NULLIF($10,'')
		WHERE id=$1 AND status='running'`, run.ID, status, completed, attemptCount, matched, diverged, inconclusive, errorCount, finished, errorMessage)
	cancel()
	return err
}

func (s *ModelCapabilityAuditService) updateBehaviorRunProgress(runID int64, completedPairs, attemptCount, matched, diverged, inconclusive, errorCount int) error {
	ctx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
		UPDATE model_capability_probe_runs
		SET completed_count=$2, completed_items=$3, verified_count=$4, mismatch_count=$5,
		    unsupported_count=$6, error_count=$7, heartbeat_at=NOW()
		WHERE id=$1 AND status='running'`, runID, completedPairs, attemptCount, matched, diverged, inconclusive, errorCount)
	return err
}

func scanModelBehaviorRun(scanner rowScanner) (*ModelBehaviorAuditRun, error) {
	var run ModelBehaviorAuditRun
	var requested sql.NullInt64
	var optionsJSON []byte
	var errorMessage sql.NullString
	var finished sql.NullTime
	if err := scanner.Scan(&run.ID, &run.Status, &requested, &run.TotalPairs, &run.TotalRequests, &run.CompletedPairs, &run.AttemptCount,
		&run.MatchedCount, &run.DivergedCount, &run.InconclusiveCount, &run.ErrorCount,
		&optionsJSON, &errorMessage, &run.StartedAt, &finished, &run.CreatedAt); err != nil {
		return nil, err
	}
	var options ModelBehaviorAuditOptions
	_ = json.Unmarshal(optionsJSON, &options)
	run.ModelID, run.CaseCount, run.ReasoningEffort = options.ModelID, options.CaseCount, options.ReasoningEffort
	if requested.Valid {
		run.RequestedBy = &requested.Int64
	}
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
	}
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	return &run, nil
}

func (s *ModelCapabilityAuditService) ListBehaviorRuns(ctx context.Context, limit int) ([]ModelBehaviorAuditRun, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("model behavior audit service unavailable")
	}
	if err := s.recoverStaleAuditRuns(ctx, modelBehaviorAuditTrigger); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, requested_by, total_count, total_items, completed_count, completed_items, verified_count,
		       mismatch_count, unsupported_count, error_count, options, error_message,
		       started_at, finished_at, created_at
		FROM model_capability_probe_runs WHERE "trigger"=$1
		ORDER BY id DESC LIMIT $2`, modelBehaviorAuditTrigger, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ModelBehaviorAuditRun, 0)
	for rows.Next() {
		run, scanErr := scanModelBehaviorRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, *run)
	}
	return result, rows.Err()
}

func (s *ModelCapabilityAuditService) GetBehaviorRun(ctx context.Context, id int64) (*ModelBehaviorAuditDetail, error) {
	if id <= 0 || s == nil || s.db == nil {
		return nil, infraerrors.NotFound("MODEL_BEHAVIOR_RUN_NOT_FOUND", "model behavior audit run not found")
	}
	if err := s.recoverStaleAuditRuns(ctx, modelBehaviorAuditTrigger); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, status, requested_by, total_count, total_items, completed_count, completed_items, verified_count,
		       mismatch_count, unsupported_count, error_count, options, error_message,
		       started_at, finished_at, created_at
		FROM model_capability_probe_runs WHERE id=$1 AND "trigger"=$2`, id, modelBehaviorAuditTrigger)
	run, err := scanModelBehaviorRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("MODEL_BEHAVIOR_RUN_NOT_FOUND", "model behavior audit run not found")
		}
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, target_account_id, target_account_name, anchor_account_id, anchor_account_name,
		       case_key, case_label, requested_model, target_mapped_model, anchor_mapped_model,
		       target_response_model, anchor_response_model, comparison_status, target_status, anchor_status,
		       target_correct, anchor_correct, target_http_status, anchor_http_status,
		       target_latency_ms, anchor_latency_ms, target_output_hash, anchor_output_hash,
		       expected_output_hash, attempt_count, success_count, failure_count, error_message, checked_at
		FROM model_behavior_audit_results
		WHERE run_id=$1
		ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]ModelBehaviorAuditResult, 0)
	for rows.Next() {
		var result ModelBehaviorAuditResult
		var targetResponseModel, anchorResponseModel, targetOutputHash, anchorOutputHash, errorMessage sql.NullString
		var targetHTTPStatus, anchorHTTPStatus, targetLatency, anchorLatency sql.NullInt64
		if err := rows.Scan(&result.ID, &result.TargetAccountID, &result.TargetAccountName,
			&result.AnchorAccountID, &result.AnchorAccountName, &result.CaseKey, &result.CaseLabel,
			&result.RequestedModel, &result.TargetMappedModel, &result.AnchorMappedModel,
			&targetResponseModel, &anchorResponseModel, &result.ComparisonStatus, &result.TargetStatus, &result.AnchorStatus,
			&result.TargetCorrect, &result.AnchorCorrect, &targetHTTPStatus, &anchorHTTPStatus,
			&targetLatency, &anchorLatency, &targetOutputHash, &anchorOutputHash,
			&result.ExpectedOutputHash, &result.AttemptCount, &result.SuccessCount, &result.FailureCount,
			&errorMessage, &result.CheckedAt); err != nil {
			return nil, err
		}
		result.TargetResponseModel = targetResponseModel.String
		result.AnchorResponseModel = anchorResponseModel.String
		result.TargetHTTPStatus = int(targetHTTPStatus.Int64)
		result.AnchorHTTPStatus = int(anchorHTTPStatus.Int64)
		result.TargetLatencyMS = targetLatency.Int64
		result.AnchorLatencyMS = anchorLatency.Int64
		result.TargetOutputHash = targetOutputHash.String
		result.AnchorOutputHash = anchorOutputHash.String
		result.Error = errorMessage.String
		results = append(results, result)
	}
	return &ModelBehaviorAuditDetail{Run: run, Results: results}, rows.Err()
}
