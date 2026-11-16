package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// batchTestRepo keeps batch progress in memory and records everything written,
// so tests can assert that no secret ever reaches the store.
type batchTestRepo struct {
	mu          sync.Mutex
	job         *OpenAIVMBatchJob
	options     string
	writes      []string
	emails      map[string]int64
	perProxy    map[int64]int
	interrupted int64
}

func (r *batchTestRepo) CreateBatch(_ context.Context, _ int64, runtime string, options json.RawMessage, items []OpenAIVMBatchItem) (*OpenAIVMBatchJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.options = string(options)
	r.job = &OpenAIVMBatchJob{ID: 1, Status: "running", RuntimeType: runtime, Total: len(items)}
	for i, item := range items {
		item.ID = int64(i + 1)
		r.job.Items = append(r.job.Items, item)
		r.writes = append(r.writes, fmt.Sprintf("%d|%s|%s|%s", item.LineNo, item.Email, item.Status, item.ErrorCode))
	}
	return r.copy(), nil
}
func (r *batchTestRepo) copy() *OpenAIVMBatchJob {
	c := *r.job
	c.Items = append([]OpenAIVMBatchItem(nil), r.job.Items...)
	return &c
}
func (r *batchTestRepo) GetBatch(context.Context, int64) (*OpenAIVMBatchJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job == nil {
		return nil, nil
	}
	return r.copy(), nil
}
func (r *batchTestRepo) SetBatchItem(_ context.Context, itemID int64, status, code string, accountID *int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.job.Items {
		if r.job.Items[i].ID == itemID {
			r.job.Items[i].Status, r.job.Items[i].ErrorCode = status, code
			if accountID != nil {
				id := *accountID
				r.job.Items[i].AccountID = &id
			}
		}
	}
	r.writes = append(r.writes, fmt.Sprintf("%d|%s|%s", itemID, status, code))
	return nil
}
func (r *batchTestRepo) FinishBatch(_ context.Context, _ int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.job.Status = status
	now := time.Now()
	r.job.FinishedAt = &now
	return nil
}
func (r *batchTestRepo) InterruptUnfinishedBatches(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interrupted++
	return 0, nil
}
func (r *batchTestRepo) ManagedEmails(context.Context) (map[string]int64, error) {
	return r.emails, nil
}
func (r *batchTestRepo) ManagedAccountsPerProxy(context.Context) (map[int64]int, error) {
	return r.perProxy, nil
}

// batchAccounts hands out one in-memory account per created line.
type batchAccounts struct {
	AccountRepository
	mu      sync.Mutex
	nextID  int64
	byID    map[int64]*Account
	deleted map[int64]bool
}

func (r *batchAccounts) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.byID[id]
	if a == nil || r.deleted[id] {
		return nil, ErrAccountNotFound
	}
	c := *a
	c.Extra, c.Credentials = shallowCopyMap(a.Extra), shallowCopyMap(a.Credentials)
	return &c, nil
}
func (r *batchAccounts) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return nil, nil
}
func (r *batchAccounts) SetSchedulable(_ context.Context, id int64, on bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[id].Schedulable = on
	return nil
}
func (r *batchAccounts) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}
func (r *batchAccounts) ClearTempUnschedulable(context.Context, int64) error { return nil }
func (r *batchAccounts) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range updates {
		r.byID[id].Extra[k] = v
	}
	return nil
}
func (r *batchAccounts) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[id].Credentials = shallowCopyMap(credentials)
	return nil
}
func (r *batchAccounts) CreateAccount(ctx context.Context, in *CreateAccountInput) (*Account, error) {
	if err := guardOpenAIVMManagedCreate(ctx, in); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	a := &Account{ID: r.nextID, Name: in.Name, Platform: in.Platform, Type: in.Type, Credentials: shallowCopyMap(in.Credentials), Extra: shallowCopyMap(in.Extra), ProxyID: in.ProxyID, Concurrency: in.Concurrency, GroupIDs: in.GroupIDs, Status: StatusActive}
	r.byID[a.ID] = a
	c := *a
	return &c, nil
}
func (r *batchAccounts) DeleteAccount(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted[id] = true
	return nil
}

// batchBindings is a per-account binding store.
type batchBindings struct {
	mu       sync.Mutex
	c        OpenAIVMConfig
	bindings map[int64]*OpenAIVMBinding
	accounts *batchAccounts
}

func (r *batchBindings) GetConfig(context.Context) (*OpenAIVMConfig, error) { c := r.c; return &c, nil }
func (r *batchBindings) SaveConfig(context.Context, *OpenAIVMConfig) error  { return nil }
func (r *batchBindings) GetBinding(_ context.Context, id int64) (*OpenAIVMBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b := r.bindings[id]; b != nil {
		c := *b
		return &c, nil
	}
	return nil, nil
}
func (r *batchBindings) WithLock(_ context.Context, _ int64, fn func() error) error { return fn() }
func (r *batchBindings) CountBindings(context.Context) (int, error)                 { return len(r.bindings), nil }
func (r *batchBindings) Reserve(context.Context, int64, string, *Proxy) (*OpenAIVMBinding, error) {
	return nil, fmt.Errorf("token-fed reservation is not part of a batch")
}
func (r *batchBindings) SetState(_ context.Context, id int64, state, slot, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bindings[id]
	b.Status = state
	if slot != "" {
		b.SlotID = slot
	}
	b.LastError = message
	return nil
}
func (r *batchBindings) Restore(context.Context, int64) error { return nil }
func (r *batchBindings) ReserveManaged(_ context.Context, id int64, runtime string, p *Proxy) (*OpenAIVMBinding, error) {
	a, err := r.accounts.GetByID(context.Background(), id)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	original := *a.ProxyID
	r.bindings[id] = &OpenAIVMBinding{AccountID: id, Mode: OpenAIVMModeManaged, ManagedProxyID: 9000 + id, OriginalProxyID: &original, RuntimeType: runtime, Status: "provisioning", CreatedAt: time.Now()}
	c := *r.bindings[id]
	return &c, nil
}
func (r *batchBindings) SetManaged(_ context.Context, id int64, status string, detail json.RawMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bindings[id].ManagedStatus = status
	if len(detail) > 0 {
		r.bindings[id].ManagedDetail = detail
	}
	return nil
}
func (r *batchBindings) MarkActivated(_ context.Context, id int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bindings[id].ActivatedAt != nil {
		return false, nil
	}
	now := time.Now()
	r.bindings[id].ActivatedAt = &now
	return true, nil
}
func (r *batchBindings) ListManaged(context.Context) ([]*OpenAIVMBinding, error) { return nil, nil }
func (r *batchBindings) SetOriginalProxy(context.Context, int64, int64) error    { return nil }
func (r *batchBindings) ReleaseManaged(context.Context, int64) error             { return nil }

type batchProxyStore struct {
	ProxyRepository
	mu      sync.Mutex
	byID    map[int64]*Proxy
	nextID  int64
	created []CreateProxyInput
}

func (r *batchProxyStore) GetByID(_ context.Context, id int64) (*Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.byID[id]; p != nil {
		return p, nil
	}
	return nil, ErrProxyNotFound
}
func (r *batchProxyStore) ListActive(context.Context) ([]Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Proxy
	for _, p := range r.byID {
		if p.IsActive() {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (r *batchProxyStore) CreateProxy(_ context.Context, in *CreateProxyInput) (*Proxy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	p := &Proxy{ID: r.nextID, Name: in.Name, Protocol: in.Protocol, Host: in.Host, Port: in.Port, Username: in.Username, Password: in.Password, Status: StatusActive}
	r.byID[p.ID] = p
	r.created = append(r.created, *in)
	return p, nil
}

// batchBridge is a fake bridge with one login per account. `outcome` decides
// how the login of an account (by its name, the email) ends.
type batchBridge struct {
	mu       sync.Mutex
	requests []string
	bodies   []string
	polls    map[string]int
	names    map[string]string // account id -> email
	outcome  map[string]string
	failSlot map[string]bool
	maxLogin int
	running  int
	hold     chan struct{}
}

func (b *batchBridge) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if req.URL.Path == "/v1/managed-capacity" {
			var in struct {
				Proxies []string `json:"proxies"`
			}
			_ = json.Unmarshal(body, &in)
			out := []map[string]any{}
			for range in.Proxies {
				out = append(out, map[string]any{"used": 0, "limit": 5, "available": true})
			}
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
		id := parts[2]
		rest := strings.Join(parts[3:], "/")
		b.mu.Lock()
		b.requests = append(b.requests, req.Method+" "+rest+" #"+id)
		b.bodies = append(b.bodies, string(body))
		b.mu.Unlock()
		if req.Header.Get("Authorization") != "Bearer "+vmTestSecret {
			t.Errorf("%s reached the bridge without its secret", req.URL.Path)
		}
		switch {
		case req.Method == http.MethodPost && rest == "managed":
			var in struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(body, &in)
			b.mu.Lock()
			b.names[id] = in.Name
			fail := b.failSlot[in.Name]
			b.mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, `{"error":{"code":"provision_failed"}}`)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"state":"awaiting_login","vm_id":"s2a-abcdefabcdef-`+id+`"}`)
		case req.Method == http.MethodPost && rest == "managed/login/password":
			b.mu.Lock()
			b.running++
			if b.running > b.maxLogin {
				b.maxLogin = b.running
			}
			outcome := b.outcome[b.names[id]]
			b.mu.Unlock()
			if outcome == "refuse" {
				b.mu.Lock()
				b.running--
				b.mu.Unlock()
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"error":{"code":"login_busy"}}`)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"job":{"id":"job-`+id+`","status":"running"}}`)
		case req.Method == http.MethodGet && rest == "managed/login/password":
			if b.hold != nil {
				<-b.hold
			}
			b.mu.Lock()
			b.polls[id]++
			outcome := b.outcome[b.names[id]]
			done := b.polls[id] >= 2
			if done && outcome != "hang" {
				b.running--
			}
			b.mu.Unlock()
			status, reason := "running", ""
			if done {
				switch outcome {
				case "", "ok":
					status = "succeeded"
				case "hang":
					status = "running"
				case "code":
					status = "awaiting_code"
				default:
					status, reason = "failed", outcome
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "job-" + id, "status": status, "reason": reason}})
		case req.Method == http.MethodGet && rest == "managed":
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready", "ready": true, "vm_id": "s2a-abcdefabcdef-" + id,
				"snapshot": map[string]any{"status": "running", "schedulable": true, "schedule_state": "on", "has_token": true, "email": b.names[id], "chatgpt_account_id": "org-" + id}})
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}
}

type batchFixture struct {
	svc      *OpenAIVMBatchService
	repo     *batchTestRepo
	accounts *batchAccounts
	proxies  *batchProxyStore
	bridge   *batchBridge
}

func newBatchFixture(t *testing.T) *batchFixture {
	t.Helper()
	bridge := &batchBridge{polls: map[string]int{}, names: map[string]string{}, outcome: map[string]string{}, failSlot: map[string]bool{}}
	server := httptest.NewServer(bridge.handler(t))
	t.Cleanup(server.Close)
	accounts := &batchAccounts{byID: map[int64]*Account{}, deleted: map[int64]bool{}}
	bindings := &batchBindings{accounts: accounts, bindings: map[int64]*OpenAIVMBinding{}, c: OpenAIVMConfig{Enabled: true, BaseURL: server.URL, ProxyURL: server.URL, EncryptedSecret: "encrypted:" + vmTestSecret, SecretConfigured: true}}
	proxies := &batchProxyStore{nextID: 100, byID: map[int64]*Proxy{
		41: {ID: 41, Name: "exit-one", Protocol: "socks5", Host: "exit-one.example", Port: 1080, Status: StatusActive},
		42: {ID: 42, Name: "exit-two", Protocol: "socks5h", Host: "exit-two.example", Port: 1080, Username: "u", Password: "existing-proxy-pass", Status: StatusActive},
		43: {ID: 43, Name: "http", Protocol: "http", Host: "http.example", Port: 8080, Status: StatusActive},
	}}
	vm := NewOpenAIVMService(bindings, accounts, proxies, vmTestEncryptor{})
	managed := NewOpenAIVMManagedService(vm, bindings, accounts)
	repo := &batchTestRepo{emails: map[string]int64{}, perProxy: map[int64]int{}}
	svc := NewOpenAIVMBatchService(managed, repo, proxies, proxies)
	svc.loginPoll, svc.loginTimeout = 5*time.Millisecond, 2*time.Second
	return &batchFixture{svc: svc, repo: repo, accounts: accounts, proxies: proxies, bridge: bridge}
}

func (f *batchFixture) wait(t *testing.T) *OpenAIVMBatchJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, err := f.svc.Status(context.Background(), 1)
		require.NoError(t, err)
		if job.FinishedAt != nil {
			f.svc.mu.Lock()
			idle := f.svc.running == 0
			f.svc.mu.Unlock()
			if idle {
				return job
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("batch did not finish")
	return nil
}

func itemsByEmail(job *OpenAIVMBatchJob) map[string]OpenAIVMBatchItem {
	out := map[string]OpenAIVMBatchItem{}
	for _, item := range job.Items {
		out[fmt.Sprintf("%d:%s", item.LineNo, item.Email)] = item
	}
	return out
}

func batchText(lines ...string) string { return strings.Join(lines, "\n") }

func TestBatchPreviewPlansWithoutCreatingAnything(t *testing.T) {
	f := newBatchFixture(t)
	f.repo.emails["taken@example.test"] = 77
	f.repo.perProxy[41] = 1
	text := batchText(
		"a@example.test---canary-pass-a---"+batchSecret,
		"taken@example.test---canary-pass-b---"+batchSecret,
		"b@example.test---canary-pass-c---"+batchSecret+"---socks5h://u:existing-proxy-pass@EXIT-TWO.example:1080",
		"c@example.test---canary-pass-d---"+batchSecret+"---socks5://new.example:1080",
		"broken line",
		"d@example.test---canary-pass-e---"+batchSecret,
		"e@example.test---canary-pass-f---"+batchSecret,
	)
	preview, err := f.svc.Preview(context.Background(), OpenAIVMBatchInput{Text: text, ProxyIDs: []int64{41}, PerProxy: 2})
	require.NoError(t, err)
	require.Equal(t, 3, preview.Importable)
	require.Equal(t, []OpenAIVMBatchPlanLine{
		{LineNo: 1, Email: "a@example.test", ProxyID: 41},
		{LineNo: 3, Email: "b@example.test", ProxyID: 42}, // the proxy on the line is one that already exists
		{LineNo: 4, Email: "c@example.test", NewProxy: true},
	}, preview.Lines)
	codes := map[int]string{}
	for _, problem := range preview.Problems {
		codes[problem.LineNo] = problem.Code
	}
	// Proxy 41 already carries one account, so it has room for one more only.
	require.Equal(t, map[int]string{2: "already_managed", 5: "invalid_format", 6: "no_proxy_available", 7: "no_proxy_available"}, codes)
	require.Equal(t, 3*1024, preview.EstimatedMemoryMB)
	require.Equal(t, 3*20, preview.EstimatedDiskGB)
	encoded, _ := json.Marshal(preview)
	for _, secret := range []string{"canary", batchSecret, "existing-proxy-pass", "new.example"} {
		require.NotContains(t, string(encoded), secret)
	}
	require.Empty(t, f.accounts.byID)
	require.Empty(t, f.proxies.created)
	require.Empty(t, f.bridge.requests)
	require.Nil(t, f.repo.job)

	docker, err := f.svc.Preview(context.Background(), OpenAIVMBatchInput{Text: text, ProxyIDs: []int64{41}, PerProxy: 2, RuntimeType: "docker"})
	require.NoError(t, err)
	require.Zero(t, docker.EstimatedMemoryMB)
}

func TestBatchRejectsBadRequestsBeforeAnythingIsCreated(t *testing.T) {
	line := "a@example.test---canary-pass---" + batchSecret
	many := make([]string, 0, OpenAIVMBatchMaxLines+1)
	for i := 0; i <= OpenAIVMBatchMaxLines; i++ {
		many = append(many, fmt.Sprintf("u%d@example.test---canary-pass---%s", i, batchSecret))
	}
	for name, in := range map[string]OpenAIVMBatchInput{
		"too many lines":  {Text: batchText(many...), ProxyIDs: []int64{41}, PerProxy: 32},
		"too much text":   {Text: line + strings.Repeat(" ", OpenAIVMBatchMaxInputBytes), ProxyIDs: []int64{41}, PerProxy: 1},
		"http proxy":      {Text: line, ProxyIDs: []int64{43}, PerProxy: 1},
		"unknown proxy":   {Text: line, ProxyIDs: []int64{999}, PerProxy: 1},
		"per proxy zero":  {Text: line, ProxyIDs: []int64{41}, PerProxy: 0},
		"per proxy huge":  {Text: line, ProxyIDs: []int64{41}, PerProxy: 33},
		"bad runtime":     {Text: line, ProxyIDs: []int64{41}, PerProxy: 1, RuntimeType: "lxc"},
		"nothing to do":   {Text: "broken line", ProxyIDs: []int64{41}, PerProxy: 1},
		"no proxy at all": {Text: line, PerProxy: 1},
		"bad concurrency": {Text: line, ProxyIDs: []int64{41}, PerProxy: 1, Concurrency: 257},
	} {
		f := newBatchFixture(t)
		_, err := f.svc.Start(context.Background(), 1, in)
		require.Errorf(t, err, name)
		require.NotContainsf(t, err.Error(), "canary", "%s echoed a password", name)
		require.Emptyf(t, f.accounts.byID, "%s created an account", name)
		require.Emptyf(t, f.bridge.requests, "%s reached the bridge", name)
		require.Nilf(t, f.repo.job, "%s recorded a batch", name)
	}
}

func TestBatchCreatesSlotsLogsInAndStoresNoSecret(t *testing.T) {
	f := newBatchFixture(t)
	f.bridge.outcome["wrongpass@example.test"] = "password_rejected"
	f.bridge.outcome["refused@example.test"] = "refuse"
	f.bridge.outcome["needcode@example.test"] = "code"
	f.bridge.failSlot["noslot@example.test"] = true
	text := batchText(
		"ok1@example.test---canary-pass-1---"+batchSecret,
		"ok2@example.test---canary-pass-2---"+batchSecret+"---socks5://np:canary-proxy-pass@fresh.example:1080",
		"wrongpass@example.test---canary-pass-3---"+batchSecret,
		"noslot@example.test---canary-pass-4---"+batchSecret,
		"refused@example.test---canary-pass-5---"+batchSecret,
		"needcode@example.test---canary-pass-6---"+batchSecret,
		"ok3@example.test---canary-pass-7---"+batchSecret+"---socks5://np:canary-proxy-pass@fresh.example:1080",
		"not a line",
	)
	job, err := f.svc.Start(context.Background(), 9, OpenAIVMBatchInput{Text: text, ProxyIDs: []int64{41, 42}, PerProxy: 5, Concurrency: 2, GroupIDs: []int64{3}})
	require.NoError(t, err)
	require.Equal(t, "kvm", job.RuntimeType, "KVM is the default runtime")
	require.Equal(t, 8, job.Total)
	job = f.wait(t)
	require.Equal(t, "finished", job.Status)

	items := itemsByEmail(job)
	want := map[string][2]string{
		"1:ok1@example.test":       {OpenAIVMBatchSucceeded, ""},
		"2:ok2@example.test":       {OpenAIVMBatchSucceeded, ""},
		"3:wrongpass@example.test": {OpenAIVMBatchFailed, "login_password_rejected"},
		"4:noslot@example.test":    {OpenAIVMBatchFailed, "slot_provision_failed"},
		"5:refused@example.test":   {OpenAIVMBatchFailed, "login_login_busy"},
		"6:needcode@example.test":  {OpenAIVMBatchFailed, "login_code_required"},
		"7:ok3@example.test":       {OpenAIVMBatchSucceeded, ""},
		"8:":                       {OpenAIVMBatchSkipped, "invalid_format"},
	}
	for key, expected := range want {
		item, found := items[key]
		require.Truef(t, found, "missing item %s in %v", key, items)
		require.Equalf(t, expected, [2]string{item.Status, item.ErrorCode}, key)
	}
	// Every line that got an account keeps it, including the failed ones, so
	// it can be retried or deleted from its panel.
	for key, item := range items {
		if strings.HasPrefix(key, "8:") {
			require.Nil(t, item.AccountID)
			continue
		}
		require.NotNilf(t, item.AccountID, key)
	}
	require.Len(t, f.accounts.byID, 7)
	require.Empty(t, f.accounts.deleted)
	for _, account := range f.accounts.byID {
		require.True(t, account.IsOpenAIVMManaged())
		require.False(t, openAIVMManagedHasToken(account.Credentials))
		require.Equal(t, 2, account.Concurrency)
		require.Equal(t, []int64{3}, account.GroupIDs)
	}
	// Two lines named the same new proxy: it is added once and shared.
	require.Len(t, f.proxies.created, 1)
	require.Equal(t, "fresh.example", f.proxies.created[0].Host)
	fresh := f.proxies.created[0]
	require.Equal(t, CreateProxyInput{Name: "fresh.example:1080", Protocol: "socks5", Host: "fresh.example", Port: 1080, Username: "np", Password: "canary-proxy-pass"}, fresh)
	// A successful login opens the account at once.
	for _, account := range f.accounts.byID {
		if strings.HasPrefix(account.Name, "ok") {
			require.Truef(t, account.Schedulable, "%s was not opened", account.Name)
		} else {
			require.Falsef(t, account.Schedulable, "%s was opened without a login", account.Name)
		}
	}

	// The password and 2FA secret went to the slot, once per login, and nowhere else.
	f.bridge.mu.Lock()
	logins := 0
	for i, route := range f.bridge.requests {
		if strings.HasPrefix(route, "POST managed/login/password #") {
			logins++
			require.Contains(t, f.bridge.bodies[i], "canary-pass-")
			require.Contains(t, f.bridge.bodies[i], batchSecret)
			require.Contains(t, f.bridge.bodies[i], `"mfa_mode":"secret"`)
		} else {
			require.NotContainsf(t, f.bridge.bodies[i], "canary-pass-", "a password was sent to %s", route)
			require.NotContainsf(t, f.bridge.bodies[i], batchSecret, "a 2FA secret was sent to %s", route)
		}
	}
	require.LessOrEqual(t, f.bridge.maxLogin, openAIVMBatchLoginWorkers, "the runtime's login limit leaves room for manual logins")
	f.bridge.mu.Unlock()
	require.Equal(t, 6, logins, "the line without a slot never logs in")

	stored, _ := json.Marshal([]any{job, f.repo.writes, f.repo.options})
	accounts, _ := json.Marshal(f.accounts.byID)
	for _, secret := range []string{"canary", batchSecret} {
		require.NotContainsf(t, string(stored), secret, "the batch record holds %q", secret)
		require.NotContainsf(t, string(accounts), secret, "an account holds %q", secret)
	}
}

func TestBatchRunsOneAtATimeAndCancelKeepsWhatExists(t *testing.T) {
	f := newBatchFixture(t)
	hold := make(chan struct{})
	f.bridge.hold = hold
	text := batchText("a@example.test---canary-1---"+batchSecret, "b@example.test---canary-2---"+batchSecret, "c@example.test---canary-3---"+batchSecret)
	_, err := f.svc.Start(context.Background(), 1, OpenAIVMBatchInput{Text: text, ProxyIDs: []int64{41}, PerProxy: 10})
	require.NoError(t, err)
	_, err = f.svc.Start(context.Background(), 1, OpenAIVMBatchInput{Text: "z@example.test---canary-9---" + batchSecret, ProxyIDs: []int64{41}, PerProxy: 10})
	require.Equal(t, "OPENAI_VM_BATCH_RUNNING", infraerrors.Reason(err))

	// Let the logins start, then cancel while they are being followed.
	require.Eventually(t, func() bool {
		f.bridge.mu.Lock()
		defer f.bridge.mu.Unlock()
		return f.bridge.running >= 1
	}, 5*time.Second, 5*time.Millisecond)
	go func() { time.Sleep(20 * time.Millisecond); close(hold) }()
	job, err := f.svc.Cancel(context.Background(), 1)
	require.NoError(t, err)
	job = f.wait(t)
	require.Equal(t, "canceled", job.Status)
	for _, item := range job.Items {
		require.Containsf(t, []string{OpenAIVMBatchCanceled, OpenAIVMBatchSucceeded}, item.Status, "line %d", item.LineNo)
		require.NotEqual(t, OpenAIVMBatchQueued, item.Status)
	}
	require.Empty(t, f.accounts.deleted, "cancelling never deletes an account that was created")
	// A finished batch frees the slot for the next one. (`hold` is closed, so polls no longer wait.)
	f.repo.mu.Lock()
	f.repo.job = nil
	f.repo.mu.Unlock()
	_, err = f.svc.Start(context.Background(), 1, OpenAIVMBatchInput{Text: "z@example.test---canary-9---" + batchSecret, ProxyIDs: []int64{41}, PerProxy: 10})
	require.NoError(t, err)
	f.wait(t)
}

func TestBatchLoginThatNeverEndsTimesOut(t *testing.T) {
	f := newBatchFixture(t)
	f.svc.loginTimeout = 60 * time.Millisecond
	f.bridge.outcome["slow@example.test"] = "hang"
	_, err := f.svc.Start(context.Background(), 1, OpenAIVMBatchInput{Text: "slow@example.test---canary-1---" + batchSecret, ProxyIDs: []int64{41}, PerProxy: 1})
	require.NoError(t, err)
	job := f.wait(t)
	require.Equal(t, [2]string{OpenAIVMBatchFailed, "login_timeout"}, [2]string{job.Items[0].Status, job.Items[0].ErrorCode})
}

func TestBatchRecoverAndReasonHelpers(t *testing.T) {
	f := newBatchFixture(t)
	f.svc.Recover(context.Background())
	require.EqualValues(t, 1, f.repo.interrupted)
	(*OpenAIVMBatchService)(nil).Recover(context.Background())
	(*OpenAIVMBatchService)(nil).Stop()
	f.svc.Stop()
	_, err := f.svc.Status(context.Background(), 5)
	require.Equal(t, "OPENAI_VM_BATCH_NOT_FOUND", infraerrors.Reason(err))
	// Only short identifiers from the runtime are stored as a reason.
	for in, want := range map[string]string{"password_rejected": "password_rejected", "": "failed", "Upstream said: token=abc": "failed", strings.Repeat("a", 60): "failed", "totp_rejected": "totp_rejected"} {
		require.Equalf(t, want, openAIVMBatchReason(in), "%q", in)
	}
	require.Nil(t, NewOpenAIVMBatchService(nil, f.repo, f.proxies, f.proxies))
}
