package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type blockingModelCapabilityAuditUpstream struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type panickingModelCapabilityAuditUpstream struct{}

type countingModelCapabilityAuditUpstream struct {
	upstream HTTPUpstream
	calls    atomic.Int32
}

func (u *countingModelCapabilityAuditUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *countingModelCapabilityAuditUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls.Add(1)
	return u.upstream.DoWithTLS(req, proxyURL, accountID, accountConcurrency, profile)
}

type firstWaveModelCapabilityAuditUpstream struct {
	calls        atomic.Int32
	started      chan int
	releaseFirst chan struct{}
}

type barrierModelCapabilityAuditUpstream struct {
	upstream HTTPUpstream
	calls    atomic.Int32
	started  chan struct{}
	release  chan struct{}
	barrier  int32
}

func (u *barrierModelCapabilityAuditUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *barrierModelCapabilityAuditUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	call := u.calls.Add(1)
	if call <= u.barrier {
		u.started <- struct{}{}
		select {
		case <-u.release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	return u.upstream.DoWithTLS(req, proxyURL, accountID, accountConcurrency, profile)
}

func (u *firstWaveModelCapabilityAuditUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *firstWaveModelCapabilityAuditUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	call := int(u.calls.Add(1))
	u.started <- call
	if call == 1 {
		select {
		case <-u.releaseFirst:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"model":"gpt-4o-mini","output":[{"type":"message","content":[{"type":"output_text","text":"pong"}]}]}`)),
			}, nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func (u *panickingModelCapabilityAuditUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *panickingModelCapabilityAuditUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	panic("synthetic upstream panic")
}

func (u *blockingModelCapabilityAuditUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *blockingModelCapabilityAuditUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.once.Do(func() { close(u.started) })
	select {
	case <-u.release:
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"model":"gpt-4o-mini","output":[{"type":"message","content":[{"type":"output_text","text":"pong"}]}]}`,
			)),
		}, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

type modelCapabilityAuditAccountRepo struct {
	AccountRepository
	account *Account
}

type modelCapabilityAuditAccountsRepo struct {
	AccountRepository
	accounts []*Account
}

func (r *modelCapabilityAuditAccountsRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return r.accounts, nil
}

func (r *modelCapabilityAuditAccountRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return []*Account{r.account}, nil
}

func (r *modelCapabilityAuditAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func TestCreateModelCapabilityAuditRunScansNullableFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	requestedBy := int64(42)
	mock.ExpectQuery("INSERT INTO model_capability_probe_runs").
		WithArgs(sql.NullInt64{Int64: requestedBy, Valid: true}, 2, 6, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "requested_by", "total_count", "total_items", "completed_count", "completed_items", "verified_count",
			"mismatch_count", "unsupported_count", "error_count", "options", "error_message",
			"started_at", "finished_at", "created_at",
		}).AddRow(7, "running", requestedBy, 2, 6, 0, 0, 0, 0, 0, 0,
			[]byte(`{"active_only":true,"max_items":2}`), nil, now, nil, now))

	service := &ModelCapabilityAuditService{db: db}
	run, err := service.createRun(context.Background(), ModelCapabilityAuditOptions{ActiveOnly: true, MaxItems: 2}, &requestedBy, 2, 6)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.RequestedBy == nil || *run.RequestedBy != requestedBy {
		t.Fatalf("requested_by = %v, want %d", run.RequestedBy, requestedBy)
	}
	if run.FinishedAt != nil {
		t.Fatalf("finished_at = %v, want nil", run.FinishedAt)
	}
	if _, ok := run.Options.(map[string]any); !ok {
		t.Fatalf("options type = %T, want decoded JSON object", run.Options)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestStartModelCapabilityAuditReturnsBeforeProbeAndDetachesFromRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	mock.ExpectExec("WITH stale").
		WithArgs("manual", int64(modelCapabilityAuditStaleAfter/time.Second)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO model_capability_probe_runs").
		WithArgs(sql.NullInt64{}, 1, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "requested_by", "total_count", "total_items", "completed_count", "completed_items", "verified_count",
			"mismatch_count", "unsupported_count", "error_count", "options", "error_message",
			"started_at", "finished_at", "created_at",
		}).AddRow(9, "running", nil, 1, 1, 0, 0, 0, 0, 0, 0,
			[]byte(`{"account_ids":[17],"model_ids":["gpt-4o-mini"],"protocols":["responses"],"active_only":true,"max_items":1}`), nil, now, nil, now))
	mock.ExpectExec("INSERT INTO model_capability_probe_results").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE model_capability_probe_runs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE model_capability_probe_runs").WillReturnResult(sqlmock.NewResult(0, 1))

	account := &Account{
		ID:          17,
		Name:        "async-probe",
		Status:      StatusActive,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "secret",
			"base_url": "http://upstream.test/v1",
		},
	}
	repo := &modelCapabilityAuditAccountRepo{account: account}
	upstream := &blockingModelCapabilityAuditUpstream{started: make(chan struct{}), release: make(chan struct{})}
	accountTestService := newModelCapabilityProbeService(upstream, account)
	auditService := &ModelCapabilityAuditService{db: db, accountRepo: repo, accountTestService: accountTestService}

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	run, err := auditService.Start(requestCtx, ModelCapabilityAuditOptions{
		AccountIDs: []int64{account.ID}, ModelIDs: []string{"gpt-4o-mini"},
		Protocols: []string{ModelCapabilityProtocolResponses}, ActiveOnly: true, MaxItems: 1,
	}, nil)
	if err != nil {
		t.Fatalf("start audit: %v", err)
	}
	if run.Status != "running" {
		t.Fatalf("status = %q, want running", run.Status)
	}

	select {
	case <-upstream.started:
	case <-time.After(2 * time.Second):
		t.Fatal("background probe did not start")
	}
	cancelRequest()
	close(upstream.release)

	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := mock.ExpectationsWereMet(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background run did not persist completion: %v", mock.ExpectationsWereMet())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestModelCapabilityAuditRunTimeoutCoversWorkerWaves(t *testing.T) {
	if got, want := modelCapabilityAuditRunTimeout(60), 19*time.Minute+15*time.Second; got != want {
		t.Fatalf("timeout = %s, want %s", got, want)
	}
}

func TestEstimateModelCapabilityAuditSkipsUnsupportedAccounts(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	accounts := []*Account{
		{ID: 1, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 2, Status: StatusActive, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
	}
	service := &ModelCapabilityAuditService{
		db: db, accountRepo: &modelCapabilityAuditAccountsRepo{accounts: accounts},
		accountTestService: &AccountTestService{},
	}
	estimate, err := service.Estimate(context.Background(), ModelCapabilityAuditOptions{
		AccountIDs: []int64{1, 2}, ModelIDs: []string{"gpt-5"},
		Protocols:  []string{ModelCapabilityProtocolResponses, ModelCapabilityProtocolResponsesTools},
		ActiveOnly: true, MaxItems: 10,
	})
	if err != nil {
		t.Fatalf("estimate audit: %v", err)
	}
	if estimate.TargetCount != 1 || estimate.ProtocolCount != 2 || estimate.RequestCount != 2 {
		t.Fatalf("estimate = %+v, want 1 target / 2 protocols / 2 requests", estimate)
	}
}

func TestExecuteModelCapabilityAuditRecoversWorkerPanic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT INTO model_capability_probe_results").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE model_capability_probe_runs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE model_capability_probe_runs").WillReturnResult(sqlmock.NewResult(0, 1))

	account := &Account{
		ID:          18,
		Name:        "panic-probe",
		Status:      StatusActive,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "secret",
			"base_url": "http://upstream.test/v1",
		},
	}
	auditService := &ModelCapabilityAuditService{
		db:                 db,
		accountTestService: newModelCapabilityProbeService(&panickingModelCapabilityAuditUpstream{}, account),
	}
	run := &ModelCapabilityAuditRun{ID: 10, Status: "running", TotalCount: 1, TotalItems: 1}
	targets := []modelCapabilityAuditTarget{{
		Account: account, ModelID: "gpt-4o-mini", Protocols: []string{ModelCapabilityProtocolResponses},
	}}

	result, err := auditService.executeRun(context.Background(), run, targets)
	if err != nil {
		t.Fatalf("execute audit: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.CompletedCount != 1 || result.CompletedItems != 1 || result.ErrorCount != 1 {
		t.Fatalf("counts = targets:%d items:%d errors:%d, want 1/1/1", result.CompletedCount, result.CompletedItems, result.ErrorCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExecuteModelCapabilityAuditStopsDispatchAfterPersistenceFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	for i := 0; i < modelCapabilityAuditWorkerCount; i++ {
		insert := mock.ExpectExec("INSERT INTO model_capability_probe_results")
		if i == 0 {
			insert.WillReturnError(errors.New("synthetic persistence failure"))
		} else {
			insert.WillReturnResult(sqlmock.NewResult(1, 1))
		}
		mock.ExpectExec("UPDATE model_capability_probe_runs").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("UPDATE model_capability_probe_runs").WillReturnResult(sqlmock.NewResult(0, 1))

	account := &Account{
		ID: 20, Name: "persistence-stop", Status: StatusActive, Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Concurrency: modelCapabilityAuditWorkerCount,
		Credentials: map[string]any{"api_key": "secret", "base_url": "http://upstream.test/v1"},
	}
	upstream := &firstWaveModelCapabilityAuditUpstream{
		started: make(chan int, modelCapabilityAuditWorkerCount), releaseFirst: make(chan struct{}),
	}
	audit := &ModelCapabilityAuditService{accountTestService: newModelCapabilityProbeService(upstream, account), db: db}
	targets := make([]modelCapabilityAuditTarget, modelCapabilityAuditWorkerCount+1)
	for i := range targets {
		targets[i] = modelCapabilityAuditTarget{
			Account: account, ModelID: "gpt-4o-mini", Protocols: []string{ModelCapabilityProtocolResponses},
		}
	}
	run := &ModelCapabilityAuditRun{ID: 21, Status: "running", TotalCount: len(targets), TotalItems: len(targets)}
	type executionResult struct {
		run *ModelCapabilityAuditRun
		err error
	}
	done := make(chan executionResult, 1)
	go func() {
		result, runErr := audit.executeRun(context.Background(), run, targets)
		done <- executionResult{run: result, err: runErr}
	}()
	for i := 0; i < modelCapabilityAuditWorkerCount; i++ {
		select {
		case <-upstream.started:
		case <-time.After(2 * time.Second):
			t.Fatal("initial worker wave did not start")
		}
	}
	close(upstream.releaseFirst)
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("execute run: %v", result.err)
		}
		if result.run.Status != "partial" {
			t.Fatalf("status = %q, want partial", result.run.Status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("audit did not stop after persistence failure")
	}
	if got := int(upstream.calls.Load()); got != modelCapabilityAuditWorkerCount {
		t.Fatalf("upstream calls = %d, want only initial worker wave %d", got, modelCapabilityAuditWorkerCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDeriveAuditVerdictDoesNotInferIdentityFromCapability(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel: "public-alias",
		MappedModel:    "upstream-model",
		Protocols: map[string]ModelCapabilityAuditEvidence{
			ModelCapabilityProtocolResponsesTools: {
				Status:             "verified",
				Capability:         "responses_tools",
				ToolCallFound:      true,
				ToolArgumentsValid: true,
			},
		},
	}

	deriveAuditVerdict(&row)

	if row.CapabilityStatus != "verified" {
		t.Fatalf("capability status = %q, want verified", row.CapabilityStatus)
	}
	if row.IdentityStatus != "evidence_only" {
		t.Fatalf("identity status = %q, want evidence_only without response model", row.IdentityStatus)
	}
}

func TestDeriveAuditVerdictRequiresMappedModelResponseForConsistency(t *testing.T) {
	tests := []struct {
		name           string
		responseModel  string
		wantIdentity   string
		wantConfidence string
	}{
		{name: "matching mapped model", responseModel: "upstream-model", wantIdentity: "self_report_consistent", wantConfidence: "medium"},
		{name: "requested alias only", responseModel: "public-alias", wantIdentity: "mismatch_suspected", wantConfidence: "high"},
		{name: "different model", responseModel: "other-model", wantIdentity: "mismatch_suspected", wantConfidence: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := ModelCapabilityAuditRow{
				RequestedModel: "public-alias",
				MappedModel:    "upstream-model",
				Protocols: map[string]ModelCapabilityAuditEvidence{
					ModelCapabilityProtocolResponses: {
						Status:        "verified",
						ResponseModel: tt.responseModel,
					},
				},
			}

			deriveAuditVerdict(&row)

			if row.IdentityStatus != tt.wantIdentity {
				t.Fatalf("identity status = %q, want %q", row.IdentityStatus, tt.wantIdentity)
			}
			if row.Confidence != tt.wantConfidence {
				t.Fatalf("confidence = %q, want %q", row.Confidence, tt.wantConfidence)
			}
		})
	}
}

func TestDeriveAuditVerdictPassiveMismatchRemainsSuspected(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel:    "public-alias",
		MappedModel:       "upstream-model",
		PassiveRequests:   3,
		PassiveMismatches: 1,
		PassiveResponseModels: []string{
			"upstream-model",
		},
	}

	deriveAuditVerdict(&row)

	if row.IdentityStatus != "mismatch_suspected" {
		t.Fatalf("identity status = %q, want mismatch_suspected", row.IdentityStatus)
	}
}

func TestDeriveAuditVerdictDetectsPassiveResponseModelMismatch(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel:  "public-alias",
		MappedModel:     "upstream-model",
		PassiveRequests: 3,
		PassiveResponseModels: []string{
			"upstream-model",
			"other-model",
		},
	}

	deriveAuditVerdict(&row)

	if row.IdentityStatus != "mismatch_suspected" {
		t.Fatalf("identity status = %q, want mismatch_suspected", row.IdentityStatus)
	}
}

func TestDeriveAuditVerdictAcceptsKnownRuntimeModelAlias(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel:  "grok-4.5-latest",
		MappedModel:     "grok-4.5-latest",
		PassiveRequests: 1,
		PassiveResponseModels: []string{
			"grok-4.5-build",
		},
	}

	deriveAuditVerdict(&row)

	if row.IdentityStatus != "self_report_consistent" {
		t.Fatalf("identity status = %q, want self_report_consistent for known runtime alias", row.IdentityStatus)
	}
}

func TestCreateModelCapabilityAuditRunRejectsIdenticalRunningRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("INSERT INTO model_capability_probe_runs").
		WithArgs(sql.NullInt64{}, 1, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "requested_by", "total_count", "total_items", "completed_count", "completed_items", "verified_count",
			"mismatch_count", "unsupported_count", "error_count", "options", "error_message",
			"started_at", "finished_at", "created_at",
		}))

	service := &ModelCapabilityAuditService{db: db}
	_, err = service.createRun(context.Background(), ModelCapabilityAuditOptions{
		ModelIDs: []string{"gpt-5"}, Protocols: []string{ModelCapabilityProtocolResponses}, MaxItems: 1,
	}, nil, 1, 1)
	if err == nil || !strings.Contains(err.Error(), "MODEL_CAPABILITY_AUDIT_ALREADY_RUNNING") {
		t.Fatalf("error = %v, want duplicate-run conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCapabilityProbeAttemptCountsRequireTransportAttempt(t *testing.T) {
	tests := []struct {
		name                 string
		result               ModelCapabilityProbeResult
		attempts, ok, failed int
	}{
		{name: "configuration failure", result: ModelCapabilityProbeResult{Status: "error"}},
		{name: "verified", result: ModelCapabilityProbeResult{Status: "verified", Attempted: true}, attempts: 1, ok: 1},
		{name: "model self-report mismatch", result: ModelCapabilityProbeResult{Status: "model_mismatch", Attempted: true}, attempts: 1, ok: 1},
		{name: "transport failure", result: ModelCapabilityProbeResult{Status: "timeout", Attempted: true}, attempts: 1, failed: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts, ok, failed := capabilityProbeAttemptCounts(tt.result)
			if attempts != tt.attempts || ok != tt.ok || failed != tt.failed {
				t.Fatalf("counts = %d/%d/%d, want %d/%d/%d", attempts, ok, failed, tt.attempts, tt.ok, tt.failed)
			}
		})
	}
}

func TestCapabilityAuditPersistsMappedModelActuallySentByProbe(t *testing.T) {
	oldSnapshot := &Account{
		ID: 19, Name: "mapping-change", Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "secret", "base_url": "http://upstream.test/v1",
			"model_mapping": map[string]any{"public-alias": "old-upstream-model"},
		},
	}
	currentAccount := &Account{
		ID: 19, Name: "mapping-change", Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "secret", "base_url": "http://upstream.test/v1",
			"model_mapping": map[string]any{"public-alias": "new-upstream-model"},
		},
	}
	audit := &ModelCapabilityAuditService{
		accountTestService: newModelCapabilityProbeService(&modelCapabilityProbeUpstream{model: "new-upstream-model"}, currentAccount),
	}
	target := modelCapabilityAuditTarget{
		Account: oldSnapshot, ModelID: "public-alias", Protocols: []string{ModelCapabilityProtocolResponses},
	}
	results, err := audit.probeAuditTarget(context.Background(), target)
	if err != nil {
		t.Fatalf("probe target: %v", err)
	}
	if len(results) != 1 || results[0].Status != "verified" || results[0].MappedModel != "new-upstream-model" {
		t.Fatalf("unexpected probe result: %+v", results)
	}
	if got := capabilityAuditResultMappedModel(target, results[0]); got != "new-upstream-model" {
		t.Fatalf("mapped model = %q, want actual sent model", got)
	}
}

func TestRecoverStaleAuditRunsUsesProtocolSpecificThresholds(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	service := &ModelCapabilityAuditService{db: db}

	mock.ExpectExec("WITH stale").
		WithArgs("manual", int64(modelCapabilityAuditStaleAfter/time.Second)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := service.recoverStaleAuditRuns(context.Background(), "manual"); err != nil {
		t.Fatalf("recover capability runs: %v", err)
	}
	mock.ExpectExec("WITH stale").
		WithArgs(modelBehaviorAuditTrigger, int64(modelBehaviorAuditStaleAfter/time.Second)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := service.recoverStaleAuditRuns(context.Background(), modelBehaviorAuditTrigger); err != nil {
		t.Fatalf("recover behavior runs: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestModelCapabilitySummaryReportsTrueTotalsBeforeRowLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	now := time.Now().UTC()

	mock.ExpectQuery("FROM usage_logs").
		WithArgs(30).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "name", "platform", "requested_model", "mapped_model", "response_models",
			"requests", "mismatches", "last_seen",
		}).AddRow(1, "one", PlatformOpenAI, "model-a", "model-a", "model-a", 1, 0, now).
			AddRow(1, "one", PlatformOpenAI, "model-b", "model-b", "model-b", 2, 0, now.Add(-time.Minute)).
			AddRow(2, "two", PlatformOpenAI, "model-c", "model-c", "model-c", 3, 0, now.Add(-2*time.Minute)))
	mock.ExpectQuery("WITH ranked AS").
		WithArgs(30).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "run_id", "account_id", "account_name", "platform", "group_id", "requested_model", "mapped_model",
			"response_model", "protocol", "status", "capability", "capability_status", "identity_status",
			"confidence", "evidence_source", "http_status", "latency_ms", "tool_call_found", "tool_arguments_valid",
			"error_message", "evidence", "evidence_code", "attempt_count", "success_count", "failure_count", "checked_at",
		}).AddRow(5, 4, 1, "one", PlatformOpenAI, 10, "model-a", "model-a", "model-a",
			ModelCapabilityProtocolResponses, "verified", "responses_text", "verified", "self_report_consistent",
			"medium", "active_probe", 200, 12, false, false, nil, "valid_json_response", "valid_json_response", 5, 4, 1, now))
	mock.ExpectQuery("FROM account_groups").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id", "name"}).AddRow(1, 10, "group-a"))

	summary, err := (&ModelCapabilityAuditService{db: db}).Summary(context.Background(), 30, 2)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalModels != 3 || summary.ReturnedModels != 2 || !summary.RowsTruncated {
		t.Fatalf("unexpected row totals: %+v", summary)
	}
	if summary.TotalAccounts != 2 || summary.PassiveRequests != 6 || summary.Stats.Models != 3 || summary.Stats.ActiveChecks != 5 {
		t.Fatalf("unexpected aggregate totals: %+v", summary.Stats)
	}
	if len(summary.RecentEvidence) != 1 || summary.RecentEvidence[0].Platform != PlatformOpenAI ||
		summary.RecentEvidence[0].GroupID == nil || *summary.RecentEvidence[0].GroupID != 10 || summary.RecentEvidence[0].GroupName != "group-a" {
		t.Fatalf("unexpected recent evidence: %+v", summary.RecentEvidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
