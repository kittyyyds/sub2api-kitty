package service

import (
	"context"
	"encoding/json"
	"errors"
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

const managedTestProxyPassword = "socks-password-never-shown"

// managedTestAccounts is an in-memory account store that records every write a
// managed account receives, so tests can assert what was — and was not — stored.
type managedTestAccounts struct {
	AccountRepository
	mu          sync.Mutex
	a           *Account
	errors      []string
	schedulable []bool
	pauses      []string
	clears      int
	deleted     bool
	deletes     int
}

func (r *managedTestAccounts) Delete(context.Context, int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletes++
	r.deleted = true
	return nil
}

func (r *managedTestAccounts) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.a == nil || r.deleted {
		return nil, ErrAccountNotFound
	}
	c := *r.a
	c.Extra = shallowCopyMap(r.a.Extra)
	c.Credentials = shallowCopyMap(r.a.Credentials)
	return &c, nil
}
func (r *managedTestAccounts) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return nil, nil
}
func (r *managedTestAccounts) SetError(_ context.Context, _ int64, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, message)
	r.a.Status = StatusError
	return nil
}
func (r *managedTestAccounts) SetSchedulable(_ context.Context, _ int64, on bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulable = append(r.schedulable, on)
	r.a.Schedulable = on
	return nil
}
func (r *managedTestAccounts) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pauses = append(r.pauses, reason)
	r.a.TempUnschedulableUntil, r.a.TempUnschedulableReason = &until, reason
	return nil
}
func (r *managedTestAccounts) ClearTempUnschedulable(context.Context, int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clears++
	r.a.TempUnschedulableUntil, r.a.TempUnschedulableReason = nil, ""
	return nil
}
func (r *managedTestAccounts) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.a.Extra == nil {
		r.a.Extra = map[string]any{}
	}
	for k, v := range updates {
		r.a.Extra[k] = v
	}
	return nil
}
func (r *managedTestAccounts) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.a.Credentials = shallowCopyMap(credentials)
	return nil
}
func (r *managedTestAccounts) snapshot() Account {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *r.a
	return c
}

// managedTestRepo adds the managed columns to the dedicated-slot test store.
type managedTestRepo struct {
	*vmTestRepo
	accounts   *managedTestAccounts
	released   int
	reserveErr error
}

func (r *managedTestRepo) ReserveManaged(_ context.Context, id int64, runtime string, p *Proxy) (*OpenAIVMBinding, error) {
	if r.reserveErr != nil {
		return nil, r.reserveErr
	}
	a := r.accounts.snapshot()
	if !a.IsOpenAIVMManaged() || a.ProxyID == nil {
		return nil, errors.New("not a managed account")
	}
	r.reserves++
	p.ID = 900
	original := *a.ProxyID
	r.b = &OpenAIVMBinding{AccountID: id, Mode: OpenAIVMModeManaged, ManagedProxyID: p.ID, OriginalProxyID: &original, RuntimeType: runtime, Status: "provisioning", CreatedAt: time.Now()}
	r.accounts.mu.Lock()
	r.accounts.a.ProxyID = &p.ID
	r.accounts.a.Extra["openai_vm_bound"] = true
	r.accounts.mu.Unlock()
	b := *r.b
	return &b, nil
}
func (r *managedTestRepo) SetManaged(_ context.Context, _ int64, status string, detail json.RawMessage) error {
	r.b.ManagedStatus = status
	if len(detail) > 0 {
		r.b.ManagedDetail = detail
	}
	now := time.Now()
	r.b.SyncedAt = &now
	return nil
}
func (r *managedTestRepo) MarkActivated(context.Context, int64) (bool, error) {
	if r.b.ActivatedAt != nil {
		return false, nil
	}
	now := time.Now()
	r.b.ActivatedAt = &now
	return true, nil
}
func (r *managedTestRepo) ListManaged(context.Context) ([]*OpenAIVMBinding, error) {
	if r.b == nil || !r.b.Managed() {
		return nil, nil
	}
	b := *r.b
	return []*OpenAIVMBinding{&b}, nil
}
func (r *managedTestRepo) SetOriginalProxy(_ context.Context, _, proxyID int64) error {
	r.b.OriginalProxyID = &proxyID
	return nil
}
func (r *managedTestRepo) ReleaseManaged(context.Context, int64) error {
	r.released++
	r.b = nil
	return nil
}

type managedTestProxies struct {
	ProxyRepository
	byID map[int64]*Proxy
}

func (r *managedTestProxies) GetByID(_ context.Context, id int64) (*Proxy, error) {
	if p := r.byID[id]; p != nil {
		return p, nil
	}
	return nil, ErrProxyNotFound
}

// managedTestAdmin stands in for the admin service's account create/delete,
// running the same creation guard the real one runs.
type managedTestAdmin struct {
	accounts *managedTestAccounts
	creates  int
	deletes  int
	created  *CreateAccountInput
}

func (a *managedTestAdmin) CreateAccount(ctx context.Context, in *CreateAccountInput) (*Account, error) {
	if err := guardOpenAIVMManagedCreate(ctx, in); err != nil {
		return nil, err
	}
	a.creates++
	a.created = in
	account := &Account{ID: 7, Name: in.Name, Platform: in.Platform, Type: in.Type, Credentials: shallowCopyMap(in.Credentials), Extra: shallowCopyMap(in.Extra), ProxyID: in.ProxyID, Concurrency: in.Concurrency, Status: StatusActive, Schedulable: !in.Extra[OpenAIVMManagedExtraKey].(bool)}
	a.accounts.mu.Lock()
	a.accounts.a, a.accounts.deleted = account, false
	a.accounts.mu.Unlock()
	c := *account
	return &c, nil
}
func (a *managedTestAdmin) DeleteAccount(context.Context, int64) error {
	a.deletes++
	a.accounts.mu.Lock()
	a.accounts.deleted = true
	a.accounts.mu.Unlock()
	return nil
}

// managedBridge is a fake bridge. It records what Sub2API sent and answers
// with the state the test put in `remote`.
type managedBridge struct {
	mu       sync.Mutex
	requests []string
	bodies   []string
	headers  []http.Header
	remote   map[string]any
	fail     map[string]int
	codes    map[string]string
}

func (b *managedBridge) set(remote map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.remote = remote
}
func (b *managedBridge) seen() ([]string, []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.requests...), append([]string(nil), b.bodies...)
}
func (b *managedBridge) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		route := req.Method + " " + req.URL.Path
		b.mu.Lock()
		b.requests = append(b.requests, route)
		b.bodies = append(b.bodies, string(body))
		b.headers = append(b.headers, req.Header.Clone())
		remote, status := b.remote, b.fail[route]
		code := "account_busy"
		if override := b.codes[route]; override != "" {
			code = override
		}
		b.mu.Unlock()
		if req.Header.Get("Authorization") != "Bearer "+vmTestSecret {
			t.Errorf("%s reached the bridge without its secret", route)
		}
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"code":"`+code+`","message":"internal detail that must not be shown"}}`)
			return
		}
		switch {
		case route == "POST /v1/accounts/7/managed":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"account_id":7,"mode":"vm_managed","state":"awaiting_login","ready":false,"vm_id":"s2a-abcdefabcdef-7"}`)
		case route == "GET /v1/accounts/7/managed":
			_ = json.NewEncoder(w).Encode(remote)
		case route == "DELETE /v1/accounts/7":
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = io.WriteString(w, `{"ok":true}`)
		}
	}
}

type managedFixture struct {
	svc      *OpenAIVMManagedService
	vm       *OpenAIVMService
	repo     *managedTestRepo
	accounts *managedTestAccounts
	admin    *managedTestAdmin
	bridge   *managedBridge
}

func newManagedFixture(t *testing.T) *managedFixture {
	t.Helper()
	bridge := &managedBridge{fail: map[string]int{}, codes: map[string]string{}}
	server := httptest.NewServer(bridge.handler(t))
	t.Cleanup(server.Close)
	accounts := &managedTestAccounts{}
	base := &vmTestRepo{c: OpenAIVMConfig{Enabled: true, BaseURL: server.URL, ProxyURL: server.URL, EncryptedSecret: "encrypted:" + vmTestSecret, SecretConfigured: true}}
	repo := &managedTestRepo{vmTestRepo: base, accounts: accounts}
	proxies := &managedTestProxies{byID: map[int64]*Proxy{
		41: {ID: 41, Protocol: "socks5", Host: "exit-one.example", Port: 1080, Username: "u", Password: managedTestProxyPassword, Status: StatusActive},
		42: {ID: 42, Protocol: "socks5h", Host: "exit-two.example", Port: 1080, Status: StatusActive},
		43: {ID: 43, Protocol: "http", Host: "http-proxy.example", Port: 8080, Status: StatusActive},
		44: {ID: 44, Protocol: "socks5", Host: "off.example", Port: 1080, Status: StatusDisabled},
	}}
	vm := NewOpenAIVMService(repo, accounts, proxies, vmTestEncryptor{})
	admin := &managedTestAdmin{accounts: accounts}
	svc := NewOpenAIVMManagedService(vm, repo, admin)
	require.NotNil(t, svc)
	return &managedFixture{svc: svc, vm: vm, repo: repo, accounts: accounts, admin: admin, bridge: bridge}
}

func (f *managedFixture) create(t *testing.T) {
	t.Helper()
	account, view, err := f.svc.Create(context.Background(), OpenAIVMManagedCreateInput{Name: "managed seven", ProxyID: 41, Concurrency: 3})
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, OpenAIVMManagedAwaitingLogin, view.Status)
}

func loggedInRemote(extra map[string]any) map[string]any {
	snap := map[string]any{"status": "running", "schedulable": true, "schedule_state": "on", "has_token": true, "has_refresh": true, "email": "demo@example.test", "chatgpt_account_id": "org-7", "plan_type": "plus", "max_concurrency": 3}
	for k, v := range extra {
		snap[k] = v
	}
	return map[string]any{"account_id": 7, "mode": "vm_managed", "state": "ready", "ready": true, "vm_id": "s2a-abcdefabcdef-7", "snapshot": snap}
}

func TestManagedCreateMakesAnAccountWithoutCredentialAndAnEmptySlot(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	a := f.accounts.snapshot()
	require.True(t, a.IsOpenAIVMManaged())
	require.False(t, a.Schedulable, "an account without a login must not be schedulable")
	require.False(t, openAIVMManagedHasToken(a.Credentials))
	require.Equal(t, "kvm", f.repo.b.RuntimeType, "KVM is the default runtime")
	require.Equal(t, "awaiting_login", f.repo.b.Status)
	require.Equal(t, "s2a-abcdefabcdef-7", f.repo.b.SlotID)
	requests, bodies := f.bridge.seen()
	require.Equal(t, []string{"POST /v1/accounts/7/managed"}, requests)
	// The slot is created with the chosen exit and nothing that looks like a login.
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodies[0]), &sent))
	require.Equal(t, map[string]any{"name": "managed seven", "upstream_proxy_url": "socks5://u:" + managedTestProxyPassword + "@exit-one.example:1080", "runtime_type": "kvm"}, sent)
	// A slot that already exists is not created twice.
	_, err := f.svc.Retry(context.Background(), 7)
	require.NoError(t, err)
	requests, _ = f.bridge.seen()
	require.Len(t, requests, 1)
	require.Equal(t, 1, f.repo.reserves)
	// A slot nobody has logged in to is never opened to scheduling by the sync.
	f.bridge.set(map[string]any{"account_id": 7, "mode": "vm_managed", "state": "awaiting_login", "vm_id": "s2a-abcdefabcdef-7",
		"snapshot": map[string]any{"status": "stopped", "schedulable": false, "has_token": false}})
	view, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedAwaitingLogin, view.Status)
	require.False(t, f.accounts.snapshot().Schedulable)
	require.Empty(t, f.accounts.schedulable)
	require.Nil(t, view.ActivatedAt)
	require.Error(t, f.svc.SetSchedulable(context.Background(), 7, true), "scheduling cannot be opened by hand before a login either")
}

func TestManagedCreateValidatesInputBeforeAnythingIsCreated(t *testing.T) {
	for name, in := range map[string]OpenAIVMManagedCreateInput{
		"no name":        {ProxyID: 41},
		"no proxy":       {Name: "x"},
		"http proxy":     {Name: "x", ProxyID: 43},
		"disabled proxy": {Name: "x", ProxyID: 44},
		"unknown proxy":  {Name: "x", ProxyID: 99},
		"bad runtime":    {Name: "x", ProxyID: 41, RuntimeType: "lxc"},
		"concurrency":    {Name: "x", ProxyID: 41, Concurrency: 257},
	} {
		f := newManagedFixture(t)
		_, _, err := f.svc.Create(context.Background(), in)
		require.Errorf(t, err, name)
		if name == "no proxy" {
			require.Equal(t, "OPENAI_VM_PROXY_REQUIRED", infraerrors.Reason(err))
		}
		requests, _ := f.bridge.seen()
		require.Emptyf(t, requests, "%s reached the bridge", name)
		require.Zerof(t, f.admin.creates, "%s created an account", name)
	}
}

func TestManagedCreateKeepsTheAccountWhenTheSlotFailsAndRetryResumes(t *testing.T) {
	f := newManagedFixture(t)
	f.bridge.fail["POST /v1/accounts/7/managed"] = http.StatusBadGateway
	account, view, err := f.svc.Create(context.Background(), OpenAIVMManagedCreateInput{Name: "managed seven", ProxyID: 41})
	require.Error(t, err)
	require.NotNil(t, account, "the account is kept so the slot can be retried")
	require.Equal(t, "error", view.BindingStatus)
	require.Equal(t, OpenAIVMManagedError, view.Status)
	require.NotContains(t, view.LastError, "internal detail")
	require.NotContains(t, err.Error(), managedTestProxyPassword)
	require.Zero(t, f.admin.deletes)
	delete(f.bridge.fail, "POST /v1/accounts/7/managed")
	view, err = f.svc.Retry(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "awaiting_login", view.BindingStatus)
	require.Equal(t, 1, f.repo.reserves, "retry reuses the reserved route")
}

// When not even the route could be reserved there is nothing to retry, so the
// half-made account is removed instead of being left without slot or token.
func TestManagedCreateRemovesTheAccountWhenNothingWasReserved(t *testing.T) {
	f := newManagedFixture(t)
	f.repo.reserveErr = errors.New("reservation refused")
	account, view, err := f.svc.Create(context.Background(), OpenAIVMManagedCreateInput{Name: "managed seven", ProxyID: 41})
	require.Error(t, err)
	require.Nil(t, account)
	require.Nil(t, view)
	require.Equal(t, 1, f.admin.creates)
	require.Equal(t, 1, f.admin.deletes)
	requests, _ := f.bridge.seen()
	require.Empty(t, requests, "no slot may be created for an account that is being removed")
}

func TestManagedMarkCanOnlyBeSetByTheManagedService(t *testing.T) {
	proxy := int64(41)
	marked := func() *CreateAccountInput {
		return &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAIVMManagedExtraKey: true}, Credentials: map[string]any{}, ProxyID: &proxy}
	}
	// An ordinary create (form, import, sync) cannot produce a managed account.
	require.Error(t, guardOpenAIVMManagedCreate(context.Background(), marked()))
	ctx := withOpenAIVMManagedCreate(context.Background())
	require.NoError(t, guardOpenAIVMManagedCreate(ctx, marked()))
	withToken := marked()
	withToken.Credentials = map[string]any{"refresh_token": "rt"}
	require.Error(t, guardOpenAIVMManagedCreate(ctx, withToken), "a managed account never starts with a token")
	apiKey := marked()
	apiKey.Type = AccountTypeAPIKey
	require.Error(t, guardOpenAIVMManagedCreate(ctx, apiKey))
	noProxy := marked()
	noProxy.ProxyID = nil
	require.Error(t, guardOpenAIVMManagedCreate(ctx, noProxy))
	// Unmarked accounts are untouched by the guard.
	require.NoError(t, guardOpenAIVMManagedCreate(context.Background(), &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "at"}}))

	managed := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxy, Extra: map[string]any{OpenAIVMManagedExtraKey: true}}
	other := int64(42)
	for name, in := range map[string]*UpdateAccountInput{
		"access token":  {Credentials: map[string]any{"access_token": "at"}},
		"refresh token": {Credentials: map[string]any{"refresh_token": "rt"}},
		"type":          {Type: AccountTypeAPIKey},
		"proxy":         {ProxyID: &other},
		"proxy pool":    {ProxyIDs: &[]int64{41, 42}},
	} {
		require.Errorf(t, guardOpenAIVMManagedUpdate(managed, in), name)
	}
	require.NoError(t, guardOpenAIVMManagedUpdate(managed, &UpdateAccountInput{Name: "renamed", Credentials: map[string]any{"model_mapping": map[string]any{}}, ProxyID: &proxy}))
	plain := &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NoError(t, guardOpenAIVMManagedUpdate(plain, &UpdateAccountInput{Credentials: map[string]any{"access_token": "at"}, ProxyID: &other}))
}

func TestManagedLoginOpensSchedulingOnceAndCopiesIdentityNotTokens(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	usage := map[string]any{"limits": map[string]any{"used_5h_percent": 12.5, "reset_5h_at": "2026-11-12T15:00:00Z", "window_5h_minutes": 300, "used_7d_percent": 40, "reset_7d_seconds": 3600}, "snapshot": map[string]any{"updated_at": "2026-11-12T10:00:00Z"}}
	f.bridge.set(loggedInRemote(map[string]any{"usage": usage, "access_token": "must-never-be-copied",
		"saved_login": map[string]any{"email": "demo@example.test", "has_password": true, "has_mfa_secret": true, "password": "saved-password-never-copied", "mfa_secret": "JBSWY3DPEHPK3PXP"}}))
	view, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedAvailable, view.Status)
	require.Equal(t, "ready", view.BindingStatus)
	a := f.accounts.snapshot()
	require.True(t, a.Schedulable)
	require.Equal(t, []bool{true}, f.accounts.schedulable)
	require.Equal(t, "demo@example.test", a.GetCredential("email"))
	require.Equal(t, "org-7", a.GetCredential("chatgpt_account_id"))
	require.Equal(t, "plus", a.GetCredential("plan_type"))
	require.False(t, openAIVMManagedHasToken(a.Credentials), "the slot's token must never be stored in Sub2API")
	// The stored and displayed slot report is an allow-list, not whatever arrived.
	stored, _ := json.Marshal([]any{a.Credentials, a.Extra, f.repo.b, view})
	for _, secret := range []string{"must-never-be-copied", "saved-password-never-copied", "JBSWY3DPEHPK3PXP"} {
		require.NotContains(t, string(stored), secret)
	}
	require.JSONEq(t, `{"email":"demo@example.test","has_password":true,"has_mfa_secret":true}`, string(mustField(t, view.Snapshot, "saved_login")))
	require.JSONEq(t, `"plus"`, string(mustField(t, view.Snapshot, "plan_type")))
	require.Equal(t, 12.5, a.Extra["codex_5h_used_percent"])
	require.Equal(t, "2026-11-12T15:00:00Z", a.Extra["codex_5h_reset_at"])
	require.Equal(t, float64(40), a.Extra["codex_7d_used_percent"])
	require.Equal(t, 3600, a.Extra["codex_7d_reset_after_seconds"])
	require.Equal(t, OpenAIVMManagedAvailable, a.Extra[OpenAIVMManagedStatusExtraKey])

	// The administrator switches scheduling off; the sync must not turn it back on.
	require.NoError(t, f.svc.SetSchedulable(context.Background(), 7, false))
	_, err = f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.False(t, f.accounts.snapshot().Schedulable)
	require.Equal(t, []bool{true, false}, f.accounts.schedulable)
}

func TestManagedStateSyncPausesWithoutDisablingAndResumes(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	f.bridge.set(loggedInRemote(nil))
	_, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)

	until := time.Now().Add(2 * time.Hour)
	f.bridge.set(loggedInRemote(map[string]any{"schedule_state": "restricted", "restriction_reason": "quota_7d_exhausted", "restriction_text": "7d", "restriction_until": until.UnixMilli()}))
	view, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedRestricted, view.Status)
	a := f.accounts.snapshot()
	require.Equal(t, StatusActive, a.Status, "a restricted slot is not an account error")
	require.True(t, a.Schedulable, "the administrator's switch is left alone")
	require.Equal(t, openAIVMManagedPausePrefix+"restricted:quota_7d_exhausted", a.TempUnschedulableReason)
	require.WithinDuration(t, until, *a.TempUnschedulableUntil, time.Second, "the pause lasts as long as the runtime says")

	// The credential dies: the bridge closes the binding, the account waits for a new login.
	dead := loggedInRemote(map[string]any{"refresh_error": "invalid_grant", "auto_relogin": map[string]any{"last_result": "running"}})
	dead["state"], dead["ready"] = "awaiting_login", false
	f.bridge.set(dead)
	view, err = f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedReloginRunning, view.Status)
	require.Equal(t, "awaiting_login", view.BindingStatus, "requests stop being routed to a slot that can only answer 401")
	// The automatic attempt was refused: a person is needed.
	dead["snapshot"].(map[string]any)["auto_relogin"] = map[string]any{"last_result": "failed", "needs_attention": true}
	f.bridge.set(dead)
	view, err = f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedNeedsAttention, view.Status)
	a = f.accounts.snapshot()
	require.Equal(t, StatusActive, a.Status)
	require.Empty(t, f.accounts.errors, "a managed account is never set to error by the sync")
	require.True(t, strings.HasPrefix(a.TempUnschedulableReason, openAIVMManagedPausePrefix))

	// Logged in again: only this feature's own pause is lifted.
	f.bridge.set(loggedInRemote(nil))
	view, err = f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedAvailable, view.Status)
	require.Equal(t, "ready", view.BindingStatus)
	require.Nil(t, f.accounts.snapshot().TempUnschedulableUntil)
	require.Equal(t, 1, f.accounts.clears)
	require.Equal(t, []bool{true}, f.accounts.schedulable, "scheduling is opened once, at the first login")
}

func TestManagedSyncLeavesOtherPausesAndUnreadableSlotsAlone(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	f.bridge.set(loggedInRemote(nil))
	_, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	// A pause written by another subsystem stays, even when the slot is fine.
	other := time.Now().Add(time.Hour)
	f.accounts.mu.Lock()
	f.accounts.a.TempUnschedulableUntil, f.accounts.a.TempUnschedulableReason = &other, "manual pause"
	f.accounts.mu.Unlock()
	_, err = f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "manual pause", f.accounts.snapshot().TempUnschedulableReason)
	require.Zero(t, f.accounts.clears)

	// The runtime cannot be read: nothing about the account changes.
	f.accounts.mu.Lock()
	f.accounts.a.TempUnschedulableUntil, f.accounts.a.TempUnschedulableReason = nil, ""
	f.accounts.mu.Unlock()
	pauses := len(f.accounts.pauses)
	unreadable := map[string]any{"account_id": 7, "mode": "vm_managed", "state": "ready", "ready": true, "vm_id": "s2a-abcdefabcdef-7", "last_error_code": "snapshot_unavailable"}
	f.bridge.set(unreadable)
	view, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedAvailable, view.Status, "the last known status is kept")
	require.Len(t, f.accounts.pauses, pauses)
	// The bridge is down: the last synced view is still returned.
	f.bridge.fail["GET /v1/accounts/7/managed"] = http.StatusBadGateway
	view, err = f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenAIVMManagedAvailable, view.Status)
	require.Len(t, f.accounts.pauses, pauses)
}

func TestManagedConcurrencyFollowsTheAccount(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	f.bridge.set(loggedInRemote(map[string]any{"max_concurrency": 1}))
	_, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	requests, bodies := f.bridge.seen()
	last := len(requests) - 1
	require.Equal(t, "PUT /v1/accounts/7/managed/policy", requests[last])
	require.JSONEq(t, `{"max_concurrency":3}`, bodies[last])
	// Already equal: nothing is sent.
	f.bridge.set(loggedInRemote(nil))
	_, err = f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	after, _ := f.bridge.seen()
	require.Equal(t, "GET /v1/accounts/7/managed", after[len(after)-1])
}

func TestManagedCallRelaysTheBodyUntouchedAndHidesBridgeDetail(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	login := `{"email":"demo@example.test","password":"account-password-never-stored","mfa_mode":"secret","mfa_secret":"JBSWY3DPEHPK3PXP"}`
	out, err := f.svc.Call(context.Background(), 7, http.MethodPost, "login/password", []byte(login))
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(out))
	requests, bodies := f.bridge.seen()
	require.Equal(t, "POST /v1/accounts/7/managed/login/password", requests[len(requests)-1])
	require.Equal(t, login, bodies[len(bodies)-1])
	// Nothing of it stays on this side.
	a := f.accounts.snapshot()
	stored, _ := json.Marshal([]any{a.Credentials, a.Extra, f.repo.b})
	require.NotContains(t, string(stored), "account-password-never-stored")
	require.NotContains(t, string(stored), "JBSWY3DPEHPK3PXP")

	// A GET carries no body; an empty POST becomes an empty object.
	_, err = f.svc.Call(context.Background(), 7, http.MethodGet, "login/password", []byte(`{"ignored":true}`))
	require.NoError(t, err)
	_, err = f.svc.Call(context.Background(), 7, http.MethodPost, "power/on", nil)
	require.NoError(t, err)
	_, bodies = f.bridge.seen()
	require.Equal(t, "", bodies[len(bodies)-2])
	require.Equal(t, "{}", bodies[len(bodies)-1])

	// A refusal keeps the bridge's short code and nothing else.
	f.bridge.fail["POST /v1/accounts/7/managed/token/refresh"] = http.StatusConflict
	_, err = f.svc.Call(context.Background(), 7, http.MethodPost, "token/refresh", nil)
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	require.Equal(t, "account_busy", OpenAIVMManagedErrorCode(err))
	require.NotContains(t, err.Error(), "internal detail")
	// A 401 from the bridge must not look like the admin's own session expiring.
	f.bridge.fail["POST /v1/accounts/7/managed/recover"] = http.StatusUnauthorized
	_, err = f.svc.Call(context.Background(), 7, http.MethodPost, "recover", nil)
	require.Error(t, err)
	require.NotEqual(t, http.StatusUnauthorized, infraerrors.Code(err))
	require.NotEqual(t, http.StatusForbidden, infraerrors.Code(err))

	_, err = f.svc.Call(context.Background(), 7, http.MethodPost, "login/import", []byte(strings.Repeat("x", openAIVMManagedRequestLimit+1)))
	require.Error(t, err)
}

// Each layer of "this is not a managed account" is checked on its own: a
// token-holding account without a slot, a managed account whose slot is
// token-fed, and a token-holding account with a token-fed slot.
func TestManagedRetryAndDeleteRefuseEachKindOfForeignAccount(t *testing.T) {
	proxy := int64(41)
	plain := func() *Account {
		return &Account{ID: 7, Name: "plain", Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxy, Credentials: map[string]any{"access_token": "at"}, Extra: map[string]any{}, Status: StatusActive}
	}
	marked := func() *Account {
		a := plain()
		a.Credentials, a.Extra = map[string]any{}, map[string]any{OpenAIVMManagedExtraKey: true, "openai_vm_bound": true}
		return a
	}
	tokenFed := func() *OpenAIVMBinding {
		return &OpenAIVMBinding{AccountID: 7, Mode: OpenAIVMModeSubToken, Status: "error", RuntimeType: "docker", OriginalProxyID: &proxy, CreatedAt: time.Now()}
	}
	for name, tc := range map[string]struct {
		account *Account
		binding *OpenAIVMBinding
	}{
		"ordinary account, no slot":        {plain(), nil},
		"managed account, token-fed slot":  {marked(), tokenFed()},
		"ordinary account, token-fed slot": {plain(), tokenFed()},
	} {
		f := newManagedFixture(t)
		f.accounts.a, f.repo.account, f.repo.b = tc.account, tc.account, tc.binding
		_, err := f.svc.Retry(context.Background(), 7)
		require.Equalf(t, "OPENAI_VM_NOT_MANAGED", infraerrors.Reason(err), "%s: retry", name)
		require.Equalf(t, "OPENAI_VM_NOT_MANAGED", infraerrors.Reason(f.svc.Delete(context.Background(), 7)), "%s: delete", name)
		requests, _ := f.bridge.seen()
		require.Emptyf(t, requests, "%s reached the bridge", name)
		require.Zerof(t, f.repo.reserves+f.repo.released+f.admin.deletes, "%s changed something", name)
		if tc.binding != nil {
			require.Equalf(t, "error", f.repo.b.Status, "%s: the token-fed slot was touched", name)
			require.Equal(t, OpenAIVMModeSubToken, f.repo.b.Mode)
		}
	}
}

func TestManagedOperationsRefuseAccountsThatAreNotManaged(t *testing.T) {
	f := newManagedFixture(t)
	plain := &Account{ID: 7, Name: "plain", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "at"}, Status: StatusActive}
	f.accounts.a = plain
	f.repo.account = plain
	f.repo.b = &OpenAIVMBinding{AccountID: 7, Mode: OpenAIVMModeSubToken, Status: "ready", CreatedAt: time.Now()}
	_, err := f.svc.Call(context.Background(), 7, http.MethodPost, "login/password", []byte(`{}`))
	require.Error(t, err)
	_, err = f.svc.Status(context.Background(), 7)
	require.Error(t, err)
	_, err = f.svc.ChangeProxy(context.Background(), 7, 42)
	require.Error(t, err)
	require.Error(t, f.svc.Delete(context.Background(), 7))
	_, err = f.svc.Retry(context.Background(), 7)
	require.Error(t, err)
	requests, _ := f.bridge.seen()
	require.Empty(t, requests)
	require.Equal(t, "ready", f.repo.b.Status)
	require.Zero(t, f.admin.deletes)
}

func TestManagedProxyChangeMovesTheSlotFirst(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	for _, bad := range []int64{0, 43, 44, 99} {
		_, err := f.svc.ChangeProxy(context.Background(), 7, bad)
		require.Error(t, err)
	}
	requests, _ := f.bridge.seen()
	require.Len(t, requests, 1, "an unusable proxy never reaches the bridge")
	// The bridge refuses (requests in flight): the recorded exit stays.
	f.bridge.fail["PUT /v1/accounts/7/managed/proxy"] = http.StatusConflict
	_, err := f.svc.ChangeProxy(context.Background(), 7, 42)
	require.Error(t, err)
	require.Equal(t, int64(41), *f.repo.b.OriginalProxyID)
	delete(f.bridge.fail, "PUT /v1/accounts/7/managed/proxy")
	f.bridge.set(map[string]any{"account_id": 7, "mode": "vm_managed", "state": "awaiting_login", "vm_id": "s2a-abcdefabcdef-7"})
	_, err = f.svc.ChangeProxy(context.Background(), 7, 42)
	require.NoError(t, err)
	require.Equal(t, int64(42), *f.repo.b.OriginalProxyID)
	requests, bodies := f.bridge.seen()
	for i, route := range requests {
		if route == "PUT /v1/accounts/7/managed/proxy" {
			require.JSONEq(t, `{"upstream_proxy_url":"socks5h://exit-two.example:1080"}`, bodies[i])
		}
	}
	// The same proxy again is a no-op.
	before := len(requests)
	_, err = f.svc.ChangeProxy(context.Background(), 7, 42)
	require.NoError(t, err)
	requests, _ = f.bridge.seen()
	for _, route := range requests[before:] {
		require.NotEqual(t, "PUT /v1/accounts/7/managed/proxy", route)
	}
}

func TestManagedDeleteDestroysTheSlotBeforeTheAccount(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	f.bridge.fail["DELETE /v1/accounts/7"] = http.StatusConflict
	require.Error(t, f.svc.Delete(context.Background(), 7))
	require.Equal(t, "delete_failed", f.repo.b.Status)
	require.Zero(t, f.admin.deletes, "the account stays while its slot still exists")
	require.Zero(t, f.repo.released)
	// Nothing but the deletion itself is relayed to a slot that is being removed.
	before, _ := f.bridge.seen()
	_, err := f.svc.Call(context.Background(), 7, http.MethodPost, "login/password", []byte(`{}`))
	require.Error(t, err)
	_, err = f.svc.ChangeProxy(context.Background(), 7, 42)
	require.Error(t, err)
	view, err := f.svc.Status(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "delete_failed", view.BindingStatus, "the state sync leaves a slot that is being removed alone")
	after, _ := f.bridge.seen()
	require.Equal(t, before, after)
	delete(f.bridge.fail, "DELETE /v1/accounts/7")
	require.NoError(t, f.svc.Delete(context.Background(), 7))
	require.Nil(t, f.repo.b)
	require.Equal(t, 1, f.repo.released)
	require.Equal(t, 1, f.admin.deletes)

	// A slot the bridge no longer knows is already gone: deletion still completes.
	g := newManagedFixture(t)
	g.create(t)
	g.bridge.fail["DELETE /v1/accounts/7"] = http.StatusNotFound
	g.bridge.codes["DELETE /v1/accounts/7"] = "account_not_found"
	require.NoError(t, g.svc.Delete(context.Background(), 7))
	require.Equal(t, 1, g.repo.released)
	require.Equal(t, 1, g.admin.deletes)
	// Any other 404 is not proof that the slot is gone.
	h := newManagedFixture(t)
	h.create(t)
	h.bridge.fail["DELETE /v1/accounts/7"] = http.StatusNotFound
	h.bridge.codes["DELETE /v1/accounts/7"] = "not_found"
	require.Error(t, h.svc.Delete(context.Background(), 7))
	require.Zero(t, h.repo.released)
	require.Zero(t, h.admin.deletes)
}

func TestDeriveManagedStatus(t *testing.T) {
	now := time.Now()
	snap := func(edit func(*openAIVMManagedSnapshot)) *openAIVMManagedSnapshot {
		s := &openAIVMManagedSnapshot{Status: "running", Schedulable: true, ScheduleState: "on", HasToken: true}
		if edit != nil {
			edit(s)
		}
		return s
	}
	for name, tc := range map[string]struct {
		state     string
		snap      *openAIVMManagedSnapshot
		activated bool
		want      string
	}{
		"never logged in":    {"awaiting_login", snap(func(s *openAIVMManagedSnapshot) { s.HasToken = false }), false, OpenAIVMManagedAwaitingLogin},
		"lost its login":     {"awaiting_login", snap(func(s *openAIVMManagedSnapshot) { s.RefreshError = "invalid_grant" }), true, OpenAIVMManagedNeedsRelogin},
		"available":          {"ready", snap(nil), true, OpenAIVMManagedAvailable},
		"refresh rejected":   {"ready", snap(func(s *openAIVMManagedSnapshot) { s.RefreshError = "invalid_grant" }), true, OpenAIVMManagedNeedsRelogin},
		"token gone":         {"ready", snap(func(s *openAIVMManagedSnapshot) { s.HasToken = false }), true, OpenAIVMManagedNeedsRelogin},
		"slot paused flag":   {"ready", snap(func(s *openAIVMManagedSnapshot) { s.Schedulable = false }), true, OpenAIVMManagedPaused},
		"stopped":            {"ready", snap(func(s *openAIVMManagedSnapshot) { s.Status = "stopped" }), true, OpenAIVMManagedStopped},
		"restricted":         {"ready", snap(func(s *openAIVMManagedSnapshot) { s.ScheduleState = "restricted" }), true, OpenAIVMManagedRestricted},
		"paused on the slot": {"ready", snap(func(s *openAIVMManagedSnapshot) { s.ScheduleState = "off" }), true, OpenAIVMManagedPaused},
		"credential reason": {"ready", snap(func(s *openAIVMManagedSnapshot) {
			s.ScheduleState, s.UnschedulableReason = "unschedulable", "oauth_revoked"
		}), true, OpenAIVMManagedNeedsRelogin},
		"organisation closed": {"ready", snap(func(s *openAIVMManagedSnapshot) {
			s.ScheduleState, s.UnschedulableReason = "unschedulable", "organization_disabled"
		}), true, OpenAIVMManagedNeedsAttention},
		"slot fault": {"ready", snap(func(s *openAIVMManagedSnapshot) {
			s.ScheduleState, s.UnschedulableReason = "unschedulable", "runtime_start_failed"
		}), true, OpenAIVMManagedSlotError},
		"unreadable":    {"ready", nil, true, OpenAIVMManagedUnknown},
		"first boot":    {"awaiting_login", snap(nil), false, OpenAIVMManagedStarting},
		"booting again": {"ready", snap(nil), true, OpenAIVMManagedStarting},
		"boot failed":   {"awaiting_login", snap(nil), false, OpenAIVMManagedSlotError},
		"failed create": {"failed", nil, false, OpenAIVMManagedError},
		"unknown state": {"surprise", snap(nil), true, OpenAIVMManagedUnknown},
	} {
		remote := &openAIVMManagedRemote{State: tc.state, Snapshot: tc.snap}
		switch name {
		case "first boot", "booting again":
			remote.Starting = true
		case "boot failed":
			remote.LastErrorCode = "activate_failed"
		}
		got, _ := deriveOpenAIVMManagedStatus(remote, tc.activated, now)
		require.Equalf(t, tc.want, got, name)
	}
	// A restriction end in the past is not a deadline.
	_, until := deriveOpenAIVMManagedStatus(&openAIVMManagedRemote{State: "ready", Snapshot: snap(func(s *openAIVMManagedSnapshot) {
		s.ScheduleState, s.RestrictionUntil = "restricted", now.Add(-time.Minute).UnixMilli()
	})}, true, now)
	require.True(t, until.IsZero())
	status, _ := deriveOpenAIVMManagedStatus(nil, true, now)
	require.Equal(t, OpenAIVMManagedUnknown, status)
	require.Nil(t, openAIVMManagedUsageExtra(json.RawMessage(`{"limits":{}}`)))
	require.Nil(t, openAIVMManagedUsageExtra(json.RawMessage(`not json`)))
}

func mustField(t *testing.T, raw json.RawMessage, name string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.Contains(t, fields, name)
	return fields[name]
}

// A slot VM2 refused because the chosen exit is full keeps that reason all the
// way to the administrator, and the account stays so it can be removed.
func TestManagedCreateReportsAFullExitByItsReason(t *testing.T) {
	f := newManagedFixture(t)
	f.bridge.fail["POST /v1/accounts/7/managed"] = http.StatusConflict
	f.bridge.codes["POST /v1/accounts/7/managed"] = "proxy_bind_limit"
	account, view, err := f.svc.Create(context.Background(), OpenAIVMManagedCreateInput{Name: "managed seven", ProxyID: 41})
	require.Error(t, err)
	require.NotNil(t, account)
	require.Equal(t, "proxy_bind_limit", OpenAIVMManagedErrorCode(err))
	require.Equal(t, "error", view.BindingStatus)
	require.Equal(t, OpenAIVMManagedError, view.Status)
	require.Contains(t, view.LastError, "proxy_bind_limit")
	require.NotContains(t, view.LastError, "internal detail")
	require.Zero(t, f.admin.deletes)
}

// A delete the bridge refuses because the slot is still booting changes
// nothing: the binding keeps its status, nothing is released, and the same
// call works once the slot is up.
func TestManagedDeleteDuringBootLeavesTheBindingAlone(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	before := f.repo.b.Status
	f.bridge.fail["DELETE /v1/accounts/7"] = http.StatusConflict
	f.bridge.codes["DELETE /v1/accounts/7"] = "slot_starting"
	err := f.svc.Delete(context.Background(), 7)
	require.Error(t, err)
	require.Equal(t, "slot_starting", OpenAIVMManagedErrorCode(err))
	require.Equal(t, before, f.repo.b.Status)
	require.Empty(t, f.repo.b.LastError)
	require.Zero(t, f.repo.released)
	require.Zero(t, f.admin.deletes)
	delete(f.bridge.fail, "DELETE /v1/accounts/7")
	require.NoError(t, f.svc.Delete(context.Background(), 7))
	require.Equal(t, 1, f.admin.deletes)
}
