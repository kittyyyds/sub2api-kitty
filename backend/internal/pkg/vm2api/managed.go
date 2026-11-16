package vm2api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/vmbridge"
)

var _ vmbridge.ManagedProvider = (*Provider)(nil)

// managedEndpoint is one vm2api panel call a managed binding may relay.
// `vm` marks operations whose body must also name the slot, because the panel
// endpoint is not slot-scoped by its path.
type managedEndpoint struct {
	method string
	path   string // %s is the slot ID
	vm     bool
}

// Nothing here returns stored login material: the panel's password-login
// "reveal" and its credential export are deliberately not mapped.
var managedEndpoints = map[string]managedEndpoint{
	vmbridge.OpOAuthURL:       {method: http.MethodPost, path: "/api/panel/vms/%s/oauth/generate-auth-url"},
	vmbridge.OpOAuthCode:      {method: http.MethodPost, path: "/api/panel/vms/%s/oauth/exchange-code"},
	vmbridge.OpPasswordStart:  {method: http.MethodPost, path: "/api/panel/vms/%s/oauth/password-login"},
	vmbridge.OpPasswordStatus: {method: http.MethodGet, path: "/api/panel/vms/%s/oauth/password-login"},
	vmbridge.OpPasswordCode:   {method: http.MethodPost, path: "/api/panel/vms/%s/oauth/password-login/code"},
	vmbridge.OpPasswordCancel: {method: http.MethodPost, path: "/api/panel/vms/%s/oauth/password-login/cancel"},
	vmbridge.OpPasswordForget: {method: http.MethodPost, path: "/api/panel/vms/%s/oauth/password-login/forget"},
	vmbridge.OpImport:         {method: http.MethodPost, path: "/api/panel/vms/import", vm: true},
	vmbridge.OpTokenRefresh:   {method: http.MethodPost, path: "/api/panel/vms/%s/oauth/refresh"},
	vmbridge.OpUsageRefresh:   {method: http.MethodPost, path: "/api/panel/vms/%s/openai-quota/refresh"},
	vmbridge.OpResetCredit:    {method: http.MethodPost, path: "/api/panel/vms/%s/openai-quota/reset"},
	vmbridge.OpPowerOn:        {method: http.MethodPost, path: "/api/panel/vms/%s/start"},
	vmbridge.OpPowerOff:       {method: http.MethodPost, path: "/api/panel/vms/%s/stop"},
	vmbridge.OpSchedulable:    {method: http.MethodPost, path: "/api/panel/vms/%s/schedulable"},
	vmbridge.OpPolicy:         {method: http.MethodPatch, path: "/api/panel/vms/%s"},
	vmbridge.OpDiagnose:       {method: http.MethodGet, path: "/api/panel/vms/%s/diagnose"},
	vmbridge.OpRecover:        {method: http.MethodPost, path: "/api/panel/vms/%s/recover"},
	vmbridge.OpExitProbe:      {method: http.MethodPost, path: "/api/panel/vms/%s/exit-probe"},
}

// Fields a caller may send per operation. Anything else is dropped before the
// request leaves the bridge: in particular `proxy_url`, which several panel
// endpoints accept as a one-off exit override, and `vm_id`, which the bridge
// sets itself from the binding.
var managedFields = map[string][]string{
	vmbridge.OpOAuthCode:      {"session_id", "code", "callback_url"},
	vmbridge.OpPasswordStart:  {"email", "password", "mfa_mode", "mfa_secret", "use_saved"},
	vmbridge.OpPasswordCode:   {"job_id", "code"},
	vmbridge.OpPasswordCancel: {"job_id"},
	vmbridge.OpImport:         {"auth_json", "raw", "mode"},
	vmbridge.OpSchedulable:    {"schedulable"},
	vmbridge.OpPolicy:         {"max_concurrency"},
}

// proxyBindLimit is the panel's refusal when an exit already carries as many
// slots as the runtime allows (5 by default, configurable up to 32).
const proxyBindLimit = "proxy_bind_limit"

// slotRefusal turns the one provisioning failure an administrator can act on
// into an error the bridge reports by its code.
func slotRefusal(err error) error {
	var api *APIError
	if errors.As(err, &api) && api.Code == proxyBindLimit {
		return &vmbridge.CallError{Status: http.StatusConflict, Code: proxyBindLimit}
	}
	return err
}

// ProvisionManaged creates the slot, binds its exit and issues its scoped key,
// and stops there: no credential is imported and the slot stays out of
// scheduling until a login succeeds. Every step is get-or-create.
func (p *Provider) ProvisionManaged(ctx context.Context, in vmbridge.ManagedProvisionRequest) (vmbridge.Resources, error) {
	var resources vmbridge.Resources
	if in.AccountID <= 0 || strings.TrimSpace(in.Name) == "" {
		return resources, errors.New("vm2api: account id and name required")
	}
	runtime := RuntimeType(in.RuntimeType)
	if runtime == "" {
		runtime = RuntimeDocker
	}
	if runtime != RuntimeDocker && runtime != RuntimeKVM {
		return resources, errors.New("vm2api: unsupported runtime")
	}
	if _, err := parseSocksProxy(in.UpstreamProxyURL); err != nil {
		return resources, err
	}
	slotID := DeterministicSlotID(p.namespace, in.AccountID)
	_, attempted, err := p.client.EnsureSlot(ctx, CreateSlotRequest{ID: slotID, Name: in.Name, RuntimeType: runtime, Start: false, GuestExecution: runtime == RuntimeKVM})
	if attempted || err == nil {
		resources.VMID = slotID
	}
	if err != nil {
		return resources, err
	}
	if err = p.client.SetSchedulable(ctx, slotID, false); err != nil {
		return resources, err
	}
	proxy, err := p.client.ImportProxy(ctx, ProxyImportRequest{URL: in.UpstreamProxyURL})
	if proxy != nil {
		resources.ProxyID = proxy.ID
	}
	if err != nil {
		return resources, err
	}
	if _, err = p.client.BindProxy(ctx, proxy.ID, slotID); err != nil {
		return resources, slotRefusal(err)
	}
	key, err := p.client.EnsureSingleSlotKey(ctx, keyName(slotID), slotID)
	if key != nil {
		resources.KeyID = key.ID
		resources.APIKey = key.Key
	}
	return resources, err
}

// Activate starts a logged-in slot and opens it to scheduling.
func (p *Provider) Activate(ctx context.Context, resources vmbridge.Resources) error {
	if err := validID(resources.VMID); err != nil {
		return err
	}
	if _, err := p.client.StartSlot(ctx, resources.VMID); err != nil {
		return err
	}
	return p.client.SetSchedulable(ctx, resources.VMID, true)
}

// Rebind moves the slot to another exit. The old proxy row is left in place:
// other slots may share it.
func (p *Provider) Rebind(ctx context.Context, resources vmbridge.Resources, upstreamProxyURL string) (string, error) {
	if err := validID(resources.VMID); err != nil {
		return "", err
	}
	proxy, err := p.client.ImportProxy(ctx, ProxyImportRequest{URL: upstreamProxyURL})
	if err != nil {
		return "", err
	}
	if _, err = p.client.BindProxy(ctx, proxy.ID, resources.VMID); err != nil {
		return "", slotRefusal(err)
	}
	return proxy.ID, nil
}

type managedDetailWire struct {
	VM struct {
		Status              string          `json:"status"`
		Platform            string          `json:"platform"`
		Schedulable         bool            `json:"schedulable"`
		ScheduleState       string          `json:"schedule_state"`
		RestrictionReason   string          `json:"restriction_reason"`
		RestrictionText     string          `json:"restriction_text"`
		UnschedulableReason string          `json:"unschedulable_reason"`
		UnschedulableText   string          `json:"unschedulable_text"`
		RestrictionUntil    json.Number     `json:"restriction_until"`
		HasToken            bool            `json:"has_token"`
		HasRefresh          bool            `json:"has_refresh"`
		RefreshError        string          `json:"refresh_error"`
		ExpiresAt           json.RawMessage `json:"expires_at"`
		Email               string          `json:"email"`
		OrgUUID             string          `json:"org_uuid"`
		PlanType            string          `json:"plan_type"`
		SubscriptionEnd     string          `json:"chatgpt_subscription_active_until"`
		RuntimeType         string          `json:"runtime_type"`
		CodexExecution      string          `json:"codex_execution"`
		MaxConcurrency      int             `json:"max_concurrency"`
		CodexUsage          json.RawMessage `json:"codex_usage"`
		CodexCredits        json.RawMessage `json:"codex_credits"`
		ResetCredits        json.RawMessage `json:"reset_credits"`
		Proxy               *struct {
			Label  string `json:"label"`
			Status string `json:"status"`
		} `json:"proxy"`
	} `json:"vm"`
	SavedLogin  *vmbridge.SavedLoginState `json:"saved_login"`
	AutoRelogin json.RawMessage           `json:"auto_relogin"`
}

func present(raw json.RawMessage) json.RawMessage {
	if s := strings.TrimSpace(string(raw)); s == "" || s == "null" {
		return nil
	}
	return raw
}

// expiry accepts the panel's encodings (an ISO string, or an epoch in
// milliseconds or seconds, as a number or as digits in a string) and always
// answers an RFC 3339 time, so nothing downstream has to guess the unit.
func expiry(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		s = strings.TrimSpace(text)
	}
	epoch, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s
	}
	if epoch <= 0 {
		return ""
	}
	if epoch < 100_000_000_000 {
		epoch *= 1000 // seconds
	}
	return time.UnixMilli(epoch).UTC().Format(time.RFC3339)
}

// Snapshot reads the slot's detail and keeps only what a controller may see.
func (p *Provider) Snapshot(ctx context.Context, resources vmbridge.Resources) (*vmbridge.ManagedSnapshot, error) {
	if err := validID(resources.VMID); err != nil {
		return nil, err
	}
	var wire managedDetailWire
	if err := p.client.control(ctx, http.MethodGet, "/api/panel/vms/"+resources.VMID, "managed_snapshot", nil, &wire); err != nil {
		return nil, err
	}
	vm := wire.VM
	if vm.Platform != "openai" {
		return nil, &APIError{Operation: "managed_snapshot", Code: "slot_identity_mismatch"}
	}
	snap := &vmbridge.ManagedSnapshot{
		Status:              vm.Status,
		Schedulable:         vm.Schedulable,
		ScheduleState:       vm.ScheduleState,
		RestrictionReason:   reasonCode(vm.RestrictionReason),
		RestrictionText:     vm.RestrictionText,
		UnschedulableReason: reasonCode(vm.UnschedulableReason),
		UnschedulableText:   vm.UnschedulableText,
		HasToken:            vm.HasToken,
		HasRefresh:          vm.HasRefresh,
		RefreshError:        vm.RefreshError,
		ExpiresAt:           expiry(vm.ExpiresAt),
		Email:               vm.Email,
		ChatGPTAccountID:    vm.OrgUUID,
		PlanType:            vm.PlanType,
		SubscriptionEnd:     vm.SubscriptionEnd,
		RuntimeType:         vm.RuntimeType,
		Execution:           vm.CodexExecution,
		MaxConcurrency:      vm.MaxConcurrency,
		Usage:               present(vm.CodexUsage),
		Credits:             present(vm.CodexCredits),
		ResetCredits:        present(vm.ResetCredits),
		SavedLogin:          wire.SavedLogin,
		AutoRelogin:         present(wire.AutoRelogin),
	}
	if until, err := vm.RestrictionUntil.Int64(); err == nil {
		snap.RestrictionUntil = until
	}
	if vm.Proxy != nil {
		snap.ProxyLabel, snap.ProxyStatus = vm.Proxy.Label, vm.Proxy.Status
	}
	return snap, nil
}

// reasonCode keeps a runtime reason only when it is a short identifier; a
// free-form reason could carry upstream text and is dropped.
func reasonCode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 64 || !resourceID.MatchString(raw) {
		return ""
	}
	return raw
}

// filterFields keeps only the fields an operation is allowed to carry.
func filterFields(operation string, body json.RawMessage) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(body)) == 0 {
		return out, nil
	}
	var in map[string]json.RawMessage
	if err := json.Unmarshal(body, &in); err != nil || in == nil {
		return nil, &vmbridge.CallError{Status: http.StatusBadRequest, Code: "invalid_json"}
	}
	for _, name := range managedFields[operation] {
		if value, ok := in[name]; ok {
			out[name] = value
		}
	}
	return out, nil
}

// Call relays one whitelisted operation to the panel and returns its `data`.
func (p *Provider) Call(ctx context.Context, resources vmbridge.Resources, call vmbridge.ManagedCall) (json.RawMessage, error) {
	endpoint, ok := managedEndpoints[call.Operation]
	if !ok {
		return nil, &vmbridge.CallError{Status: http.StatusNotFound, Code: "not_found"}
	}
	if err := validID(resources.VMID); err != nil {
		return nil, err
	}
	path := endpoint.path
	if strings.Contains(path, "%s") {
		path = fmt.Sprintf(path, resources.VMID)
	}
	var payload io.Reader
	if endpoint.method != http.MethodGet {
		fields, err := filterFields(call.Operation, call.Body)
		if err != nil {
			return nil, err
		}
		if endpoint.vm {
			// The binding decides which slot an import lands in, never the caller.
			id, _ := json.Marshal(resources.VMID)
			fields["vm_id"] = id
		}
		encoded, err := json.Marshal(fields)
		if err != nil {
			return nil, &vmbridge.CallError{Status: http.StatusBadRequest, Code: "invalid_json"}
		}
		payload = bytes.NewReader(encoded)
	}
	return p.client.relay(ctx, endpoint.method, path, "managed_"+call.Operation, payload)
}

// relay is control() for calls whose failure the caller needs to tell apart:
// it surfaces the panel's status and short error code, and nothing else of the
// error body, which can echo what was sent (a password, a pasted token).
func (c *Client) relay(ctx context.Context, method, path, operation string, body io.Reader) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.managementTimeout)
	defer cancel()
	req, err := c.request(ctx, method, path, c.masterKey, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(req, operation)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxControlBody+1))
	if err != nil || len(data) > maxControlBody {
		return nil, &APIError{Operation: operation, StatusCode: resp.StatusCode, Code: "read_failed"}
	}
	var env envelope
	parsed := json.Unmarshal(data, &env) == nil
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !parsed || env.OK == nil || !*env.OK {
		return nil, &vmbridge.CallError{Status: relayStatus(resp.StatusCode), Code: relayCode(env.Error)}
	}
	if len(env.Data) > 0 && string(env.Data) != "null" {
		return env.Data, nil
	}
	return json.RawMessage("{}"), nil
}

func relayStatus(status int) int {
	if status >= 400 && status <= 599 {
		return status
	}
	return http.StatusBadGateway
}

// relayCode extracts `error.code` only when it looks like an identifier.
func relayCode(raw json.RawMessage) string {
	var detail struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &detail) == nil && detail.Code != "" && len(detail.Code) <= 64 && resourceID.MatchString(detail.Code) {
		return detail.Code
	}
	return "runtime_error"
}
