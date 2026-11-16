package vmbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Operations a managed binding relays to the runtime. The names are the
// bridge's own; the provider maps each to one runtime endpoint. Anything that
// would hand a stored secret back (reveal a saved login, export a credential)
// is deliberately absent.
const (
	OpOAuthURL       = "login.oauth_url"
	OpOAuthCode      = "login.oauth_code"
	OpPasswordStart  = "login.password"
	OpPasswordStatus = "login.password_status"
	OpPasswordCode   = "login.password_code"
	OpPasswordCancel = "login.password_cancel"
	OpPasswordForget = "login.password_forget"
	OpImport         = "login.import"
	OpTokenRefresh   = "token.refresh"
	OpUsageRefresh   = "usage.refresh"
	OpResetCredit    = "usage.reset_credit"
	OpPowerOn        = "power.on"
	OpPowerOff       = "power.off"
	OpSchedulable    = "schedulable"
	OpPolicy         = "policy"
	OpDiagnose       = "diagnose"
	OpRecover        = "recover"
	OpExitProbe      = "exit_probe"
)

// opRule says in which binding states an operation may run, and whether it can
// change the slot's credential (those are followed by a state reconcile).
type opRule struct {
	awaitingLogin bool
	ready         bool
	failed        bool
	credential    bool
}

var managedOps = map[string]opRule{
	OpOAuthURL:       {awaitingLogin: true, ready: true},
	OpOAuthCode:      {awaitingLogin: true, ready: true, credential: true},
	OpPasswordStart:  {awaitingLogin: true, ready: true},
	OpPasswordStatus: {awaitingLogin: true, ready: true, failed: true, credential: true},
	OpPasswordCode:   {awaitingLogin: true, ready: true},
	OpPasswordCancel: {awaitingLogin: true, ready: true, failed: true},
	OpPasswordForget: {awaitingLogin: true, ready: true, failed: true},
	OpImport:         {awaitingLogin: true, ready: true, credential: true},
	OpTokenRefresh:   {ready: true},
	OpUsageRefresh:   {ready: true},
	OpResetCredit:    {ready: true},
	OpPowerOn:        {awaitingLogin: true, ready: true},
	OpPowerOff:       {awaitingLogin: true, ready: true},
	OpSchedulable:    {ready: true},
	OpPolicy:         {awaitingLogin: true, ready: true},
	OpDiagnose:       {awaitingLogin: true, ready: true, failed: true},
	OpRecover:        {awaitingLogin: true, ready: true},
	OpExitProbe:      {awaitingLogin: true, ready: true},
}

func (r opRule) allows(state string) bool {
	switch state {
	case stateAwaitingLogin:
		return r.awaitingLogin
	case stateReady:
		return r.ready
	case stateFailed:
		return r.failed
	}
	return false
}

// ManagedStatus is a binding's lifecycle record together with what the runtime
// currently reports about its slot. Snapshot is nil when the runtime could not
// be read; the lifecycle record is still authoritative for routing.
type ManagedStatus struct {
	Status
	Snapshot *ManagedSnapshot `json:"snapshot,omitempty"`
}

func (m *Manager) managedProvider() (ManagedProvider, error) {
	p, ok := m.provider.(ManagedProvider)
	if !ok {
		return nil, bridgeError(501, "managed_unsupported", "this bridge's runtime does not support managed accounts")
	}
	return p, nil
}

func validManagedRequest(in ManagedProvisionRequest) error {
	if in.AccountID <= 0 || strings.TrimSpace(in.Name) == "" || len(in.Name) > 256 || strings.ContainsAny(in.Name, "\r\n") {
		return bridgeError(400, "invalid_account", "account id and name are required")
	}
	if in.RuntimeType != "docker" && in.RuntimeType != "kvm" {
		return bridgeError(400, "invalid_runtime", "runtime_type must be docker or kvm")
	}
	// The runtime's password login refuses a slot without an exit, and a managed
	// account must never fall back to the runtime host's own address unnoticed.
	proxy, err := parseUpstreamProxy(in.UpstreamProxyURL)
	if err != nil || proxy == nil || (proxy.Scheme != "socks5" && proxy.Scheme != "socks5h") {
		return bridgeError(400, "invalid_proxy", "a managed account requires a SOCKS5 or SOCKS5H upstream proxy")
	}
	return nil
}

// ProvisionManaged creates (or resumes creating) a managed slot. It is
// idempotent: a retry with the same routing re-runs the provider's
// get-or-create steps and never produces a second slot.
func (m *Manager) ProvisionManaged(ctx context.Context, in ManagedProvisionRequest) (Status, bool, error) {
	provider, err := m.managedProvider()
	if err != nil {
		return Status{}, false, err
	}
	if in.RuntimeType == "" {
		in.RuntimeType = "docker"
	}
	if err := validManagedRequest(in); err != nil {
		return Status{}, false, err
	}
	m.mu.Lock()
	e := m.accounts[in.AccountID]
	created := e == nil
	if e == nil {
		now := time.Now().UTC()
		e = newEntry(persistentAccount{AccountID: in.AccountID, Name: in.Name, Mode: ModeVMManaged, RuntimeType: in.RuntimeType, UpstreamProxyURL: in.UpstreamProxyURL, State: stateProvisioning, CreatedAt: now, UpdatedAt: now})
		m.accounts[in.AccountID] = e
	}
	m.mu.Unlock()
	if err := e.lock(ctx); err != nil {
		return Status{}, false, bridgeError(408, "operation_cancelled", "account operation was cancelled")
	}
	defer e.unlock()
	m.mu.Lock()
	if m.accounts[in.AccountID] != e {
		m.mu.Unlock()
		return Status{}, false, bridgeError(409, "binding_changed", "account binding changed; retry")
	}
	if !e.data.managed() || e.data.UpstreamProxyURL != in.UpstreamProxyURL || e.data.RuntimeType != in.RuntimeType {
		m.mu.Unlock()
		return Status{}, false, bridgeError(409, "binding_conflict", "account is already bound with a different mode or routing")
	}
	if e.data.State == stateDeleting {
		m.mu.Unlock()
		return Status{}, false, bridgeError(409, "account_draining", "account binding is being removed")
	}
	// A slot that already exists is left exactly as it is: re-running the
	// provider on a logged-in slot would take it out of scheduling.
	if e.data.State == stateAwaitingLogin || e.data.State == stateReady {
		status := statusOf(e)
		m.mu.Unlock()
		return status, false, nil
	}
	resources := e.data.Resources
	e.data.State = stateProvisioning
	e.data.LastErrorCode = ""
	e.data.UpdatedAt = time.Now().UTC()
	if err := m.store.save(e.data); err != nil {
		if created {
			delete(m.accounts, in.AccountID)
		}
		m.mu.Unlock()
		return Status{}, false, bridgeError(500, "state_persist_failed", "cannot persist account binding")
	}
	m.mu.Unlock()

	newResources, err := provider.ProvisionManaged(ctx, in)
	m.mu.Lock()
	defer m.mu.Unlock()
	// Keep whatever was created so a later delete can still find it.
	resources = mergeResources(resources, newResources)
	e.data.Resources = resources
	e.data.UpdatedAt = time.Now().UTC()
	if e.data.State == stateDeleting {
		if m.store.save(e.data) != nil {
			return statusOf(e), created, bridgeError(500, "state_persist_failed", "cannot persist account binding")
		}
		return statusOf(e), created, bridgeError(409, "account_draining", "account binding is being removed")
	}
	if err != nil || resources.VMID == "" || resources.APIKey == "" {
		status, code, message := 502, "provision_failed", "upstream provisioning failed"
		var refused *CallError
		if errors.As(err, &refused) && refused.Code != "" {
			status, code, message = 409, refused.Code, "the runtime refused to create the slot"
		}
		e.data.State = stateFailed
		e.data.LastErrorCode = code
		if m.store.save(e.data) != nil {
			return statusOf(e), created, bridgeError(500, "state_persist_failed", "cannot persist account binding")
		}
		return statusOf(e), created, bridgeError(status, code, message)
	}
	e.data.State = stateAwaitingLogin
	e.data.LastErrorCode = ""
	if m.store.save(e.data) != nil {
		e.data.State = stateFailed
		return statusOf(e), created, bridgeError(500, "state_persist_failed", "cannot persist account binding")
	}
	return statusOf(e), created, nil
}

// managedEntry returns the binding only if it is a managed one that is not
// being removed. The caller must hold shared header admission or the entry's
// exclusive operation lock so the resources cannot be replaced or removed.
func (m *Manager) managedEntry(id int64, e *accountEntry) (persistentAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.accounts[id] != e {
		return persistentAccount{}, bridgeError(409, "binding_changed", "account binding changed; retry")
	}
	if !e.data.managed() {
		return persistentAccount{}, bridgeError(409, "not_managed", "account is not a managed binding")
	}
	if e.data.State == stateDeleting {
		return persistentAccount{}, bridgeError(409, "account_draining", "account binding is being removed")
	}
	return e.data, nil
}

// reconcile moves a binding between awaiting_login and ready according to what
// the runtime reports. The caller holds the operation lock.
//
// Going ready requires a usable credential and pins the ChatGPT account the
// first time one is seen; a later login into a different account is refused
// rather than silently re-pointing the binding. Losing the credential (cleared,
// or its refresh token rejected) closes the binding again so no request is
// routed to a slot that can only answer 401.
func (m *Manager) reconcile(ctx context.Context, provider ManagedProvider, id int64, e *accountEntry, snap *ManagedSnapshot) error {
	m.mu.Lock()
	data := e.data
	m.mu.Unlock()
	if snap == nil || (data.State != stateAwaitingLogin && data.State != stateReady) {
		return nil
	}
	usable := snap.HasToken && snap.RefreshError == ""
	switch {
	case data.State == stateAwaitingLogin && usable:
		if strings.TrimSpace(snap.ChatGPTAccountID) == "" {
			return nil
		}
		if data.ChatGPTAccountID != "" && data.ChatGPTAccountID != snap.ChatGPTAccountID {
			return bridgeError(409, "identity_mismatch", "the slot is logged in to a different ChatGPT account than this binding")
		}
		return m.activate(ctx, provider, id, e, data.Resources, snap.ChatGPTAccountID)
	case data.State == stateReady && !usable:
		return m.setManagedState(id, e, stateAwaitingLogin, "")
	case data.State == stateReady && data.ChatGPTAccountID != "" && snap.ChatGPTAccountID != "" && data.ChatGPTAccountID != snap.ChatGPTAccountID:
		// Someone re-logged the slot into another account behind the bridge's back.
		_ = m.setManagedState(id, e, stateAwaitingLogin, "")
		return bridgeError(409, "identity_mismatch", "the slot is logged in to a different ChatGPT account than this binding")
	}
	return nil
}

// activation is one background start of a managed slot: the first boot after
// a login, a power-on or a restart. result and err are written before done is
// closed.
type activation struct {
	done   chan struct{}
	result json.RawMessage
	err    error
}

var (
	errActivateFailed = bridgeError(502, "activate_failed", "the slot logged in but could not be started")
	errSlotStarting   = bridgeError(409, "slot_starting", "the slot is starting; wait until it is up")
)

// A KVM slot boots for over a minute. No management request is held for that:
// a start runs detached from the request that asked for it. The request waits
// ActivateWait (a slot that is already up, or a container slot, finishes
// within it) and then answers with the start still running; Status reports
// Starting until it ends. Only one start runs per slot.
//
// begin must be called with m.mu held and e.activation nil.
func (m *Manager) begin(e *accountEntry, work func(context.Context) (json.RawMessage, error)) *activation {
	run := &activation{done: make(chan struct{})}
	e.activation = run
	m.background.Add(1)
	go func() {
		defer m.background.Done()
		ctx, cancel := context.WithTimeout(m.base, m.cfg.ActivateTimeout)
		defer cancel()
		result, err := work(ctx)
		m.mu.Lock()
		e.activation = nil
		run.result, run.err = result, err
		close(run.done)
		m.mu.Unlock()
	}()
	return run
}

// finished reports whether the start ended within the request's wait.
func (m *Manager) finished(ctx context.Context, run *activation) bool {
	timer := time.NewTimer(m.cfg.ActivateWait)
	defer timer.Stop()
	select {
	case <-run.done:
		return true
	case <-timer.C:
	case <-ctx.Done():
	}
	return false
}

// untilUp repeats a start that outlived one management call. The boot keeps
// running on the runtime, and asking again waits for that same boot, so a
// timeout here is not a failure. Any other error ends it.
func (m *Manager) untilUp(ctx context.Context, start func(context.Context) error) error {
	for {
		err := start(ctx)
		if err == nil || ctx.Err() != nil || !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		select {
		case <-ctx.Done():
		case <-time.After(m.cfg.askAgain):
		}
	}
}

// activate starts a freshly logged-in slot and opens the binding once it is up.
func (m *Manager) activate(ctx context.Context, provider ManagedProvider, id int64, e *accountEntry, resources Resources, chatgptAccountID string) error {
	m.mu.Lock()
	if m.accounts[id] != e || e.data.State != stateAwaitingLogin || e.activation != nil {
		m.mu.Unlock()
		return nil
	}
	if time.Now().Before(e.activateRetryAt) {
		// The last start failed moments ago; say so rather than boot again on every read.
		m.mu.Unlock()
		return errActivateFailed
	}
	run := m.begin(e, func(ctx context.Context) (json.RawMessage, error) {
		return nil, m.open(ctx, provider, id, e, resources, chatgptAccountID)
	})
	m.mu.Unlock()
	if m.finished(ctx, run) {
		return run.err
	}
	return nil
}

// open is the body of an activation: start the slot, then open the binding.
func (m *Manager) open(ctx context.Context, provider ManagedProvider, id int64, e *accountEntry, resources Resources, chatgptAccountID string) error {
	err := m.untilUp(ctx, func(ctx context.Context) error { return provider.Activate(ctx, resources) })
	m.mu.Lock()
	e.activateRetryAt = time.Time{}
	if err != nil {
		e.activateRetryAt = time.Now().Add(m.cfg.ActivateRetry)
	}
	m.mu.Unlock()
	if err != nil {
		return errActivateFailed
	}
	// Open only on what is still true after the boot: the login may have been
	// cleared or replaced, or the binding removed, while the slot was starting.
	snap, err := provider.Snapshot(ctx, resources)
	if err != nil || snap == nil || !snap.HasToken || snap.RefreshError != "" || snap.ChatGPTAccountID != chatgptAccountID {
		return nil
	}
	m.mu.Lock()
	waiting := m.accounts[id] == e && e.data.State == stateAwaitingLogin
	m.mu.Unlock()
	if !waiting {
		return nil
	}
	return m.setManagedState(id, e, stateReady, chatgptAccountID)
}

// power runs a power-on or a restart as a background start. A quick one is
// answered with the runtime's own result; a slow one with {"starting":true}.
func (m *Manager) power(ctx context.Context, provider ManagedProvider, e *accountEntry, resources Resources, call ManagedCall) (json.RawMessage, error) {
	m.mu.Lock()
	if e.activation != nil {
		m.mu.Unlock()
		return nil, errSlotStarting
	}
	run := m.begin(e, func(ctx context.Context) (json.RawMessage, error) {
		var out json.RawMessage
		once := func(ctx context.Context) (err error) {
			out, err = provider.Call(ctx, resources, call)
			return err
		}
		if call.Operation != OpPowerOn {
			// A restart is not repeated: asking again would restart the slot again.
			return out, once(ctx)
		}
		return out, m.untilUp(ctx, once)
	})
	m.mu.Unlock()
	if !m.finished(ctx, run) {
		return json.RawMessage(`{"starting":true}`), nil
	}
	if run.err != nil {
		return nil, relayedError(run.err)
	}
	return run.result, nil
}

// relayedError reduces a runtime failure to what may be shown.
func relayedError(err error) error {
	var ce *CallError
	if errors.As(err, &ce) {
		status := ce.Status
		if status < 400 || status > 599 {
			status = 502
		}
		return bridgeError(status, ce.Code, "the runtime refused this operation")
	}
	return bridgeError(502, "upstream_unavailable", "the runtime could not be reached")
}

func (m *Manager) setManagedState(id int64, e *accountEntry, state, chatgptAccountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.accounts[id] != e || e.data.State == stateDeleting {
		return nil
	}
	previous := e.data
	e.data.State = state
	if chatgptAccountID != "" {
		e.data.ChatGPTAccountID = chatgptAccountID
	}
	e.data.LastErrorCode = ""
	e.data.UpdatedAt = time.Now().UTC()
	if m.store.save(e.data) != nil {
		e.data = previous
		return bridgeError(500, "state_persist_failed", "cannot persist account binding")
	}
	return nil
}

// managedTransitionNeeded distinguishes an ordinary observation from a read
// that must change lifecycle state. Observations may share inference admission;
// transitions must reread the runtime under exclusive admission.
func managedTransitionNeeded(data persistentAccount, snap *ManagedSnapshot) bool {
	if snap == nil {
		return false
	}
	usable := snap.HasToken && snap.RefreshError == ""
	switch data.State {
	case stateAwaitingLogin:
		return usable && strings.TrimSpace(snap.ChatGPTAccountID) != ""
	case stateReady:
		return !usable || (data.ChatGPTAccountID != "" && snap.ChatGPTAccountID != "" && data.ChatGPTAccountID != snap.ChatGPTAccountID)
	}
	return false
}

// ManagedSnapshot observes the runtime without queuing inference behind a
// pending upstream header. A login/lifecycle transition retains the exclusive
// path and fetches a fresh snapshot after acquiring it, never upgrading a
// shared admission in place or acting on a stale observation.
func (m *Manager) ManagedSnapshot(ctx context.Context, id int64) (ManagedStatus, error) {
	provider, err := m.managedProvider()
	if err != nil {
		return ManagedStatus{}, err
	}
	e, err := m.entry(id)
	if err != nil {
		return ManagedStatus{}, err
	}
	if err := e.lockHeaders(ctx); err != nil {
		return ManagedStatus{}, bridgeError(408, "operation_cancelled", "account operation was cancelled")
	}
	out, transition, err := m.managedSnapshot(ctx, provider, id, e, false)
	e.unlockHeaders()
	if err != nil || !transition {
		return out, err
	}
	if err := e.lock(ctx); err != nil {
		return ManagedStatus{}, bridgeError(408, "operation_cancelled", "account operation was cancelled")
	}
	defer e.unlock()
	out, _, err = m.managedSnapshot(ctx, provider, id, e, true)
	return out, err
}

// managedSnapshot runs with shared admission for observation, or exclusive
// admission when reconcile is allowed to change the binding's lifecycle.
func (m *Manager) managedSnapshot(ctx context.Context, provider ManagedProvider, id int64, e *accountEntry, allowReconcile bool) (ManagedStatus, bool, error) {
	data, err := m.managedEntry(id, e)
	if err != nil {
		return ManagedStatus{}, false, err
	}
	var snap *ManagedSnapshot
	if data.Resources.VMID != "" {
		snap, err = provider.Snapshot(ctx, data.Resources)
		if err != nil {
			status, _ := m.Status(id)
			status.LastErrorCode = "snapshot_unavailable"
			return ManagedStatus{Status: status}, false, nil
		}
	}
	if !allowReconcile && managedTransitionNeeded(data, snap) {
		return ManagedStatus{}, true, nil
	}
	var reconcileErr error
	if allowReconcile {
		reconcileErr = m.reconcile(ctx, provider, id, e, snap)
	}
	status, statusErr := m.Status(id)
	if statusErr != nil {
		return ManagedStatus{}, false, statusErr
	}
	var be *Error
	if errors.As(reconcileErr, &be) {
		status.LastErrorCode = be.Code
	}
	return ManagedStatus{Status: status, Snapshot: snap}, false, nil
}

// ManagedCall relays one operation to the runtime, then reconciles the binding
// if the operation could have changed the slot's credential.
func (m *Manager) ManagedCall(ctx context.Context, id int64, call ManagedCall) (json.RawMessage, error) {
	provider, err := m.managedProvider()
	if err != nil {
		return nil, err
	}
	rule, known := managedOps[call.Operation]
	if !known {
		return nil, bridgeError(404, "not_found", "endpoint not found")
	}
	e, err := m.entry(id)
	if err != nil {
		return nil, err
	}
	if err := e.lock(ctx); err != nil {
		return nil, bridgeError(408, "operation_cancelled", "account operation was cancelled")
	}
	defer e.unlock()
	data, err := m.managedEntry(id, e)
	if err != nil {
		return nil, err
	}
	if !rule.allows(data.State) {
		return nil, bridgeError(409, "account_not_ready", "this operation is not available in the account's current state")
	}
	switch call.Operation {
	case OpPowerOn, OpRecover:
		return m.power(ctx, provider, e, data.Resources, call)
	case OpPowerOff:
		// Stopping a slot in the middle of its boot would queue behind the boot.
		if m.starting(e) {
			return nil, errSlotStarting
		}
	}
	out, err := provider.Call(ctx, data.Resources, call)
	if err != nil {
		return nil, relayedError(err)
	}
	if rule.credential {
		if snap, snapErr := provider.Snapshot(ctx, data.Resources); snapErr == nil {
			if recErr := m.reconcile(ctx, provider, id, e, snap); recErr != nil {
				return nil, recErr
			}
		}
	}
	return out, nil
}

func (m *Manager) starting(e *accountEntry) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return e.activation != nil
}

// RebindManaged moves a managed slot to another exit. It is refused while the
// account has requests in flight (their tunnels captured the old exit) and
// while the slot is starting.
func (m *Manager) RebindManaged(ctx context.Context, id int64, upstreamProxyURL string) (Status, error) {
	provider, err := m.managedProvider()
	if err != nil {
		return Status{}, err
	}
	proxy, err := parseUpstreamProxy(upstreamProxyURL)
	if err != nil || proxy == nil || (proxy.Scheme != "socks5" && proxy.Scheme != "socks5h") {
		return Status{}, bridgeError(400, "invalid_proxy", "a managed account requires a SOCKS5 or SOCKS5H upstream proxy")
	}
	e, err := m.entry(id)
	if err != nil {
		return Status{}, err
	}
	if err := e.lock(ctx); err != nil {
		return Status{}, bridgeError(408, "operation_cancelled", "account operation was cancelled")
	}
	defer e.unlock()
	data, err := m.managedEntry(id, e)
	if err != nil {
		return Status{}, err
	}
	if data.State != stateAwaitingLogin && data.State != stateReady {
		return Status{}, bridgeError(409, "account_not_ready", "this operation is not available in the account's current state")
	}
	if data.UpstreamProxyURL == upstreamProxyURL {
		return m.Status(id)
	}
	m.mu.Lock()
	busy, booting := e.active > 0, e.activation != nil
	m.mu.Unlock()
	if booting {
		return Status{}, errSlotStarting
	}
	if busy {
		return Status{}, bridgeError(409, "account_busy", "account has active requests; change the proxy when it is idle")
	}
	proxyID, err := provider.Rebind(ctx, data.Resources, upstreamProxyURL)
	if err != nil {
		var ce *CallError
		if errors.As(err, &ce) {
			return Status{}, bridgeError(409, ce.Code, "the runtime refused the proxy change")
		}
		return Status{}, bridgeError(502, "rebind_failed", "the runtime could not move the slot to the new proxy")
	}
	m.mu.Lock()
	if m.accounts[id] == e && e.data.State != stateDeleting {
		previous := e.data
		e.data.UpstreamProxyURL = upstreamProxyURL
		if proxyID != "" {
			e.data.Resources.ProxyID = proxyID
		}
		e.data.UpdatedAt = time.Now().UTC()
		if m.store.save(e.data) != nil {
			// The runtime already moved the slot. Keep the new exit in memory so
			// CONNECT matches it; the next successful save persists it.
			e.data.UpdatedAt = previous.UpdatedAt
			m.mu.Unlock()
			return Status{}, bridgeError(500, "state_persist_failed", "the proxy was changed but the binding could not be saved; retry")
		}
	}
	m.mu.Unlock()
	return m.Status(id)
}
