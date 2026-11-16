package vmbridge

import (
	"encoding/json"
	"io"
	"net/http"
)

// managedRoutes maps "<METHOD> <path under /v1/accounts/{id}/managed/>" to the
// operation it relays. Every route here is one runtime management call; none
// returns a stored password, 2FA secret or token.
var managedRoutes = map[string]string{
	"POST login/oauth-url":       OpOAuthURL,
	"POST login/oauth-code":      OpOAuthCode,
	"POST login/password":        OpPasswordStart,
	"GET login/password":         OpPasswordStatus,
	"POST login/password/code":   OpPasswordCode,
	"POST login/password/cancel": OpPasswordCancel,
	"POST login/password/forget": OpPasswordForget,
	"POST login/import":          OpImport,
	"POST token/refresh":         OpTokenRefresh,
	"POST usage/refresh":         OpUsageRefresh,
	"POST usage/reset-credit":    OpResetCredit,
	"POST power/on":              OpPowerOn,
	"POST power/off":             OpPowerOff,
	"POST schedulable":           OpSchedulable,
	"PUT policy":                 OpPolicy,
	"GET diagnose":               OpDiagnose,
	"POST recover":               OpRecover,
	"POST exit-probe":            OpExitProbe,
}

// An account file can be a full Codex auth.json; everything else is a few fields.
const (
	managedBodyLimit = 64 << 10
	managedImportMax = 256 << 10
)

func (m *Manager) serveManaged(w http.ResponseWriter, r *http.Request, id int64, path string) {
	switch {
	case path == "" && r.Method == http.MethodPost:
		var input ManagedProvisionRequest
		if !decodeBody(w, r, &input) {
			return
		}
		input.AccountID = id
		status, created, err := m.ProvisionManaged(r.Context(), input)
		if err != nil {
			writeError(w, err)
			return
		}
		code := 200
		if created {
			code = 201
		}
		writeJSON(w, code, status)
		return
	case path == "" && r.Method == http.MethodGet:
		status, err := m.ManagedSnapshot(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, status)
		return
	case path == "proxy" && r.Method == http.MethodPut:
		var input struct {
			UpstreamProxyURL string `json:"upstream_proxy_url"`
		}
		if !decodeBody(w, r, &input) {
			return
		}
		status, err := m.RebindManaged(r.Context(), id, input.UpstreamProxyURL)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, status)
		return
	}
	operation, ok := managedRoutes[r.Method+" "+path]
	if !ok {
		writeError(w, bridgeError(404, "not_found", "endpoint not found"))
		return
	}
	limit := int64(managedBodyLimit)
	if operation == OpImport {
		limit = managedImportMax
	}
	body, ok := readManagedBody(w, r, limit)
	if !ok {
		return
	}
	out, err := m.ManagedCall(r.Context(), id, ManagedCall{Operation: operation, Body: body})
	if err != nil {
		writeError(w, err)
		return
	}
	if len(out) == 0 {
		out = json.RawMessage("{}")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// readManagedBody accepts an empty body or exactly one JSON object. The object
// is passed on as-is: the runtime validates its fields, and it may hold a
// password or 2FA secret, so it is never decoded into anything that is logged.
func readManagedBody(w http.ResponseWriter, r *http.Request, limit int64) (json.RawMessage, bool) {
	if r.Method == http.MethodGet {
		return nil, true
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		writeError(w, bridgeError(400, "invalid_json", "request body is too large or unreadable"))
		return nil, false
	}
	if len(data) == 0 {
		return json.RawMessage("{}"), true
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		writeError(w, bridgeError(400, "invalid_json", "exactly one JSON object is required"))
		return nil, false
	}
	return data, true
}

// ManagedRouteKnown reports whether the bridge relays this method and path
// (the part after /managed/). Controllers use it to check that every operation
// they offer is one the bridge accepts.
func ManagedRouteKnown(method, path string) bool {
	_, ok := managedRoutes[method+" "+path]
	return ok
}
