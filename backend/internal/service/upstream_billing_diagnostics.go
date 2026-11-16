package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// Diagnostics contain classifications only, never response bodies, request
// URLs, API keys or management authorization values.
type newAPIProbeDiagnostics struct {
	version string
	stage   string
	status  int
}

type newAPIProbeDiagnosticsKey struct{}

type newAPIProbeError struct {
	error
	reason string
}

func (e *newAPIProbeError) Unwrap() error { return e.error }

func newAPIProbeFailureReason(err error) string {
	var classified *newAPIProbeError
	if errors.As(err, &classified) {
		return classified.reason
	}
	return "new_api_request_failed"
}

func sanitizeUpstreamProbeVersion(value string) string {
	var out strings.Builder
	for _, c := range strings.TrimSpace(value) {
		if c >= 32 && c <= 126 {
			out.WriteRune(c)
			if out.Len() >= 96 {
				break
			}
		}
	}
	return out.String()
}

func classifyUpstreamBillingHTTPFailure(status int, header http.Header, body []byte) string {
	// A CF header alone is also present on ordinary application errors. Require
	// a challenge marker so a real authorization failure stays distinguishable.
	if status == 403 || status == 429 || status == 503 {
		text := strings.ToLower(string(body))
		isCF := strings.Contains(strings.ToLower(header.Get("Server")), "cloudflare") || header.Get("CF-Ray") != ""
		if header.Get("CF-Mitigated") == "challenge" || (isCF && (strings.Contains(text, "cf-chl-") || strings.Contains(text, "/cdn-cgi/challenge-platform/") || strings.Contains(text, "just a moment"))) {
			return "cloudflare_blocked"
		}
	}
	switch {
	case status == http.StatusUnauthorized:
		return "unauthorized"
	case status == http.StatusForbidden:
		return "forbidden"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status >= 500:
		return "upstream_unavailable"
	default:
		return "http_error"
	}
}

func applyUpstreamProbeDiagnostics(snapshot *UpstreamBillingProbeSnapshot, header http.Header, protocol, source, stage string) {
	snapshot.Protocol, snapshot.RateSource, snapshot.ProbeStage = protocol, source, stage
	if version := sanitizeUpstreamProbeVersion(header.Get("X-New-Api-Version")); version != "" {
		snapshot.Protocol, snapshot.UpstreamVersion = "newapi", version
	}
}

func observeNewAPIProbeRequest(ctx context.Context, path string) *newAPIProbeDiagnostics {
	diagnostics, _ := ctx.Value(newAPIProbeDiagnosticsKey{}).(*newAPIProbeDiagnostics)
	if diagnostics == nil {
		return nil
	}
	diagnostics.status = 0
	switch {
	case path == "/api/status":
		diagnostics.stage = "site_status"
	case path == "/api/user/self":
		diagnostics.stage = "user_wallet"
	case path == "/api/pricing":
		diagnostics.stage = "group_pricing"
	case strings.HasSuffix(path, "/key"):
		diagnostics.stage = "token_ownership"
	default:
		diagnostics.stage = "token_inventory"
	}
	return diagnostics
}

func applyNewAPIProbeDiagnostics(ctx context.Context, snapshot *UpstreamBillingProbeSnapshot) {
	snapshot.Protocol, snapshot.RateSource = "newapi", "bound_group_pricing"
	if diagnostics, _ := ctx.Value(newAPIProbeDiagnosticsKey{}).(*newAPIProbeDiagnostics); diagnostics != nil {
		snapshot.UpstreamVersion, snapshot.ProbeStage, snapshot.HTTPStatus = diagnostics.version, diagnostics.stage, diagnostics.status
	}
}
