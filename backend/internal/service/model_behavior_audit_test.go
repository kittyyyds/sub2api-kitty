package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type modelBehaviorAuditAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *modelBehaviorAuditAccountRepo) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return r.accounts, nil
}

func (r *modelBehaviorAuditAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	wanted := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	result := make([]*Account, 0, len(ids))
	for i := range r.accounts {
		if _, ok := wanted[r.accounts[i].ID]; ok {
			result = append(result, &r.accounts[i])
		}
	}
	return result, nil
}

func behaviorAuditTestAccount(id int64, name, baseURL string) Account {
	return Account{
		ID: id, Name: name, Status: StatusActive, Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": baseURL, "api_key": "test-only"},
	}
}

func TestEstimateBehaviorPairsThirdPartyWithOfficialAnchor(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	repo := &modelBehaviorAuditAccountRepo{accounts: []Account{
		behaviorAuditTestAccount(1, "official", "https://api.openai.com/v1"),
		behaviorAuditTestAccount(2, "relay", "https://relay.example/v1"),
	}}
	service := &ModelCapabilityAuditService{db: db, accountRepo: repo, accountTestService: &AccountTestService{}}
	estimate, err := service.EstimateBehavior(context.Background(), ModelBehaviorAuditOptions{
		ModelID: "gpt-5", CaseCount: 3, ReasoningEffort: "low",
	})
	if err != nil {
		t.Fatalf("estimate behavior audit: %v", err)
	}
	if !estimate.CanRun || estimate.AnchorCount != 1 || estimate.TargetCount != 1 || estimate.PairCount != 3 || estimate.RequestCount != 6 {
		t.Fatalf("unexpected estimate: %+v", estimate)
	}
}

func TestOfficialOpenAIEndpointRequiresExactHTTPSHost(t *testing.T) {
	valid := behaviorAuditTestAccount(1, "official", "https://api.openai.com/v1")
	if !isOfficialOpenAIAPIEndpoint(&valid) {
		t.Fatal("expected official endpoint")
	}
	for _, baseURL := range []string{
		"http://api.openai.com/v1",
		"https://api.openai.com.evil.example/v1",
		"https://relay.example/v1",
		"https://api.openai.com:8443/v1",
		"https://user@api.openai.com/v1",
		"https://api.openai.com/v1?proxy=true",
		"https://api.openai.com/v1#fragment",
	} {
		account := behaviorAuditTestAccount(2, "not-official", baseURL)
		if isOfficialOpenAIAPIEndpoint(&account) {
			t.Fatalf("%s must not be accepted as an official anchor", baseURL)
		}
	}
}

func TestBehaviorAnchorRejectsOfficialAccountMappedToAnotherModel(t *testing.T) {
	account := behaviorAuditTestAccount(1, "mapped-official", "https://api.openai.com/v1")
	account.Credentials["model_mapping"] = map[string]any{"gpt-5": "gpt-4.1"}
	if isBehaviorAnchorForModel(&account, "gpt-5") {
		t.Fatal("an official endpoint mapped to another model must not be used as the baseline")
	}
}

func TestBehaviorAnchorRequiresAPIKeyAndSkipsEmptyCredentialAccount(t *testing.T) {
	emptyKey := behaviorAuditTestAccount(1, "empty-key", "https://api.openai.com/v1")
	emptyKey.Credentials["api_key"] = "  "
	if isBehaviorAnchorForModel(&emptyKey, "gpt-5") {
		t.Fatal("an official account without an API key must not be used as the baseline")
	}

	validAnchor := behaviorAuditTestAccount(2, "official", "https://api.openai.com/v1")
	target := behaviorAuditTestAccount(3, "relay", "https://relay.example/v1")
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	service := &ModelCapabilityAuditService{
		db: db,
		accountRepo: &modelBehaviorAuditAccountRepo{accounts: []Account{
			emptyKey, validAnchor, target,
		}},
		accountTestService: &AccountTestService{},
	}
	targets, anchorCount, _, _, err := service.prepareBehaviorTargets(context.Background(), ModelBehaviorAuditOptions{ModelID: "gpt-5"})
	if err != nil {
		t.Fatalf("prepare behavior targets: %v", err)
	}
	if anchorCount != 1 || len(targets) != 1 || targets[0].Anchor.ID != validAnchor.ID {
		t.Fatalf("unexpected anchor selection: count=%d targets=%+v", anchorCount, targets)
	}
}

func TestProbeBehaviorEndpointRejectsOverlongMappedModelBeforeTransport(t *testing.T) {
	account := behaviorAuditTestAccount(4, "relay", "http://upstream.test/v1")
	audit := &ModelCapabilityAuditService{
		accountTestService: newModelCapabilityProbeService(&panickingModelCapabilityAuditUpstream{}, &account),
	}
	result := audit.probeBehaviorEndpoint(context.Background(), &account,
		strings.Repeat("m", modelCapabilityAuditTextMaxRunes+1), "low", "audit_tool", modelBehaviorCases[0])
	if result.Attempted || result.Status != "error" || !strings.Contains(result.Error, "exceeds 200") {
		t.Fatalf("unexpected overlong mapped-model result: %+v", result)
	}
}

func TestProbeBehaviorEndpointDisablesRedirects(t *testing.T) {
	account := behaviorAuditTestAccount(5, "relay", "http://upstream.test/v1")
	upstream := &modelCapabilityProbeContextUpstream{}
	audit := &ModelCapabilityAuditService{accountTestService: newModelCapabilityProbeService(upstream, &account)}
	result := audit.probeBehaviorEndpoint(context.Background(), &account, "gpt-5", "low", modelBehaviorToolNamePrefix+"test", modelBehaviorCases[0])
	if !upstream.redirectsDisabled || !result.Attempted {
		t.Fatalf("redirect policy was not applied: result=%+v disabled=%v", result, upstream.redirectsDisabled)
	}
}

func TestProbeBehaviorPairSkipsBillableAnchorWhenTargetIsNotComparable(t *testing.T) {
	target := behaviorAuditTestAccount(6, "relay", "http://upstream.test/v1")
	target.Credentials["api_key"] = ""
	anchor := behaviorAuditTestAccount(7, "official", "https://api.openai.com/v1")
	upstream := &countingModelCapabilityAuditUpstream{upstream: &panickingModelCapabilityAuditUpstream{}}
	audit := &ModelCapabilityAuditService{accountTestService: newModelCapabilityProbeService(upstream, &target)}
	targetResult, anchorResult := audit.probeBehaviorPair(context.Background(), ModelBehaviorAuditOptions{
		ModelID: "gpt-5", ReasoningEffort: "low",
	}, modelBehaviorAuditTarget{
		Target: &target, Anchor: &anchor, TargetMappedModel: "gpt-5", AnchorMappedModel: "gpt-5",
	}, modelBehaviorCases[0])
	if behaviorObservationComparable(targetResult) || anchorResult.Status != "skipped" || upstream.calls.Load() != 0 {
		t.Fatalf("unexpected skipped-anchor result: target=%+v anchor=%+v calls=%d", targetResult, anchorResult, upstream.calls.Load())
	}
}

func TestBehaviorAuditToolAnswerRequiresExpectedToolAndString(t *testing.T) {
	body := []byte(`{"output":[{"type":"function_call","name":"audit_tool","arguments":"{\"answer\":\"319\"}"}]}`)
	answer, found, valid := behaviorAuditToolAnswer(body, "audit_tool")
	if answer != "319" || !found || !valid {
		t.Fatalf("answer=%q found=%v valid=%v", answer, found, valid)
	}
	if _, found, _ := behaviorAuditToolAnswer(body, "other_tool"); found {
		t.Fatal("unexpected tool name must not be accepted")
	}
}

func TestBehaviorAuditPayloadScalesOutputBudgetForReasoningEffort(t *testing.T) {
	tests := []struct {
		effort string
		want   string
	}{
		{effort: "low", want: `"max_output_tokens":512`},
		{effort: "medium", want: `"max_output_tokens":1024`},
		{effort: "high", want: `"max_output_tokens":2048`},
		{effort: "xhigh", want: `"max_output_tokens":2048`},
	}
	for _, tt := range tests {
		body, err := behaviorAuditPayload("gpt-5", tt.effort, "audit_tool", modelBehaviorCases[0])
		if err != nil {
			t.Fatalf("build %s payload: %v", tt.effort, err)
		}
		if !strings.Contains(string(body), tt.want) {
			t.Fatalf("%s payload = %s, want %s", tt.effort, body, tt.want)
		}
	}
}

func TestProbeBehaviorEndpointTreatsBudgetExhaustionAsInconclusive(t *testing.T) {
	account := behaviorAuditTestAccount(9, "relay", "http://upstream.test/v1")
	upstream := &modelCapabilityProbeUpstream{
		responseBody: `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`,
	}
	audit := &ModelCapabilityAuditService{accountTestService: newModelCapabilityProbeService(upstream, &account)}
	result := audit.probeBehaviorEndpoint(context.Background(), &account, "gpt-5", "high", "audit_tool", modelBehaviorCases[0])
	if result.Status != "inconclusive" || !result.Attempted || strings.Contains(result.Error, "unsupported") {
		t.Fatalf("unexpected incomplete behavior result: %+v", result)
	}
	comparison := compareBehaviorObservations(result, modelBehaviorObservation{Status: "skipped"})
	if comparison != "inconclusive" {
		t.Fatalf("comparison=%q, want inconclusive", comparison)
	}
}

func TestProbeBehaviorEndpointWaitsForAccountSlot(t *testing.T) {
	account := behaviorAuditTestAccount(10, "relay", "http://upstream.test/v1")
	account.Concurrency = 1
	cache := &modelCapabilityAuditRetryCache{failAttempts: 2}
	audit := &ModelCapabilityAuditService{
		accountTestService: newModelCapabilityProbeService(&modelCapabilityProbeUpstream{}, &account),
		concurrencyService: NewConcurrencyService(cache),
	}
	result := audit.probeBehaviorEndpoint(context.Background(), &account, "gpt-5", "low", modelBehaviorToolNamePrefix+"test", modelBehaviorCases[0])
	if result.Status != "correct" || !result.Attempted {
		t.Fatalf("unexpected behavior result: %+v", result)
	}
	if got := cache.attempts.Load(); got != 3 {
		t.Fatalf("slot attempts = %d, want 3", got)
	}
	if got := cache.releases.Load(); got != 1 {
		t.Fatalf("slot releases = %d, want 1", got)
	}
}

func TestProbeBehaviorEndpointDoesNotExposeCredentialEcho(t *testing.T) {
	account := behaviorAuditTestAccount(11, "relay", "http://upstream.test/v1")
	account.Credentials["api_key"] = "behavior-secret-token"
	upstream := &modelCapabilityProbeUpstream{
		status:       400,
		responseBody: `{"error":{"message":"Authorization: Bearer behavior-secret-token"}}`,
	}
	audit := &ModelCapabilityAuditService{accountTestService: newModelCapabilityProbeService(upstream, &account)}
	result := audit.probeBehaviorEndpoint(context.Background(), &account, "gpt-5", "low", "audit_tool", modelBehaviorCases[0])
	if strings.Contains(result.Error, "behavior-secret-token") || result.Error != "upstream returned an error" {
		t.Fatalf("unsafe upstream error persisted: %q", result.Error)
	}
}

func TestCompareBehaviorObservationsSeparatesMatchFromDivergence(t *testing.T) {
	correct := modelBehaviorObservation{Status: "correct", Correct: true}
	incorrect := modelBehaviorObservation{Status: "incorrect"}
	if got := compareBehaviorObservations(correct, correct); got != "matched" {
		t.Fatalf("comparison=%q, want matched", got)
	}
	if got := compareBehaviorObservations(incorrect, correct); got != "diverged" {
		t.Fatalf("comparison=%q, want diverged", got)
	}
	if got := compareBehaviorObservations(correct, incorrect); got != "inconclusive" {
		t.Fatalf("comparison=%q, want inconclusive", got)
	}
	if got := compareBehaviorObservations(modelBehaviorObservation{Status: "invalid", Attempted: true}, correct); got != "error" {
		t.Fatalf("comparison=%q, want error for an invalid tool response", got)
	}
}

func TestBehaviorObservationCountsOnlyAttemptedTransportCalls(t *testing.T) {
	attempts, successes, failures := behaviorObservationCounts(
		modelBehaviorObservation{Status: "correct", Attempted: true},
		modelBehaviorObservation{Status: "incorrect", Attempted: true},
		modelBehaviorObservation{Status: "invalid", Attempted: true},
		modelBehaviorObservation{Status: "error", Attempted: false},
	)
	if attempts != 3 || successes != 2 || failures != 1 {
		t.Fatalf("counts = %d/%d/%d, want 3/2/1", attempts, successes, failures)
	}
}

func TestCreateBehaviorRunRejectsIdenticalRunningRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("INSERT INTO model_capability_probe_runs").
		WithArgs(modelBehaviorAuditTrigger, sql.NullInt64{}, 3, 6, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "requested_by", "total_count", "completed_count", "verified_count",
			"mismatch_count", "unsupported_count", "error_count", "error_message",
			"started_at", "finished_at", "created_at",
		}))
	service := &ModelCapabilityAuditService{db: db}
	_, err = service.createBehaviorRun(context.Background(), ModelBehaviorAuditOptions{
		ModelID: "gpt-5", CaseCount: 3, ReasoningEffort: "low",
	}, nil, 3)
	if err == nil || !strings.Contains(err.Error(), "MODEL_BEHAVIOR_AUDIT_ALREADY_RUNNING") {
		t.Fatalf("error = %v, want duplicate-run conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestProbeBehaviorEndpointRecoversPanicAndPreservesAttempt(t *testing.T) {
	account := behaviorAuditTestAccount(7, "panic", "http://upstream.test/v1")
	audit := &ModelCapabilityAuditService{
		accountTestService: newModelCapabilityProbeService(&panickingModelCapabilityAuditUpstream{}, &account),
	}
	result := audit.probeBehaviorEndpoint(context.Background(), &account, "gpt-5", "low", "audit_tool", modelBehaviorCases[0])
	if result.Status != "error" || !result.Attempted || result.Error != "behavior probe panicked" {
		t.Fatalf("unexpected panic result: %+v", result)
	}
}

func TestProbeBehaviorEndpointClassifiesCanceledTransportAsTimeout(t *testing.T) {
	account := behaviorAuditTestAccount(8, "timeout", "http://upstream.test/v1")
	upstream := &blockingModelCapabilityAuditUpstream{started: make(chan struct{}), release: make(chan struct{})}
	audit := &ModelCapabilityAuditService{accountTestService: newModelCapabilityProbeService(upstream, &account)}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	result := audit.probeBehaviorEndpoint(ctx, &account, "gpt-5", "low", "audit_tool", modelBehaviorCases[0])
	if result.Status != "timeout" || !result.Attempted {
		t.Fatalf("unexpected timeout result: %+v", result)
	}
}

func TestGetBehaviorRunReadsDedicatedRedactedResultTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectExec("WITH stale").
		WithArgs(modelBehaviorAuditTrigger, int64(modelBehaviorAuditStaleAfter/time.Second)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM model_capability_probe_runs WHERE id=\\$1 AND .*trigger.*=\\$2").
		WithArgs(int64(12), modelBehaviorAuditTrigger).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "requested_by", "total_count", "total_items", "completed_count", "completed_items", "verified_count",
			"mismatch_count", "unsupported_count", "error_count", "options", "error_message", "started_at", "finished_at", "created_at",
		}).AddRow(12, "completed", nil, 1, 2, 1, 2, 1, 0, 0, 0,
			[]byte(`{"model_id":"gpt-5","case_count":1,"reasoning_effort":"low"}`), nil, now, now, now))
	mock.ExpectQuery("FROM model_behavior_audit_results").
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "target_account_id", "target_account_name", "anchor_account_id", "anchor_account_name",
			"case_key", "case_label", "requested_model", "target_mapped_model", "anchor_mapped_model",
			"target_response_model", "anchor_response_model", "comparison_status", "target_status", "anchor_status",
			"target_correct", "anchor_correct", "target_http_status", "anchor_http_status",
			"target_latency_ms", "anchor_latency_ms", "target_output_hash", "anchor_output_hash",
			"expected_output_hash", "attempt_count", "success_count", "failure_count", "error_message", "checked_at",
		}).AddRow(3, 22, "relay", 11, "official", "arithmetic", "deterministic arithmetic", "gpt-5",
			"mapped-target", "gpt-5", "mapped-target", "gpt-5", "matched", "correct", "correct", true, true,
			200, 200, 10, 11, "target-hash", "anchor-hash", "expected-hash", 2, 2, 0, nil, now))

	detail, err := (&ModelCapabilityAuditService{db: db}).GetBehaviorRun(context.Background(), 12)
	if err != nil {
		t.Fatalf("get behavior run: %v", err)
	}
	if detail.Run.TotalRequests != 2 || detail.Run.AttemptCount != 2 || len(detail.Results) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	result := detail.Results[0]
	if result.TargetStatus != "correct" || result.AnchorStatus != "correct" || result.AttemptCount != 2 || result.SuccessCount != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExecuteBehaviorRunCountsFailedPersistenceAttemptsAndStopsDispatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO model_behavior_audit_results").
		WillReturnError(errors.New("synthetic persistence failure"))
	mock.ExpectExec("UPDATE model_capability_probe_runs").
		WithArgs(int64(30), 0, 2, 0, 0, 0, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO model_behavior_audit_results").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE model_capability_probe_runs").
		WithArgs(int64(30), 1, 4, 1, 0, 0, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE model_capability_probe_runs").
		WithArgs(int64(30), "partial", 1, 4, 1, 0, 0, 1, sqlmock.AnyArg(), "one or more paired results could not be persisted").
		WillReturnResult(sqlmock.NewResult(0, 1))

	targetAccount := behaviorAuditTestAccount(31, "relay", "http://upstream.test/v1")
	anchorAccount := behaviorAuditTestAccount(32, "official", "http://upstream.test/v1")
	upstream := &barrierModelCapabilityAuditUpstream{
		upstream: &modelCapabilityProbeUpstream{},
		started:  make(chan struct{}, modelBehaviorAuditWorkerCount),
		release:  make(chan struct{}),
		barrier:  modelBehaviorAuditWorkerCount,
	}
	audit := &ModelCapabilityAuditService{
		db: db, accountTestService: newModelCapabilityProbeService(upstream, &targetAccount),
	}
	run := &ModelBehaviorAuditRun{ID: 30, Status: "running", TotalPairs: 3, TotalRequests: 6}
	targets := []modelBehaviorAuditTarget{{
		Target: &targetAccount, Anchor: &anchorAccount, TargetMappedModel: "gpt-5", AnchorMappedModel: "gpt-5",
	}}
	done := make(chan error, 1)
	go func() {
		done <- audit.executeBehaviorRun(context.Background(), run, ModelBehaviorAuditOptions{
			ModelID: "gpt-5", CaseCount: 3, ReasoningEffort: "low",
		}, targets)
	}()
	for i := 0; i < modelBehaviorAuditWorkerCount; i++ {
		select {
		case <-upstream.started:
		case <-time.After(2 * time.Second):
			t.Fatal("initial behavior worker wave did not start")
		}
	}
	close(upstream.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execute behavior run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("behavior audit did not stop after persistence failure")
	}
	if got := int(upstream.calls.Load()); got != modelBehaviorAuditWorkerCount*2 {
		t.Fatalf("upstream calls = %d, want only two in-flight pairs (%d requests)", got, modelBehaviorAuditWorkerCount*2)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
