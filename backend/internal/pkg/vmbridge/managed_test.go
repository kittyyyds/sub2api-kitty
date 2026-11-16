package vmbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const managedProxy = "socks5h://user:proxy-password-never-shown@proxy.example:1080"

// managedTestProvider is a testProvider that also offers managed bindings. The
// snapshot is whatever the test last set, as the runtime would report it.
type managedTestProvider struct {
	testProvider
	mu           sync.Mutex
	snap         ManagedSnapshot
	provisions   int
	activations  int
	rebinds      []string
	calls        []ManagedCall
	provisionErr error
	activateErr  error
	// activateHook, when set, replaces the plain activateErr answer.
	activateHook func(context.Context) error
	callErr      error
	callResult   json.RawMessage
	onCall       func(ManagedCall)
}

func (p *managedTestProvider) ProvisionManaged(_ context.Context, in ManagedProvisionRequest) (Resources, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provisions++
	resources := Resources{VMID: fmt.Sprintf("vm-%d", in.AccountID), ProxyID: "px-1"}
	if p.provisionErr != nil {
		return resources, p.provisionErr
	}
	resources.KeyID, resources.APIKey = fmt.Sprintf("key-%d", in.AccountID), fmt.Sprintf("private-key-%d", in.AccountID)
	return resources, nil
}
func (p *managedTestProvider) Snapshot(context.Context, Resources) (*ManagedSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	snap := p.snap
	return &snap, nil
}
func (p *managedTestProvider) Activate(ctx context.Context, _ Resources) error {
	p.mu.Lock()
	p.activations++
	hook, err := p.activateHook, p.activateErr
	p.mu.Unlock()
	if hook != nil {
		return hook(ctx)
	}
	return err
}
func (p *managedTestProvider) count() (activations, calls int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.activations, len(p.calls)
}
func (p *managedTestProvider) Rebind(_ context.Context, _ Resources, url string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rebinds = append(p.rebinds, url)
	return "px-2", nil
}
func (p *managedTestProvider) Call(_ context.Context, _ Resources, call ManagedCall) (json.RawMessage, error) {
	p.mu.Lock()
	p.calls = append(p.calls, call)
	hook, result, err := p.onCall, p.callResult, p.callErr
	p.mu.Unlock()
	if hook != nil {
		hook(call)
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = json.RawMessage(`{"ok":true}`)
	}
	return result, nil
}
func (p *managedTestProvider) set(snap ManagedSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snap = snap
}

func managedInput(id int64) ManagedProvisionRequest {
	return ManagedProvisionRequest{AccountID: id, Name: fmt.Sprintf("managed %d", id), UpstreamProxyURL: managedProxy, RuntimeType: "kvm"}
}
func loggedIn(account string) ManagedSnapshot {
	return ManagedSnapshot{Status: "stopped", HasToken: true, HasRefresh: true, ChatGPTAccountID: account, Email: "demo@example.test"}
}
func newManaged(t *testing.T, id int64) (*Manager, *managedTestProvider) {
	t.Helper()
	p := &managedTestProvider{}
	m := newTestManager(t, p)
	status, created, err := m.ProvisionManaged(context.Background(), managedInput(id))
	if err != nil || !created || status.State != stateAwaitingLogin || status.Mode != ModeVMManaged || status.Ready {
		t.Fatalf("managed provision: %+v created=%v err=%v", status, created, err)
	}
	return m, p
}
func stateFiles(t *testing.T, m *Manager) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(m.cfg.StateDir, "account-*.json"))
	var all strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = all.Write(data)
	}
	return all.String()
}

func TestManagedSlotIsCreatedWithoutCredentialAndServesNothingUntilLogin(t *testing.T) {
	m, p := newManaged(t, 7)
	// A slot without a credential is closed to inference and to CONNECT alike.
	_, err := m.Forward(context.Background(), 7, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}")), nil)
	requireCode(t, err, "account_not_ready")
	if _, _, _, _, err = m.acquire(7); err == nil {
		t.Fatal("CONNECT lease granted before login")
	}
	// Repeating the create is a no-op: no second slot, no re-run of the provider.
	status, created, err := m.ProvisionManaged(context.Background(), managedInput(7))
	if err != nil || created || status.State != stateAwaitingLogin || p.provisions != 1 {
		t.Fatalf("repeat create: %+v created=%v err=%v provisions=%d", status, created, err, p.provisions)
	}
	// Different routing for the same account is a conflict, not a silent change.
	other := managedInput(7)
	other.RuntimeType = "docker"
	_, _, err = m.ProvisionManaged(context.Background(), other)
	requireCode(t, err, "binding_conflict")
	// The key and the proxy credential are encrypted at rest; the public status has neither.
	public, _ := json.Marshal(status)
	for _, secret := range []string{"private-key-7", "proxy-password-never-shown"} {
		if strings.Contains(string(public), secret) || strings.Contains(stateFiles(t, m), secret) {
			t.Fatalf("%q exposed", secret)
		}
	}
}

func TestManagedCreateRequiresSocksProxyAndValidInput(t *testing.T) {
	p := &managedTestProvider{}
	m := newTestManager(t, p)
	for name, mutate := range map[string]func(*ManagedProvisionRequest){
		"no proxy":      func(in *ManagedProvisionRequest) { in.UpstreamProxyURL = "" },
		"http proxy":    func(in *ManagedProvisionRequest) { in.UpstreamProxyURL = "http://proxy.example:8080" },
		"bad runtime":   func(in *ManagedProvisionRequest) { in.RuntimeType = "lxc" },
		"empty name":    func(in *ManagedProvisionRequest) { in.Name = " " },
		"bad accountID": func(in *ManagedProvisionRequest) { in.AccountID = 0 },
	} {
		in := managedInput(9)
		mutate(&in)
		if _, _, err := m.ProvisionManaged(context.Background(), in); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if p.provisions != 0 {
		t.Fatal("provider reached with invalid input")
	}
	if _, err := m.Status(9); err == nil {
		t.Fatal("invalid input left a binding behind")
	}
}

func TestManagedLoginOpensTheBindingAndPinsTheAccount(t *testing.T) {
	m, p := newManaged(t, 7)
	// Nothing logged in yet: a status read changes nothing.
	got, err := m.ManagedSnapshot(context.Background(), 7)
	if err != nil || got.State != stateAwaitingLogin || p.activations != 0 {
		t.Fatalf("pre-login snapshot: %+v %v", got, err)
	}
	// A credential-changing call is followed by a reconcile.
	p.onCall = func(ManagedCall) { p.set(loggedIn("chatgpt-7")) }
	if _, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpOAuthCode, Body: json.RawMessage(`{"session_id":"s","code":"c"}`)}); err != nil {
		t.Fatal(err)
	}
	status, _ := m.Status(7)
	if !status.Ready || p.activations != 1 {
		t.Fatalf("login did not open the binding: %+v activations=%d", status, p.activations)
	}
	if _, err = m.Forward(context.Background(), 7, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}")), nil); err != nil {
		t.Fatalf("ready managed binding refused inference: %v", err)
	}
	// The pinned account survives a restart of the bridge.
	reloaded, err := New(Config{Secret: testSecret, StateDir: m.cfg.StateDir}, p)
	if err != nil {
		t.Fatal(err)
	}
	if e := reloaded.accounts[7]; e == nil || !e.data.managed() || e.data.ChatGPTAccountID != "chatgpt-7" || e.data.State != stateReady {
		t.Fatalf("managed binding not restored: %+v", e)
	}
	// A later login into a different account is refused and closes the binding.
	p.set(loggedIn("someone-else"))
	got, err = m.ManagedSnapshot(context.Background(), 7)
	if err != nil || got.Ready || got.LastErrorCode != "identity_mismatch" {
		t.Fatalf("account swap not caught: %+v %v", got, err)
	}
	_, err = m.Forward(context.Background(), 7, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}")), nil)
	requireCode(t, err, "account_not_ready")
}

func TestManagedBindingClosesWhenTheCredentialDiesAndReopensAfterRelogin(t *testing.T) {
	m, p := newManaged(t, 7)
	p.set(loggedIn("chatgpt-7"))
	if got, err := m.ManagedSnapshot(context.Background(), 7); err != nil || !got.Ready {
		t.Fatalf("login not picked up by a status read: %+v %v", got, err)
	}
	// The runtime marks the refresh token as dead: no request may be routed there.
	dead := loggedIn("chatgpt-7")
	dead.RefreshError = "invalid_grant"
	p.set(dead)
	got, err := m.ManagedSnapshot(context.Background(), 7)
	if err != nil || got.Ready || got.State != stateAwaitingLogin || got.Snapshot.RefreshError != "invalid_grant" {
		t.Fatalf("dead credential left the binding open: %+v %v", got, err)
	}
	_, err = m.Forward(context.Background(), 7, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}")), nil)
	requireCode(t, err, "account_not_ready")
	// The automatic re-login (or a person) fixes it; the same account reopens.
	p.set(loggedIn("chatgpt-7"))
	if got, err = m.ManagedSnapshot(context.Background(), 7); err != nil || !got.Ready || p.activations != 2 {
		t.Fatalf("re-login did not reopen: %+v %v activations=%d", got, err, p.activations)
	}
	// A slot that logged in but cannot be started stays closed and says why.
	p.set(ManagedSnapshot{})
	_, _ = m.ManagedSnapshot(context.Background(), 7)
	p.activateErr = errors.New("boot failed with internal detail")
	p.set(loggedIn("chatgpt-7"))
	got, err = m.ManagedSnapshot(context.Background(), 7)
	if err != nil || got.Ready || got.LastErrorCode != "activate_failed" {
		t.Fatalf("failed activation reported as ready: %+v %v", got, err)
	}
}

func TestManagedBindingRefusesEverySubTokenPath(t *testing.T) {
	m, p := newManaged(t, 7)
	p.set(loggedIn("chatgpt-7"))
	_, _ = m.ManagedSnapshot(context.Background(), 7)
	identity := &Identity{AccessToken: "access-token-from-sub", ChatGPTAccountID: "chatgpt-7"}
	var pushed int
	p.identity = func(context.Context, Resources, Identity) error { pushed++; return nil }
	// A token header on an inference request would replace the runtime's credential.
	_, err := m.Forward(context.Background(), 7, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}")), identity)
	requireCode(t, err, "managed_credential")
	_, err = m.UpdateIdentity(context.Background(), 7, *identity)
	requireCode(t, err, "managed_credential")
	in := testInput(7)
	_, _, err = m.Provision(context.Background(), in)
	requireCode(t, err, "managed_credential")
	if pushed != 0 {
		t.Fatal("a Sub2API token reached a managed slot")
	}
	// And the reverse: managed operations do not apply to a sub_token binding.
	provisionAccount(t, m, 8)
	_, err = m.ManagedCall(context.Background(), 8, ManagedCall{Operation: OpUsageRefresh})
	requireCode(t, err, "not_managed")
	_, err = m.ManagedSnapshot(context.Background(), 8)
	requireCode(t, err, "not_managed")
	_, err = m.RebindManaged(context.Background(), 8, managedProxy)
	requireCode(t, err, "not_managed")
	_, _, err = m.ProvisionManaged(context.Background(), managedInput(8))
	requireCode(t, err, "binding_conflict")
	// Same exit and runtime make no difference: a binding whose token Sub2API
	// holds is never turned into a managed one in place.
	same := testInput(9)
	same.UpstreamProxyURL, same.RuntimeType = managedProxy, "kvm"
	if _, _, err = m.Provision(context.Background(), same); err != nil {
		t.Fatal(err)
	}
	before := p.provisions
	_, _, err = m.ProvisionManaged(context.Background(), managedInput(9))
	requireCode(t, err, "binding_conflict")
	if status, _ := m.Status(9); p.provisions != before || status.Mode != ModeSubToken || !status.Ready {
		t.Fatalf("sub_token binding was converted: %+v", status)
	}
}

func TestManagedOperationsFollowTheBindingState(t *testing.T) {
	m, p := newManaged(t, 7)
	// Before login: logging in and diagnosing are possible, using the account is not.
	for _, op := range []string{OpOAuthURL, OpPasswordStart, OpPasswordStatus, OpImport, OpDiagnose, OpExitProbe, OpPowerOn, OpPolicy} {
		if _, err := m.ManagedCall(context.Background(), 7, ManagedCall{Operation: op}); err != nil {
			t.Fatalf("%s refused before login: %v", op, err)
		}
	}
	for _, op := range []string{OpTokenRefresh, OpUsageRefresh, OpResetCredit, OpSchedulable} {
		_, err := m.ManagedCall(context.Background(), 7, ManagedCall{Operation: op})
		requireCode(t, err, "account_not_ready")
	}
	_, err := m.ManagedCall(context.Background(), 7, ManagedCall{Operation: "login.reveal"})
	requireCode(t, err, "not_found")
	_, err = m.ManagedCall(context.Background(), 99, ManagedCall{Operation: OpDiagnose})
	requireCode(t, err, "account_not_found")
	// The runtime's own refusal is passed on by code, never by body.
	p.callErr = &CallError{Status: 409, Code: "login_busy"}
	_, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpPasswordStart, Body: json.RawMessage(`{"password":"never-echo-this"}`)})
	requireCode(t, err, "login_busy")
	if strings.Contains(err.Error(), "never-echo-this") {
		t.Fatal("relayed error echoed the request")
	}
	p.callErr = errors.New("dial tcp 10.0.0.1:18788: secret internal address")
	_, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpDiagnose})
	requireCode(t, err, "upstream_unavailable")
	if strings.Contains(err.Error(), "10.0.0.1") {
		t.Fatal("internal address exposed")
	}
	// A password passing through is never written to the state directory.
	p.callErr = nil
	_, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpPasswordStart, Body: json.RawMessage(`{"email":"a@b.test","password":"login-password-never-persist","mfa_secret":"JBSWY3DPEHPK3PXP"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if state := stateFiles(t, m); strings.Contains(state, "login-password-never-persist") || strings.Contains(state, "JBSWY3DPEHPK3PXP") {
		t.Fatal("login material persisted by the bridge")
	}
}

func TestManagedProxyChangeIsPersistedAndRefusedWhileBusy(t *testing.T) {
	m, p := newManaged(t, 7)
	p.set(loggedIn("chatgpt-7"))
	_, _ = m.ManagedSnapshot(context.Background(), 7)
	const next = "socks5h://other:second-proxy-password@other.example:1080"
	// A tunnel in flight captured the old exit; the change has to wait for it.
	_, _, _, release, err := m.acquire(7)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.RebindManaged(context.Background(), 7, next)
	requireCode(t, err, "account_busy")
	release()
	for _, bad := range []string{"", "http://proxy.example:8080", "socks5://"} {
		_, err = m.RebindManaged(context.Background(), 7, bad)
		requireCode(t, err, "invalid_proxy")
	}
	if len(p.rebinds) != 0 {
		t.Fatal("runtime reached before the change was allowed")
	}
	status, err := m.RebindManaged(context.Background(), 7, next)
	if err != nil || !status.Ready || len(p.rebinds) != 1 {
		t.Fatalf("rebind: %+v %v", status, err)
	}
	// The same exit again is a no-op.
	if _, err = m.RebindManaged(context.Background(), 7, next); err != nil || len(p.rebinds) != 1 {
		t.Fatalf("repeat rebind reached the runtime: %v", err)
	}
	// CONNECT now dials the new exit, also after a restart.
	if _, _, url, rel, err := m.acquire(7); err != nil || url != next {
		t.Fatalf("lease still on the old exit: %q %v", url, err)
	} else {
		rel()
	}
	reloaded, err := New(Config{Secret: testSecret, StateDir: m.cfg.StateDir}, p)
	if err != nil || reloaded.accounts[7].data.UpstreamProxyURL != next || reloaded.accounts[7].data.Resources.ProxyID != "px-2" {
		t.Fatalf("new exit not persisted: %v", err)
	}
	if strings.Contains(stateFiles(t, m), "second-proxy-password") {
		t.Fatal("proxy credential stored in clear")
	}
}

func TestManagedFailedCreateKeepsPartialResourcesForCleanupAndRetry(t *testing.T) {
	p := &managedTestProvider{provisionErr: errors.New("key creation failed")}
	m := newTestManager(t, p)
	status, _, err := m.ProvisionManaged(context.Background(), managedInput(7))
	requireCode(t, err, "provision_failed")
	if status.State != stateFailed || status.VMID != "vm-7" {
		t.Fatalf("partial slot not retained: %+v", status)
	}
	// Only diagnosis and cancelling are available on a failed binding.
	_, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpPasswordStart})
	requireCode(t, err, "account_not_ready")
	// The retry finishes the same slot.
	p.provisionErr = nil
	status, created, err := m.ProvisionManaged(context.Background(), managedInput(7))
	if err != nil || created || status.State != stateAwaitingLogin || p.provisions != 2 {
		t.Fatalf("retry: %+v created=%v err=%v", status, created, err)
	}
	// Deleting a managed binding goes through the ordinary teardown.
	var deleted Resources
	p.delete = func(_ context.Context, r Resources) error { deleted = r; return nil }
	if err = m.Delete(context.Background(), 7); err != nil || deleted.VMID != "vm-7" {
		t.Fatalf("delete: %v %+v", err, deleted)
	}
	if _, err = m.Status(7); err == nil {
		t.Fatal("binding survived delete")
	}
}

func TestManagedRoutesOverHTTP(t *testing.T) {
	p := &managedTestProvider{callResult: json.RawMessage(`{"auth_url":"https://auth.example/authorize"}`)}
	m := newTestManager(t, p)
	h := m.Handler()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, authorizedRequest(method, path, body))
		return w
	}
	// Unauthenticated callers get nothing.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/accounts/7/managed", nil))
	if w.Code != 401 {
		t.Fatalf("unauthenticated managed read: %d", w.Code)
	}
	create := `{"name":"managed 7","upstream_proxy_url":"` + managedProxy + `","runtime_type":"kvm"}`
	if w = do(http.MethodPost, "/v1/accounts/7/managed", create); w.Code != 201 {
		t.Fatalf("create=%d %s", w.Code, w.Body)
	}
	if w = do(http.MethodPost, "/v1/accounts/7/managed", create); w.Code != 200 {
		t.Fatalf("repeat create=%d", w.Code)
	}
	// A token field has no place in a managed create.
	if w = do(http.MethodPost, "/v1/accounts/8/managed", `{"name":"x","upstream_proxy_url":"`+managedProxy+`","access_token":"AT"}`); w.Code != 400 {
		t.Fatalf("managed create accepted a token: %d", w.Code)
	}
	var status ManagedStatus
	w = do(http.MethodGet, "/v1/accounts/7/managed", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.State != stateAwaitingLogin || status.Mode != ModeVMManaged {
		t.Fatalf("read=%d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "private-key-7") || strings.Contains(w.Body.String(), "proxy-password-never-shown") {
		t.Fatal("managed status exposed a secret")
	}
	// Each route reaches the provider as its own operation, with the body untouched.
	for _, tc := range []struct{ method, path, op string }{
		{http.MethodPost, "login/oauth-url", OpOAuthURL},
		{http.MethodPost, "login/password", OpPasswordStart},
		{http.MethodGet, "login/password", OpPasswordStatus},
		{http.MethodPost, "login/password/code", OpPasswordCode},
		{http.MethodPost, "login/password/cancel", OpPasswordCancel},
		{http.MethodPost, "login/password/forget", OpPasswordForget},
		{http.MethodPost, "login/import", OpImport},
		{http.MethodGet, "diagnose", OpDiagnose},
		{http.MethodPost, "exit-probe", OpExitProbe},
		{http.MethodPut, "policy", OpPolicy},
	} {
		body := ""
		if tc.method != http.MethodGet {
			body = `{"marker":"` + tc.op + `"}`
		}
		w = do(tc.method, "/v1/accounts/7/managed/"+tc.path, body)
		last := p.calls[len(p.calls)-1]
		if w.Code != 200 || last.Operation != tc.op || (body != "" && string(last.Body) != body) {
			t.Fatalf("%s %s -> %d op=%s body=%s", tc.method, tc.path, w.Code, last.Operation, last.Body)
		}
		if !strings.Contains(w.Body.String(), "auth.example") {
			t.Fatalf("%s did not return the runtime's answer", tc.path)
		}
	}
	before := len(p.calls)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "login/password/reveal", "{}", 404}, // never relayed
		{http.MethodGet, "login/oauth-url", "", 404},          // wrong method
		{http.MethodPost, "login/password", `[1,2]`, 400},     // not an object
		{http.MethodPost, "login/password", `{"a":1} trailing`, 400},
		{http.MethodPost, "usage/refresh", "", 409}, // not logged in yet
		{http.MethodPost, "login/password", `null`, 400},
		// Well-formed but over the limit: only the size check can refuse these.
		{http.MethodPost, "login/password", `{"password":"` + strings.Repeat("a", managedBodyLimit) + `"}`, 400},
		{http.MethodPost, "login/import", `{"auth_json":"` + strings.Repeat("a", managedImportMax) + `"}`, 400},
		{http.MethodPut, "proxy", `{"upstream_proxy_url":"http://proxy.example:8080"}`, 400},
		{http.MethodPut, "proxy", `{"upstream_proxy_url":"x","extra":1}`, 400},
	} {
		if w = do(tc.method, "/v1/accounts/7/managed/"+tc.path, tc.body); w.Code != tc.status {
			t.Fatalf("%s %s = %d, want %d (%s)", tc.method, tc.path, w.Code, tc.status, w.Body)
		}
	}
	if len(p.calls) != before {
		t.Fatal("a refused request reached the runtime")
	}
	// An account file may be larger than the ordinary management limit.
	big := `{"auth_json":"` + strings.Repeat("a", managedBodyLimit) + `"}`
	if w = do(http.MethodPost, "/v1/accounts/7/managed/login/import", big); w.Code != 200 {
		t.Fatalf("large account file refused: %d", w.Code)
	}
	if w = do(http.MethodPut, "/v1/accounts/7/managed/proxy", `{"upstream_proxy_url":"socks5://other.example:1080"}`); w.Code != 200 || len(p.rebinds) != 1 {
		t.Fatalf("proxy change=%d %s", w.Code, w.Body)
	}
	// The model list is served by the runtime with the slot's own key, never a token.
	p.forward = func(_ context.Context, r Resources, req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/v1/models" || req.Header.Get("Authorization") != "" || r.APIKey == "" {
			t.Errorf("models request reached the runtime as %s %s", req.Method, req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"gpt-5.5"}]}`))}, nil
	}
	if w = do(http.MethodGet, "/v1/accounts/7/models", ""); w.Code != 409 {
		t.Fatalf("models served before login: %d", w.Code)
	}
	p.set(loggedIn("chatgpt-7"))
	if w = do(http.MethodGet, "/v1/accounts/7/managed", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"ready":true`) {
		t.Fatalf("login not picked up: %d %s", w.Code, w.Body)
	}
	if w = do(http.MethodGet, "/v1/accounts/7/models", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "gpt-5.5") {
		t.Fatalf("models=%d %s", w.Code, w.Body)
	}
	if w = do(http.MethodPost, "/v1/accounts/7/models", "{}"); w.Code == 200 {
		t.Fatal("POST models accepted")
	}
	// The original routes are untouched by the new prefix.
	if w = do(http.MethodGet, "/v1/accounts/7", ""); w.Code != 200 {
		t.Fatalf("plain status=%d", w.Code)
	}
	if w = do(http.MethodPost, "/v1/accounts/7/unknown/deeper/path", "{}"); w.Code != 404 {
		t.Fatalf("unknown deep path=%d", w.Code)
	}
}

func TestManagedRoutesNeedAManagedCapableRuntime(t *testing.T) {
	m := newTestManager(t, &testProvider{})
	_, _, err := m.ProvisionManaged(context.Background(), managedInput(7))
	requireCode(t, err, "managed_unsupported")
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, authorizedRequest(http.MethodGet, "/v1/accounts/7/managed", ""))
	if w.Code != 501 {
		t.Fatalf("status=%d", w.Code)
	}
}

// A slot the runtime refused for a reason an administrator can fix keeps that
// reason; the binding is still recorded as failed so it can be cleaned up.
func TestManagedCreateRefusedByTheRuntimeKeepsTheReason(t *testing.T) {
	p := &managedTestProvider{provisionErr: &CallError{Status: 409, Code: "proxy_bind_limit"}}
	m := newTestManager(t, p)
	status, _, err := m.ProvisionManaged(context.Background(), managedInput(7))
	requireCode(t, err, "proxy_bind_limit")
	var be *Error
	if !errors.As(err, &be) || be.Status != 409 {
		t.Fatalf("want a 409 refusal, got %v", err)
	}
	if status.State != stateFailed || status.LastErrorCode != "proxy_bind_limit" || status.VMID != "vm-7" {
		t.Fatalf("refused slot not recorded: %+v", status)
	}
	// The reason survives a restart, and a retry that succeeds clears it.
	reloaded, err := New(Config{Secret: testSecret, StateDir: m.cfg.StateDir}, p)
	if err != nil || reloaded.accounts[7].data.LastErrorCode != "proxy_bind_limit" {
		t.Fatalf("reason not persisted: %v", err)
	}
	p.provisionErr = nil
	status, _, err = m.ProvisionManaged(context.Background(), managedInput(7))
	if err != nil || status.State != stateAwaitingLogin || status.LastErrorCode != "" {
		t.Fatalf("retry did not recover: %+v %v", status, err)
	}
}

// newSlowManaged is a managed binding on a bridge that waits only briefly for
// a slot start, so a start that takes longer runs on in the background.
func newSlowManaged(t *testing.T, id int64) (*Manager, *managedTestProvider) {
	t.Helper()
	p := &managedTestProvider{}
	m, err := New(Config{Secret: testSecret, StateDir: t.TempDir(), DrainTimeout: 30 * time.Millisecond, DialTimeout: time.Second,
		ActivateWait: 30 * time.Millisecond, ActivateTimeout: 5 * time.Second, ActivateRetry: 200 * time.Millisecond, askAgain: 5 * time.Millisecond}, p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	if _, _, err = m.ProvisionManaged(context.Background(), managedInput(id)); err != nil {
		t.Fatal(err)
	}
	return m, p
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The first boot of a KVM slot takes over a minute. The read that notices the
// login must not wait for it: it answers "starting" and the binding opens when
// the slot is up. Reads in between never start a second boot.
func TestManagedSlowFirstStartRunsInTheBackground(t *testing.T) {
	m, p := newSlowManaged(t, 7)
	boot := make(chan struct{})
	p.activateHook = func(ctx context.Context) error {
		select {
		case <-boot:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	p.set(loggedIn("chatgpt-7"))
	started := time.Now()
	got, err := m.ManagedSnapshot(context.Background(), 7)
	if took := time.Since(started); took > time.Second {
		t.Fatalf("status read waited %v for the boot", took)
	}
	if err != nil || got.Ready || !got.Starting || got.State != stateAwaitingLogin || got.LastErrorCode != "" {
		t.Fatalf("read during boot: %+v %v", got, err)
	}
	// Still closed to inference while it boots.
	_, err = m.Forward(context.Background(), 7, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}")), nil)
	requireCode(t, err, "account_not_ready")
	for i := 0; i < 3; i++ {
		if got, err = m.ManagedSnapshot(context.Background(), 7); err != nil || !got.Starting {
			t.Fatalf("repeat read: %+v %v", got, err)
		}
	}
	if activations, _ := p.count(); activations != 1 {
		t.Fatalf("a second boot was started: %d", activations)
	}
	close(boot)
	eventually(t, "the binding to open", func() bool { s, _ := m.Status(7); return s.Ready && !s.Starting })
	m.mu.Lock()
	pinned := m.accounts[7].data.ChatGPTAccountID
	m.mu.Unlock()
	if pinned != "chatgpt-7" {
		t.Fatalf("account not pinned: %q", pinned)
	}
	if _, err = m.Forward(context.Background(), 7, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}")), nil); err != nil {
		t.Fatalf("opened binding refused inference: %v", err)
	}
}

// A start that outlives one management call keeps running on the runtime;
// asking again waits for the same boot, and it is not reported as a failure.
func TestManagedStartThatOutlivesOneCallIsAskedAgain(t *testing.T) {
	m, p := newSlowManaged(t, 7)
	var asks int
	p.activateHook = func(context.Context) error {
		asks++
		if asks < 3 {
			return fmt.Errorf("vm2api start_slot: %w", context.DeadlineExceeded)
		}
		return nil
	}
	p.set(loggedIn("chatgpt-7"))
	_, _ = m.ManagedSnapshot(context.Background(), 7)
	eventually(t, "the binding to open", func() bool { s, _ := m.Status(7); return s.Ready })
	if activations, _ := p.count(); activations != 3 {
		t.Fatalf("asks=%d", activations)
	}
}

// A start that really failed is reported, is not repeated on every read, and
// is tried again once the retry pause has passed.
func TestManagedFailedStartIsReportedAndRetriedLater(t *testing.T) {
	m, p := newSlowManaged(t, 7)
	p.activateErr = errors.New("guest did not boot: internal detail")
	p.set(loggedIn("chatgpt-7"))
	got, err := m.ManagedSnapshot(context.Background(), 7)
	if err != nil || got.Ready || got.LastErrorCode != "activate_failed" || strings.Contains(fmt.Sprint(got), "internal detail") {
		t.Fatalf("failed start: %+v %v", got, err)
	}
	for i := 0; i < 3; i++ {
		got, _ = m.ManagedSnapshot(context.Background(), 7)
		if got.LastErrorCode != "activate_failed" || got.Starting {
			t.Fatalf("read during the retry pause: %+v", got)
		}
	}
	if activations, _ := p.count(); activations != 1 {
		t.Fatalf("failed start repeated on every read: %d", activations)
	}
	p.mu.Lock()
	p.activateErr = nil
	p.mu.Unlock()
	time.Sleep(250 * time.Millisecond)
	got, err = m.ManagedSnapshot(context.Background(), 7)
	if err != nil || !got.Ready || got.LastErrorCode != "" {
		t.Fatalf("retry after the pause: %+v %v", got, err)
	}
}

// What was true when the boot began may not be true when it ends.
func TestManagedStartOpensOnlyWhatIsStillTrue(t *testing.T) {
	for name, change := range map[string]func(*managedTestProvider){
		"login cleared":      func(p *managedTestProvider) { p.set(ManagedSnapshot{}) },
		"refresh token died": func(p *managedTestProvider) { s := loggedIn("chatgpt-7"); s.RefreshError = "invalid_grant"; p.set(s) },
		"another account":    func(p *managedTestProvider) { p.set(loggedIn("someone-else")) },
	} {
		t.Run(name, func(t *testing.T) {
			m, p := newSlowManaged(t, 7)
			boot := make(chan struct{})
			p.activateHook = func(context.Context) error { <-boot; return nil }
			p.set(loggedIn("chatgpt-7"))
			_, _ = m.ManagedSnapshot(context.Background(), 7)
			change(p)
			close(boot)
			eventually(t, "the start to end", func() bool { s, _ := m.Status(7); return !s.Starting })
			if s, _ := m.Status(7); s.Ready {
				t.Fatalf("opened after %s: %+v", name, s)
			}
		})
	}
	// A delete during the boot is refused before anything changes, and works once it is up.
	m, p := newSlowManaged(t, 8)
	boot := make(chan struct{})
	var deletes int
	p.delete = func(context.Context, Resources) error { deletes++; return nil }
	p.activateHook = func(context.Context) error { <-boot; return nil }
	p.set(loggedIn("chatgpt-8"))
	_, _ = m.ManagedSnapshot(context.Background(), 8)
	requireCode(t, m.Delete(context.Background(), 8), "slot_starting")
	if s, err := m.Status(8); err != nil || s.State != stateAwaitingLogin || !s.Starting || deletes != 0 {
		t.Fatalf("refused delete changed the binding: %+v %v deletes=%d", s, err, deletes)
	}
	close(boot)
	eventually(t, "the start to end", func() bool { s, _ := m.Status(8); return !s.Starting })
	if err := m.Delete(context.Background(), 8); err != nil || deletes != 1 {
		t.Fatalf("delete after the boot: %v deletes=%d", err, deletes)
	}
	if _, err := m.Status(8); err == nil {
		t.Fatal("binding survived delete")
	}
}

// Power-on boots the same guest. A slow one answers at once with "starting";
// while it runs, power and proxy changes are refused instead of queueing.
func TestManagedSlowPowerOnRunsInTheBackground(t *testing.T) {
	m, p := newSlowManaged(t, 7)
	p.set(loggedIn("chatgpt-7"))
	_, _ = m.ManagedSnapshot(context.Background(), 7)
	eventually(t, "login to open the binding", func() bool { s, _ := m.Status(7); return s.Ready })
	boot := make(chan struct{})
	p.mu.Lock()
	p.onCall = func(call ManagedCall) {
		if call.Operation == OpPowerOn {
			<-boot
		}
	}
	p.mu.Unlock()
	started := time.Now()
	out, err := m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpPowerOn})
	if err != nil || string(out) != `{"starting":true}` || time.Since(started) > time.Second {
		t.Fatalf("slow power-on: %s %v after %v", out, err, time.Since(started))
	}
	if s, _ := m.Status(7); !s.Starting || !s.Ready {
		t.Fatalf("status during power-on: %+v", s)
	}
	for _, op := range []string{OpPowerOn, OpPowerOff, OpRecover} {
		_, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: op})
		requireCode(t, err, "slot_starting")
	}
	_, err = m.RebindManaged(context.Background(), 7, "socks5h://other:x@other.example:1080")
	requireCode(t, err, "slot_starting")
	// Reads and other operations are not held up.
	if _, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpDiagnose}); err != nil {
		t.Fatalf("diagnose during boot: %v", err)
	}
	close(boot)
	eventually(t, "the power-on to end", func() bool { s, _ := m.Status(7); return !s.Starting })
	// A quick power-on is answered with the runtime's own result.
	if out, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpPowerOn}); err != nil || string(out) != `{"ok":true}` {
		t.Fatalf("quick power-on: %s %v", out, err)
	}
	// A refused power-on keeps the runtime's short code, never its text.
	p.mu.Lock()
	p.callErr = &CallError{Status: 409, Code: "proxy_required"}
	p.mu.Unlock()
	_, err = m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpPowerOn})
	requireCode(t, err, "proxy_required")
}

// A restart is never repeated by the bridge: asking again would restart again.
func TestManagedRestartIsNotRepeated(t *testing.T) {
	m, p := newSlowManaged(t, 7)
	p.callErr = fmt.Errorf("vm2api managed_recover: %w", context.DeadlineExceeded)
	_, err := m.ManagedCall(context.Background(), 7, ManagedCall{Operation: OpRecover})
	requireCode(t, err, "upstream_unavailable")
	if _, calls := p.count(); calls != 1 {
		t.Fatalf("restart sent %d times", calls)
	}
}

// Shutting the bridge down ends a running boot and waits for it.
func TestManagedCloseEndsABootInProgress(t *testing.T) {
	m, p := newSlowManaged(t, 7)
	p.activateHook = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
	p.set(loggedIn("chatgpt-7"))
	_, _ = m.ManagedSnapshot(context.Background(), 7)
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not end the boot")
	}
	if s, _ := m.Status(7); s.Ready || s.Starting {
		t.Fatalf("after close: %+v", s)
	}
}
