package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// OpenAIVMManagedStatusExtraKey mirrors the managed status onto the account
	// so the account list can show it without a second query.
	OpenAIVMManagedStatusExtraKey = "openai_vm_managed_status"

	openAIVMManagedSyncInterval = time.Minute
	// A slot that cannot serve is kept out of selection with a rolling pause the
	// sync renews; it lapses by itself if the sync stops.
	openAIVMManagedSyncPause    = 3 * time.Minute
	openAIVMManagedResponseMax  = 1 << 20
	openAIVMManagedRequestLimit = 256 << 10
)

// openAIVMManagedAccounts is the slice of the admin service the managed
// service needs: creating and deleting the Sub2API account row.
type openAIVMManagedAccounts interface {
	CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error)
	DeleteAccount(ctx context.Context, id int64) error
}

// OpenAIVMManagedService runs accounts whose login and tokens live in a VM2
// slot. Sub2API creates the slot through the bridge, relays login and
// maintenance operations to it, and copies the slot's state back; it never
// receives, stores or forwards the account's token, password or 2FA secret.
type OpenAIVMManagedService struct {
	vm       *OpenAIVMService
	repo     OpenAIVMManagedRepository
	accounts AccountRepository
	proxies  ProxyRepository
	admin    openAIVMManagedAccounts

	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
	stop      chan struct{}
	done      chan struct{}
}

func NewOpenAIVMManagedService(vm *OpenAIVMService, repo OpenAIVMManagedRepository, admin openAIVMManagedAccounts) *OpenAIVMManagedService {
	if vm == nil || repo == nil {
		return nil
	}
	return &OpenAIVMManagedService{vm: vm, repo: repo, accounts: vm.accounts, proxies: vm.proxies, admin: admin, stop: make(chan struct{}), done: make(chan struct{})}
}

type OpenAIVMManagedCreateInput struct {
	LoginEmail            string                   `json:"-"`
	Settings              *OpenAIVMManagedSettings `json:"settings,omitempty"`
	Name                  string                   `json:"name"`
	Notes                 *string                  `json:"notes"`
	GroupIDs              []int64                  `json:"group_ids"`
	ProxyID               int64                    `json:"proxy_id"`
	RuntimeType           string                   `json:"runtime_type"`
	Concurrency           int                      `json:"concurrency"`
	Priority              int                      `json:"priority"`
	SkipMixedChannelCheck bool                     `json:"-"`
}

// OpenAIVMManagedView is what the managed panel shows. Snapshot is the bridge's
// already-reduced slot report: no token, password, 2FA secret or proxy credential.
type OpenAIVMManagedView struct {
	AccountID     int64           `json:"account_id"`
	Status        string          `json:"status"`
	BindingStatus string          `json:"binding_status"`
	RuntimeType   string          `json:"runtime_type"`
	SlotID        string          `json:"slot_id"`
	ProxyID       *int64          `json:"proxy_id"`
	LastError     string          `json:"last_error,omitempty"`
	ActivatedAt   *time.Time      `json:"activated_at,omitempty"`
	SyncedAt      *time.Time      `json:"synced_at,omitempty"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
}

func openAIVMManagedView(b *OpenAIVMBinding) *OpenAIVMManagedView {
	if b == nil {
		return nil
	}
	return &OpenAIVMManagedView{AccountID: b.AccountID, Status: b.ManagedStatus, BindingStatus: b.Status, RuntimeType: b.RuntimeType, SlotID: b.SlotID, ProxyID: b.OriginalProxyID, LastError: b.LastError, ActivatedAt: b.ActivatedAt, SyncedAt: b.SyncedAt, Snapshot: b.ManagedDetail}
}

// bridge sends one management request and returns the bridge's JSON answer.
// A failure is reduced to the bridge's short error code; the request body,
// which can hold a password or a 2FA secret, is never logged or echoed.
func (s *OpenAIVMManagedService) bridge(ctx context.Context, method, path string, body []byte) (json.RawMessage, error) {
	c, key, err := s.vm.configured(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	var payload io.Reader
	if body != nil {
		payload = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.vm.client.Do(req)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_VM_BRIDGE_UNREACHABLE", "The VM bridge could not be reached")
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, openAIVMManagedResponseMax+1))
	if err != nil || len(data) > openAIVMManagedResponseMax {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_VM_BRIDGE_PROTOCOL", "The VM bridge returned an invalid response")
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if len(bytes.TrimSpace(data)) == 0 {
			return json.RawMessage("{}"), nil
		}
		if !json.Valid(data) {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_VM_BRIDGE_PROTOCOL", "The VM bridge returned an invalid response")
		}
		return data, nil
	}
	return nil, openAIVMManagedBridgeError(resp.StatusCode, data)
}

// openAIVMManagedBridgeError keeps the bridge's error code and drops the rest.
// A 401/403 is never passed on as such: the admin client would read it as its
// own session being rejected.
func openAIVMManagedBridgeError(status int, body []byte) error {
	var parsed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	code := strings.TrimSpace(parsed.Error.Code)
	if len(code) > 64 || strings.ContainsAny(code, " \t\r\n<>\"'") {
		code = ""
	}
	if code == "unauthorized" {
		return infraerrors.BadRequest("OPENAI_VM_BRIDGE_AUTH", "The VM bridge rejected its shared secret")
	}
	if code == "" {
		code = "bridge_error"
	}
	out := http.StatusBadGateway
	switch {
	case status == http.StatusConflict:
		out = http.StatusConflict
	case status == http.StatusNotFound || status == http.StatusBadRequest || status == http.StatusUnprocessableEntity || status == http.StatusTooManyRequests:
		out = http.StatusBadRequest
	}
	return infraerrors.New(out, "OPENAI_VM_MANAGED_REJECTED", "The slot refused this operation: "+code).WithMetadata(map[string]string{"code": code})
}

func (s *OpenAIVMManagedService) socksProxy(ctx context.Context, id int64) (*Proxy, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("OPENAI_VM_PROXY_REQUIRED", "A VM2-managed account requires a SOCKS5 proxy")
	}
	p, err := s.proxies.GetByID(ctx, id)
	if err != nil || p == nil {
		return nil, infraerrors.BadRequest("OPENAI_VM_PROXY_UNAVAILABLE", "The selected proxy does not exist")
	}
	if !p.IsActive() || p.IsExpired(time.Now()) {
		return nil, infraerrors.BadRequest("OPENAI_VM_PROXY_UNAVAILABLE", "The selected proxy is inactive or expired")
	}
	if p.Protocol != "socks5" && p.Protocol != "socks5h" {
		return nil, infraerrors.BadRequest("OPENAI_VM_PROXY_PROTOCOL", "A VM2-managed account requires a SOCKS5 proxy")
	}
	return p, nil
}

func normalizeOpenAIVMManagedRuntime(runtime string) (string, error) {
	switch strings.TrimSpace(runtime) {
	case "", "kvm":
		return "kvm", nil
	case "docker":
		return "docker", nil
	}
	return "", infraerrors.BadRequest("OPENAI_VM_RUNTIME", "Runtime must be kvm or docker")
}

// Create makes the Sub2API account (no credential, not schedulable) and its
// empty slot. The account is returned even when the slot could not be created:
// the binding then records the error and Retry resumes from there.
func (s *OpenAIVMManagedService) Create(ctx context.Context, in OpenAIVMManagedCreateInput) (*Account, *OpenAIVMManagedView, error) {
	if err := in.Settings.validate(); err != nil {
		return nil, nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 200 || strings.ContainsAny(name, "\r\n") {
		return nil, nil, infraerrors.BadRequest("OPENAI_VM_MANAGED_NAME", "An account name is required")
	}
	runtime, err := normalizeOpenAIVMManagedRuntime(in.RuntimeType)
	if err != nil {
		return nil, nil, err
	}
	if _, _, err = s.vm.configured(ctx); err != nil {
		return nil, nil, err
	}
	if _, err = s.socksProxy(ctx, in.ProxyID); err != nil {
		return nil, nil, err
	}
	concurrency := in.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 256 {
		return nil, nil, infraerrors.BadRequest("OPENAI_VM_MANAGED_CONCURRENCY", "Concurrency must be between 1 and 256")
	}
	proxyID := in.ProxyID
	createInput := &CreateAccountInput{
		Name: name, Notes: in.Notes, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{}, Extra: map[string]any{OpenAIVMManagedExtraKey: true},
		ProxyID: &proxyID, Concurrency: concurrency, Priority: in.Priority, GroupIDs: in.GroupIDs,
		SkipMixedChannelCheck: in.SkipMixedChannelCheck,
	}
	in.Settings.apply(createInput)
	if validOpenAIVMBatchEmail(in.LoginEmail) {
		createInput.Credentials["email"] = in.LoginEmail
	}
	account, err := s.admin.CreateAccount(withOpenAIVMManagedCreate(ctx), createInput)
	if err != nil {
		return nil, nil, err
	}

	if in.Settings != nil && len(in.Settings.GroupAllowedModels) > 0 {
		updater, ok := s.admin.(interface {
			UpdateAccount(context.Context, int64, *UpdateAccountInput) (*Account, error)
		})
		if !ok {
			err = infraerrors.BadRequest("OPENAI_VM_SETTINGS_INVALID", "Group model settings unavailable")
		} else {
			_, err = updater.UpdateAccount(ctx, account.ID, &UpdateAccountInput{GroupAllowedModels: in.Settings.GroupAllowedModels})
		}
		if err != nil {
			clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_ = s.admin.DeleteAccount(withOpenAIVMManagedDelete(clean), account.ID)
			return nil, nil, err
		}
	}
	binding, err := s.provision(ctx, account.ID, runtime)
	if err != nil && binding == nil {
		// Nothing was reserved, so nothing can be retried: do not leave an
		// account behind that has neither a slot nor a token.
		cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if delErr := s.admin.DeleteAccount(withOpenAIVMManagedDelete(cleanCtx), account.ID); delErr != nil {
			slog.Warn("openai_vm_managed_create_cleanup_failed", "account_id", account.ID, "error", delErr)
		}
		return nil, nil, err
	}
	if fresh, getErr := s.accounts.GetByID(ctx, account.ID); getErr == nil && fresh != nil {
		account = fresh
	}
	return account, openAIVMManagedView(binding), err
}

// Retry resumes a slot creation that failed part-way.
func (s *OpenAIVMManagedService) Retry(ctx context.Context, id int64) (*OpenAIVMManagedView, error) {
	b, err := s.vm.repo.GetBinding(ctx, id)
	if err != nil {
		return nil, err
	}
	runtime := "kvm"
	if b != nil {
		runtime = b.RuntimeType
	}
	b, err = s.provision(ctx, id, runtime)
	return openAIVMManagedView(b), err
}

func (s *OpenAIVMManagedService) provision(ctx context.Context, id int64, runtime string) (*OpenAIVMBinding, error) {
	// Leave time for cancellation cleanup before the repository lock expires.
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	var result *OpenAIVMBinding
	err := s.vm.repo.WithLock(ctx, id, func() error {
		a, err := s.accounts.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if !a.IsOpenAIVMManaged() {
			return infraerrors.BadRequest("OPENAI_VM_NOT_MANAGED", "This account is not VM2-managed")
		}
		c, key, err := s.vm.configured(ctx)
		if err != nil {
			return err
		}
		b, err := s.vm.repo.GetBinding(ctx, id)
		if err != nil {
			return err
		}
		if b != nil && !b.Managed() {
			return infraerrors.Conflict("OPENAI_VM_NOT_MANAGED", "This account already has a dedicated slot fed by Sub2API tokens")
		}
		if b != nil && (b.Status == "deleting" || b.Status == "delete_failed") {
			return infraerrors.Conflict("OPENAI_VM_DELETING", "This account is being deleted; retry the deletion")
		}
		if b == nil {
			p, err := openAIVMProxy(c, key, id)
			if err != nil {
				return err
			}
			if b, err = s.repo.ReserveManaged(ctx, id, runtime, p); err != nil {
				return err
			}
			s.vm.refreshSnapshots(ctx, id)
		}
		result = b
		if b.Status == "ready" || b.Status == "awaiting_login" {
			return nil
		}
		fail := func(cause error) error {
			cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			stateErr := s.vm.repo.SetState(cleanCtx, id, "error", "", infraerrors.Message(cause))
			_ = s.repo.SetManaged(cleanCtx, id, OpenAIVMManagedError, nil)
			if fresh, getErr := s.vm.repo.GetBinding(cleanCtx, id); getErr == nil && fresh != nil {
				result = fresh
			}
			return errors.Join(cause, stateErr)
		}
		if err = s.vm.repo.SetState(ctx, id, "provisioning", "", ""); err != nil {
			return err
		}
		if b.OriginalProxyID == nil {
			return fail(infraerrors.BadRequest("OPENAI_VM_PROXY_REQUIRED", "A VM2-managed account requires a SOCKS5 proxy"))
		}
		exit, err := s.socksProxy(ctx, *b.OriginalProxyID)
		if err != nil {
			return fail(err)
		}
		input, err := json.Marshal(struct {
			Name             string `json:"name"`
			UpstreamProxyURL string `json:"upstream_proxy_url"`
			RuntimeType      string `json:"runtime_type"`
		}{a.Name, exit.URL(), b.RuntimeType})
		if err != nil {
			return fail(err)
		}
		raw, err := s.bridge(ctx, http.MethodPost, fmt.Sprintf("/v1/accounts/%d/managed", id), input)
		if err != nil {
			return fail(err)
		}
		var remote struct {
			VMID  string `json:"vm_id"`
			State string `json:"state"`
		}
		status := ""
		if json.Unmarshal(raw, &remote) == nil {
			status = openAIVMManagedBindingStatus(remote.State)
		}
		if remote.VMID == "" || (status != "ready" && status != "awaiting_login") {
			return fail(infraerrors.New(http.StatusBadGateway, "OPENAI_VM_BRIDGE_PROTOCOL", "The VM bridge did not confirm the slot"))
		}
		if err = s.vm.repo.SetState(ctx, id, status, remote.VMID, ""); err != nil {
			return err
		}
		if err = s.repo.SetManaged(ctx, id, OpenAIVMManagedAwaitingLogin, nil); err != nil {
			return err
		}
		result, err = s.vm.repo.GetBinding(ctx, id)
		return err
	})
	return result, err
}

func (s *OpenAIVMManagedService) managedBinding(ctx context.Context, id int64) (*OpenAIVMBinding, error) {
	b, err := s.vm.repo.GetBinding(ctx, id)
	if err != nil {
		return nil, err
	}
	if b == nil || !b.Managed() {
		return nil, infraerrors.NotFound("OPENAI_VM_NOT_MANAGED", "This account is not VM2-managed")
	}
	return b, nil
}

// Status reads the slot through the bridge, applies what it reports and returns
// the refreshed view. While another operation holds the account, or the bridge
// cannot be reached, the last synced view is returned instead.
func (s *OpenAIVMManagedService) Status(ctx context.Context, id int64) (*OpenAIVMManagedView, error) {
	if _, err := s.managedBinding(ctx, id); err != nil {
		return nil, err
	}
	if err := s.sync(ctx, id); err != nil && !infraerrors.IsConflict(err) {
		slog.Debug("openai_vm_managed_status_sync_failed", "account_id", id, "error", err)
	}
	b, err := s.managedBinding(ctx, id)
	return openAIVMManagedView(b), err
}

// Call relays one whitelisted management operation. The body goes to the
// bridge untouched and is not retained here.
func (s *OpenAIVMManagedService) Call(ctx context.Context, id int64, method, operation string, body []byte) (json.RawMessage, error) {
	b, err := s.managedBinding(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.Status != "ready" && b.Status != "awaiting_login" && b.Status != "error" {
		return nil, infraerrors.Conflict("OPENAI_VM_MANAGED_NOT_READY", "The slot is not ready for this operation")
	}
	if len(body) > openAIVMManagedRequestLimit {
		return nil, infraerrors.BadRequest("OPENAI_VM_MANAGED_TOO_LARGE", "The request is too large")
	}
	if method == http.MethodGet {
		body = nil
	} else if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}
	return s.bridge(ctx, method, fmt.Sprintf("/v1/accounts/%d/managed/%s", id, operation), body)
}

// ChangeProxy moves the slot to another exit, then records it. The bridge
// refuses while requests are in flight, and the runtime while a login runs.
func (s *OpenAIVMManagedService) ChangeProxy(ctx context.Context, id, proxyID int64) (*OpenAIVMManagedView, error) {
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	err := s.vm.repo.WithLock(ctx, id, func() error {
		b, err := s.managedBinding(ctx, id)
		if err != nil {
			return err
		}
		if b.Status != "ready" && b.Status != "awaiting_login" {
			return infraerrors.Conflict("OPENAI_VM_MANAGED_NOT_READY", "The slot is not ready for this operation")
		}
		exit, err := s.socksProxy(ctx, proxyID)
		if err != nil {
			return err
		}
		if b.OriginalProxyID != nil && *b.OriginalProxyID == proxyID {
			return nil
		}
		input, err := json.Marshal(map[string]string{"upstream_proxy_url": exit.URL()})
		if err != nil {
			return err
		}
		if _, err = s.bridge(ctx, http.MethodPut, fmt.Sprintf("/v1/accounts/%d/managed/proxy", id), input); err != nil {
			return err
		}
		return s.repo.SetOriginalProxy(ctx, id, proxyID)
	})
	if err != nil {
		return nil, err
	}
	return s.Status(ctx, id)
}

// Delete destroys the slot (the runtime also forgets its saved login), drops
// the binding and deletes the account. A failure leaves the binding in
// delete_failed so the same call can be repeated.
func (s *OpenAIVMManagedService) Delete(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !a.IsOpenAIVMManaged() {
		return infraerrors.BadRequest("OPENAI_VM_NOT_MANAGED", "This account is not VM2-managed")
	}
	err = s.vm.repo.WithLock(ctx, id, func() error {
		b, err := s.vm.repo.GetBinding(ctx, id)
		if err != nil || b == nil {
			return err
		}
		if !b.Managed() {
			return infraerrors.Conflict("OPENAI_VM_NOT_MANAGED", "This account's slot is not VM2-managed")
		}
		previous, previousError := b.Status, b.LastError
		if err = s.vm.repo.SetState(ctx, id, "deleting", "", ""); err != nil {
			return err
		}
		_, err = s.bridge(ctx, http.MethodDelete, fmt.Sprintf("/v1/accounts/%d", id), nil)
		var app *infraerrors.ApplicationError
		if errors.As(err, &app) && app.Metadata["code"] == "account_not_found" {
			err = nil
		}
		if OpenAIVMManagedErrorCode(err) == "slot_starting" {
			// Refused before anything was removed: the slot is booting. Nothing
			// failed, so the binding goes back to what it was.
			cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			return errors.Join(err, s.vm.repo.SetState(cleanCtx, id, previous, "", previousError))
		}
		if err == nil {
			err = s.repo.ReleaseManaged(ctx, id)
		}
		if err != nil {
			cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			return errors.Join(err, s.vm.repo.SetState(cleanCtx, id, "delete_failed", "", infraerrors.Message(err)))
		}
		s.vm.refreshSnapshots(ctx, id)
		return nil
	})
	if err != nil {
		return err
	}
	return s.admin.DeleteAccount(withOpenAIVMManagedDelete(ctx), id)
}

// sync reads one slot and applies its state. It holds the account's operation
// lock, so it never interleaves with a create, proxy change or delete.
func (s *OpenAIVMManagedService) sync(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	return s.vm.repo.WithLock(ctx, id, func() error {
		b, err := s.vm.repo.GetBinding(ctx, id)
		if err != nil || b == nil || !b.Managed() {
			return err
		}
		if b.Status != "ready" && b.Status != "awaiting_login" {
			// Being created, failed or being deleted: those flows own the state.
			return nil
		}
		raw, err := s.bridge(ctx, http.MethodGet, fmt.Sprintf("/v1/accounts/%d/managed", id), nil)
		if err != nil {
			return err
		}
		var remote openAIVMManagedRemote
		if err = json.Unmarshal(raw, &remote); err != nil {
			return infraerrors.New(http.StatusBadGateway, "OPENAI_VM_BRIDGE_PROTOCOL", "The VM bridge returned an invalid response")
		}
		return s.apply(ctx, b, &remote, raw)
	})
}

func (s *OpenAIVMManagedService) apply(ctx context.Context, b *OpenAIVMBinding, remote *openAIVMManagedRemote, raw json.RawMessage) error {
	id := b.AccountID
	if next := openAIVMManagedBindingStatus(remote.State); (next == "ready" || next == "awaiting_login") && next != b.Status {
		if err := s.vm.repo.SetState(ctx, id, next, remote.VMID, ""); err != nil {
			return err
		}
	}
	now := time.Now()
	status, until := deriveOpenAIVMManagedStatus(remote, b.ActivatedAt != nil, now)
	var detail json.RawMessage
	if remote.Snapshot != nil {
		var envelope struct {
			Snapshot json.RawMessage `json:"snapshot"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			detail = openAIVMManagedDetail(envelope.Snapshot)
		}
	}
	if status == OpenAIVMManagedUnknown {
		// No reading: keep the last known status and touch nothing else.
		return s.repo.SetManaged(ctx, id, b.ManagedStatus, nil)
	}
	if err := s.repo.SetManaged(ctx, id, status, detail); err != nil {
		return err
	}
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil || a == nil {
		return err
	}
	s.applyIdentity(ctx, a, remote.Snapshot)
	updates := map[string]any{}
	if remote.Snapshot != nil {
		for key, value := range openAIVMManagedUsageExtra(remote.Snapshot.Usage) {
			if fmt.Sprint(a.Extra[key]) != fmt.Sprint(value) {
				updates[key] = value
			}
		}
	}
	if fmt.Sprint(a.Extra[OpenAIVMManagedStatusExtraKey]) != status {
		updates[OpenAIVMManagedStatusExtraKey] = status
	}
	if len(updates) > 0 {
		if err = s.accounts.UpdateExtra(ctx, id, updates); err != nil {
			slog.Warn("openai_vm_managed_extra_update_failed", "account_id", id, "error", err)
		}
	}
	serving := remote.State == "ready" && (status == OpenAIVMManagedAvailable || status == OpenAIVMManagedRestricted)
	if serving {
		// The first login opens the account once. After that the scheduling
		// switch belongs to the administrator and is never turned back on here.
		if first, markErr := s.repo.MarkActivated(ctx, id); markErr != nil {
			return markErr
		} else if first {
			if err = s.accounts.SetSchedulable(ctx, id, true); err != nil {
				return err
			}
		}
	}
	s.applyPause(ctx, a, status, until, remote.Snapshot, now)
	s.applyConcurrency(ctx, a, remote)
	return nil
}

// applyIdentity copies who the slot is logged in as. Never a token: the
// snapshot has none, and the keys written here are fixed.
func (s *OpenAIVMManagedService) applyIdentity(ctx context.Context, a *Account, snap *openAIVMManagedSnapshot) {
	if snap == nil || !snap.HasToken {
		return
	}
	next := shallowCopyMap(a.Credentials)
	changed := false
	for key, value := range map[string]string{"email": snap.Email, "chatgpt_account_id": snap.ChatGPTAccountID, "plan_type": snap.PlanType, "subscription_expires_at": snap.SubscriptionEnd} {
		if value = strings.TrimSpace(value); value != "" && a.GetCredential(key) != value {
			next[key] = value
			changed = true
		}
	}
	if !changed || openAIVMManagedHasToken(next) {
		return
	}
	if err := persistAccountCredentials(ctx, s.accounts, a, next); err != nil {
		slog.Warn("openai_vm_managed_identity_update_failed", "account_id", a.ID, "error", err)
	}
}

// applyPause keeps an account whose slot cannot serve out of selection, and
// lifts only the pauses this feature wrote once the slot serves again.
func (s *OpenAIVMManagedService) applyPause(ctx context.Context, a *Account, status string, until time.Time, snap *openAIVMManagedSnapshot, now time.Time) {
	if status == OpenAIVMManagedAvailable {
		if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) && strings.HasPrefix(a.TempUnschedulableReason, openAIVMManagedPausePrefix) {
			if err := s.accounts.ClearTempUnschedulable(ctx, a.ID); err != nil {
				slog.Warn("openai_vm_managed_resume_failed", "account_id", a.ID, "error", err)
			}
		}
		return
	}
	reason := openAIVMManagedPausePrefix + status
	if status == OpenAIVMManagedRestricted && snap != nil && snap.RestrictionReason != "" {
		reason += ":" + snap.RestrictionReason
	}
	if until.IsZero() {
		until = now.Add(openAIVMManagedSyncPause)
	}
	if err := s.accounts.SetTempUnschedulable(ctx, a.ID, until, reason); err != nil {
		slog.Warn("openai_vm_managed_pause_failed", "account_id", a.ID, "error", err)
	}
}

// applyConcurrency keeps the slot's limit equal to the account's, so the
// runtime never queues what Sub2API already admitted.
func (s *OpenAIVMManagedService) applyConcurrency(ctx context.Context, a *Account, remote *openAIVMManagedRemote) {
	if remote.Snapshot == nil || a.Concurrency < 1 || a.Concurrency > 256 || remote.Snapshot.MaxConcurrency == a.Concurrency {
		return
	}
	body, _ := json.Marshal(map[string]int{"max_concurrency": a.Concurrency})
	if _, err := s.bridge(ctx, http.MethodPut, fmt.Sprintf("/v1/accounts/%d/managed/policy", a.ID), body); err != nil {
		slog.Debug("openai_vm_managed_concurrency_sync_failed", "account_id", a.ID, "error", err)
	}
}

// RefreshUsage asks the slot to re-read its usage and returns the window
// fields now stored on the account.
func (s *OpenAIVMManagedService) RefreshUsage(ctx context.Context, id int64) (map[string]any, error) {
	if _, err := s.Call(ctx, id, http.MethodPost, "usage/refresh", nil); err != nil {
		return nil, err
	}
	if err := s.sync(ctx, id); err != nil {
		return nil, err
	}
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil || a == nil {
		return nil, err
	}
	out := map[string]any{}
	for key, value := range a.Extra {
		if strings.HasPrefix(key, "codex_5h_") || strings.HasPrefix(key, "codex_7d_") || key == "codex_usage_updated_at" {
			out[key] = value
		}
	}
	return out, nil
}

// SyncAll reads every managed slot once.
func (s *OpenAIVMManagedService) SyncAll(ctx context.Context) {
	bindings, err := s.repo.ListManaged(ctx)
	if err != nil {
		slog.Warn("openai_vm_managed_sync_list_failed", "error", err)
		return
	}
	for _, b := range bindings {
		if ctx.Err() != nil {
			return
		}
		if err := s.sync(ctx, b.AccountID); err != nil && !infraerrors.IsConflict(err) {
			slog.Debug("openai_vm_managed_sync_failed", "account_id", b.AccountID, "error", err)
		}
	}
}

// Start runs the state sync until Stop.
func (s *OpenAIVMManagedService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		openAIVMManagedUsageRefresher = s.RefreshUsage
		openAIVMManagedDeleter = s.Delete
		s.started.Store(true)
		go func() {
			defer close(s.done)
			ticker := time.NewTicker(openAIVMManagedSyncInterval)
			defer ticker.Stop()
			for {
				select {
				case <-s.stop:
					return
				case <-ticker.C:
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
					s.SyncAll(ctx)
					cancel()
				}
			}
		}()
	})
}

func (s *OpenAIVMManagedService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stop)
		if !s.started.Load() {
			return
		}
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
	})
}

// SetSchedulable is the administrator's scheduling switch for a managed account.
func (s *OpenAIVMManagedService) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	b, err := s.managedBinding(ctx, id)
	if err != nil {
		return err
	}
	if schedulable && b.ActivatedAt == nil {
		return infraerrors.Conflict("OPENAI_VM_MANAGED_NOT_READY", "Log the account in before opening it to scheduling")
	}
	return s.accounts.SetSchedulable(ctx, id, schedulable)
}

// OpenAIVMManagedErrorCode is the short code of a managed-operation failure,
// for responses that report a partial result.
func OpenAIVMManagedErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var app *infraerrors.ApplicationError
	if errors.As(err, &app) {
		if code := app.Metadata["code"]; code != "" {
			return code
		}
		return app.Reason
	}
	return "internal_error"
}
