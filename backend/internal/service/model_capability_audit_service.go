package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const (
	modelCapabilityAuditPersistenceTimeout  = 10 * time.Second
	modelCapabilityAuditTargetTimeout       = 75 * time.Second
	modelCapabilityAuditWorkerCount         = 4
	modelCapabilityAuditStaleAfter          = 2 * time.Minute
	modelBehaviorAuditStaleAfter            = 90 * time.Second
	modelCapabilityAuditSlotRetryInitial    = 25 * time.Millisecond
	modelCapabilityAuditSlotRetryMax        = 250 * time.Millisecond
	modelCapabilityAuditPreviewTTL          = 5 * time.Minute
	modelCapabilityAuditPreviewMaxEntries   = 128
	modelCapabilityAuditPreviewTokenBytes   = 32
	modelCapabilityAuditPreviewKindManual   = "manual"
	modelCapabilityAuditPreviewKindBehavior = "paired_behavior"
	modelCapabilityAuditPassiveModelLimit   = 16
)

// ModelCapabilityAuditOptions controls one bounded, administrator-triggered
// audit run. The bounds are intentional: an audit must never turn into an
// unbounded upstream traffic generator.
type ModelCapabilityAuditOptions struct {
	AccountIDs   []int64  `json:"account_ids,omitempty"`
	ModelIDs     []string `json:"model_ids,omitempty"`
	Protocols    []string `json:"protocols,omitempty"`
	ActiveOnly   bool     `json:"active_only"`
	MaxItems     int      `json:"max_items,omitempty"`
	PreviewToken string   `json:"-"`
}

type ModelCapabilityAuditEstimate struct {
	TargetCount         int       `json:"target_count"`
	ProtocolCount       int       `json:"protocol_count"`
	RequestCount        int       `json:"request_count"`
	EstimatedMaxSeconds int64     `json:"estimated_max_seconds"`
	MaxItems            int       `json:"max_items"`
	PreviewToken        string    `json:"preview_token"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type ModelCapabilityAuditRun struct {
	ID               int64      `json:"id"`
	Trigger          string     `json:"trigger,omitempty"`
	Status           string     `json:"status"`
	RequestedBy      *int64     `json:"requested_by,omitempty"`
	CreatedBy        *int64     `json:"created_by,omitempty"`
	TotalCount       int        `json:"total_count"`
	TotalItems       int        `json:"total_items,omitempty"`
	CompletedCount   int        `json:"completed_count"`
	CompletedItems   int        `json:"completed_items,omitempty"`
	VerifiedCount    int        `json:"verified_count"`
	MismatchCount    int        `json:"mismatch_count"`
	UnsupportedCount int        `json:"unsupported_count"`
	ErrorCount       int        `json:"error_count"`
	Options          any        `json:"options,omitempty"`
	RequestedScope   any        `json:"requested_scope,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	ErrorSummary     string     `json:"error_summary,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type ModelCapabilityAuditEvidence struct {
	ID                 int64     `json:"id,omitempty"`
	RunID              int64     `json:"run_id,omitempty"`
	AccountID          int64     `json:"account_id,omitempty"`
	AccountName        string    `json:"account_name,omitempty"`
	Platform           string    `json:"platform,omitempty"`
	GroupID            *int64    `json:"group_id,omitempty"`
	GroupName          string    `json:"group_name,omitempty"`
	RequestedModel     string    `json:"requested_model,omitempty"`
	MappedModel        string    `json:"mapped_model,omitempty"`
	ResponseModel      string    `json:"response_model,omitempty"`
	Protocol           string    `json:"protocol"`
	Status             string    `json:"status"`
	Capability         string    `json:"capability"`
	HTTPStatus         int       `json:"http_status"`
	LatencyMS          int64     `json:"latency_ms"`
	UpstreamModel      string    `json:"upstream_model,omitempty"`
	ToolCallFound      bool      `json:"tool_call_found"`
	ToolArgumentsValid bool      `json:"tool_arguments_valid"`
	Error              string    `json:"error,omitempty"`
	Evidence           string    `json:"evidence"`
	EvidenceCode       string    `json:"evidence_code,omitempty"`
	CapabilityStatus   string    `json:"capability_status,omitempty"`
	IdentityStatus     string    `json:"identity_status,omitempty"`
	Confidence         string    `json:"confidence,omitempty"`
	EvidenceSource     string    `json:"evidence_source,omitempty"`
	AttemptCount       int       `json:"attempt_count"`
	SuccessCount       int       `json:"success_count"`
	FailureCount       int       `json:"failure_count"`
	CheckedAt          time.Time `json:"checked_at"`
}

type ModelCapabilityAuditRow struct {
	Key                    string                                  `json:"key"`
	AccountID              int64                                   `json:"account_id"`
	AccountName            string                                  `json:"account_name"`
	Platform               string                                  `json:"platform"`
	GroupID                *int64                                  `json:"group_id,omitempty"`
	GroupName              string                                  `json:"group_name,omitempty"`
	RequestedModel         string                                  `json:"requested_model"`
	MappedModel            string                                  `json:"mapped_model,omitempty"`
	ObservedModel          string                                  `json:"observed_model,omitempty"`
	PassiveRequests        int64                                   `json:"passive_requests"`
	PassiveMismatches      int64                                   `json:"passive_mismatches"`
	PassiveResponseModels  []string                                `json:"passive_response_models,omitempty"`
	PassiveLastSeen        *time.Time                              `json:"passive_last_seen,omitempty"`
	ActiveLastSeen         *time.Time                              `json:"active_last_seen,omitempty"`
	CapabilityStatus       string                                  `json:"capability_status"`
	ActiveStatus           string                                  `json:"active_status,omitempty"`
	IdentityStatus         string                                  `json:"identity_status"`
	Confidence             string                                  `json:"confidence"`
	LastSeen               *time.Time                              `json:"last_seen,omitempty"`
	LastChecked            *time.Time                              `json:"last_checked,omitempty"`
	EvidenceCount          int                                     `json:"evidence_count,omitempty"`
	Protocols              map[string]ModelCapabilityAuditEvidence `json:"protocols"`
	passiveNeedsModelMatch bool
}

type ModelCapabilityAuditSummary struct {
	Days                 int                            `json:"days"`
	GeneratedAt          time.Time                      `json:"generated_at"`
	TotalModels          int                            `json:"total_models"`
	ReturnedModels       int                            `json:"returned_models"`
	RowsTruncated        bool                           `json:"rows_truncated"`
	CapabilityVerified   int                            `json:"capability_verified"`
	IdentityConsistent   int                            `json:"identity_consistent"`
	SelfReportConsistent int                            `json:"self_report_consistent"`
	MismatchSuspected    int                            `json:"mismatch_suspected"`
	Unverified           int                            `json:"unverified"`
	PassiveRequests      int64                          `json:"passive_requests"`
	TotalAccounts        int                            `json:"total_accounts,omitempty"`
	Stats                ModelCapabilityAuditStats      `json:"stats"`
	RecentEvidence       []ModelCapabilityAuditEvidence `json:"recent_evidence,omitempty"`
	Rows                 []ModelCapabilityAuditRow      `json:"rows"`
}

type ModelCapabilityAuditStats struct {
	Models          int   `json:"models"`
	Accounts        int   `json:"accounts"`
	Verified        int   `json:"verified"`
	SelfReported    int   `json:"self_reported"`
	Suspect         int   `json:"suspect"`
	Unverified      int   `json:"unverified"`
	PassiveRequests int64 `json:"passive_requests"`
	ActiveChecks    int64 `json:"active_checks"`
}

type modelCapabilityAuditTarget struct {
	Account   *Account
	GroupID   *int64
	ModelID   string
	Protocols []string
}

type modelCapabilityPassive struct {
	AccountID         int64
	AccountName       string
	Platform          string
	RequestedModel    string
	MappedModel       string
	ResponseModelList string
	Requests          int64
	Mismatches        int64
	LastSeen          time.Time
	NeedsModelMatch   bool
}

type modelCapabilityPassiveTotals struct {
	TotalModels                   int
	TotalAccounts                 int
	PassiveRequests               int64
	PassiveOnlyMismatchModels     int
	PassiveOnlySelfReportedModels int
	PassiveOnlyUnverifiedModels   int
}

type modelCapabilityAuditPreviewSnapshot struct {
	kind               string
	optionsFingerprint string
	targetsFingerprint string
	expiresAt          time.Time
}

// ModelCapabilityAuditService combines durable active probes with passive
// usage-log evidence. It deliberately lives beside AccountTestService so the
// existing single-account probe API remains backwards compatible.
type ModelCapabilityAuditService struct {
	db                 *sql.DB
	accountRepo        AccountRepository
	accountTestService *AccountTestService
	concurrencyService *ConcurrencyService

	previewMu        sync.Mutex
	previewSnapshots map[string]modelCapabilityAuditPreviewSnapshot
}

func NewModelCapabilityAuditService(db *sql.DB, accountRepo AccountRepository, accountTestService *AccountTestService, concurrencyService *ConcurrencyService) *ModelCapabilityAuditService {
	return &ModelCapabilityAuditService{
		db: db, accountRepo: accountRepo, accountTestService: accountTestService,
		concurrencyService: concurrencyService,
	}
}

func (s *ModelCapabilityAuditService) normalizeOptions(input ModelCapabilityAuditOptions) (ModelCapabilityAuditOptions, []string, error) {
	if s == nil || s.db == nil || s.accountRepo == nil {
		return input, nil, errors.New("model capability audit service unavailable")
	}
	if s.accountTestService == nil {
		return input, nil, errors.New("model capability active probe service unavailable")
	}
	input.PreviewToken = strings.TrimSpace(input.PreviewToken)
	protocols, err := normalizeModelCapabilityProtocols(input.Protocols)
	if err != nil {
		return input, nil, err
	}
	if input.MaxItems <= 0 {
		input.MaxItems = 60
	}
	if input.MaxItems > 240 {
		input.MaxItems = 240
	}
	input.AccountIDs = uniquePositiveIDs(input.AccountIDs)
	input.ModelIDs = uniqueNonEmpty(input.ModelIDs)
	for _, modelID := range input.ModelIDs {
		if modelCapabilityAuditTextTooLong(modelID) {
			return input, nil, infraerrors.BadRequest("MODEL_CAPABILITY_MODEL_INVALID", "model ID must not exceed 200 characters")
		}
	}
	sort.Slice(input.AccountIDs, func(i, j int) bool { return input.AccountIDs[i] < input.AccountIDs[j] })
	sort.Strings(input.ModelIDs)
	sort.Strings(protocols)
	input.Protocols = append([]string(nil), protocols...)
	return input, protocols, nil
}

func uniquePositiveIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *ModelCapabilityAuditService) loadAccounts(ctx context.Context, options ModelCapabilityAuditOptions) ([]*Account, error) {
	if len(options.AccountIDs) > 0 {
		accounts, err := s.accountRepo.GetByIDs(ctx, options.AccountIDs)
		if err != nil {
			return nil, err
		}
		out := make([]*Account, 0, len(accounts))
		for _, account := range accounts {
			if account == nil {
				continue
			}
			if options.ActiveOnly && !strings.EqualFold(account.Status, StatusActive) {
				continue
			}
			out = append(out, account)
		}
		return out, nil
	}
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return nil, err
	}
	out := make([]*Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account == nil || (options.ActiveOnly && !strings.EqualFold(account.Status, StatusActive)) {
			continue
		}
		out = append(out, account)
	}
	return out, nil
}

func isCapabilityProbeAccount(account *Account) bool {
	// Kept as a small helper for callers that want to distinguish accounts that
	// can use the OpenAI-compatible active probe. The audit itself still keeps
	// unsupported/OAuth accounts in its result set so their lack of evidence is
	// visible instead of silently disappearing from the report.
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	return account.IsOpenAI() || account.IsCNProvider()
}

func (s *ModelCapabilityAuditService) modelsForAccount(ctx context.Context, account *Account, requested []string) []string {
	if len(requested) > 0 {
		return append([]string(nil), requested...)
	}
	set := make(map[string]struct{})
	for model := range account.GetModelMapping() {
		if model != "" && model != "*" && !strings.Contains(model, "*") && !modelCapabilityAuditTextTooLong(model) {
			set[model] = struct{}{}
		}
	}
	for _, model := range account.GetModelMapping() {
		model = strings.TrimSpace(model)
		if model != "" && model != "*" && !strings.Contains(model, "*") && !modelCapabilityAuditTextTooLong(model) {
			set[model] = struct{}{}
		}
	}
	// Include models observed in real traffic so the audit follows actual use,
	// even when the account has no explicit mapping entry.
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT COALESCE(NULLIF(requested_model, ''), NULLIF(model, ''))
		FROM usage_logs
		WHERE account_id = $1
		  AND created_at >= NOW() - INTERVAL '90 days'
		  AND COALESCE(NULLIF(requested_model, ''), NULLIF(model, '')) IS NOT NULL
		ORDER BY 1
		LIMIT 24`, account.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var model string
			if rows.Scan(&model) == nil && strings.TrimSpace(model) != "" && !modelCapabilityAuditTextTooLong(model) {
				set[strings.TrimSpace(model)] = struct{}{}
			}
		}
	}
	if len(set) == 0 {
		set[defaultCapabilityAuditModel(account)] = struct{}{}
	}
	models := make([]string, 0, len(set))
	for model := range set {
		models = append(models, model)
	}
	sort.Strings(models)
	if len(models) > 24 {
		models = models[:24]
	}
	return models
}

func defaultCapabilityAuditModel(account *Account) string {
	if account == nil {
		return openai.DefaultTestModel
	}
	switch strings.ToLower(strings.TrimSpace(account.Platform)) {
	case PlatformAnthropic:
		return "claude-3-5-sonnet-latest"
	case PlatformGemini, PlatformAntigravity:
		return "gemini-2.0-flash"
	default:
		return openai.DefaultTestModel
	}
}

func accountPrimaryGroupID(account *Account) *int64 {
	if account == nil || len(account.GroupIDs) == 0 || account.GroupIDs[0] <= 0 {
		return nil
	}
	id := account.GroupIDs[0]
	return &id
}

func (s *ModelCapabilityAuditService) buildTargets(ctx context.Context, options ModelCapabilityAuditOptions, protocols []string) ([]modelCapabilityAuditTarget, error) {
	accounts, err := s.loadAccounts(ctx, options)
	if err != nil {
		return nil, err
	}
	targets := make([]modelCapabilityAuditTarget, 0, options.MaxItems)
	for _, account := range accounts {
		if !isCapabilityProbeAccount(account) {
			continue
		}
		for _, model := range s.modelsForAccount(ctx, account, options.ModelIDs) {
			targets = append(targets, modelCapabilityAuditTarget{
				Account:   account,
				GroupID:   accountPrimaryGroupID(account),
				ModelID:   model,
				Protocols: append([]string(nil), protocols...),
			})
			if len(targets) >= options.MaxItems {
				return targets, nil
			}
		}
	}
	return targets, nil
}

func modelCapabilityAuditPreviewKey(options ModelCapabilityAuditOptions) (string, error) {
	encoded, err := json.Marshal(options)
	if err != nil {
		return "", err
	}
	return auditRequestKey("manual-preview", encoded), nil
}

func modelCapabilityAuditTargetsFingerprint(targets []modelCapabilityAuditTarget) (string, error) {
	type fingerprintTarget struct {
		AccountID        int64    `json:"account_id"`
		GroupID          int64    `json:"group_id,omitempty"`
		ModelID          string   `json:"model_id"`
		MappedModel      string   `json:"mapped_model"`
		Protocols        []string `json:"protocols"`
		Platform         string   `json:"platform"`
		AccountType      string   `json:"account_type"`
		AccountStatus    string   `json:"account_status"`
		Concurrency      int      `json:"concurrency"`
		AccountUpdated   int64    `json:"account_updated"`
		AccountAvailable bool     `json:"account_available"`
	}
	encodedTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		item := fingerprintTarget{ModelID: target.ModelID, Protocols: append([]string(nil), target.Protocols...)}
		if target.GroupID != nil {
			item.GroupID = *target.GroupID
		}
		if target.Account != nil {
			item.AccountAvailable = true
			item.AccountID = target.Account.ID
			item.MappedModel = target.Account.GetMappedModel(target.ModelID)
			item.Platform = target.Account.Platform
			item.AccountType = target.Account.Type
			item.AccountStatus = target.Account.Status
			item.Concurrency = target.Account.Concurrency
			item.AccountUpdated = target.Account.UpdatedAt.UnixNano()
		}
		encoded, err := json.Marshal(item)
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
	return auditRequestKey("manual-preview-targets", encoded), nil
}

// issueModelAuditPreview creates a short-lived, one-time bearer token bound to
// both the normalized request and the credential-free target fingerprint.
func (s *ModelCapabilityAuditService) issueModelAuditPreview(kind, optionsFingerprint, targetsFingerprint string) (string, time.Time, error) {
	random := make([]byte, modelCapabilityAuditPreviewTokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	now := time.Now()
	expiresAt := now.Add(modelCapabilityAuditPreviewTTL)

	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	if s.previewSnapshots == nil {
		s.previewSnapshots = make(map[string]modelCapabilityAuditPreviewSnapshot)
	}
	for existingToken, snapshot := range s.previewSnapshots {
		if !snapshot.expiresAt.After(now) {
			delete(s.previewSnapshots, existingToken)
		}
	}
	if len(s.previewSnapshots) >= modelCapabilityAuditPreviewMaxEntries {
		var oldestToken string
		var oldestExpiry time.Time
		for existingToken, snapshot := range s.previewSnapshots {
			if oldestToken == "" || snapshot.expiresAt.Before(oldestExpiry) {
				oldestToken, oldestExpiry = existingToken, snapshot.expiresAt
			}
		}
		delete(s.previewSnapshots, oldestToken)
	}
	s.previewSnapshots[token] = modelCapabilityAuditPreviewSnapshot{
		kind: kind, optionsFingerprint: optionsFingerprint,
		targetsFingerprint: targetsFingerprint, expiresAt: expiresAt,
	}
	return token, expiresAt, nil
}

func (s *ModelCapabilityAuditService) consumeModelAuditPreview(kind, errorPrefix, token, optionsFingerprint, targetsFingerprint string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return infraerrors.BadRequest(errorPrefix+"_PREVIEW_REQUIRED", "a fresh preview token is required before starting this audit")
	}

	now := time.Now()
	s.previewMu.Lock()
	snapshot, ok := s.previewSnapshots[token]
	if ok {
		delete(s.previewSnapshots, token)
	}
	s.previewMu.Unlock()
	if !ok {
		return infraerrors.Conflict(errorPrefix+"_PREVIEW_INVALID", "the preview token is invalid or was lost after a server restart; refresh the confirmation")
	}
	if !snapshot.expiresAt.After(now) {
		return infraerrors.Conflict(errorPrefix+"_PREVIEW_EXPIRED", "the preview token expired; refresh the confirmation")
	}
	if snapshot.kind != kind {
		return infraerrors.Conflict(errorPrefix+"_PREVIEW_INVALID", "the preview token is not valid for this audit type")
	}
	if snapshot.optionsFingerprint != optionsFingerprint || snapshot.targetsFingerprint != targetsFingerprint {
		return infraerrors.Conflict(errorPrefix+"_PREVIEW_STALE", "audit options or targets changed after preview; refresh the confirmation")
	}
	return nil
}

func (s *ModelCapabilityAuditService) rememberPreviewTargets(options ModelCapabilityAuditOptions, targets []modelCapabilityAuditTarget) (string, time.Time, error) {
	optionsFingerprint, err := modelCapabilityAuditPreviewKey(options)
	if err != nil {
		return "", time.Time{}, err
	}
	targetsFingerprint, err := modelCapabilityAuditTargetsFingerprint(targets)
	if err != nil {
		return "", time.Time{}, err
	}
	return s.issueModelAuditPreview(modelCapabilityAuditPreviewKindManual, optionsFingerprint, targetsFingerprint)
}

// Estimate resolves exactly the same bounded target set as Start without
// creating a run or sending traffic. It lets the admin UI obtain informed
// confirmation before a potentially billable batch.
func (s *ModelCapabilityAuditService) Estimate(ctx context.Context, input ModelCapabilityAuditOptions) (*ModelCapabilityAuditEstimate, error) {
	options, protocols, err := s.normalizeOptions(input)
	if err != nil {
		return nil, err
	}
	targets, err := s.buildTargets(ctx, options, protocols)
	if err != nil {
		return nil, err
	}
	previewToken, expiresAt, err := s.rememberPreviewTargets(options, targets)
	if err != nil {
		return nil, err
	}
	requestCount := 0
	for _, target := range targets {
		requestCount += len(target.Protocols)
	}
	maxSeconds := int64(0)
	if len(targets) > 0 {
		maxSeconds = int64(modelCapabilityAuditRunTimeout(len(targets)) / time.Second)
	}
	return &ModelCapabilityAuditEstimate{
		TargetCount:         len(targets),
		ProtocolCount:       len(protocols),
		RequestCount:        requestCount,
		EstimatedMaxSeconds: maxSeconds,
		MaxItems:            options.MaxItems,
		PreviewToken:        previewToken,
		ExpiresAt:           expiresAt,
	}, nil
}

func (s *ModelCapabilityAuditService) createRun(ctx context.Context, options ModelCapabilityAuditOptions, requestedBy *int64, totalTargets, totalItems int) (*ModelCapabilityAuditRun, error) {
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
		VALUES ('manual', 'running', $1, $1, $2, $3, $4, $5::jsonb, $5::jsonb)
		ON CONFLICT DO NOTHING
		RETURNING id, status, requested_by, total_count, total_items, completed_count, completed_items, verified_count,
		          mismatch_count, unsupported_count, error_count, options, error_message,
		          started_at, finished_at, created_at`, requested, totalTargets, totalItems,
		auditRequestKey("manual", encoded), string(encoded))
	run, err := scanCapabilityAuditRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.Conflict("MODEL_CAPABILITY_AUDIT_ALREADY_RUNNING", "an identical model capability audit is already running")
	}
	return run, err
}

func (s *ModelCapabilityAuditService) insertResult(ctx context.Context, runID int64, target modelCapabilityAuditTarget, result ModelCapabilityProbeResult, checkedAt time.Time, source string) error {
	var groupID any
	if target.GroupID != nil {
		groupID = *target.GroupID
	}
	if strings.TrimSpace(source) == "" {
		source = "active"
	}
	evidenceSource := source
	if source == "active" {
		evidenceSource = "active_probe"
	}
	identityStatus := "unobserved"
	mappedModel := capabilityAuditResultMappedModel(target, result)
	observedModel := truncateModelCapabilityAuditText(result.UpstreamModel)
	if matches, observed := capabilityProbeResponseModelMatch(result, mappedModel, observedModel); observed {
		if matches {
			identityStatus = "self_report_consistent"
		} else {
			identityStatus = "mismatch"
		}
	}
	capabilityStatus := "unknown"
	switch result.Status {
	case "verified", "model_mismatch":
		capabilityStatus = "verified"
	case "unsupported":
		capabilityStatus = "unsupported"
	case "timeout":
		capabilityStatus = "timeout"
	case "error":
		capabilityStatus = "error"
	default:
		if result.Status != "" {
			capabilityStatus = result.Status
		}
	}
	confidence := "low"
	if identityStatus == "mismatch" {
		confidence = "high"
	} else if identityStatus == "self_report_consistent" {
		confidence = "medium"
	}
	attemptCount, successCount, failureCount := capabilityProbeAttemptCounts(result)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO model_capability_probe_results
		(run_id, account_id, group_id, account_name, platform, requested_model, mapped_model,
		 response_model, upstream_model, protocol, status, capability, capability_status, identity_status,
		 confidence, evidence_source, source, http_status, latency_ms, attempt_count,
		 success_count, failure_count, tool_name, tool_call_found, tool_arguments_valid,
		 error_message, evidence, evidence_code, response_summary, checked_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($8,''),$9,$10,$11,$12,$13,$14,$15,$16,
		 NULLIF($17,0),NULLIF($18,0),$19,$20,$21,NULLIF($22,''),$23,$24,
		 NULLIF($25,''),NULLIF($26,''),NULLIF($27,''),NULLIF($28,''),$29)`,
		runID, target.Account.ID, groupID, target.Account.Name, target.Account.Platform,
		truncateModelCapabilityAuditText(result.RequestedModel), mappedModel, observedModel, result.Protocol, result.Status,
		result.Capability, capabilityStatus, identityStatus, confidence, evidenceSource, source,
		result.HTTPStatus, result.LatencyMS, attemptCount, successCount, failureCount,
		truncateModelCapabilityAuditText(result.ToolName), result.ToolCallFound, result.ToolArgumentsValid, nullableText(result.Error),
		result.Evidence, result.Evidence, nullableText(result.Error), checkedAt)
	return err
}

func capabilityProbeAttemptCounts(result ModelCapabilityProbeResult) (attempts, successes, failures int) {
	if !result.Attempted {
		return 0, 0, 0
	}
	if result.Status == "verified" || result.Status == "model_mismatch" {
		return 1, 1, 0
	}
	return 1, 0, 1
}

func capabilityAuditResultMappedModel(target modelCapabilityAuditTarget, result ModelCapabilityProbeResult) string {
	if mappedModel := strings.TrimSpace(result.MappedModel); mappedModel != "" {
		return truncateModelCapabilityAuditText(mappedModel)
	}
	if target.Account != nil {
		if mappedModel := strings.TrimSpace(target.Account.GetMappedModel(result.RequestedModel)); mappedModel != "" {
			return truncateModelCapabilityAuditText(mappedModel)
		}
	}
	return truncateModelCapabilityAuditText(result.RequestedModel)
}

func capabilityProbeResponseModelMatch(result ModelCapabilityProbeResult, mappedModel, observedModel string) (bool, bool) {
	if result.responseModelMatch != nil {
		return *result.responseModelMatch, true
	}
	observedModel = strings.TrimSpace(observedModel)
	if observedModel == "" || capabilityProbeMetadataIsRedacted(observedModel) {
		return false, false
	}
	return strings.TrimSpace(mappedModel) != "" && upstreamModelsMatchForAudit(mappedModel, observedModel), true
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (s *ModelCapabilityAuditService) prepareRun(ctx context.Context, input ModelCapabilityAuditOptions, requestedBy *int64) (*ModelCapabilityAuditRun, []modelCapabilityAuditTarget, error) {
	options, protocols, err := s.normalizeOptions(input)
	if err != nil {
		return nil, nil, err
	}
	targets, err := s.buildTargets(ctx, options, protocols)
	if err != nil {
		return nil, nil, err
	}
	optionsFingerprint, err := modelCapabilityAuditPreviewKey(options)
	if err != nil {
		return nil, nil, err
	}
	targetsFingerprint, err := modelCapabilityAuditTargetsFingerprint(targets)
	if err != nil {
		return nil, nil, err
	}
	if err := s.consumeModelAuditPreview(
		modelCapabilityAuditPreviewKindManual,
		"MODEL_CAPABILITY_AUDIT",
		options.PreviewToken,
		optionsFingerprint,
		targetsFingerprint,
	); err != nil {
		return nil, nil, err
	}
	totalItems := 0
	for _, target := range targets {
		totalItems += len(target.Protocols)
	}
	run, err := s.createRun(ctx, options, requestedBy, len(targets), totalItems)
	if err != nil {
		return nil, nil, err
	}
	return run, targets, nil
}

// Start persists a run synchronously, then executes non-empty batches outside
// the HTTP request lifetime. The returned run is normally in "running" state;
// callers can follow it through ListRuns/GetRun.
func (s *ModelCapabilityAuditService) Start(ctx context.Context, input ModelCapabilityAuditOptions, requestedBy *int64) (*ModelCapabilityAuditRun, error) {
	if err := s.recoverStaleAuditRuns(ctx, "manual"); err != nil {
		return nil, err
	}
	run, targets, err := s.prepareRun(ctx, input, requestedBy)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return s.executeRun(ctx, run, targets)
	}

	backgroundRun := *run
	runCtx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditRunTimeout(len(targets)))
	go s.executeRunInBackground(runCtx, cancel, &backgroundRun, targets)
	return run, nil
}

// Run executes a bounded batch synchronously. The admin HTTP endpoint uses
// Start so reverse-proxy and browser timeouts cannot cancel a long batch.
func (s *ModelCapabilityAuditService) Run(ctx context.Context, input ModelCapabilityAuditOptions, requestedBy *int64) (*ModelCapabilityAuditRun, error) {
	if err := s.recoverStaleAuditRuns(ctx, "manual"); err != nil {
		return nil, err
	}
	run, targets, err := s.prepareRun(ctx, input, requestedBy)
	if err != nil {
		return nil, err
	}
	return s.executeRun(ctx, run, targets)
}

func modelCapabilityAuditRunTimeout(targetCount int) time.Duration {
	if targetCount <= 0 {
		return 2 * time.Minute
	}
	waves := (targetCount + modelCapabilityAuditWorkerCount - 1) / modelCapabilityAuditWorkerCount
	timeout := time.Duration(waves)*modelCapabilityAuditTargetTimeout + 30*time.Second
	if timeout < 2*time.Minute {
		return 2 * time.Minute
	}
	return timeout
}

func (s *ModelCapabilityAuditService) executeRunInBackground(ctx context.Context, cancel context.CancelFunc, run *ModelCapabilityAuditRun, targets []modelCapabilityAuditTarget) {
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("model_capability_audit.background_panic", "run_id", run.ID, "recover", recovered)
			s.markRunFailed(run.ID, "background capability audit panicked")
		}
	}()
	if _, err := s.executeRun(ctx, run, targets); err != nil {
		slog.Error("model_capability_audit.background_failed", "run_id", run.ID, "error", err)
		s.markRunFailed(run.ID, "background capability audit failed")
	}
}

func (s *ModelCapabilityAuditService) markRunFailed(runID int64, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
	defer cancel()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE model_capability_probe_runs
		SET status=CASE WHEN completed_count > 0 OR completed_items > 0 THEN 'partial' ELSE 'failed' END,
		    error_count=error_count+1, error_message=$2,
		    error_summary=$2, heartbeat_at=NOW(), finished_at=NOW()
		WHERE id=$1 AND status='running'`, runID, message); err != nil {
		slog.Error("model_capability_audit.mark_failed", "run_id", runID, "error", err)
	}
}

// recoverStaleAuditRuns closes runs left behind by a process restart. Counts
// are rebuilt from durable result rows so a recovered run never claims work
// that was only held in memory.
func (s *ModelCapabilityAuditService) recoverStaleAuditRuns(ctx context.Context, trigger string) error {
	if s == nil || s.db == nil {
		return errors.New("model capability audit service unavailable")
	}
	staleAfter := modelCapabilityAuditStaleAfter
	if trigger == modelBehaviorAuditTrigger {
		staleAfter = modelBehaviorAuditStaleAfter
	}
	if trigger != "manual" && trigger != modelBehaviorAuditTrigger {
		return errors.New("unsupported model audit trigger")
	}

	var query string
	if trigger == modelBehaviorAuditTrigger {
		query = `
			WITH stale AS (
				SELECT id
				FROM model_capability_probe_runs
				WHERE "trigger"=$1 AND status='running'
				  AND heartbeat_at < NOW() - ($2 * INTERVAL '1 second')
			), totals AS (
				SELECT s.id,
				       COUNT(r.id)::integer AS completed_count,
				       COALESCE(SUM(r.attempt_count), 0)::integer AS completed_items,
				       COUNT(r.id) FILTER (WHERE r.comparison_status='matched')::integer AS verified_count,
				       COUNT(r.id) FILTER (WHERE r.comparison_status='diverged')::integer AS mismatch_count,
				       COUNT(r.id) FILTER (WHERE r.comparison_status='inconclusive')::integer AS unsupported_count,
				       COUNT(r.id) FILTER (WHERE r.comparison_status='error')::integer AS result_error_count
				FROM stale s
				LEFT JOIN model_behavior_audit_results r ON r.run_id=s.id
				GROUP BY s.id
			)
			UPDATE model_capability_probe_runs AS runs
			SET status=CASE WHEN totals.completed_count > 0 THEN 'partial' ELSE 'failed' END,
			    completed_count=totals.completed_count,
			    completed_items=totals.completed_items,
			    verified_count=totals.verified_count,
			    mismatch_count=totals.mismatch_count,
			    unsupported_count=totals.unsupported_count,
			    error_count=totals.result_error_count + CASE WHEN totals.completed_count=0 THEN 1 ELSE 0 END,
			    error_message='paired behavior audit interrupted; recovered from persisted results',
			    error_summary='paired behavior audit interrupted; recovered from persisted results',
			    heartbeat_at=NOW(), finished_at=NOW()
			FROM totals
			WHERE runs.id=totals.id AND runs.status='running'
			  AND runs.heartbeat_at < NOW() - ($2 * INTERVAL '1 second')`
	} else {
		query = `
			WITH stale AS (
				SELECT id, GREATEST(1, COALESCE(CEIL(total_items::numeric / NULLIF(total_count, 0))::integer, 1)) AS expected_items
				FROM model_capability_probe_runs
				WHERE "trigger"=$1 AND status='running'
				  AND heartbeat_at < NOW() - ($2 * INTERVAL '1 second')
			), latest_results AS (
				SELECT DISTINCT ON (r.run_id, r.account_id, r.requested_model,
				                    COALESCE(NULLIF(r.mapped_model, ''), r.requested_model), r.protocol)
				       r.*
				FROM model_capability_probe_results r
				JOIN stale s ON s.id=r.run_id
				ORDER BY r.run_id, r.account_id, r.requested_model,
				         COALESCE(NULLIF(r.mapped_model, ''), r.requested_model), r.protocol,
				         r.checked_at DESC, r.id DESC
			), per_target AS (
				SELECT r.run_id, r.account_id, r.requested_model,
				       COALESCE(NULLIF(r.mapped_model, ''), r.requested_model) AS mapped_model,
				       COUNT(DISTINCT r.protocol)::integer AS result_count
				FROM latest_results r
				GROUP BY r.run_id, r.account_id, r.requested_model,
				         COALESCE(NULLIF(r.mapped_model, ''), r.requested_model)
			), totals AS (
				SELECT s.id,
				       COUNT(r.id)::integer AS completed_items,
				       (SELECT COUNT(*)::integer FROM per_target target
				        WHERE target.run_id=s.id AND target.result_count >= s.expected_items) AS completed_count,
				       COUNT(r.id) FILTER (WHERE r.status IN ('verified','model_mismatch'))::integer AS verified_count,
				       COUNT(r.id) FILTER (WHERE r.status='model_mismatch' OR r.identity_status='mismatch')::integer AS mismatch_count,
				       COUNT(r.id) FILTER (WHERE r.status='unsupported')::integer AS unsupported_count,
				       COUNT(r.id) FILTER (WHERE r.status IN ('error','timeout','needs_retest'))::integer AS result_error_count
				FROM stale s
				LEFT JOIN latest_results r ON r.run_id=s.id
				GROUP BY s.id, s.expected_items
			)
			UPDATE model_capability_probe_runs AS runs
			SET status=CASE WHEN totals.completed_items > 0 THEN 'partial' ELSE 'failed' END,
			    completed_count=totals.completed_count,
			    completed_items=totals.completed_items,
			    verified_count=totals.verified_count,
			    mismatch_count=totals.mismatch_count,
			    unsupported_count=totals.unsupported_count,
			    error_count=totals.result_error_count + CASE WHEN totals.completed_items=0 THEN 1 ELSE 0 END,
			    error_message='capability audit interrupted; recovered from persisted results',
			    error_summary='capability audit interrupted; recovered from persisted results',
			    heartbeat_at=NOW(), finished_at=NOW()
			FROM totals
			WHERE runs.id=totals.id AND runs.status='running'
			  AND runs.heartbeat_at < NOW() - ($2 * INTERVAL '1 second')`
	}
	_, err := s.db.ExecContext(ctx, query, trigger, int64(staleAfter/time.Second))
	return err
}

func (s *ModelCapabilityAuditService) executeRun(ctx context.Context, run *ModelCapabilityAuditRun, targets []modelCapabilityAuditTarget) (*ModelCapabilityAuditRun, error) {
	if len(targets) == 0 {
		now := time.Now()
		run.Status = "completed"
		run.Trigger = "manual"
		run.FinishedAt = &now
		persistCtx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
		_, updateErr := s.db.ExecContext(persistCtx, `UPDATE model_capability_probe_runs SET status='completed', heartbeat_at=NOW(), finished_at=$2, completed_items=total_items, completed_count=total_count WHERE id=$1 AND status='running'`, run.ID, now)
		cancel()
		if updateErr != nil {
			return nil, updateErr
		}
		return run, nil
	}

	runCtx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	type outcome struct {
		target  modelCapabilityAuditTarget
		results []ModelCapabilityProbeResult
		err     error
		ack     chan bool
	}
	jobs := make(chan modelCapabilityAuditTarget)
	outcomes := make(chan outcome, len(targets))
	workerCount := modelCapabilityAuditWorkerCount
	if len(targets) < workerCount {
		workerCount = len(targets)
	}
	var workers sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for target := range jobs {
				ack := make(chan bool, 1)
				if runCtx.Err() != nil {
					outcomes <- outcome{target: target, err: runCtx.Err(), ack: ack}
					return
				}
				results, err := s.probeAuditTarget(runCtx, target)
				outcomes <- outcome{target: target, results: results, err: err, ack: ack}
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
			select {
			case jobs <- target:
			case <-runCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(outcomes)
	}()

	completedTargets, completedItems, verified, mismatch, unsupported, errorsCount := 0, 0, 0, 0, 0, 0
	persistenceFailed := false
	for item := range outcomes {
		checkedAt := time.Now()
		results := item.results
		if len(results) == 0 {
			message := "probe failed"
			if item.err != nil {
				message = item.err.Error()
			}
			for _, protocol := range item.target.Protocols {
				results = append(results, ModelCapabilityProbeResult{
					Protocol: protocol, Status: "error", Capability: "unknown",
					RequestedModel: item.target.ModelID, Error: message, Evidence: "probe_error",
				})
			}
		}
		persistedForTarget := 0
		for _, result := range results {
			persistCtx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
			err := s.insertResult(persistCtx, run.ID, item.target, result, checkedAt, "active")
			cancel()
			if err != nil {
				errorsCount++
				persistenceFailed = true
				stopRun()
			} else {
				persistedForTarget++
				if result.Status == "verified" || result.Status == "model_mismatch" {
					verified++
				}
				mappedModel := capabilityAuditResultMappedModel(item.target, result)
				observedModel := strings.TrimSpace(result.UpstreamModel)
				responseModelMatches, responseModelObserved := capabilityProbeResponseModelMatch(result, mappedModel, observedModel)
				responseModelMismatch := responseModelObserved && !responseModelMatches
				if result.Status == "model_mismatch" || responseModelMismatch {
					mismatch++
				}
				if result.Status == "unsupported" {
					unsupported++
				}
				if result.Status == "error" || result.Status == "timeout" || result.Status == "needs_retest" {
					errorsCount++
				}
			}
		}
		completedItems += persistedForTarget
		if persistedForTarget == len(results) {
			completedTargets++
		}
		if err := s.updateCapabilityRunProgress(run.ID, completedTargets, completedItems, verified, mismatch, unsupported, errorsCount); err != nil {
			slog.Warn("model_capability_audit.progress_update_failed", "run_id", run.ID, "error", err)
			persistenceFailed = true
			errorsCount++
			stopRun()
		}
		item.ack <- !persistenceFailed && runCtx.Err() == nil
	}
	status := "completed"
	errorMessage := ""
	if persistenceFailed {
		errorMessage = "one or more probe results could not be persisted"
	}
	if ctx.Err() != nil || completedTargets < run.TotalCount || completedItems < run.TotalItems || persistenceFailed {
		status = "partial"
	}
	finished := time.Now()
	persistCtx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
	_, updateErr := s.db.ExecContext(persistCtx, `
		UPDATE model_capability_probe_runs
		SET status=$2, completed_count=$3, completed_items=$4, verified_count=$5, mismatch_count=$6,
		    unsupported_count=$7, error_count=$8, heartbeat_at=NOW(), finished_at=$9,
		    error_message=NULLIF($10,''), error_summary=NULLIF($10,'')
		WHERE id=$1 AND status='running'`, run.ID, status, completedTargets, completedItems, verified, mismatch, unsupported, errorsCount, finished, errorMessage)
	cancel()
	if updateErr != nil {
		return nil, updateErr
	}
	run.Status, run.CompletedCount, run.CompletedItems = status, completedTargets, completedItems
	run.VerifiedCount, run.MismatchCount = verified, mismatch
	run.UnsupportedCount, run.ErrorCount, run.FinishedAt = unsupported, errorsCount, &finished
	run.ErrorMessage, run.ErrorSummary = errorMessage, errorMessage
	run.Trigger = "manual"
	return run, nil
}

func (s *ModelCapabilityAuditService) updateCapabilityRunProgress(runID int64, completedTargets, completedItems, verified, mismatch, unsupported, errorCount int) error {
	ctx, cancel := context.WithTimeout(context.Background(), modelCapabilityAuditPersistenceTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
		UPDATE model_capability_probe_runs
		SET completed_count=$2, completed_items=$3, verified_count=$4, mismatch_count=$5,
		    unsupported_count=$6, error_count=$7, heartbeat_at=NOW()
		WHERE id=$1 AND status='running'`, runID, completedTargets, completedItems, verified, mismatch, unsupported, errorCount)
	return err
}

func (s *ModelCapabilityAuditService) probeAuditTarget(ctx context.Context, target modelCapabilityAuditTarget) (results []ModelCapabilityProbeResult, err error) {
	// This boundary runs inside a worker goroutine, so it must recover locally;
	// a recover in the parent background goroutine cannot catch worker panics.
	accountID := int64(0)
	if target.Account != nil {
		accountID = target.Account.ID
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("model_capability_audit.target_panic", "account_id", accountID, "model", target.ModelID)
			results = nil
			err = errors.New("active capability probe panicked")
		}
	}()

	probeCtx, cancel := context.WithTimeout(ctx, modelCapabilityAuditTargetTimeout)
	defer cancel()
	if s.accountTestService == nil {
		return nil, errors.New("active probe service unavailable")
	}
	if target.Account == nil {
		return nil, errors.New("audit target account unavailable")
	}
	if s.concurrencyService != nil && s.concurrencyService.cache != nil {
		release, acquireErr := s.acquireAuditAccountSlot(probeCtx, accountID, target.Account.Concurrency)
		if acquireErr != nil {
			return nil, acquireErr
		}
		if release != nil {
			defer release()
		}
	}
	response, probeErr := s.accountTestService.ProbeModelCapabilities(probeCtx, accountID, ModelCapabilityProbeInput{
		ModelID:   target.ModelID,
		Protocols: target.Protocols,
	})
	if probeErr != nil {
		return nil, probeErr
	}
	if response == nil {
		return nil, errors.New("active probe returned no response")
	}
	return response.Results, nil
}

func (s *ModelCapabilityAuditService) acquireAuditAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (func(), error) {
	backoff := modelCapabilityAuditSlotRetryInitial
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		acquired, err := s.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
		if err != nil {
			return nil, err
		}
		if acquired != nil && acquired.Acquired {
			return acquired.ReleaseFunc, nil
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
		if backoff < modelCapabilityAuditSlotRetryMax {
			backoff *= 2
			if backoff > modelCapabilityAuditSlotRetryMax {
				backoff = modelCapabilityAuditSlotRetryMax
			}
		}
	}
}

func (s *ModelCapabilityAuditService) ListRuns(ctx context.Context, limit int) ([]ModelCapabilityAuditRun, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("model capability audit service unavailable")
	}
	if err := s.recoverStaleAuditRuns(ctx, "manual"); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, requested_by, total_count, total_items, completed_count, completed_items, verified_count,
		       mismatch_count, unsupported_count, error_count, options, error_message,
		       started_at, finished_at, created_at
		FROM model_capability_probe_runs
		WHERE "trigger" = 'manual'
		ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ModelCapabilityAuditRun, 0)
	for rows.Next() {
		run, err := scanCapabilityAuditRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *run)
	}
	return result, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanCapabilityAuditRun(scanner rowScanner) (*ModelCapabilityAuditRun, error) {
	var run ModelCapabilityAuditRun
	var requested sql.NullInt64
	var finished sql.NullTime
	var options []byte
	var errorMessage sql.NullString
	if err := scanner.Scan(&run.ID, &run.Status, &requested, &run.TotalCount, &run.TotalItems, &run.CompletedCount, &run.CompletedItems,
		&run.VerifiedCount, &run.MismatchCount, &run.UnsupportedCount, &run.ErrorCount,
		&options, &errorMessage, &run.StartedAt, &finished, &run.CreatedAt); err != nil {
		return nil, err
	}
	if requested.Valid {
		run.RequestedBy = &requested.Int64
	}
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
		run.ErrorSummary = errorMessage.String
	}
	if len(options) > 0 {
		var decoded any
		if json.Unmarshal(options, &decoded) == nil {
			run.Options = decoded
			run.RequestedScope = decoded
		}
	}
	run.Trigger = "manual"
	if run.TotalItems == 0 && run.TotalCount > 0 {
		run.TotalItems = run.TotalCount
	}
	if run.CompletedItems == 0 && run.CompletedCount > 0 {
		run.CompletedItems = run.CompletedCount
	}
	run.CreatedBy = run.RequestedBy
	return &run, nil
}

func (s *ModelCapabilityAuditService) GetRun(ctx context.Context, id int64) (*ModelCapabilityAuditRun, []ModelCapabilityAuditEvidence, error) {
	if id <= 0 || s == nil || s.db == nil {
		return nil, nil, errors.New("invalid model capability audit run")
	}
	if err := s.recoverStaleAuditRuns(ctx, "manual"); err != nil {
		return nil, nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, status, requested_by, total_count, total_items, completed_count, completed_items, verified_count,
		       mismatch_count, unsupported_count, error_count, options, error_message,
		       started_at, finished_at, created_at
		FROM model_capability_probe_runs WHERE id=$1 AND "trigger"='manual'`, id)
	run, err := scanCapabilityAuditRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, infraerrors.NotFound("MODEL_CAPABILITY_AUDIT_RUN_NOT_FOUND", "model capability audit run not found")
		}
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, account_id, account_name, platform, group_id, requested_model, mapped_model,
		       response_model, upstream_model, protocol, status, capability, capability_status,
		       identity_status, confidence, evidence_source, http_status, latency_ms,
		       attempt_count, success_count, failure_count,
		       tool_call_found, tool_arguments_valid, error_message, evidence,
		       evidence_code, checked_at
		FROM model_capability_probe_results WHERE run_id=$1 ORDER BY checked_at ASC, id ASC`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	evidence := make([]ModelCapabilityAuditEvidence, 0)
	for rows.Next() {
		var item ModelCapabilityAuditEvidence
		var upstream, mapped, responseModel, capability, errorMessage, capabilityStatus, identityStatus, confidence, evidenceSource, evidenceCode sql.NullString
		var evidenceText sql.NullString
		var groupID sql.NullInt64
		var httpStatus, latency sql.NullInt64
		if err := rows.Scan(&item.ID, &item.RunID, &item.AccountID, &item.AccountName, &item.Platform, &groupID, &item.RequestedModel,
			&mapped, &responseModel, &upstream, &item.Protocol, &item.Status, &capability, &capabilityStatus,
			&identityStatus, &confidence, &evidenceSource, &httpStatus, &latency,
			&item.AttemptCount, &item.SuccessCount, &item.FailureCount,
			&item.ToolCallFound, &item.ToolArgumentsValid, &errorMessage, &evidenceText,
			&evidenceCode, &item.CheckedAt); err != nil {
			return nil, nil, err
		}
		if upstream.Valid {
			item.UpstreamModel = upstream.String
		}
		if groupID.Valid {
			id := groupID.Int64
			item.GroupID = &id
		}
		if mapped.Valid {
			item.MappedModel = mapped.String
		}
		if responseModel.Valid {
			item.ResponseModel = responseModel.String
			item.UpstreamModel = responseModel.String
		}
		if capability.Valid {
			item.Capability = capability.String
		}
		if capabilityStatus.Valid {
			item.CapabilityStatus = capabilityStatus.String
		}
		if identityStatus.Valid {
			item.IdentityStatus = identityStatus.String
		}
		if confidence.Valid {
			item.Confidence = confidence.String
		}
		if evidenceSource.Valid {
			item.EvidenceSource = evidenceSource.String
		}
		if evidenceCode.Valid {
			item.EvidenceCode = evidenceCode.String
		}
		if httpStatus.Valid {
			item.HTTPStatus = int(httpStatus.Int64)
		}
		if latency.Valid {
			item.LatencyMS = latency.Int64
		}
		if errorMessage.Valid {
			item.Error = errorMessage.String
		}
		if evidenceText.Valid {
			item.Evidence = evidenceText.String
		}
		evidence = append(evidence, item)
	}
	return run, evidence, rows.Err()
}

func (s *ModelCapabilityAuditService) passiveRows(ctx context.Context, days, limit int) ([]modelCapabilityPassive, modelCapabilityPassiveTotals, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH active_keys AS (
			SELECT DISTINCT r.account_id,
			       LOWER(BTRIM(r.requested_model)) AS requested_key,
			       LOWER(BTRIM(COALESCE(NULLIF(BTRIM(r.mapped_model), ''), r.requested_model))) AS mapped_key
			FROM model_capability_probe_results r
			JOIN model_capability_probe_runs runs ON runs.id=r.run_id AND runs."trigger"='manual'
			WHERE r.checked_at >= NOW() - ($1 * INTERVAL '1 day')
			  AND r.evidence_source <> 'paired_behavior' AND r.source <> 'paired_behavior'
		), passive_groups AS (
			SELECT source.account_id, MIN(source.account_name) AS account_name, MIN(source.platform) AS platform,
			       source.requested_key, source.mapped_key,
			       MIN(source.requested_model) AS requested_model,
			       MIN(source.mapped_model) AS mapped_model,
			       COUNT(*)::bigint AS requests,
			       COUNT(*) FILTER (WHERE COALESCE(source.upstream_model_mismatch, FALSE))::bigint AS mismatches,
			       MAX(source.created_at) AS last_seen,
			       BOOL_OR(BTRIM(source.response_model) <> '' AND source.upstream_model_mismatch IS NULL) AS needs_model_match,
			       BOOL_OR(COALESCE(source.upstream_model_mismatch, FALSE)) AS has_mismatch,
			       BOOL_OR(BTRIM(source.response_model) <> '' AND source.upstream_model_mismatch IS FALSE) AS has_matching_response
			FROM (
				SELECT normalized.*,
				       LOWER(BTRIM(normalized.requested_model)) AS requested_key,
				       LOWER(BTRIM(COALESCE(NULLIF(BTRIM(normalized.mapped_model), ''), normalized.requested_model))) AS mapped_key
				FROM (
					SELECT u.account_id, a.name AS account_name, a.platform,
					       COALESCE(NULLIF(u.requested_model, ''), NULLIF(u.model, ''), '') AS requested_model,
					       COALESCE(NULLIF(u.upstream_model, ''), '') AS mapped_model,
					       COALESCE(NULLIF(u.upstream_response_model, ''), '') AS response_model,
					       u.upstream_model_mismatch, u.created_at
					FROM usage_logs u
					JOIN accounts a ON a.id=u.account_id AND a.deleted_at IS NULL
					WHERE u.created_at >= NOW() - ($1 * INTERVAL '1 day')
				) normalized
			) source
			GROUP BY source.account_id, source.requested_key, source.mapped_key
		), selected_keys AS (
			SELECT p.account_id, p.requested_key, p.mapped_key
			FROM passive_groups p
			ORDER BY CASE WHEN EXISTS (
				SELECT 1 FROM active_keys a
				WHERE a.account_id=p.account_id AND a.requested_key=p.requested_key AND a.mapped_key=p.mapped_key
			) THEN 0 ELSE 1 END,
			p.last_seen DESC, p.account_id, p.requested_key, p.mapped_key
			LIMIT $2
		), selected_groups AS (
			SELECT p.*
			FROM passive_groups p
			JOIN selected_keys selected USING (account_id, requested_key, mapped_key)
		), detailed AS (
			SELECT p.account_id, p.account_name, p.platform, p.requested_model, p.mapped_model,
			       COALESCE(response_models.response_model_list, '') AS response_models,
			       p.requests, p.mismatches, p.last_seen, p.needs_model_match
			FROM selected_groups p
			LEFT JOIN LATERAL (
				SELECT STRING_AGG(bounded.response_model, ',' ORDER BY bounded.response_model) AS response_model_list
				FROM (
					SELECT DISTINCT NULLIF(u.upstream_response_model, '') AS response_model
					FROM usage_logs u
					WHERE u.account_id=p.account_id
					  AND u.created_at >= NOW() - ($1 * INTERVAL '1 day')
					  AND LOWER(BTRIM(COALESCE(NULLIF(u.requested_model, ''), NULLIF(u.model, ''), '')))=p.requested_key
					  AND LOWER(BTRIM(COALESCE(
					        NULLIF(BTRIM(COALESCE(NULLIF(u.upstream_model, ''), '')), ''),
					        COALESCE(NULLIF(u.requested_model, ''), NULLIF(u.model, ''), '')
					      )))=p.mapped_key
					ORDER BY NULLIF(u.upstream_response_model, '')
					LIMIT $3
				) bounded
			) response_models ON TRUE
		), combined_keys AS (
			SELECT account_id, requested_key, mapped_key FROM passive_groups
			UNION
			SELECT account_id, requested_key, mapped_key FROM active_keys
		), passive_only_reliable AS (
			SELECT p.*
			FROM passive_groups p
			WHERE NOT p.needs_model_match
			  AND NOT EXISTS (
				SELECT 1 FROM active_keys a
				WHERE a.account_id=p.account_id AND a.requested_key=p.requested_key AND a.mapped_key=p.mapped_key
			  )
		), passive_only_legacy_unselected AS (
			SELECT p.*
			FROM passive_groups p
			WHERE p.needs_model_match
			  AND NOT EXISTS (
				SELECT 1 FROM active_keys a
				WHERE a.account_id=p.account_id AND a.requested_key=p.requested_key AND a.mapped_key=p.mapped_key
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM selected_keys selected
				WHERE selected.account_id=p.account_id AND selected.requested_key=p.requested_key AND selected.mapped_key=p.mapped_key
			  )
		), totals AS (
			SELECT (SELECT COUNT(*)::bigint FROM combined_keys) AS total_models,
			       (SELECT COUNT(DISTINCT account_id)::bigint FROM combined_keys) AS total_accounts,
			       (SELECT COALESCE(SUM(requests), 0)::bigint FROM passive_groups) AS passive_requests,
			       (SELECT COUNT(*)::bigint FROM passive_only_reliable WHERE has_mismatch) AS passive_only_mismatch_models,
			       (SELECT COUNT(*)::bigint FROM passive_only_reliable WHERE NOT has_mismatch AND has_matching_response) AS passive_only_self_report_models,
			       ((SELECT COUNT(*)::bigint FROM passive_only_reliable WHERE NOT has_mismatch AND NOT has_matching_response) +
			        (SELECT COUNT(*)::bigint FROM passive_only_legacy_unselected)) AS passive_only_unverified_models
		)
		SELECT detailed.account_id, detailed.account_name, detailed.platform, detailed.requested_model,
		       detailed.mapped_model, detailed.response_models, detailed.requests, detailed.mismatches,
		       detailed.last_seen, detailed.needs_model_match,
		       totals.total_models, totals.total_accounts, totals.passive_requests,
		       totals.passive_only_mismatch_models, totals.passive_only_self_report_models,
		       totals.passive_only_unverified_models
		FROM totals
		LEFT JOIN detailed ON TRUE
		ORDER BY detailed.last_seen DESC NULLS LAST, detailed.account_id,
		         detailed.requested_model, detailed.mapped_model`, days, limit, modelCapabilityAuditPassiveModelLimit)
	if err != nil {
		return nil, modelCapabilityPassiveTotals{}, err
	}
	defer rows.Close()
	result := make([]modelCapabilityPassive, 0)
	var totals modelCapabilityPassiveTotals
	for rows.Next() {
		var accountID, requests, mismatches sql.NullInt64
		var accountName, platform, requestedModel, mappedModel, responseModels sql.NullString
		var lastSeen sql.NullTime
		var needsModelMatch sql.NullBool
		var totalModels, totalAccounts, passiveRequests int64
		var passiveOnlyMismatch, passiveOnlySelfReported, passiveOnlyUnverified int64
		if err := rows.Scan(&accountID, &accountName, &platform, &requestedModel, &mappedModel,
			&responseModels, &requests, &mismatches, &lastSeen, &needsModelMatch,
			&totalModels, &totalAccounts, &passiveRequests,
			&passiveOnlyMismatch, &passiveOnlySelfReported, &passiveOnlyUnverified); err != nil {
			return nil, modelCapabilityPassiveTotals{}, err
		}
		totals = modelCapabilityPassiveTotals{
			TotalModels: int(totalModels), TotalAccounts: int(totalAccounts), PassiveRequests: passiveRequests,
			PassiveOnlyMismatchModels: int(passiveOnlyMismatch), PassiveOnlySelfReportedModels: int(passiveOnlySelfReported),
			PassiveOnlyUnverifiedModels: int(passiveOnlyUnverified),
		}
		if !accountID.Valid {
			continue
		}
		item := modelCapabilityPassive{
			AccountID: accountID.Int64, AccountName: accountName.String, Platform: platform.String,
			RequestedModel: requestedModel.String, MappedModel: mappedModel.String, ResponseModelList: responseModels.String,
			Requests: requests.Int64, Mismatches: mismatches.Int64, LastSeen: lastSeen.Time,
			NeedsModelMatch: needsModelMatch.Bool,
		}
		if len(result) < limit {
			result = append(result, item)
		}
	}
	return result, totals, rows.Err()
}

func (s *ModelCapabilityAuditService) groupNames(ctx context.Context) (map[int64]struct {
	id   int64
	name string
}, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ag.account_id, g.id, g.name
		FROM account_groups ag JOIN groups g ON g.id=ag.group_id
		ORDER BY ag.account_id, ag.priority, g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]struct {
		id   int64
		name string
	})
	for rows.Next() {
		var accountID, groupID int64
		var name string
		if err := rows.Scan(&accountID, &groupID, &name); err != nil {
			return nil, err
		}
		if _, exists := result[accountID]; !exists {
			result[accountID] = struct {
				id   int64
				name string
			}{groupID, name}
		}
	}
	return result, rows.Err()
}

func (s *ModelCapabilityAuditService) activeRows(ctx context.Context, days int) ([]ModelCapabilityAuditRow, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH ranked AS (
			SELECT results.*,
			       SUM(attempt_count) OVER audit_window AS cumulative_attempt_count,
			       SUM(success_count) OVER audit_window AS cumulative_success_count,
			       SUM(failure_count) OVER audit_window AS cumulative_failure_count,
			       ROW_NUMBER() OVER (audit_window ORDER BY checked_at DESC, results.id DESC) AS row_rank
			FROM model_capability_probe_results AS results
			JOIN model_capability_probe_runs AS runs ON runs.id=results.run_id AND runs."trigger"='manual'
			WHERE checked_at >= NOW() - ($1 * INTERVAL '1 day')
			  AND evidence_source <> 'paired_behavior' AND source <> 'paired_behavior'
			WINDOW audit_window AS (
				PARTITION BY account_id, requested_model,
				             COALESCE(NULLIF(mapped_model, ''), requested_model), protocol
			)
		)
		SELECT id, run_id, account_id, account_name, platform, group_id, requested_model, mapped_model,
		       COALESCE(NULLIF(response_model, ''), upstream_model), protocol, status, capability, capability_status, identity_status,
		       confidence, evidence_source, http_status, latency_ms, tool_call_found,
		       tool_arguments_valid, error_message, evidence, evidence_code,
		       cumulative_attempt_count, cumulative_success_count, cumulative_failure_count, checked_at
		FROM ranked WHERE row_rank=1
		ORDER BY account_id, requested_model, COALESCE(NULLIF(mapped_model, ''), requested_model), protocol`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ModelCapabilityAuditRow, 0)
	byKey := make(map[string]int)
	for rows.Next() {
		var evidenceID, runID, accountID int64
		var accountName, platform, requestedModel, protocol, status string
		var evidence sql.NullString
		var capability sql.NullString
		var mappedModel, upstreamModel, capabilityStatus, identityStatus, confidence, evidenceSource, evidenceCode sql.NullString
		var groupID sql.NullInt64
		var httpStatus, latency, attemptCount, successCount, failureCount sql.NullInt64
		var toolFound, toolValid bool
		var errorMessage sql.NullString
		var checkedAt time.Time
		if err := rows.Scan(&evidenceID, &runID, &accountID, &accountName, &platform, &groupID, &requestedModel, &mappedModel,
			&upstreamModel, &protocol, &status, &capability, &capabilityStatus, &identityStatus,
			&confidence, &evidenceSource, &httpStatus, &latency, &toolFound, &toolValid,
			&errorMessage, &evidence, &evidenceCode, &attemptCount, &successCount, &failureCount, &checkedAt); err != nil {
			return nil, err
		}
		key := auditRowKey(accountID, requestedModel, mappedModel.String)
		idx, exists := byKey[key]
		if !exists {
			row := ModelCapabilityAuditRow{Key: key, AccountID: accountID, AccountName: accountName, Platform: platform,
				RequestedModel: requestedModel, Protocols: make(map[string]ModelCapabilityAuditEvidence)}
			if mappedModel.Valid {
				row.MappedModel = mappedModel.String
			}
			if strings.TrimSpace(row.MappedModel) == "" {
				row.MappedModel = requestedModel
			}
			if groupID.Valid {
				row.GroupID = &groupID.Int64
			}
			result = append(result, row)
			idx = len(result) - 1
			byKey[key] = idx
		}
		item := ModelCapabilityAuditEvidence{ID: evidenceID, RunID: runID, AccountID: accountID,
			AccountName: accountName, Platform: platform, RequestedModel: requestedModel, Protocol: protocol, Status: status,
			HTTPStatus: int(httpStatus.Int64), LatencyMS: latency.Int64,
			ToolCallFound: toolFound, ToolArgumentsValid: toolValid, CheckedAt: checkedAt}
		if groupID.Valid {
			id := groupID.Int64
			item.GroupID = &id
		}
		if evidence.Valid {
			item.Evidence = evidence.String
		}
		if upstreamModel.Valid {
			item.UpstreamModel = upstreamModel.String
			item.ResponseModel = upstreamModel.String
		}
		if mappedModel.Valid {
			item.MappedModel = mappedModel.String
		}
		if strings.TrimSpace(item.MappedModel) == "" {
			item.MappedModel = requestedModel
		}
		if capability.Valid {
			item.Capability = capability.String
		}
		if capabilityStatus.Valid {
			item.CapabilityStatus = capabilityStatus.String
		}
		if identityStatus.Valid {
			item.IdentityStatus = identityStatus.String
		}
		if confidence.Valid {
			item.Confidence = confidence.String
		}
		if evidenceSource.Valid {
			item.EvidenceSource = evidenceSource.String
		}
		if evidenceCode.Valid {
			item.EvidenceCode = evidenceCode.String
		}
		if errorMessage.Valid {
			item.Error = errorMessage.String
		}
		item.AttemptCount = int(attemptCount.Int64)
		item.SuccessCount = int(successCount.Int64)
		item.FailureCount = int(failureCount.Int64)
		result[idx].Protocols[protocol] = item
		if result[idx].ActiveLastSeen == nil || checkedAt.After(*result[idx].ActiveLastSeen) {
			t := checkedAt
			result[idx].ActiveLastSeen = &t
		}
	}
	return result, rows.Err()
}

func auditRowKey(accountID int64, requestedModel, mappedModel string) string {
	if strings.TrimSpace(mappedModel) == "" {
		mappedModel = requestedModel
	}
	return strconv.FormatInt(accountID, 10) + ":" + strings.ToLower(strings.TrimSpace(requestedModel)) + ":" + strings.ToLower(strings.TrimSpace(mappedModel))
}

func splitResponseModels(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := uniqueNonEmpty(strings.Split(value, ","))
	if len(parts) > modelCapabilityAuditPassiveModelLimit {
		return parts[:modelCapabilityAuditPassiveModelLimit]
	}
	return parts
}

func (s *ModelCapabilityAuditService) Summary(ctx context.Context, days, limit int) (*ModelCapabilityAuditSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("model capability audit service unavailable")
	}
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	passive, passiveTotals, err := s.passiveRows(ctx, days, limit)
	if err != nil {
		return nil, err
	}
	active, err := s.activeRows(ctx, days)
	if err != nil {
		return nil, err
	}
	groups, err := s.groupNames(ctx)
	if err != nil {
		return nil, err
	}
	rowsByKey := make(map[string]int)
	rows := make([]ModelCapabilityAuditRow, 0, len(active)+len(passive))
	for _, item := range active {
		if group, ok := groups[item.AccountID]; ok {
			item.GroupID = &group.id
			item.GroupName = group.name
		}
		for protocol, evidence := range item.Protocols {
			evidence.Platform = item.Platform
			evidence.GroupID = item.GroupID
			evidence.GroupName = item.GroupName
			item.Protocols[protocol] = evidence
		}
		rowsByKey[item.Key] = len(rows)
		rows = append(rows, item)
	}
	for _, item := range passive {
		key := auditRowKey(item.AccountID, item.RequestedModel, item.MappedModel)
		idx, ok := rowsByKey[key]
		if !ok {
			row := ModelCapabilityAuditRow{Key: key, AccountID: item.AccountID, AccountName: item.AccountName,
				Platform: item.Platform, RequestedModel: item.RequestedModel,
				MappedModel: item.MappedModel, Protocols: make(map[string]ModelCapabilityAuditEvidence)}
			if group, exists := groups[item.AccountID]; exists {
				row.GroupID = &group.id
				row.GroupName = group.name
			}
			rows = append(rows, row)
			idx = len(rows) - 1
			rowsByKey[key] = idx
		}
		row := &rows[idx]
		if row.AccountName == "" {
			row.AccountName = item.AccountName
		}
		if row.Platform == "" {
			row.Platform = item.Platform
		}
		if row.MappedModel == "" {
			row.MappedModel = item.MappedModel
			if row.MappedModel == "" {
				row.MappedModel = item.RequestedModel
			}
		}
		row.PassiveRequests += item.Requests
		row.PassiveMismatches += item.Mismatches
		row.PassiveResponseModels = append(row.PassiveResponseModels, splitResponseModels(item.ResponseModelList)...)
		row.PassiveResponseModels = uniqueNonEmpty(row.PassiveResponseModels)
		row.passiveNeedsModelMatch = row.passiveNeedsModelMatch || item.NeedsModelMatch
		if row.PassiveLastSeen == nil || item.LastSeen.After(*row.PassiveLastSeen) {
			t := item.LastSeen
			row.PassiveLastSeen = &t
		}
	}
	for i := range rows {
		deriveAuditVerdict(&rows[i])
		if rows[i].PassiveLastSeen != nil {
			rows[i].LastSeen = rows[i].PassiveLastSeen
		}
		if rows[i].ActiveLastSeen != nil {
			rows[i].LastChecked = rows[i].ActiveLastSeen
		}
		rows[i].EvidenceCount = len(rows[i].Protocols)
		if rows[i].PassiveRequests > 0 {
			rows[i].EvidenceCount++
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i].PassiveLastSeen
		right := rows[j].PassiveLastSeen
		if left == nil && right == nil {
			return rows[i].AccountID < rows[j].AccountID
		}
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.After(*right)
	})
	activeChecks := int64(0)
	for _, row := range rows {
		for _, evidence := range row.Protocols {
			activeChecks += int64(evidence.AttemptCount)
		}
	}
	summary := &ModelCapabilityAuditSummary{
		Days: days, GeneratedAt: time.Now(), TotalModels: passiveTotals.TotalModels,
		TotalAccounts: passiveTotals.TotalAccounts, PassiveRequests: passiveTotals.PassiveRequests,
	}
	summary.Stats = ModelCapabilityAuditStats{
		Models: passiveTotals.TotalModels, Accounts: passiveTotals.TotalAccounts,
		PassiveRequests: passiveTotals.PassiveRequests, ActiveChecks: activeChecks,
	}
	for _, row := range rows {
		// Passive-only rows with complete mismatch metadata are counted by the
		// aggregate SQL, including those omitted by the detail limit. Active
		// rows and legacy/null-metadata rows need the full Go verdict instead.
		if len(row.Protocols) == 0 && !row.passiveNeedsModelMatch {
			continue
		}
		switch row.CapabilityStatus {
		case "verified":
			summary.CapabilityVerified++
			summary.Stats.Verified++
		}
		switch row.IdentityStatus {
		case "consistent":
			summary.IdentityConsistent++
		case "self_report_consistent":
			summary.SelfReportConsistent++
			summary.Stats.SelfReported++
		case "mismatch_suspected":
			summary.MismatchSuspected++
			summary.Stats.Suspect++
		default:
			summary.Unverified++
			summary.Stats.Unverified++
		}
	}
	summary.MismatchSuspected += passiveTotals.PassiveOnlyMismatchModels
	summary.Stats.Suspect += passiveTotals.PassiveOnlyMismatchModels
	summary.SelfReportConsistent += passiveTotals.PassiveOnlySelfReportedModels
	summary.Stats.SelfReported += passiveTotals.PassiveOnlySelfReportedModels
	summary.Unverified += passiveTotals.PassiveOnlyUnverifiedModels
	summary.Stats.Unverified += passiveTotals.PassiveOnlyUnverifiedModels
	if len(rows) > limit {
		summary.Rows = rows[:limit]
	} else {
		summary.Rows = rows
	}
	summary.ReturnedModels = len(summary.Rows)
	summary.RowsTruncated = summary.TotalModels > summary.ReturnedModels
	// Include a compact, redacted evidence tail for dashboards that do not
	// need to load a separate run detail endpoint.
	for _, row := range summary.Rows {
		for protocol, evidence := range row.Protocols {
			evidence.Protocol = protocol
			summary.RecentEvidence = append(summary.RecentEvidence, evidence)
			if len(summary.RecentEvidence) >= 50 {
				break
			}
		}
		if len(summary.RecentEvidence) >= 50 {
			break
		}
	}
	return summary, nil
}

func deriveAuditVerdict(row *ModelCapabilityAuditRow) {
	if row == nil {
		return
	}
	verified, unsupported, failed := 0, 0, 0
	identityMismatch := false
	activeSelfReportMatches := false
	for _, evidence := range row.Protocols {
		switch evidence.Status {
		case "verified":
			verified++
		case "unsupported":
			unsupported++
		case "model_mismatch":
			verified++
			identityMismatch = true
		default:
			failed++
		}
		// A successful capability response can still carry an identity
		// mismatch. Preserve that distinction when aggregating the row.
		if evidence.Status == "model_mismatch" || evidence.IdentityStatus == "mismatch" || evidence.IdentityStatus == "mismatch_suspected" {
			identityMismatch = true
		}
		if evidence.IdentityStatus == "consistent" || evidence.IdentityStatus == "self_report_consistent" {
			activeSelfReportMatches = true
		}
		if evidence.ResponseModel != "" {
			row.ObservedModel = evidence.ResponseModel
			if capabilityProbeMetadataIsRedacted(evidence.ResponseModel) {
				continue
			}
			if row.MappedModel != "" && upstreamModelsMatchForAudit(row.MappedModel, evidence.ResponseModel) {
				activeSelfReportMatches = true
			} else {
				identityMismatch = true
			}
		} else if evidence.UpstreamModel != "" {
			row.ObservedModel = evidence.UpstreamModel
			if capabilityProbeMetadataIsRedacted(evidence.UpstreamModel) {
				continue
			}
			if row.MappedModel != "" && upstreamModelsMatchForAudit(row.MappedModel, evidence.UpstreamModel) {
				activeSelfReportMatches = true
			} else {
				identityMismatch = true
			}
		}
	}
	row.PassiveResponseModels = uniqueNonEmpty(row.PassiveResponseModels)
	for _, model := range row.PassiveResponseModels {
		if capabilityProbeMetadataIsRedacted(model) {
			continue
		}
		if row.MappedModel != "" && !upstreamModelsMatchForAudit(row.MappedModel, model) {
			identityMismatch = true
			break
		}
	}
	switch {
	case verified > 0:
		row.CapabilityStatus = "verified"
	case unsupported > 0 && failed == 0:
		row.CapabilityStatus = "unsupported"
	default:
		row.CapabilityStatus = "unverified"
	}
	if verified > 0 {
		row.ActiveStatus = "verified"
	} else if unsupported > 0 && failed == 0 {
		row.ActiveStatus = "unsupported"
	} else if len(row.Protocols) > 0 {
		row.ActiveStatus = "error"
	} else {
		row.ActiveStatus = "not_checked"
	}
	noMismatch := row.PassiveMismatches == 0 && !identityMismatch
	selfReportMatches := activeSelfReportMatches
	for _, model := range row.PassiveResponseModels {
		if row.MappedModel != "" && upstreamModelsMatchForAudit(row.MappedModel, model) {
			selfReportMatches = true
			break
		}
	}
	if noMismatch && selfReportMatches {
		row.IdentityStatus = "self_report_consistent"
		row.Confidence = "medium"
	} else if row.PassiveMismatches > 0 || identityMismatch {
		row.IdentityStatus = "mismatch_suspected"
		row.Confidence = "high"
	} else if row.PassiveRequests > 0 || verified > 0 {
		row.IdentityStatus = "evidence_only"
		row.Confidence = "medium"
	} else {
		row.IdentityStatus = "unverified"
		row.Confidence = "low"
	}
}
