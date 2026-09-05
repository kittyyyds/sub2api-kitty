package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

type modelCapabilityAuditRetryCache struct {
	ConcurrencyCache
	failAttempts int32
	attempts     atomic.Int32
	releases     atomic.Int32
}

type modelCapabilityAuditExclusiveCache struct {
	ConcurrencyCache
	mu       sync.Mutex
	held     int
	maxHeld  int
	attempts int
}

func (c *modelCapabilityAuditRetryCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return c.attempts.Add(1) > c.failAttempts, nil
}

func (c *modelCapabilityAuditRetryCache) ReleaseAccountSlot(context.Context, int64, string) error {
	c.releases.Add(1)
	return nil
}

func (c *modelCapabilityAuditExclusiveCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempts++
	if c.held >= 1 {
		return false, nil
	}
	c.held++
	if c.held > c.maxHeld {
		c.maxHeld = c.held
	}
	return true, nil
}

func (c *modelCapabilityAuditExclusiveCache) ReleaseAccountSlot(context.Context, int64, string) error {
	c.mu.Lock()
	c.held--
	c.mu.Unlock()
	return nil
}

func (c *modelCapabilityAuditExclusiveCache) snapshot() (attempts, held, maxHeld int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts, c.held, c.maxHeld
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

func (r *modelCapabilityAuditAccountsRepo) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	accounts := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account != nil {
			accounts = append(accounts, *account)
		}
	}
	return accounts, nil
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
	options := ModelCapabilityAuditOptions{
		AccountIDs: []int64{account.ID}, ModelIDs: []string{"gpt-4o-mini"},
		Protocols: []string{ModelCapabilityProtocolResponses}, ActiveOnly: true, MaxItems: 1,
	}
	estimate, err := auditService.Estimate(context.Background(), options)
	if err != nil {
		t.Fatalf("estimate audit: %v", err)
	}
	options.PreviewToken = estimate.PreviewToken

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	run, err := auditService.Start(requestCtx, options, nil)
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
	if estimate.PreviewToken == "" || !estimate.ExpiresAt.After(time.Now()) {
		t.Fatalf("estimate did not return a live preview token: %+v", estimate)
	}
}

func TestModelCapabilityAuditRejectsChangedPreviewTargets(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	accountOne := &Account{ID: 1, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	accountTwo := &Account{ID: 2, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	repo := &modelCapabilityAuditAccountsRepo{accounts: []*Account{accountOne}}
	service := &ModelCapabilityAuditService{db: db, accountRepo: repo, accountTestService: &AccountTestService{}}
	options := ModelCapabilityAuditOptions{
		ModelIDs:  []string{"gpt-5"},
		Protocols: []string{ModelCapabilityProtocolResponses}, ActiveOnly: true, MaxItems: 10,
	}
	estimate, err := service.Estimate(context.Background(), options)
	if err != nil {
		t.Fatalf("estimate audit: %v", err)
	}
	if estimate.TargetCount != 1 || estimate.RequestCount != 1 {
		t.Fatalf("estimate = %+v, want one target/request", estimate)
	}
	options.PreviewToken = estimate.PreviewToken

	// A fresh all-account resolution would now expand this run to two paid
	// requests, so Start must require a new confirmation instead.
	repo.accounts = []*Account{accountOne, accountTwo}
	_, _, err = service.prepareRun(context.Background(), options, nil)
	if err == nil || !strings.Contains(err.Error(), "MODEL_CAPABILITY_AUDIT_PREVIEW_STALE") {
		t.Fatalf("error = %v, want stale-preview conflict", err)
	}
}

func TestModelCapabilityAuditAcceptsUnchangedPreviewTargets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	account := &Account{
		ID: 4, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{"public-model": "upstream-model"}},
		UpdatedAt:   time.Unix(1700000000, 0).UTC(),
	}
	repo := &modelCapabilityAuditAccountsRepo{accounts: []*Account{account}}
	service := &ModelCapabilityAuditService{db: db, accountRepo: repo, accountTestService: &AccountTestService{}}
	options := ModelCapabilityAuditOptions{
		ModelIDs: []string{"public-model"}, Protocols: []string{ModelCapabilityProtocolResponses},
		ActiveOnly: true, MaxItems: 10,
	}
	estimate, err := service.Estimate(context.Background(), options)
	if err != nil {
		t.Fatalf("estimate audit: %v", err)
	}
	options.PreviewToken = estimate.PreviewToken

	now := time.Now().UTC()
	mock.ExpectQuery("INSERT INTO model_capability_probe_runs").
		WithArgs(sql.NullInt64{}, 1, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "requested_by", "total_count", "total_items", "completed_count", "completed_items", "verified_count",
			"mismatch_count", "unsupported_count", "error_count", "options", "error_message",
			"started_at", "finished_at", "created_at",
		}).AddRow(13, "running", nil, 1, 1, 0, 0, 0, 0, 0, 0,
			[]byte(`{"model_ids":["public-model"],"protocols":["responses"],"active_only":true,"max_items":10}`), nil, now, nil, now))

	run, targets, err := service.prepareRun(context.Background(), options, nil)
	if err != nil {
		t.Fatalf("prepare run: %v", err)
	}
	if run.TotalCount != 1 || len(targets) != 1 || targets[0].Account.ID != account.ID {
		t.Fatalf("unexpected run/targets: run=%+v targets=%+v", run, targets)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestModelCapabilityAuditRequiresFreshExplicitPreviewToken(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	account := &Account{ID: 8, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	repo := &modelCapabilityAuditAccountsRepo{accounts: []*Account{account}}
	service := &ModelCapabilityAuditService{db: db, accountRepo: repo, accountTestService: &AccountTestService{}}
	options := ModelCapabilityAuditOptions{
		ModelIDs: []string{"gpt-5"}, Protocols: []string{ModelCapabilityProtocolResponses},
		ActiveOnly: true, MaxItems: 10,
	}

	if _, _, err := service.prepareRun(context.Background(), options, nil); err == nil || !strings.Contains(err.Error(), "MODEL_CAPABILITY_AUDIT_PREVIEW_REQUIRED") {
		t.Fatalf("missing-token error = %v", err)
	}

	estimate, err := service.Estimate(context.Background(), options)
	if err != nil {
		t.Fatalf("estimate audit: %v", err)
	}
	options.PreviewToken = estimate.PreviewToken
	service.previewMu.Lock()
	snapshot := service.previewSnapshots[estimate.PreviewToken]
	snapshot.expiresAt = time.Now().Add(-time.Second)
	service.previewSnapshots[estimate.PreviewToken] = snapshot
	service.previewMu.Unlock()
	if _, _, err := service.prepareRun(context.Background(), options, nil); err == nil || !strings.Contains(err.Error(), "MODEL_CAPABILITY_AUDIT_PREVIEW_EXPIRED") {
		t.Fatalf("expired-token error = %v", err)
	}

	estimate, err = service.Estimate(context.Background(), ModelCapabilityAuditOptions{
		ModelIDs: []string{"gpt-5"}, Protocols: []string{ModelCapabilityProtocolResponses},
		ActiveOnly: true, MaxItems: 10,
	})
	if err != nil {
		t.Fatalf("second estimate audit: %v", err)
	}
	restarted := &ModelCapabilityAuditService{db: db, accountRepo: repo, accountTestService: &AccountTestService{}}
	options.PreviewToken = estimate.PreviewToken
	if _, _, err := restarted.prepareRun(context.Background(), options, nil); err == nil || !strings.Contains(err.Error(), "MODEL_CAPABILITY_AUDIT_PREVIEW_INVALID") {
		t.Fatalf("restart-token error = %v", err)
	}
}

func TestModelAuditPreviewTokenIsSingleUseAndAuditTypeBound(t *testing.T) {
	service := &ModelCapabilityAuditService{}
	token, _, err := service.issueModelAuditPreview(modelCapabilityAuditPreviewKindManual, "options", "targets")
	if err != nil {
		t.Fatalf("issue preview token: %v", err)
	}
	if err := service.consumeModelAuditPreview(modelCapabilityAuditPreviewKindManual, "MODEL_CAPABILITY_AUDIT", token, "options", "targets"); err != nil {
		t.Fatalf("consume preview token: %v", err)
	}
	if err := service.consumeModelAuditPreview(modelCapabilityAuditPreviewKindManual, "MODEL_CAPABILITY_AUDIT", token, "options", "targets"); err == nil || !strings.Contains(err.Error(), "MODEL_CAPABILITY_AUDIT_PREVIEW_INVALID") {
		t.Fatalf("replayed-token error = %v", err)
	}

	behaviorToken, _, err := service.issueModelAuditPreview(modelCapabilityAuditPreviewKindBehavior, "options", "targets")
	if err != nil {
		t.Fatalf("issue behavior preview token: %v", err)
	}
	if err := service.consumeModelAuditPreview(modelCapabilityAuditPreviewKindManual, "MODEL_CAPABILITY_AUDIT", behaviorToken, "options", "targets"); err == nil || !strings.Contains(err.Error(), "MODEL_CAPABILITY_AUDIT_PREVIEW_INVALID") {
		t.Fatalf("cross-audit token error = %v", err)
	}
}

func TestModelCapabilityAuditWaitsForAccountSlot(t *testing.T) {
	account := &Account{
		ID: 31, Name: "slot-retry", Status: StatusActive, Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "secret", "base_url": "http://upstream.test/v1"},
	}
	cache := &modelCapabilityAuditRetryCache{failAttempts: 2}
	upstream := &countingModelCapabilityAuditUpstream{upstream: &modelCapabilityProbeUpstream{model: "gpt-4o-mini"}}
	audit := &ModelCapabilityAuditService{
		accountTestService: newModelCapabilityProbeService(upstream, account),
		concurrencyService: NewConcurrencyService(cache),
	}
	results, err := audit.probeAuditTarget(context.Background(), modelCapabilityAuditTarget{
		Account: account, ModelID: "gpt-4o-mini", Protocols: []string{ModelCapabilityProtocolResponses},
	})
	if err != nil {
		t.Fatalf("probe target: %v", err)
	}
	if len(results) != 1 || results[0].Status != "verified" {
		t.Fatalf("unexpected probe results: %+v", results)
	}
	if got := cache.attempts.Load(); got != 3 {
		t.Fatalf("slot attempts = %d, want 3", got)
	}
	if got := cache.releases.Load(); got != 1 {
		t.Fatalf("slot releases = %d, want 1", got)
	}
	if got := upstream.calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestModelCapabilityAuditSerializesCompetingTargetsAtAccountLimit(t *testing.T) {
	account := &Account{
		ID: 33, Name: "slot-contention", Status: StatusActive, Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "secret", "base_url": "http://upstream.test/v1"},
	}
	cache := &modelCapabilityAuditExclusiveCache{}
	blocking := &blockingModelCapabilityAuditUpstream{started: make(chan struct{}), release: make(chan struct{})}
	upstream := &countingModelCapabilityAuditUpstream{upstream: blocking}
	audit := &ModelCapabilityAuditService{
		accountTestService: newModelCapabilityProbeService(upstream, account),
		concurrencyService: NewConcurrencyService(cache),
	}
	target := modelCapabilityAuditTarget{
		Account: account, ModelID: "gpt-4o-mini", Protocols: []string{ModelCapabilityProtocolResponses},
	}
	type probeOutcome struct {
		results []ModelCapabilityProbeResult
		err     error
	}
	done := make(chan probeOutcome, 2)
	probe := func() {
		results, probeErr := audit.probeAuditTarget(context.Background(), target)
		done <- probeOutcome{results: results, err: probeErr}
	}
	go probe()
	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first probe did not occupy the account slot")
	}
	go probe()

	deadline := time.Now().Add(2 * time.Second)
	for {
		attempts, _, _ := cache.snapshot()
		if attempts >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second probe never attempted the occupied slot")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(blocking.release)
	for i := 0; i < 2; i++ {
		select {
		case outcome := <-done:
			if outcome.err != nil || len(outcome.results) != 1 || outcome.results[0].Status != "verified" {
				t.Fatalf("probe outcome = %+v", outcome)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("competing probe did not finish after slot release")
		}
	}
	attempts, held, maxHeld := cache.snapshot()
	if attempts < 3 {
		t.Fatalf("slot attempts = %d, want a failed attempt followed by retry", attempts)
	}
	if held != 0 || maxHeld != 1 {
		t.Fatalf("slot occupancy = held:%d max:%d, want 0/1", held, maxHeld)
	}
	if got := upstream.calls.Load(); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
}

func TestModelCapabilityAuditSlotWaitHonorsContext(t *testing.T) {
	cache := &modelCapabilityAuditRetryCache{failAttempts: 1 << 30}
	audit := &ModelCapabilityAuditService{concurrencyService: NewConcurrencyService(cache)}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	release, err := audit.acquireAuditAccountSlot(ctx, 32, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if release != nil {
		t.Fatal("release function must be nil when no slot was acquired")
	}
	if got := cache.attempts.Load(); got < 1 {
		t.Fatalf("slot attempts = %d, want at least one", got)
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

func TestDeriveAuditVerdictKeepsActiveModelsOutOfPassiveEvidence(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel: "public-alias",
		MappedModel:    "upstream-model",
		Protocols: map[string]ModelCapabilityAuditEvidence{
			ModelCapabilityProtocolResponses: {
				Status:        "verified",
				ResponseModel: "upstream-model",
			},
		},
	}

	deriveAuditVerdict(&row)

	if row.ObservedModel != "upstream-model" || row.IdentityStatus != "self_report_consistent" {
		t.Fatalf("unexpected active verdict: %+v", row)
	}
	if len(row.PassiveResponseModels) != 0 {
		t.Fatalf("active response leaked into passive evidence: %+v", row.PassiveResponseModels)
	}
}

func TestDeriveAuditVerdictUsesPersistedIdentityForRedactedActiveModel(t *testing.T) {
	row := ModelCapabilityAuditRow{
		RequestedModel: "gpt-5",
		MappedModel:    "gpt-5",
		Protocols: map[string]ModelCapabilityAuditEvidence{
			ModelCapabilityProtocolResponses: {
				Status:         "verified",
				ResponseModel:  "[redacted]",
				IdentityStatus: "self_report_consistent",
			},
		},
	}

	deriveAuditVerdict(&row)

	if row.ObservedModel != "[redacted]" || row.IdentityStatus != "self_report_consistent" || row.Confidence != "medium" {
		t.Fatalf("unexpected redacted active verdict: %+v", row)
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

func TestCapabilityProbeResponseModelMatchUsesPreRedactionVerdict(t *testing.T) {
	matches, observed := capabilityProbeResponseModelMatch(ModelCapabilityProbeResult{
		UpstreamModel:      "[redacted]",
		responseModelMatch: boolPointerForModelCapabilityAudit(true),
	}, "gpt-5", "[redacted]")
	if !observed || !matches {
		t.Fatalf("pre-redaction match = %v/%v, want true/true", matches, observed)
	}

	matches, observed = capabilityProbeResponseModelMatch(ModelCapabilityProbeResult{
		UpstreamModel:      "[redacted]",
		responseModelMatch: boolPointerForModelCapabilityAudit(false),
	}, "gpt-5", "[redacted]")
	if !observed || matches {
		t.Fatalf("pre-redaction mismatch = %v/%v, want false/true", matches, observed)
	}

	if matches, observed = capabilityProbeResponseModelMatch(ModelCapabilityProbeResult{
		UpstreamModel: "[redacted]",
	}, "gpt-5", "[redacted]"); observed || matches {
		t.Fatalf("legacy redacted placeholder = %v/%v, want false/false", matches, observed)
	}
}

func boolPointerForModelCapabilityAudit(value bool) *bool {
	return &value
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

	mock.ExpectQuery("(?s)selected_keys AS.*LIMIT \\$2.*STRING_AGG.*LIMIT \\$3.*passive_only_legacy_unselected AS").
		WithArgs(30, 2, modelCapabilityAuditPassiveModelLimit).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "name", "platform", "requested_model", "mapped_model", "response_models",
			"requests", "mismatches", "last_seen", "needs_model_match", "total_models", "total_accounts",
			"passive_requests", "passive_only_mismatch_models", "passive_only_self_report_models", "passive_only_unverified_models",
		}).AddRow(1, "one", PlatformOpenAI, "model-a", "model-a", "model-a", 1, 0, now, false, 3, 2, 6, 0, 2, 0).
			AddRow(1, "one", PlatformOpenAI, "model-b", "model-b", "model-b", 2, 0, now.Add(-time.Minute), false, 3, 2, 6, 0, 2, 0))
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
	if summary.SelfReportConsistent != 3 || summary.Stats.SelfReported != 3 || summary.MismatchSuspected != 0 || summary.Unverified != 0 {
		t.Fatalf("unexpected identity totals: %+v", summary)
	}
	if len(summary.RecentEvidence) != 1 || summary.RecentEvidence[0].Platform != PlatformOpenAI ||
		summary.RecentEvidence[0].GroupID == nil || *summary.RecentEvidence[0].GroupID != 10 || summary.RecentEvidence[0].GroupName != "group-a" {
		t.Fatalf("unexpected recent evidence: %+v", summary.RecentEvidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSplitResponseModelsEnforcesEvidenceLimit(t *testing.T) {
	models := make([]string, 0, modelCapabilityAuditPassiveModelLimit+5)
	for i := 0; i < modelCapabilityAuditPassiveModelLimit+5; i++ {
		models = append(models, fmt.Sprintf("model-%02d", i))
	}
	got := splitResponseModels(strings.Join(models, ","))
	if len(got) != modelCapabilityAuditPassiveModelLimit {
		t.Fatalf("response models = %d, want strict limit %d", len(got), modelCapabilityAuditPassiveModelLimit)
	}
}
