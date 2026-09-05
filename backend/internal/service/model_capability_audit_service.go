package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// ModelCapabilityAuditOptions controls one bounded, administrator-triggered
// audit run. The bounds are intentional: an audit must never turn into an
// unbounded upstream traffic generator.
type ModelCapabilityAuditOptions struct {
	AccountIDs []int64  `json:"account_ids,omitempty"`
	ModelIDs   []string `json:"model_ids,omitempty"`
	Protocols  []string `json:"protocols,omitempty"`
	ActiveOnly bool     `json:"active_only"`
	MaxItems   int      `json:"max_items,omitempty"`
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
	CheckedAt          time.Time `json:"checked_at"`
}

type ModelCapabilityAuditRow struct {
	Key                   string                                  `json:"key"`
	AccountID             int64                                   `json:"account_id"`
	AccountName           string                                  `json:"account_name"`
	Platform              string                                  `json:"platform"`
	GroupID               *int64                                  `json:"group_id,omitempty"`
	GroupName             string                                  `json:"group_name,omitempty"`
	RequestedModel        string                                  `json:"requested_model"`
	MappedModel           string                                  `json:"mapped_model,omitempty"`
	ObservedModel         string                                  `json:"observed_model,omitempty"`
	PassiveRequests       int64                                   `json:"passive_requests"`
	PassiveMismatches     int64                                   `json:"passive_mismatches"`
	PassiveResponseModels []string                                `json:"passive_response_models,omitempty"`
	PassiveLastSeen       *time.Time                              `json:"passive_last_seen,omitempty"`
	ActiveLastSeen        *time.Time                              `json:"active_last_seen,omitempty"`
	CapabilityStatus      string                                  `json:"capability_status"`
	ActiveStatus          string                                  `json:"active_status,omitempty"`
	IdentityStatus        string                                  `json:"identity_status"`
	Confidence            string                                  `json:"confidence"`
	LastSeen              *time.Time                              `json:"last_seen,omitempty"`
	LastChecked           *time.Time                              `json:"last_checked,omitempty"`
	EvidenceCount         int                                     `json:"evidence_count,omitempty"`
	Protocols             map[string]ModelCapabilityAuditEvidence `json:"protocols"`
}

type ModelCapabilityAuditSummary struct {
	Days               int                            `json:"days"`
	GeneratedAt        time.Time                      `json:"generated_at"`
	TotalModels        int                            `json:"total_models"`
	CapabilityVerified int                            `json:"capability_verified"`
	IdentityConsistent int                            `json:"identity_consistent"`
	MismatchSuspected  int                            `json:"mismatch_suspected"`
	Unverified         int                            `json:"unverified"`
	PassiveRequests    int64                          `json:"passive_requests"`
	TotalAccounts      int                            `json:"total_accounts,omitempty"`
	Stats              ModelCapabilityAuditStats      `json:"stats"`
	RecentEvidence     []ModelCapabilityAuditEvidence `json:"recent_evidence,omitempty"`
	Rows               []ModelCapabilityAuditRow      `json:"rows"`
}

type ModelCapabilityAuditStats struct {
	Models          int   `json:"models"`
	Accounts        int   `json:"accounts"`
	Verified        int   `json:"verified"`
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
}

// ModelCapabilityAuditService combines durable active probes with passive
// usage-log evidence. It deliberately lives beside AccountTestService so the
// existing single-account probe API remains backwards compatible.
type ModelCapabilityAuditService struct {
	db                 *sql.DB
	accountRepo        AccountRepository
	accountTestService *AccountTestService
}

func NewModelCapabilityAuditService(db *sql.DB, accountRepo AccountRepository, accountTestService *AccountTestService) *ModelCapabilityAuditService {
	return &ModelCapabilityAuditService{db: db, accountRepo: accountRepo, accountTestService: accountTestService}
}

func (s *ModelCapabilityAuditService) normalizeOptions(input ModelCapabilityAuditOptions) (ModelCapabilityAuditOptions, []string, error) {
	if s == nil || s.db == nil || s.accountRepo == nil {
		return input, nil, errors.New("model capability audit service unavailable")
	}
	if input.ActiveOnly && s.accountTestService == nil {
		return input, nil, errors.New("model capability active probe service unavailable")
	}
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
	status := ""
	if options.ActiveOnly {
		status = StatusActive
	}
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", status, "", 0, "")
	if err != nil {
		return nil, err
	}
	out := make([]*Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account != nil {
			out = append(out, account)
		}
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
		if model != "" && model != "*" && !strings.Contains(model, "*") {
			set[model] = struct{}{}
		}
	}
	for _, model := range account.GetModelMapping() {
		model = strings.TrimSpace(model)
		if model != "" && model != "*" && !strings.Contains(model, "*") {
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
			if rows.Scan(&model) == nil && strings.TrimSpace(model) != "" {
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

func (s *ModelCapabilityAuditService) createRun(ctx context.Context, options ModelCapabilityAuditOptions, requestedBy *int64, total int) (*ModelCapabilityAuditRun, error) {
	encoded, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	var run ModelCapabilityAuditRun
	var requested sql.NullInt64
	if requestedBy != nil {
		requested = sql.NullInt64{Int64: *requestedBy, Valid: true}
	}
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO model_capability_probe_runs
			(status, requested_by, total_count, total_items, options, requested_scope)
		VALUES ('running', $1, $2, $2, $3::jsonb, $3::jsonb)
		RETURNING id, status, requested_by, total_count, completed_count, verified_count,
		          mismatch_count, unsupported_count, error_count, options, error_message,
		          started_at, finished_at, created_at`, requested, total, string(encoded)).Scan(
		&run.ID, &run.Status, &requested, &run.TotalCount, &run.CompletedCount,
		&run.VerifiedCount, &run.MismatchCount, &run.UnsupportedCount, &run.ErrorCount,
		&run.Options, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	if err != nil {
		return nil, err
	}
	if requested.Valid {
		run.RequestedBy = &requested.Int64
		run.CreatedBy = &requested.Int64
	}
	run.Trigger = "manual"
	run.TotalItems = run.TotalCount
	run.CompletedItems = run.CompletedCount
	run.RequestedScope = run.Options
	return &run, nil
}

func (s *ModelCapabilityAuditService) insertResult(ctx context.Context, runID int64, target modelCapabilityAuditTarget, result ModelCapabilityProbeResult, checkedAt time.Time, source string) error {
	var groupID any
	if target.GroupID != nil {
		groupID = *target.GroupID
	}
	if strings.TrimSpace(source) == "" {
		source = "active"
	}
	identityStatus := "unobserved"
	mappedModel := strings.TrimSpace(target.Account.GetMappedModel(result.RequestedModel))
	if mappedModel == "" {
		mappedModel = strings.TrimSpace(result.RequestedModel)
	}
	observedModel := strings.TrimSpace(result.UpstreamModel)
	if observedModel != "" {
		if mappedModel != "" && strings.EqualFold(observedModel, mappedModel) {
			identityStatus = "consistent"
		} else {
			identityStatus = "mismatch"
		}
	}
	capabilityStatus := "unknown"
	switch result.Status {
	case "verified":
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
	} else if identityStatus == "consistent" && capabilityStatus == "verified" {
		confidence = "medium"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO model_capability_probe_results
		(run_id, account_id, group_id, account_name, platform, requested_model, mapped_model,
		 response_model, upstream_model, protocol, status, capability, capability_status, identity_status,
		 confidence, evidence_source, source, http_status, latency_ms, attempt_count,
		 success_count, failure_count, tool_name, tool_call_found, tool_arguments_valid,
		 error_message, evidence, evidence_code, response_summary, checked_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($8,''),$9,$10,$11,$12,$13,$14,$15,$16,
		 NULLIF($17,0),NULLIF($18,0),1,CASE WHEN $10='verified' THEN 1 ELSE 0 END,
		 CASE WHEN $10='verified' THEN 0 ELSE 1 END,NULLIF($19,''),$20,$21,
		 NULLIF($22,''),NULLIF($23,''),NULLIF($24,''),NULLIF($25,''),$26)`,
		runID, target.Account.ID, groupID, target.Account.Name, target.Account.Platform,
		result.RequestedModel, mappedModel, observedModel, result.Protocol, result.Status,
		result.Capability, capabilityStatus, identityStatus, confidence, source, source,
		result.HTTPStatus, result.LatencyMS, result.ToolName, result.ToolCallFound,
		result.ToolArgumentsValid, nullableText(result.Error), result.Evidence, result.Evidence,
		nullableText(result.Error), checkedAt)
	return err
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// Run executes a bounded batch and persists every protocol observation. It is
// synchronous by design: the caller receives a complete run ID and can
// immediately refresh the aggregate table. The per-target workers keep normal
// upstream traffic bounded.
func (s *ModelCapabilityAuditService) Run(ctx context.Context, input ModelCapabilityAuditOptions, requestedBy *int64) (*ModelCapabilityAuditRun, error) {
	options, protocols, err := s.normalizeOptions(input)
	if err != nil {
		return nil, err
	}
	targets, err := s.buildTargets(ctx, options, protocols)
	if err != nil {
		return nil, err
	}
	run, err := s.createRun(ctx, options, requestedBy, len(targets))
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		now := time.Now()
		run.Status = "completed"
		run.Trigger = "manual"
		run.TotalItems = run.TotalCount
		run.CompletedItems = run.CompletedCount
		run.FinishedAt = &now
		_, _ = s.db.ExecContext(context.Background(), `UPDATE model_capability_probe_runs SET status='completed', finished_at=$2, completed_items=total_items, completed_count=total_count WHERE id=$1`, run.ID, now)
		return run, nil
	}

	type outcome struct {
		target  modelCapabilityAuditTarget
		results []ModelCapabilityProbeResult
		err     error
	}
	jobs := make(chan modelCapabilityAuditTarget)
	outcomes := make(chan outcome, len(targets))
	workerCount := 4
	if len(targets) < workerCount {
		workerCount = len(targets)
	}
	var workers sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for target := range jobs {
				if ctx.Err() != nil {
					outcomes <- outcome{target: target, err: ctx.Err()}
					continue
				}
				probeCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
				if s.accountTestService == nil {
					cancel()
					outcomes <- outcome{target: target, err: errors.New("active probe service unavailable")}
					continue
				}
				response, probeErr := s.accountTestService.ProbeModelCapabilities(probeCtx, target.Account.ID, ModelCapabilityProbeInput{
					ModelID:   target.ModelID,
					Protocols: target.Protocols,
				})
				cancel()
				if probeErr != nil || response == nil {
					outcomes <- outcome{target: target, err: probeErr}
					continue
				}
				outcomes <- outcome{target: target, results: response.Results}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, target := range targets {
			jobs <- target
		}
	}()
	go func() {
		workers.Wait()
		close(outcomes)
	}()

	completed, verified, mismatch, unsupported, errorsCount := 0, 0, 0, 0, 0
	for item := range outcomes {
		checkedAt := time.Now()
		results := item.results
		if len(results) == 0 {
			errorsCount++
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
		} else {
			completed++
		}
		for _, result := range results {
			if result.Status == "verified" {
				verified++
			}
			if result.Status == "model_mismatch" {
				mismatch++
			}
			if result.Status == "unsupported" {
				unsupported++
			}
			if result.Status == "error" || result.Status == "timeout" {
				errorsCount++
			}
			if err := s.insertResult(context.Background(), run.ID, item.target, result, checkedAt, "active"); err != nil {
				errorsCount++
			}
		}
	}
	status := "completed"
	if ctx.Err() != nil {
		status = "partial"
	}
	finished := time.Now()
	_, updateErr := s.db.ExecContext(context.Background(), `
		UPDATE model_capability_probe_runs
		SET status=$2, completed_count=$3, completed_items=$3, verified_count=$4, mismatch_count=$5,
		    unsupported_count=$6, error_count=$7, finished_at=$8
		WHERE id=$1`, run.ID, status, completed, verified, mismatch, unsupported, errorsCount, finished)
	if updateErr != nil {
		return nil, updateErr
	}
	run.Status, run.CompletedCount, run.VerifiedCount, run.MismatchCount = status, completed, verified, mismatch
	run.UnsupportedCount, run.ErrorCount, run.FinishedAt = unsupported, errorsCount, &finished
	run.Trigger = "manual"
	run.TotalItems = run.TotalCount
	run.CompletedItems = run.CompletedCount
	return run, nil
}

func (s *ModelCapabilityAuditService) ListRuns(ctx context.Context, limit int) ([]ModelCapabilityAuditRun, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("model capability audit service unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, requested_by, total_count, completed_count, verified_count,
		       mismatch_count, unsupported_count, error_count, options, error_message,
		       started_at, finished_at, created_at
		FROM model_capability_probe_runs
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
	if err := scanner.Scan(&run.ID, &run.Status, &requested, &run.TotalCount, &run.CompletedCount,
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
	run.TotalItems = run.TotalCount
	run.CompletedItems = run.CompletedCount
	run.CreatedBy = run.RequestedBy
	return &run, nil
}

func (s *ModelCapabilityAuditService) GetRun(ctx context.Context, id int64) (*ModelCapabilityAuditRun, []ModelCapabilityAuditEvidence, error) {
	if id <= 0 || s == nil || s.db == nil {
		return nil, nil, errors.New("invalid model capability audit run")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, status, requested_by, total_count, completed_count, verified_count,
		       mismatch_count, unsupported_count, error_count, options, error_message,
		       started_at, finished_at, created_at
		FROM model_capability_probe_runs WHERE id=$1`, id)
	run, err := scanCapabilityAuditRun(row)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, account_id, account_name, platform, group_id, requested_model, mapped_model,
		       response_model, upstream_model, protocol, status, capability, capability_status,
		       identity_status, confidence, evidence_source, http_status, latency_ms,
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

func (s *ModelCapabilityAuditService) passiveRows(ctx context.Context, days, limit int) ([]modelCapabilityPassive, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.account_id, a.name, a.platform,
		       COALESCE(NULLIF(u.requested_model, ''), NULLIF(u.model, ''), ''),
		       COALESCE(NULLIF(u.upstream_model, ''), ''),
		       COALESCE(NULLIF(u.upstream_response_model, ''), ''),
		       COUNT(*)::bigint,
		       COUNT(*) FILTER (WHERE COALESCE(u.upstream_model_mismatch, FALSE))::bigint,
		       MAX(u.created_at)
		FROM usage_logs u
		JOIN accounts a ON a.id=u.account_id AND a.deleted_at IS NULL
		WHERE u.created_at >= NOW() - ($1 * INTERVAL '1 day')
		GROUP BY u.account_id, a.name, a.platform,
		         COALESCE(NULLIF(u.requested_model, ''), NULLIF(u.model, ''), ''),
		         COALESCE(NULLIF(u.upstream_model, ''), ''),
		         COALESCE(NULLIF(u.upstream_response_model, ''), '')
		ORDER BY MAX(u.created_at) DESC
		LIMIT $2`, days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]modelCapabilityPassive, 0)
	for rows.Next() {
		var item modelCapabilityPassive
		if err := rows.Scan(&item.AccountID, &item.AccountName, &item.Platform, &item.RequestedModel, &item.MappedModel,
			&item.ResponseModelList, &item.Requests, &item.Mismatches, &item.LastSeen); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
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
		SELECT DISTINCT ON (account_id, requested_model, protocol)
		       id, run_id, account_id, account_name, platform, group_id, requested_model, mapped_model,
		       response_model, protocol, status, capability, capability_status, identity_status,
		       confidence, evidence_source, http_status, latency_ms, tool_call_found,
		       tool_arguments_valid, error_message, evidence, evidence_code, checked_at
		FROM model_capability_probe_results
		WHERE checked_at >= NOW() - ($1 * INTERVAL '1 day')
		ORDER BY account_id, requested_model, protocol, checked_at DESC, id DESC`, days)
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
		var httpStatus, latency sql.NullInt64
		var toolFound, toolValid bool
		var errorMessage sql.NullString
		var checkedAt time.Time
		if err := rows.Scan(&evidenceID, &runID, &accountID, &accountName, &platform, &groupID, &requestedModel, &mappedModel,
			&upstreamModel, &protocol, &status, &capability, &capabilityStatus, &identityStatus,
			&confidence, &evidenceSource, &httpStatus, &latency, &toolFound, &toolValid,
			&errorMessage, &evidence, &evidenceCode, &checkedAt); err != nil {
			return nil, err
		}
		key := auditRowKey(accountID, requestedModel)
		idx, exists := byKey[key]
		if !exists {
			row := ModelCapabilityAuditRow{Key: key, AccountID: accountID, AccountName: accountName, Platform: platform,
				RequestedModel: requestedModel, Protocols: make(map[string]ModelCapabilityAuditEvidence)}
			if mappedModel.Valid {
				row.MappedModel = mappedModel.String
			}
			if groupID.Valid {
				row.GroupID = &groupID.Int64
			}
			result = append(result, row)
			idx = len(result) - 1
			byKey[key] = idx
		}
		item := ModelCapabilityAuditEvidence{ID: evidenceID, RunID: runID, AccountID: accountID,
			AccountName: accountName, RequestedModel: requestedModel, Protocol: protocol, Status: status,
			HTTPStatus: int(httpStatus.Int64), LatencyMS: latency.Int64,
			ToolCallFound: toolFound, ToolArgumentsValid: toolValid, CheckedAt: checkedAt}
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
		result[idx].Protocols[protocol] = item
		if result[idx].ActiveLastSeen == nil || checkedAt.After(*result[idx].ActiveLastSeen) {
			t := checkedAt
			result[idx].ActiveLastSeen = &t
		}
	}
	return result, rows.Err()
}

func auditRowKey(accountID int64, model string) string {
	return strconv.FormatInt(accountID, 10) + ":" + model
}

func splitResponseModels(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	return uniqueNonEmpty(parts)
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
	passive, err := s.passiveRows(ctx, days, limit*8)
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
		rowsByKey[item.Key] = len(rows)
		rows = append(rows, item)
	}
	for _, item := range passive {
		key := auditRowKey(item.AccountID, item.RequestedModel)
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
	if len(rows) > limit {
		rows = rows[:limit]
	}
	accounts := make(map[int64]struct{}, len(rows))
	activeChecks := int64(0)
	for _, row := range rows {
		accounts[row.AccountID] = struct{}{}
		activeChecks += int64(len(row.Protocols))
	}
	summary := &ModelCapabilityAuditSummary{Days: days, GeneratedAt: time.Now(), Rows: rows, TotalModels: len(rows), TotalAccounts: len(accounts)}
	summary.Stats = ModelCapabilityAuditStats{Models: len(rows), Accounts: len(accounts), PassiveRequests: summary.PassiveRequests, ActiveChecks: activeChecks}
	for _, row := range rows {
		summary.PassiveRequests += row.PassiveRequests
		switch row.CapabilityStatus {
		case "verified":
			summary.CapabilityVerified++
			summary.Stats.Verified++
		}
		switch row.IdentityStatus {
		case "consistent":
			summary.IdentityConsistent++
		case "mismatch_suspected":
			summary.MismatchSuspected++
			summary.Stats.Suspect++
		default:
			summary.Unverified++
			summary.Stats.Unverified++
		}
	}
	summary.Stats.PassiveRequests = summary.PassiveRequests
	// Include a compact, redacted evidence tail for dashboards that do not
	// need to load a separate run detail endpoint.
	for _, row := range rows {
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
	activeMismatch := false
	for _, evidence := range row.Protocols {
		switch evidence.Status {
		case "verified":
			verified++
		case "unsupported":
			unsupported++
		case "model_mismatch":
			verified++
			activeMismatch = true
		default:
			failed++
		}
		// A successful capability response can still carry an identity
		// mismatch. Preserve that distinction when aggregating the row.
		if evidence.Status == "model_mismatch" || evidence.IdentityStatus == "mismatch" || evidence.IdentityStatus == "mismatch_suspected" {
			activeMismatch = true
		}
		if evidence.ResponseModel != "" {
			row.ObservedModel = evidence.ResponseModel
			row.PassiveResponseModels = append(row.PassiveResponseModels, evidence.ResponseModel)
			if row.MappedModel == "" || !strings.EqualFold(evidence.ResponseModel, row.MappedModel) {
				activeMismatch = true
			}
		} else if evidence.UpstreamModel != "" {
			row.ObservedModel = evidence.UpstreamModel
			row.PassiveResponseModels = append(row.PassiveResponseModels, evidence.UpstreamModel)
			if row.MappedModel == "" || !strings.EqualFold(evidence.UpstreamModel, row.MappedModel) {
				activeMismatch = true
			}
		}
	}
	row.PassiveResponseModels = uniqueNonEmpty(row.PassiveResponseModels)
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
	consistent := row.PassiveMismatches == 0 && !activeMismatch
	responseMatches := false
	for _, model := range row.PassiveResponseModels {
		if row.MappedModel != "" && strings.EqualFold(model, row.MappedModel) {
			responseMatches = true
			break
		}
	}
	if consistent && responseMatches {
		row.IdentityStatus = "consistent"
		row.Confidence = "high"
	} else if row.PassiveMismatches > 0 || activeMismatch {
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
