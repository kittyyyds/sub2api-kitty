package service

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Stage real bridge session evidence before identity isolation and version
// selection. A cache key or equal message text does not identify a conversation.
// Anonymous requests get an attempt-stable seed, never an account-wide session.
func stageCodexHTTPClientSession(c *gin.Context, account *Account, body []byte) func() {
	restore := func() {}
	if c == nil || c.Request == nil || !codexIdentityLaneAccount(account) {
		return restore
	}
	parsed := gjson.ParseBytes(body)
	ev := collectCodexTurnEvidence(parsed.Get("client_metadata"), c.Request.Header, parsed)
	session := firstNonEmptyString(ev.ClientSessionID, ev.ClientThreadID)
	thread := firstNonEmptyString(ev.ClientThreadID, session)
	if session != "" {
		c.Set("codex_http_session_scope", session+"\x00"+thread)
	} else if ev.ClientTurnID != "" {
		c.Set("codex_http_session_scope", "turn:"+ev.ClientTurnID)
	}
	if ev.ClientSessionID != "" || ev.ClientThreadID != "" || ev.ClientTurnID != "" {
		return restore
	}
	session = strings.TrimSpace(extractClaudeCodeSessionID(c, body))
	if session == "" {
		session = codexAnonymousRequestSession(c)
	}
	c.Set("codex_http_session_scope", session+"\x00"+session)
	original := c.Request.Header
	headers := original.Clone()
	headers.Set("session-id", session)
	c.Request.Header = headers
	return func() { c.Request.Header = original }
}

// codexOAuthExtraClientHeaders are part of the Codex CLI HTTP request surface
// that the legacy whitelists drop. They are copied only on the Codex OAuth
// lane (not VM-routed) and only when the client actually sent them.
var codexOAuthExtraClientHeaders = map[string]bool{
	"session-id":                            true,
	"thread-id":                             true,
	"x-openai-subagent":                     true,
	"x-codex-parent-thread-id":              true,
	"x-openai-memgen-request":               true,
	"x-responsesapi-include-timing-metrics": true,
	"x-codex-inference-call-id":             true,
}

// allowOpenAIClientHeader extends a whitelist with the Codex OAuth extras.
// API-key/SetupToken/VM-routed/non-OpenAI accounts keep the whitelist byte-for-byte.
func allowOpenAIClientHeader(lowerKey string, allow map[string]bool, account *Account) bool {
	if allow[lowerKey] {
		return true
	}
	return isCodexOAuthAccount(account) && codexOAuthExtraClientHeaders[lowerKey]
}

// codexTraceparentPattern is the W3C trace-context version-00 shape
// (version-traceid-parentid-flags, lowercase hex).
var codexTraceparentPattern = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// copyOpenAICodexTraceHeaders propagates only a valid, caller-supplied W3C
// trace. A gateway request without tracing never acquires a synthetic trace
// just to populate CLI headers; an invalid traceparent drops tracestate too.
func copyOpenAICodexTraceHeaders(dst, src http.Header, account *Account) {
	if !isCodexOAuthAccount(account) || dst == nil || src == nil {
		return
	}
	parent := strings.TrimSpace(src.Get("traceparent"))
	if !codexTraceparentPattern.MatchString(parent) || parent[0:2] == "ff" ||
		parent[3:35] == strings.Repeat("0", 32) || parent[36:52] == strings.Repeat("0", 16) {
		return
	}
	dst.Set("traceparent", parent)
	if state := strings.TrimSpace(src.Get("tracestate")); state != "" {
		dst.Set("tracestate", state)
	}
}
