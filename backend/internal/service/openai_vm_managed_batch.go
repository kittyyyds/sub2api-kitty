package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Batch import of VM2-managed accounts: every line becomes one account with
// its own slot, and its login is started on that slot. Passwords and 2FA
// secrets live only in this process's memory until the login request has been
// sent; the database records line number, email, proxy and outcome.

const (
	// The runtime runs at most four password logins at once. Two are left for
	// logins started by hand and for automatic re-login.
	openAIVMBatchCreateWorkers = 2
	openAIVMBatchLoginWorkers  = 2
	openAIVMBatchLoginPoll     = 3 * time.Second
	openAIVMBatchLoginTimeout  = 12 * time.Minute
	// vm2api never lets one exit carry more than 32 slots (5 unless raised in its settings).
	openAIVMBatchProxyCapMax = 32
	// What one KVM slot reserves: the runtime gives its runner the guest's
	// 512 MB plus 512 MB of overhead. A new slot uses well under 1 GB of disk;
	// 20 GB is the most it can grow to.
	openAIVMBatchSlotMemoryMB = 1024
	openAIVMBatchSlotDiskGB   = 20
)

// Item states. "interrupted" is what a queued or running line becomes when the
// process restarts: its secrets were only in memory and are gone.
const (
	OpenAIVMBatchQueued      = "queued"
	OpenAIVMBatchCreating    = "creating"
	OpenAIVMBatchLoggingIn   = "logging_in"
	OpenAIVMBatchSucceeded   = "succeeded"
	OpenAIVMBatchFailed      = "failed"
	OpenAIVMBatchSkipped     = "skipped"
	OpenAIVMBatchCanceled    = "canceled"
	OpenAIVMBatchInterrupted = "interrupted"
)

type OpenAIVMBatchItem struct {
	ID        int64     `json:"id"`
	LineNo    int       `json:"line_no"`
	Email     string    `json:"email"`
	ProxyID   *int64    `json:"proxy_id"`
	AccountID *int64    `json:"account_id"`
	Status    string    `json:"status"`
	ErrorCode string    `json:"error_code,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OpenAIVMBatchJob struct {
	ID          int64               `json:"id"`
	Status      string              `json:"status"`
	RuntimeType string              `json:"runtime_type"`
	Total       int                 `json:"total"`
	CreatedAt   time.Time           `json:"created_at"`
	FinishedAt  *time.Time          `json:"finished_at,omitempty"`
	Items       []OpenAIVMBatchItem `json:"items"`
}

// OpenAIVMBatchRepository stores batch progress. Nothing it stores can hold a
// password or a 2FA secret: the schema has no column for one.
type OpenAIVMBatchRepository interface {
	CreateBatch(ctx context.Context, createdBy int64, runtime string, options json.RawMessage, items []OpenAIVMBatchItem) (*OpenAIVMBatchJob, error)
	GetBatch(ctx context.Context, id int64) (*OpenAIVMBatchJob, error)
	SetBatchItem(ctx context.Context, itemID int64, status, errorCode string, accountID *int64) error
	FinishBatch(ctx context.Context, id int64, status string) error
	// InterruptUnfinishedBatches marks everything a previous process left
	// queued or running as interrupted and reports how many items that was.
	InterruptUnfinishedBatches(ctx context.Context) (int64, error)
	// ManagedEmails returns the emails VM2-managed accounts already use.
	ManagedEmails(ctx context.Context) (map[string]int64, error)
	// ManagedAccountsPerProxy counts managed accounts by the proxy their slot exits through.
	ManagedAccountsPerProxy(ctx context.Context) (map[int64]int, error)
}

// openAIVMBatchProxies is the slice of the admin service that finds or adds a proxy.
type openAIVMBatchProxies interface {
	CreateProxy(ctx context.Context, input *CreateProxyInput) (*Proxy, error)
}

type OpenAIVMBatchInput struct {
	Count      int                      `json:"count,omitempty"`
	LoginMode  string                   `json:"login_mode,omitempty"`
	NamePrefix string                   `json:"name_prefix,omitempty"`
	Entries    []OpenAIVMBatchEntry     `json:"entries,omitempty"`
	Settings   *OpenAIVMManagedSettings `json:"settings,omitempty"`
	PlanToken  string                   `json:"plan_token,omitempty"`

	Text        string  `json:"text"`
	ProxyIDs    []int64 `json:"proxy_ids"`
	PerProxy    int     `json:"per_proxy"`
	RuntimeType string  `json:"runtime_type"`
	GroupIDs    []int64 `json:"group_ids"`
	Concurrency int     `json:"concurrency"`
	Priority    int     `json:"priority"`
	// SkipMixedChannelCheck is set after the administrator confirmed the risk.
	SkipMixedChannelCheck bool `json:"-"`
}

// OpenAIVMBatchPreview is what the administrator confirms before anything is
// created. It lists what will happen per line and never echoes a secret.
type OpenAIVMBatchPreview struct {
	PlanToken  string                   `json:"plan_token,omitempty"`
	Importable int                      `json:"importable"`
	Problems   []OpenAIVMBatchLineError `json:"problems"`
	Lines      []OpenAIVMBatchPlanLine  `json:"lines"`
	// Estimated footprint of the new slots, for the confirmation screen.
	EstimatedMemoryMB int `json:"estimated_memory_mb"`
	EstimatedDiskGB   int `json:"estimated_disk_gb"`
}

type OpenAIVMBatchPlanLine struct {
	Name    string `json:"name,omitempty"`
	LineNo  int    `json:"line_no"`
	Email   string `json:"email"`
	ProxyID int64  `json:"proxy_id,omitempty"`
	// NewProxy is true when the line carries a proxy that is not in the list yet.
	NewProxy bool `json:"new_proxy,omitempty"`
}

type openAIVMBatchPlan struct {
	runtime string
	lines   []OpenAIVMBatchLine
	proxy   map[int]int64 // line number -> existing proxy id
	fresh   map[int]OpenAIVMBatchProxy
	preview OpenAIVMBatchPreview
}

// OpenAIVMBatchService runs batches. One batch runs at a time: the slot
// footprint and the runtime's login limit are sized for that.
type OpenAIVMBatchService struct {
	planKey      [32]byte
	startedPlans map[string]int64
	managed      *OpenAIVMManagedService
	repo         OpenAIVMBatchRepository
	proxies      ProxyRepository
	admin        openAIVMBatchProxies

	mu      sync.Mutex
	running int64
	cancel  context.CancelFunc
	done    chan struct{}
	// Tunable in tests.
	loginPoll    time.Duration
	loginTimeout time.Duration
}

func NewOpenAIVMBatchService(managed *OpenAIVMManagedService, repo OpenAIVMBatchRepository, proxies ProxyRepository, admin openAIVMBatchProxies) *OpenAIVMBatchService {
	if managed == nil || repo == nil {
		return nil
	}
	s := &OpenAIVMBatchService{managed: managed, repo: repo, proxies: proxies, admin: admin, loginPoll: openAIVMBatchLoginPoll, loginTimeout: openAIVMBatchLoginTimeout, startedPlans: map[string]int64{}}
	if _, err := rand.Read(s.planKey[:]); err != nil {
		panic("cannot initialize batch preview authentication")
	}
	return s
}

// Recover marks what a previous process left unfinished. Call once at start-up.
func (s *OpenAIVMBatchService) Recover(ctx context.Context) {
	if s == nil {
		return
	}
	if n, err := s.repo.InterruptUnfinishedBatches(ctx); err != nil {
		slog.Warn("openai_vm_batch_recover_failed", "error", err)
	} else if n > 0 {
		slog.Info("openai_vm_batch_interrupted", "items", n)
	}
}

func (s *OpenAIVMBatchService) plan(ctx context.Context, in OpenAIVMBatchInput) (*openAIVMBatchPlan, error) {
	if in.Concurrency < 1 {
		in.Concurrency = 1
	}
	if len(in.Text) > OpenAIVMBatchMaxInputBytes {
		return nil, infraerrors.BadRequest("OPENAI_VM_BATCH_TOO_LARGE", "The pasted text is too large")
	}
	runtime, err := normalizeOpenAIVMManagedRuntime(in.RuntimeType)
	if err != nil {
		return nil, err
	}
	if in.PerProxy < 1 || in.PerProxy > openAIVMBatchProxyCapMax {
		return nil, infraerrors.BadRequest("OPENAI_VM_BATCH_PER_PROXY", "Accounts per proxy must be between 1 and 32")
	}

	if err := in.Settings.validate(); err != nil {
		return nil, err
	}
	if in.Settings != nil {
		if validator, ok := s.managed.admin.(interface {
			ValidateAccountGroupBindings(context.Context, []int64) error
		}); ok {
			if err := validator.ValidateAccountGroupBindings(ctx, in.Settings.GroupIDs); err != nil {
				return nil, err
			}
		}
	}
	lines, problems, parseErr := openAIVMBatchInputLines(in)
	if parseErr != nil {
		return nil, parseErr
	}
	if len(lines)+len(problems) > OpenAIVMBatchMaxLines {
		return nil, infraerrors.BadRequest("OPENAI_VM_BATCH_TOO_MANY", fmt.Sprintf("A batch holds at most %d accounts", OpenAIVMBatchMaxLines)).
			WithMetadata(map[string]string{"max": fmt.Sprint(OpenAIVMBatchMaxLines)})
	}
	// Every selected proxy must be one a slot can use, checked now rather than line by line later.
	seen := map[int64]bool{}
	var selected []int64
	for _, id := range in.ProxyIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := s.managed.socksProxy(ctx, id); err != nil {
			return nil, err
		}
		selected = append(selected, id)
	}
	taken, err := s.repo.ManagedEmails(ctx)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.ManagedAccountsPerProxy(ctx)
	if err != nil {
		return nil, err
	}
	plan := &openAIVMBatchPlan{runtime: runtime, proxy: map[int]int64{}, fresh: map[int]OpenAIVMBatchProxy{}}
	var kept []OpenAIVMBatchLine
	for _, line := range lines {
		if _, exists := taken[strings.ToLower(line.Email)]; exists {
			// Never overwrite or re-login an account that is already managed.
			problems = append(problems, OpenAIVMBatchLineError{LineNo: line.LineNo, Email: line.Email, Code: "already_managed"})
			continue
		}
		kept = append(kept, line)
	}
	// Lines with their own proxy: reuse the matching proxy when one exists.
	existing, err := s.proxies.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for _, line := range kept {
		if line.ProxyURL == "" {
			continue
		}
		own, _ := ParseOpenAIVMBatchProxy(line.ProxyURL)
		matched := false
		for i := range existing {
			p := &existing[i]
			if (p.Protocol == "socks5" || p.Protocol == "socks5h") && strings.EqualFold(p.Host, own.Host) && p.Port == own.Port && p.Username == own.Username && p.Password == own.Password && !p.IsExpired(time.Now()) {
				plan.proxy[line.LineNo] = p.ID
				matched = true
				break
			}
		}
		if !matched {
			plan.fresh[line.LineNo] = own
		}
	}
	assigned, unplaced := AssignOpenAIVMBatchProxies(kept, selected, used, in.PerProxy)
	if in.Count > 0 {
		selected, used = uniqueVMProxyPools(selected, existing, used)
		caps, capErr := s.proxyCapacity(ctx, selected)
		if capErr != nil {
			return nil, capErr
		}
		load, limits := map[int64]int{}, map[int64]int{}
		for i, id := range selected {
			load[id] = caps[i].Used
			if used[id] > load[id] {
				load[id] = used[id]
			}
			limits[id] = min(in.PerProxy, caps[i].Limit)
			if !caps[i].Available {
				limits[id] = 0
			}
		}
		assigned, unplaced = assignOpenAIVMBalancedWithLimits(kept, selected, load, limits)
	}
	short := map[int]bool{}
	for _, lineNo := range unplaced {
		short[lineNo] = true
	}
	for _, line := range kept {
		if short[line.LineNo] {
			problems = append(problems, OpenAIVMBatchLineError{LineNo: line.LineNo, Email: line.Email, Code: "no_proxy_available"})
			continue
		}
		if id, ok := assigned[line.LineNo]; ok {
			plan.proxy[line.LineNo] = id
		}
		plan.lines = append(plan.lines, line)
		_, fresh := plan.fresh[line.LineNo]
		plan.preview.Lines = append(plan.preview.Lines, OpenAIVMBatchPlanLine{LineNo: line.LineNo, Name: line.Name, Email: line.Email, ProxyID: plan.proxy[line.LineNo], NewProxy: fresh})
	}
	plan.preview.Importable = len(plan.lines)
	plan.preview.Problems = problems
	if runtime == "kvm" {
		plan.preview.EstimatedMemoryMB, plan.preview.EstimatedDiskGB = openAIVMBatchSlotMemoryMB*len(plan.lines), openAIVMBatchSlotDiskGB*len(plan.lines)
	}

	if in.Count > 0 {
		in.PlanToken = ""
		raw, _ := json.Marshal(struct {
			Input   OpenAIVMBatchInput
			Preview OpenAIVMBatchPreview
		}{in, plan.preview})
		mac := hmac.New(sha256.New, s.planKey[:])
		_, _ = mac.Write(raw)
		plan.preview.PlanToken = hex.EncodeToString(mac.Sum(nil))
	}
	return plan, nil
}

// Preview reports what a batch would do. Nothing is created.
func (s *OpenAIVMBatchService) Preview(ctx context.Context, in OpenAIVMBatchInput) (*OpenAIVMBatchPreview, error) {
	if _, _, err := s.managed.vm.configured(ctx); err != nil {
		return nil, err
	}
	plan, err := s.plan(ctx, in)
	if err != nil {
		return nil, err
	}
	return &plan.preview, nil
}

// Start validates the batch again, records it and begins working through it in
// the background. The returned job is the progress record to poll.
func (s *OpenAIVMBatchService) Start(ctx context.Context, createdBy int64, in OpenAIVMBatchInput) (*OpenAIVMBatchJob, error) {
	if _, _, err := s.managed.vm.configured(ctx); err != nil {
		return nil, err
	}
	concurrency := in.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 256 {
		return nil, infraerrors.BadRequest("OPENAI_VM_MANAGED_CONCURRENCY", "Concurrency must be between 1 and 256")
	}
	in.Concurrency = concurrency
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.startedPlans[in.PlanToken]; in.PlanToken != "" && ok {
		return s.repo.GetBatch(ctx, id)
	}
	if s.running != 0 {
		return nil, infraerrors.Conflict("OPENAI_VM_BATCH_RUNNING", "Another batch import is still running")
	}
	plan, err := s.plan(ctx, in)
	if err != nil {
		return nil, err
	}

	if in.Settings != nil && !in.SkipMixedChannelCheck {
		if checker, ok := s.managed.admin.(interface {
			CheckMixedChannelRisk(context.Context, int64, string, []int64) error
		}); ok {
			if err := checker.CheckMixedChannelRisk(ctx, 0, PlatformOpenAI, in.Settings.GroupIDs); err != nil {
				return nil, err
			}
		}
	}
	if in.Count > 0 && (len(plan.lines) != in.Count || len(plan.preview.Problems) > 0) {
		return nil, infraerrors.BadRequest("OPENAI_VM_BATCH_COUNT", "Every requested KVM needs a valid distinct account and available proxy; correct the preview before starting")
	}
	if in.Count > 0 && (in.PlanToken == "" || in.PlanToken != plan.preview.PlanToken) {
		return nil, infraerrors.Conflict("OPENAI_VM_BATCH_PLAN_CHANGED", "The allocation or settings changed; preview the batch again")
	}
	if len(plan.lines) == 0 {
		return nil, infraerrors.BadRequest("OPENAI_VM_BATCH_EMPTY", "No line can be imported")
	}
	items := make([]OpenAIVMBatchItem, 0, len(plan.lines)+len(plan.preview.Problems))
	for _, line := range plan.lines {
		item := OpenAIVMBatchItem{LineNo: line.LineNo, Email: line.Email, Status: OpenAIVMBatchQueued}
		if id := plan.proxy[line.LineNo]; id > 0 {
			item.ProxyID = &id
		}
		items = append(items, item)
	}
	for _, problem := range plan.preview.Problems {
		items = append(items, OpenAIVMBatchItem{LineNo: problem.LineNo, Email: problem.Email, Status: OpenAIVMBatchSkipped, ErrorCode: problem.Code})
	}
	options, _ := json.Marshal(map[string]any{"per_proxy": in.PerProxy, "group_ids": in.GroupIDs, "concurrency": in.Concurrency, "priority": in.Priority, "login_mode": in.LoginMode})
	job, err := s.repo.CreateBatch(ctx, createdBy, plan.runtime, options, items)
	if err != nil {
		return nil, err
	}
	byLine := map[int]int64{}
	for _, item := range job.Items {
		byLine[item.LineNo] = item.ID
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.running, s.cancel, s.done = job.ID, cancel, make(chan struct{})
	if in.PlanToken != "" {
		s.startedPlans[in.PlanToken] = job.ID
	}
	go s.run(runCtx, job.ID, plan, in, byLine, s.done)
	return job, nil
}

// Status returns a batch's progress.
func (s *OpenAIVMBatchService) Status(ctx context.Context, id int64) (*OpenAIVMBatchJob, error) {
	job, err := s.repo.GetBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, infraerrors.NotFound("OPENAI_VM_BATCH_NOT_FOUND", "Batch import not found")
	}
	for i := range job.Items {
		item := &job.Items[i]
		if item.Status == "awaiting_login" && item.AccountID != nil {
			if binding, e := s.managed.vm.repo.GetBinding(ctx, *item.AccountID); e == nil && binding != nil && (binding.ManagedStatus == OpenAIVMManagedAvailable || binding.ManagedStatus == OpenAIVMManagedRestricted) {
				item.Status = OpenAIVMBatchSucceeded
			}
		}
	}
	return job, nil
}

// Cancel stops a running batch after the line it is working on. Accounts that
// were already created stay; lines not started yet are marked canceled.
func (s *OpenAIVMBatchService) Cancel(ctx context.Context, id int64) (*OpenAIVMBatchJob, error) {
	s.mu.Lock()
	if s.running == id && s.cancel != nil {
		s.cancel()
	}
	done := s.done
	running := s.running == id
	s.mu.Unlock()
	if running && done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		case <-time.After(30 * time.Second):
		}
	}
	return s.Status(ctx, id)
}

// Stop cancels a running batch at shutdown.
func (s *OpenAIVMBatchService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

type openAIVMBatchLogin struct {
	itemID    int64
	accountID int64
	line      OpenAIVMBatchLine
}

func (s *OpenAIVMBatchService) run(ctx context.Context, jobID int64, plan *openAIVMBatchPlan, in OpenAIVMBatchInput, itemOf map[int]int64, done chan struct{}) {
	defer close(done)
	defer func() {
		// Whatever happens, no password or 2FA secret outlives the batch.
		for i := range plan.lines {
			plan.lines[i].Password, plan.lines[i].MFASecret, plan.lines[i].ProxyURL = "", "", ""
		}
		for k := range plan.fresh {
			delete(plan.fresh, k)
		}
		s.mu.Lock()
		if s.running == jobID {
			s.running, s.cancel, s.done = 0, nil, nil
		}
		s.mu.Unlock()
	}()
	record := func(itemID int64, status, code string, accountID *int64) {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := s.repo.SetBatchItem(writeCtx, itemID, status, code, accountID); err != nil {
			slog.Warn("openai_vm_batch_item_update_failed", "job_id", jobID, "item_id", itemID, "error", err)
		}
	}

	creates := make(chan OpenAIVMBatchLine)
	logins := make(chan openAIVMBatchLogin, len(plan.lines))
	var proxyMu sync.Mutex
	freshIDs := map[string]int64{}
	var createWG, loginWG sync.WaitGroup

	for i := 0; i < openAIVMBatchCreateWorkers; i++ {
		createWG.Add(1)
		go func() {
			defer createWG.Done()
			for line := range creates {
				itemID := itemOf[line.LineNo]
				if ctx.Err() != nil {
					record(itemID, OpenAIVMBatchCanceled, "", nil)
					continue
				}
				record(itemID, OpenAIVMBatchCreating, "", nil)
				proxyID := plan.proxy[line.LineNo]
				if own, fresh := plan.fresh[line.LineNo]; fresh {
					// Two lines naming the same new proxy share one proxy row.
					proxyMu.Lock()
					key := fmt.Sprintf("%s|%s|%d|%s|%s", own.Protocol, own.Host, own.Port, own.Username, own.Password)
					proxyID = freshIDs[key]
					if proxyID == 0 {
						created, err := s.admin.CreateProxy(ctx, &CreateProxyInput{Name: fmt.Sprintf("%s:%d", own.Host, own.Port), Protocol: own.Protocol, Host: own.Host, Port: own.Port, Username: own.Username, Password: own.Password})
						if err == nil && created != nil {
							proxyID = created.ID
							freshIDs[key] = proxyID
						}
					}
					proxyMu.Unlock()
					if proxyID == 0 {
						record(itemID, OpenAIVMBatchFailed, "proxy_create_failed", nil)
						continue
					}
				}
				name := line.Name
				if name == "" {
					name = line.Email
				}
				account, _, err := s.managed.Create(ctx, OpenAIVMManagedCreateInput{
					Settings: in.Settings, LoginEmail: line.Email, Name: name, GroupIDs: in.GroupIDs, ProxyID: proxyID, RuntimeType: plan.runtime,
					Concurrency: in.Concurrency, Priority: in.Priority, SkipMixedChannelCheck: in.SkipMixedChannelCheck,
				})
				if account == nil {
					record(itemID, OpenAIVMBatchFailed, openAIVMBatchErrorCode(err), nil)
					continue
				}
				accountID := account.ID
				if err != nil {
					// The account exists with an empty or failed slot; it can be
					// retried or deleted from its panel.
					record(itemID, OpenAIVMBatchFailed, "slot_"+openAIVMBatchErrorCode(err), &accountID)
					continue
				}

				if in.LoginMode == "manual" {
					record(itemID, "awaiting_login", "", &accountID)
					continue
				}
				record(itemID, OpenAIVMBatchLoggingIn, "", &accountID)
				logins <- openAIVMBatchLogin{itemID: itemID, accountID: accountID, line: line}
			}
		}()
	}
	for i := 0; i < openAIVMBatchLoginWorkers; i++ {
		loginWG.Add(1)
		go func() {
			defer loginWG.Done()
			for job := range logins {
				status, code := s.login(ctx, job)
				accountID := job.accountID
				record(job.itemID, status, code, &accountID)
			}
		}()
	}
	for _, line := range plan.lines {
		creates <- line
	}
	close(creates)
	createWG.Wait()
	close(logins)
	loginWG.Wait()

	final := "finished"
	if ctx.Err() != nil {
		final = "canceled"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.repo.FinishBatch(finishCtx, jobID, final); err != nil {
		slog.Warn("openai_vm_batch_finish_failed", "job_id", jobID, "error", err)
	}
}

// login starts one password login on the slot and follows it to its end.
func (s *OpenAIVMBatchService) login(ctx context.Context, job openAIVMBatchLogin) (string, string) {
	if ctx.Err() != nil {
		return OpenAIVMBatchCanceled, ""
	}
	body, err := json.Marshal(map[string]string{"email": job.line.Email, "password": job.line.Password, "mfa_mode": "secret", "mfa_secret": job.line.MFASecret})
	if err != nil {
		return OpenAIVMBatchFailed, "internal_error"
	}
	_, err = s.managed.Call(ctx, job.accountID, http.MethodPost, "login/password", body)
	for i := range body {
		body[i] = 0
	}
	if err != nil {
		return OpenAIVMBatchFailed, "login_" + openAIVMBatchErrorCode(err)
	}
	deadline := time.Now().Add(s.loginTimeout)
	for {
		select {
		case <-ctx.Done():
			// The login keeps running on the slot; its result shows in the account's panel.
			return OpenAIVMBatchCanceled, "login_still_running"
		case <-time.After(s.loginPoll):
		}
		raw, err := s.managed.Call(ctx, job.accountID, http.MethodGet, "login/password", nil)
		if err == nil {
			var state struct {
				Job *struct {
					Status string `json:"status"`
					Reason string `json:"reason"`
				} `json:"job"`
			}
			if json.Unmarshal(raw, &state) == nil && state.Job != nil {
				switch state.Job.Status {
				case "succeeded":
					// Let the state sync open the account now rather than a minute later.
					if _, syncErr := s.managed.Status(ctx, job.accountID); syncErr != nil {
						slog.Debug("openai_vm_batch_sync_failed", "account_id", job.accountID, "error", syncErr)
					}
					return OpenAIVMBatchSucceeded, ""
				case "failed", "canceled":
					return OpenAIVMBatchFailed, "login_" + openAIVMBatchReason(state.Job.Reason)
				case "awaiting_code":
					// The 2FA secret was not accepted as a code source; nobody can type one here.
					_, _ = s.managed.Call(ctx, job.accountID, http.MethodPost, "login/password/cancel", nil)
					return OpenAIVMBatchFailed, "login_code_required"
				}
			}
		}
		if time.Now().After(deadline) {
			return OpenAIVMBatchFailed, "login_timeout"
		}
	}
}

// openAIVMBatchReason keeps a runtime reason only when it is a short identifier.
func openAIVMBatchReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 48 {
		return "failed"
	}
	for _, r := range reason {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' {
			return "failed"
		}
	}
	return reason
}

func openAIVMBatchErrorCode(err error) string {
	if err == nil {
		return "unknown"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	code := strings.ToLower(OpenAIVMManagedErrorCode(err))
	if code == "" || len(code) > 48 {
		return "failed"
	}
	return code
}
