package vm2api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/vmbridge"
)

const managedProxyURL = "socks5h://user:proxy-secret@proxy.example:1080"

// managedPanel is a small stateful fake of the vm2api panel: enough of slots,
// proxies and keys to follow a managed slot from creation to a proxy change.
type managedPanel struct {
	mu        sync.Mutex
	t         *testing.T
	slotID    string
	slot      map[string]any
	key       *keyWire
	proxies   map[string]map[string]any // id -> row, with the reveal URI under "uri"
	events    []string
	bodies    map[string]map[string]any
	rawBodies map[string]string
	fail      map[string]int // "<METHOD> <path>" -> status to answer once
	failBody  string
}

func (p *managedPanel) handler(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	path, route := r.URL.Path, r.Method+" "+r.URL.Path
	p.events = append(p.events, route)
	if r.Header.Get("Authorization") != "Bearer master-secret" {
		p.t.Error("wrong management authentication")
	}
	raw, _ := io.ReadAll(r.Body)
	p.rawBodies[route] = string(raw)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	p.bodies[route] = body
	if status, ok := p.fail[route]; ok {
		delete(p.fail, route)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, p.failBody)
		return
	}
	slotPath := "/api/panel/vms/" + p.slotID
	switch {
	case route == "GET "+slotPath:
		if p.slot == nil {
			http.NotFound(w, r)
			return
		}
		replyData(w, map[string]any{
			"vm":           p.slot,
			"saved_login":  map[string]any{"email": "demo@example.test", "has_password": true, "has_mfa_secret": true, "mfa_mode": "secret"},
			"auto_relogin": map[string]any{"attempts_today": 1, "needs_attention": false},
			"billing":      map[string]any{"ignored": true},
		})
	case route == "POST /api/panel/vms/create":
		p.slot = map[string]any{"id": p.slotID, "name": body["name"], "platform": "openai", "family": "codex", "runtime_type": body["runtime_type"], "codex_execution": body["codex_execution"], "status": "stopped", "schedulable": false, "has_token": false}
		replyData(w, map[string]any{"vm": p.slot})
	case route == "POST "+slotPath+"/schedulable":
		p.slot["schedulable"] = body["schedulable"]
		replyData(w, map[string]any{"id": p.slotID, "schedulable": body["schedulable"]})
	case route == "POST "+slotPath+"/start":
		p.slot["status"] = "running"
		replyData(w, map[string]any{"vm": p.slot, "boot": map[string]any{"ok": true}})
	case route == "GET /api/panel/proxies":
		rows := []map[string]any{}
		for _, row := range p.proxies {
			public := map[string]any{}
			for k, v := range row {
				if k != "uri" {
					public[k] = v
				}
			}
			rows = append(rows, public)
		}
		replyData(w, map[string]any{"proxies": rows})
	case route == "POST /api/panel/proxies/import":
		id := "px-" + strings.SplitN(body["host"].(string), ".", 2)[0]
		port := int(body["port"].(float64))
		uri := "socks5h://" + body["username"].(string) + ":" + body["password"].(string) + "@" + body["host"].(string) + ":" + itoa(port)
		p.proxies[id] = map[string]any{"id": id, "host": body["host"], "port": port, "enabled": true, "bound_vm_ids": []string{}, "uri": uri}
		replyData(w, map[string]any{"added": 1, "items": []any{map[string]any{"id": id, "host": body["host"], "port": port, "enabled": true}}})
	case strings.HasPrefix(path, "/api/panel/proxies/") && strings.HasSuffix(path, "/reveal") && r.Method == http.MethodPost:
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/panel/proxies/"), "/reveal")
		replyData(w, map[string]any{"id": id, "uri": p.proxies[id]["uri"]})
	case strings.HasPrefix(path, "/api/panel/proxies/") && strings.HasSuffix(path, "/bind") && r.Method == http.MethodPost:
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/panel/proxies/"), "/bind")
		if body["vm_id"] != p.slotID {
			p.t.Error("proxy bound to a different slot")
		}
		// vm2api moves the slot off whatever exit it had before.
		for _, row := range p.proxies {
			row["bound_vm_ids"] = []string{}
		}
		p.proxies[id]["bound_vm_ids"] = []string{p.slotID}
		p.slot["proxy"] = map[string]any{"id": id, "label": "exit " + id, "status": "ok"}
		replyData(w, map[string]any{"proxy": map[string]any{"id": id, "enabled": true, "bound_vm_ids": []string{p.slotID}}})
	case route == "GET /api/panel/api-keys":
		keys := []keyWire{}
		if p.key != nil {
			masked := *p.key
			masked.Key = "masked"
			keys = append(keys, masked)
		}
		replyJSON(w, map[string]any{"ok": true, "keys": keys})
	case route == "POST /api/panel/api-keys":
		var k keyWire
		_ = json.Unmarshal(raw, &k)
		if k.Name != keyName(p.slotID) || !singleSlotKey(k.managed(), p.slotID) {
			p.t.Error("key not scoped to this slot")
		}
		k.ID, k.Key, k.Status = "key-1", "slot-secret", "active"
		p.key = &k
		replyJSON(w, map[string]any{"ok": true, "item": p.key})
	case route == "POST /api/panel/api-keys/key-1/reveal":
		replyJSON(w, map[string]any{"ok": true, "id": p.key.ID, "key": p.key.Key})
	default:
		// Every relayed operation lands here and is answered generically.
		replyData(w, map[string]any{"echo": route})
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func newManagedPanel(t *testing.T) (*managedPanel, *Provider) {
	t.Helper()
	panel := &managedPanel{t: t, slotID: DeterministicSlotID("installation-a", 17), proxies: map[string]map[string]any{}, bodies: map[string]map[string]any{}, rawBodies: map[string]string{}, fail: map[string]int{}}
	provider, err := NewProvider(testClient(t, panel.handler), "installation-a")
	if err != nil {
		t.Fatal(err)
	}
	return panel, provider
}

func managedRequest(runtime string) vmbridge.ManagedProvisionRequest {
	return vmbridge.ManagedProvisionRequest{AccountID: 17, Name: "managed 17", UpstreamProxyURL: managedProxyURL, RuntimeType: runtime}
}

func count(events []string, route string) int {
	n := 0
	for _, e := range events {
		if e == route {
			n++
		}
	}
	return n
}

func TestProvisionManagedCreatesAnEmptyStoppedSlotAndIsIdempotent(t *testing.T) {
	panel, provider := newManagedPanel(t)
	first, err := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	if err != nil {
		t.Fatal(err)
	}
	if first.VMID != panel.slotID || first.KeyID != "key-1" || first.APIKey != "slot-secret" || first.ProxyID != "px-proxy" {
		t.Fatalf("resources: %+v", first)
	}
	create := panel.bodies["POST /api/panel/vms/create"]
	// A KVM slot runs the kernel inside its guest; it is created stopped.
	if create["runtime_type"] != "kvm" || create["codex_execution"] != "kvm" || create["start"] != false || create["platform"] != "openai" {
		t.Fatalf("create body: %#v", create)
	}
	// No credential is ever imported, and the slot is neither started nor scheduled.
	for _, forbidden := range []string{"POST /api/panel/vms/import", "POST /api/panel/vms/" + panel.slotID + "/start", "POST /api/panel/vms/" + panel.slotID + "/bridge-identity"} {
		if count(panel.events, forbidden) != 0 {
			t.Fatalf("%s called while creating an empty slot", forbidden)
		}
	}
	if panel.slot["schedulable"] != false || panel.slot["status"] != "stopped" {
		t.Fatalf("slot left open: %#v", panel.slot)
	}
	// The proxy is bound before any login could happen.
	if !reflect.DeepEqual(panel.proxies["px-proxy"]["bound_vm_ids"], []string{panel.slotID}) {
		t.Fatal("exit not bound to the slot")
	}
	second, err := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	if err != nil || second != first {
		t.Fatalf("retry changed resources: %+v %v", second, err)
	}
	if count(panel.events, "POST /api/panel/vms/create") != 1 || count(panel.events, "POST /api/panel/proxies/import") != 1 || count(panel.events, "POST /api/panel/api-keys") != 1 {
		t.Fatal("retry created a second slot, proxy row or key")
	}
}

func TestProvisionManagedDockerSlotKeepsHostExecutionAndValidatesInput(t *testing.T) {
	panel, provider := newManagedPanel(t)
	if _, err := provider.ProvisionManaged(context.Background(), managedRequest("docker")); err != nil {
		t.Fatal(err)
	}
	if create := panel.bodies["POST /api/panel/vms/create"]; create["runtime_type"] != "docker" || create["codex_execution"] != nil {
		t.Fatalf("docker create body: %#v", create)
	}
	otherPanel, other := newManagedPanel(t)
	for name, in := range map[string]vmbridge.ManagedProvisionRequest{
		"no proxy":    {AccountID: 17, Name: "x", RuntimeType: "kvm"},
		"http proxy":  {AccountID: 17, Name: "x", RuntimeType: "kvm", UpstreamProxyURL: "http://proxy.example:8080"},
		"bad runtime": {AccountID: 17, Name: "x", RuntimeType: "lxc", UpstreamProxyURL: managedProxyURL},
		"no name":     {AccountID: 17, RuntimeType: "kvm", UpstreamProxyURL: managedProxyURL},
	} {
		if _, err := other.ProvisionManaged(context.Background(), in); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	// Bad input is refused before anything is created: a late refusal would
	// leave an empty slot behind on the runtime for every mistyped proxy.
	if len(otherPanel.events) != 0 {
		t.Fatalf("invalid input reached the runtime: %v", otherPanel.events)
	}
	// Guest execution is only meaningful on a KVM slot: asking for it on a
	// Docker slot is refused locally, without a request to the runtime.
	for _, runtime := range []RuntimeType{RuntimeDocker, ""} {
		if _, err := other.client.CreateSlot(context.Background(), CreateSlotRequest{ID: "slot-a", Name: "x", RuntimeType: runtime, GuestExecution: true}); err == nil {
			t.Fatalf("guest execution accepted on runtime %q", runtime)
		}
	}
	if len(otherPanel.events) != 0 {
		t.Fatalf("refused create reached the runtime: %v", otherPanel.events)
	}
}

func TestProvisionManagedReportsPartialResourcesWhenAStepFails(t *testing.T) {
	panel, provider := newManagedPanel(t)
	panel.fail["POST /api/panel/api-keys"] = 500
	resources, err := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	if err == nil || resources.VMID != panel.slotID || resources.ProxyID == "" || resources.APIKey != "" {
		t.Fatalf("partial resources not reported: %+v %v", resources, err)
	}
}

func TestSnapshotKeepsStatusAndDropsEverythingElse(t *testing.T) {
	panel, provider := newManagedPanel(t)
	resources, _ := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	panel.mu.Lock()
	for k, v := range map[string]any{
		"status": "running", "schedulable": true, "schedule_state": "restricted", "restriction_reason": "quota_7d_exhausted", "restriction_text": "7d 限制", "restriction_until": 1795182461158,
		"unschedulable_reason": "upstream said: token=abc was rejected", "unschedulable_text": "凭证失效",
		"has_token": true, "has_refresh": true, "refresh_error": "", "expires_at": 1795182461158, "email": "demo@example.test",
		"org_uuid": "chatgpt-17", "plan_type": "plus", "chatgpt_subscription_active_until": "2026-12-01T00:00:00Z",
		"max_concurrency": 2, "codex_usage": map[string]any{"quota": map[string]any{"utilization_5h": 0.4}},
		"codex_credits": map[string]any{"has_credits": false}, "reset_credits": nil,
		// Things a panel operator sees but a controller must not.
		"access_token": "leak-at", "owner_user_id": "u1",
	} {
		panel.slot[k] = v
	}
	panel.mu.Unlock()
	snap, err := provider.Snapshot(context.Background(), resources)
	if err != nil {
		t.Fatal(err)
	}
	// A reason is passed on only as a short code; free text from upstream is dropped.
	if snap.RestrictionReason != "quota_7d_exhausted" || snap.UnschedulableReason != "" || snap.UnschedulableText != "凭证失效" {
		t.Fatalf("reasons: %+v", snap)
	}
	if snap.Status != "running" || !snap.Schedulable || snap.ScheduleState != "restricted" || snap.RestrictionText != "7d 限制" || snap.RestrictionUntil != 1795182461158 {
		t.Fatalf("schedule fields: %+v", snap)
	}
	if !snap.HasToken || !snap.HasRefresh || snap.ChatGPTAccountID != "chatgpt-17" || snap.Email != "demo@example.test" || snap.PlanType != "plus" || snap.ExpiresAt != "2026-11-20T13:47:41Z" || snap.SubscriptionEnd == "" {
		t.Fatalf("credential fields: %+v", snap)
	}
	if snap.RuntimeType != "kvm" || snap.Execution != "kvm" || snap.MaxConcurrency != 2 || snap.ProxyLabel == "" || snap.ProxyStatus != "ok" {
		t.Fatalf("runtime fields: %+v", snap)
	}
	if snap.SavedLogin == nil || !snap.SavedLogin.HasMFASecret || !strings.Contains(string(snap.AutoRelogin), "attempts_today") || snap.ResetCredits != nil || !strings.Contains(string(snap.Usage), "utilization_5h") {
		t.Fatalf("login fields: %+v", snap)
	}
	out, _ := json.Marshal(snap)
	for _, leak := range []string{"leak-at", "owner_user_id", "ignored", "proxy-secret", "slot-secret"} {
		if strings.Contains(string(out), leak) {
			t.Fatalf("snapshot leaked %q: %s", leak, out)
		}
	}
	// An ISO expiry is passed through as text.
	panel.mu.Lock()
	panel.slot["expires_at"] = "2026-12-01T00:00:00Z"
	panel.mu.Unlock()
	if snap, _ = provider.Snapshot(context.Background(), resources); snap.ExpiresAt != "2026-12-01T00:00:00Z" {
		t.Fatalf("ISO expiry: %q", snap.ExpiresAt)
	}
	// A slot that is not an OpenAI slot is never reported as this account's.
	panel.mu.Lock()
	panel.slot["platform"] = "anthropic"
	panel.mu.Unlock()
	if _, err = provider.Snapshot(context.Background(), resources); err == nil {
		t.Fatal("foreign slot accepted")
	}
}

func TestActivateStartsThenSchedulesAndRebindMovesTheSlot(t *testing.T) {
	panel, provider := newManagedPanel(t)
	resources, _ := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	if err := provider.Activate(context.Background(), resources); err != nil {
		t.Fatal(err)
	}
	if panel.slot["status"] != "running" || panel.slot["schedulable"] != true {
		t.Fatalf("not activated: %#v", panel.slot)
	}
	start, schedule := -1, -1
	for i, e := range panel.events {
		if e == "POST /api/panel/vms/"+panel.slotID+"/start" {
			start = i
		}
		if e == "POST /api/panel/vms/"+panel.slotID+"/schedulable" {
			schedule = i
		}
	}
	if start < 0 || schedule < start {
		t.Fatal("slot scheduled before it was started")
	}
	id, err := provider.Rebind(context.Background(), resources, "socks5h://other:second-secret@other.example:1080")
	if err != nil || id != "px-other" {
		t.Fatalf("rebind: %q %v", id, err)
	}
	if !reflect.DeepEqual(panel.proxies["px-other"]["bound_vm_ids"], []string{panel.slotID}) || len(panel.proxies["px-proxy"]["bound_vm_ids"].([]string)) != 0 {
		t.Fatal("slot not moved to the new exit")
	}
	// The old row is kept: other slots may share it.
	if _, kept := panel.proxies["px-proxy"]; !kept {
		t.Fatal("old proxy row deleted")
	}
	// Going back reuses the existing row instead of importing a duplicate.
	if id, err = provider.Rebind(context.Background(), resources, managedProxyURL); err != nil || id != "px-proxy" || count(panel.events, "POST /api/panel/proxies/import") != 2 {
		t.Fatalf("rebind back: %q %v imports=%d", id, err, count(panel.events, "POST /api/panel/proxies/import"))
	}
	if _, err = provider.Rebind(context.Background(), resources, "http://proxy.example:8080"); err == nil {
		t.Fatal("http proxy accepted")
	}
}

func TestCallMapsEachOperationToItsEndpointAndFiltersFields(t *testing.T) {
	panel, provider := newManagedPanel(t)
	resources, _ := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	slot := "/api/panel/vms/" + panel.slotID
	for op, route := range map[string]string{
		vmbridge.OpOAuthURL:       "POST " + slot + "/oauth/generate-auth-url",
		vmbridge.OpOAuthCode:      "POST " + slot + "/oauth/exchange-code",
		vmbridge.OpPasswordStart:  "POST " + slot + "/oauth/password-login",
		vmbridge.OpPasswordStatus: "GET " + slot + "/oauth/password-login",
		vmbridge.OpPasswordCode:   "POST " + slot + "/oauth/password-login/code",
		vmbridge.OpPasswordCancel: "POST " + slot + "/oauth/password-login/cancel",
		vmbridge.OpPasswordForget: "POST " + slot + "/oauth/password-login/forget",
		vmbridge.OpImport:         "POST /api/panel/vms/import",
		vmbridge.OpTokenRefresh:   "POST " + slot + "/oauth/refresh",
		vmbridge.OpUsageRefresh:   "POST " + slot + "/openai-quota/refresh",
		vmbridge.OpResetCredit:    "POST " + slot + "/openai-quota/reset",
		vmbridge.OpPowerOn:        "POST " + slot + "/start",
		vmbridge.OpPowerOff:       "POST " + slot + "/stop",
		vmbridge.OpSchedulable:    "POST " + slot + "/schedulable",
		vmbridge.OpPolicy:         "PATCH " + slot,
		vmbridge.OpDiagnose:       "GET " + slot + "/diagnose",
		vmbridge.OpRecover:        "POST " + slot + "/recover",
		vmbridge.OpExitProbe:      "POST " + slot + "/exit-probe",
	} {
		before := count(panel.events, route)
		if _, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: op}); err != nil {
			// start/stop/schedulable answer with their own shapes in the fake; only the route matters here.
			var ce *vmbridge.CallError
			if !errors.As(err, &ce) {
				t.Fatalf("%s: %v", op, err)
			}
		}
		if count(panel.events, route) != before+1 {
			t.Fatalf("%s did not reach %s", op, route)
		}
	}
	// The panel's reveal and credential-export endpoints are not reachable at all.
	for _, op := range []string{"login.reveal", "credential.export", ""} {
		if _, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: op}); err == nil {
			t.Fatalf("unmapped operation %q relayed", op)
		}
	}
	for _, e := range panel.events {
		if strings.Contains(e, "/reveal") && strings.Contains(e, "password-login") || strings.Contains(e, "/oauth/credential") {
			t.Fatalf("secret-returning endpoint called: %s", e)
		}
	}

	// Only the fields an operation needs are passed on. A one-off exit override
	// and a caller-chosen slot are dropped; the slot always comes from the binding.
	login := `{"email":"a@b.test","password":"p w","mfa_mode":"secret","mfa_secret":"JBSWY3DPEHPK3PXP","use_saved":false,"proxy_url":"socks5://attacker.example:1","vm_id":"other-slot","extra":1}`
	if _, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: vmbridge.OpPasswordStart, Body: json.RawMessage(login)}); err != nil {
		t.Fatal(err)
	}
	sent := panel.bodies["POST "+slot+"/oauth/password-login"]
	want := map[string]any{"email": "a@b.test", "password": "p w", "mfa_mode": "secret", "mfa_secret": "JBSWY3DPEHPK3PXP", "use_saved": false}
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("password login body: %#v", sent)
	}
	if _, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: vmbridge.OpImport, Body: json.RawMessage(`{"auth_json":{"tokens":{"access_token":"x"}},"vm_id":"other-slot","proxy_url":"socks5://attacker.example:1"}`)}); err != nil {
		t.Fatal(err)
	}
	imported := panel.bodies["POST /api/panel/vms/import"]
	if imported["vm_id"] != panel.slotID || imported["proxy_url"] != nil || imported["auth_json"] == nil {
		t.Fatalf("import body: %#v", imported)
	}
	if _, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: vmbridge.OpPolicy, Body: json.RawMessage(`{"max_concurrency":4,"weight":99,"allowed_models":["x"]}`)}); err != nil {
		t.Fatal(err)
	}
	if policy := panel.bodies["PATCH "+slot]; !reflect.DeepEqual(policy, map[string]any{"max_concurrency": float64(4)}) {
		t.Fatalf("policy body: %#v", policy)
	}
	// An operation with no allowed fields sends an empty object, whatever came in.
	if _, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: vmbridge.OpOAuthURL, Body: json.RawMessage(`{"proxy_url":"socks5://attacker.example:1"}`)}); err != nil {
		t.Fatal(err)
	}
	if raw := panel.rawBodies["POST "+slot+"/oauth/generate-auth-url"]; raw != "{}" {
		t.Fatalf("oauth-url body: %s", raw)
	}
	if _, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: vmbridge.OpPasswordStart, Body: json.RawMessage(`[1]`)}); err == nil {
		t.Fatal("non-object body relayed")
	}
}

func TestCallPassesOnTheRuntimeCodeButNeverItsMessage(t *testing.T) {
	panel, provider := newManagedPanel(t)
	resources, _ := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	route := "POST /api/panel/vms/" + panel.slotID + "/oauth/password-login"
	for _, tc := range []struct {
		status       int
		body         string
		wantStatus   int
		wantCode     string
		mustNotExist string
	}{
		{409, `{"ok":false,"error":{"code":"login_busy","message":"当前槽位已有登录任务 password=echoed-secret"}}`, 409, "login_busy", "echoed-secret"},
		{400, `{"ok":false,"error":{"code":"proxy_required","message":"x"}}`, 400, "proxy_required", ""},
		{400, `{"ok":false,"error":{"message":"no code, only text with secret-in-text"}}`, 400, "runtime_error", "secret-in-text"},
		{502, `<html>bad gateway internal-host-name</html>`, 502, "runtime_error", "internal-host-name"},
		{400, `{"ok":false,"error":{"code":"bad code with spaces and secret","message":"x"}}`, 400, "runtime_error", "secret"},
		{200, `{"ok":false,"error":{"code":"credential_changed"}}`, 502, "credential_changed", ""},
	} {
		panel.mu.Lock()
		panel.fail[route], panel.failBody = tc.status, tc.body
		panel.mu.Unlock()
		_, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: vmbridge.OpPasswordStart, Body: json.RawMessage(`{"password":"sent-password"}`)})
		var ce *vmbridge.CallError
		if !errors.As(err, &ce) || ce.Status != tc.wantStatus || ce.Code != tc.wantCode {
			t.Fatalf("status %d body %s -> %v", tc.status, tc.body, err)
		}
		if tc.mustNotExist != "" && strings.Contains(err.Error(), tc.mustNotExist) || strings.Contains(err.Error(), "sent-password") {
			t.Fatalf("error text leaked: %v", err)
		}
	}
	// A normal answer returns the runtime's data object untouched.
	out, err := provider.Call(context.Background(), resources, vmbridge.ManagedCall{Operation: vmbridge.OpPasswordStart})
	if err != nil || !strings.Contains(string(out), "password-login") {
		t.Fatalf("data not returned: %s %v", out, err)
	}
}

// An exit that already carries as many slots as the runtime allows is the one
// provisioning failure reported by its own code; anything else stays generic,
// and the runtime's message never travels.
func TestAFullExitIsReportedByItsCodeOnCreateAndOnProxyChange(t *testing.T) {
	panel, provider := newManagedPanel(t)
	full := `{"ok":false,"error":{"type":"invalid_request_error","code":"proxy_bind_limit","message":"SOCKS5 最多绑定 5 台虚拟机 internal-detail","details":{"max":5}}}`
	panel.fail["POST /api/panel/proxies/px-proxy/bind"], panel.failBody = 400, full
	resources, err := provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	var refused *vmbridge.CallError
	if !errors.As(err, &refused) || refused.Code != "proxy_bind_limit" || refused.Status != 409 {
		t.Fatalf("full exit not reported by its code: %v", err)
	}
	if strings.Contains(err.Error(), "internal-detail") || strings.Contains(err.Error(), "最多") {
		t.Fatalf("runtime message leaked: %v", err)
	}
	if resources.VMID != panel.slotID || resources.ProxyID == "" || resources.APIKey != "" {
		t.Fatalf("partial resources not kept for cleanup: %+v", resources)
	}

	// Any other refusal of the same call stays a generic failure.
	panel.mu.Lock()
	panel.fail["POST /api/panel/proxies/px-proxy/bind"], panel.failBody = 400, `{"ok":false,"error":{"code":"local_proxy_node_mismatch","message":"x"}}`
	panel.mu.Unlock()
	if _, err = provider.ProvisionManaged(context.Background(), managedRequest("kvm")); err == nil || errors.As(err, &refused) {
		t.Fatalf("an unrelated refusal was reported as a slot refusal: %v", err)
	}

	// The same reason on a later proxy change.
	resources, err = provider.ProvisionManaged(context.Background(), managedRequest("kvm"))
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	panel.mu.Lock()
	panel.fail["POST /api/panel/proxies/px-other/bind"], panel.failBody = 400, full
	panel.mu.Unlock()
	if _, err = provider.Rebind(context.Background(), resources, "socks5h://other:second-secret@other.example:1080"); !errors.As(err, &refused) || refused.Code != "proxy_bind_limit" {
		t.Fatalf("full exit on rebind not reported by its code: %v", err)
	}
}

// The panel reports a token's expiry as epoch milliseconds (as a number or as
// digits in a string), sometimes as seconds, and sometimes as ISO text. The
// bridge always answers an RFC 3339 time.
func TestExpiryIsAlwaysATime(t *testing.T) {
	for raw, want := range map[string]string{
		`1795352052000`:          "2026-11-22T12:54:12Z",
		`"1795352052000"`:        "2026-11-22T12:54:12Z",
		`1795352052`:             "2026-11-22T12:54:12Z",
		`"2026-12-01T00:00:00Z"`: "2026-12-01T00:00:00Z",
		`null`:                   "",
		`""`:                     "",
		`0`:                      "",
		`-5`:                     "",
	} {
		if got := expiry(json.RawMessage(raw)); got != want {
			t.Errorf("expiry(%s) = %q, want %q", raw, got, want)
		}
	}
}
