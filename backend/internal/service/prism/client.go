// Package prism contains the narrow protocol adapter for prism.openai.com.
//
// Prism is a private, browser-oriented API.  It is deliberately kept behind
// this package so the OpenAI gateway does not accidentally reuse the OAuth,
// BPS, or normal Responses transport.  The adapter accepts exactly one
// already-selected account; account pooling and billing stay in SUB.
package prism

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// Protocol is the account extra value used by the future gateway dispatch.
	// Keeping it in this package avoids scattering a private-protocol string
	// through the normal OpenAI and Excel/BPS paths.
	Protocol             = "prism"
	DefaultBaseURL       = "https://prism.openai.com"
	defaultRequestTTL    = 90 * time.Second
	defaultPollInterval  = 1500 * time.Millisecond
	defaultPollAttempts  = 80
	maxResponseBodyBytes = 16 << 20
)

// DoFunc is injected by the gateway so Prism requests use the account's SUB
// proxy and HTTP/TLS policy. It must not follow redirects or log request
// headers/bodies. The adapter cannot constrain a transport after Do is called.
type DoFunc func(*http.Request) (*http.Response, error)

type Sandbox struct {
	ProjectID        string
	SandboxToken     string
	SandboxURL       string
	SandboxSessionID string
	YSweetURL        string
	YSweetBaseURL    string
}

type Config struct {
	BaseURL      string
	Cookie       string
	ProjectID    string
	UserID       string
	Do           DoFunc
	RequestTTL   time.Duration
	PollInterval time.Duration
	PollAttempts int
	SkipSandbox  bool // only for protocol tests; production must leave false
}

type Client struct {
	cfg Config
	mu  sync.Mutex
	sb  *Sandbox
	run chan struct{}
	// A timeout after start does not cancel the remote operation. Keep this
	// client quarantined rather than overlap another turn on that sandbox.
	uncertain bool
}

type InputMessage struct {
	Type    string        `json:"type,omitempty"`
	Role    string        `json:"role"`
	Content []ContentPart `json:"content"`
}

type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Request struct {
	Model           string
	ReasoningEffort string
	ConversationID  string
	Input           []InputMessage
}

type Usage struct {
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
	ReasoningTokens   int
	TotalTokens       int
}

type Result struct {
	ResponseID string
	// Model is the upstream payload's label, not proof of the model weights.
	// It stays empty when Prism omits that field; never echo the requested name.
	Model         string
	Text          string
	ReasoningText string
	Usage         *Usage
	// UsageSource is unknown when only session-cumulative usage is available.
	UsageSource string
	RequestID   string
	Duration    time.Duration
}

type ErrorKind string

const (
	ErrorUnauthorized   ErrorKind = "unauthorized"
	ErrorRateLimited    ErrorKind = "rate_limited"
	ErrorProject        ErrorKind = "project"
	ErrorTimeout        ErrorKind = "timeout"
	ErrorProtocol       ErrorKind = "protocol"
	ErrorTransport      ErrorKind = "transport"
	ErrorOutcomeUnknown ErrorKind = "outcome_unknown"
)

// Error never contains the Cookie or an upstream response body.
type Error struct {
	Kind   ErrorKind
	Status int
	Op     string
	Msg    string
	// RequestStarted means a model turn may have been accepted. The host must
	// not retry it on this or another account merely because it saw an error.
	RequestStarted bool
}

func (e *Error) Error() string {
	if e == nil {
		return "prism error"
	}
	if e.Status > 0 {
		return fmt.Sprintf("prism %s failed (HTTP %d): %s", e.Op, e.Status, e.Msg)
	}
	return fmt.Sprintf("prism %s failed: %s", e.Op, e.Msg)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return nil
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Cookie) == "" {
		return nil, &Error{Kind: ErrorUnauthorized, Op: "config", Msg: "Prism Cookie is required"}
	}
	// Cookie is copied into an HTTP header. Reject control characters up front
	// so malformed account data cannot become header injection when a gateway
	// transport is added later. Do not include the value in the returned error.
	if strings.ContainsAny(cfg.Cookie, "\r\n") {
		return nil, &Error{Kind: ErrorUnauthorized, Op: "config", Msg: "Prism Cookie contains invalid characters"}
	}
	if cfg.Do == nil {
		return nil, &Error{Kind: ErrorTransport, Op: "config", Msg: "Prism HTTP transport is required"}
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if strings.TrimRight(cfg.BaseURL, "/") != DefaultBaseURL {
		return nil, &Error{Kind: ErrorProtocol, Op: "config", Msg: "only the official Prism HTTPS origin is allowed"}
	}
	if cfg.ProjectID != "" && !validUUID(cfg.ProjectID) {
		return nil, &Error{Kind: ErrorProject, Op: "config", Msg: "project ID must be a UUID"}
	}
	if cfg.RequestTTL <= 0 {
		cfg.RequestTTL = defaultRequestTTL
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.PollAttempts <= 0 {
		cfg.PollAttempts = defaultPollAttempts
	}
	return &Client{cfg: cfg, run: make(chan struct{}, 1)}, nil
}

func (c *Client) Sandbox() *Sandbox {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sb == nil {
		return nil
	}
	cp := *c.sb
	return &cp
}

func (c *Client) InvalidateSandbox() {
	c.mu.Lock()
	c.sb = nil
	c.mu.Unlock()
}

func (c *Client) Run(ctx context.Context, req Request) (result Result, runErr error) {
	started := time.Now()
	if strings.TrimSpace(req.Model) == "" {
		return Result{}, &Error{Kind: ErrorProtocol, Op: "request", Msg: "model is required"}
	}
	if len(req.Input) == 0 {
		return Result{}, &Error{Kind: ErrorProtocol, Op: "request", Msg: "input is required"}
	}
	if req.ConversationID == "" {
		req.ConversationID = "cdx1_" + randomID()
	}
	if req.ReasoningEffort == "" {
		req.ReasoningEffort = "medium"
	}
	callCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTTL)
	defer cancel()
	select {
	case c.run <- struct{}{}:
		defer func() { <-c.run }()
	case <-callCtx.Done():
		return Result{}, &Error{Kind: ErrorTimeout, Op: "queue", Msg: "request cancelled or timed out before start"}
	}
	if callCtx.Err() != nil {
		return Result{}, &Error{Kind: ErrorTimeout, Op: "queue", Msg: "request cancelled or timed out before start"}
	}
	if c.uncertain {
		return Result{}, &Error{Kind: ErrorOutcomeUnknown, Op: "start", Msg: "previous turn outcome is unknown; reconcile before another request", RequestStarted: true}
	}
	startedRemote := false
	defer func() {
		if runErr != nil && startedRemote {
			var e *Error
			if errors.As(runErr, &e) {
				e.RequestStarted = true
			}
			c.uncertain = true
		}
	}()

	sb, err := c.ensureSandbox(callCtx)
	if err != nil {
		return Result{}, err
	}
	metadata := map[string]any{
		"projectId":        sb.ProjectID,
		"userId":           c.cfg.UserID,
		"reasoning_effort": req.ReasoningEffort,
		"frontend_origin":  c.baseURL(),
		"sandbox_url":      strings.TrimRight(sb.SandboxURL, "/") + "/",
		"sandbox_token":    sb.SandboxToken,
	}
	if strings.ToLower(req.Model) != "auto" && strings.ToLower(req.Model) != "default" {
		metadata["model"] = req.Model
	}
	input := append([]InputMessage(nil), req.Input...)
	for i := range input {
		input[i].Type = "message"
	}
	startBody := map[string]any{
		"input":          input,
		"metadata":       metadata,
		"conversationId": req.ConversationID,
	}
	start, err := c.json(callCtx, http.MethodPost, "/api/llm/response_with_tools_start", startBody, nil, "start")
	if err != nil {
		var e *Error
		if errors.As(err, &e) && (e.Status == 0 || e.Kind == ErrorTimeout || e.Status >= 500) {
			startedRemote = true
		}
		return Result{}, err
	}
	startedRemote = true
	requestID := stringValue(start, "request_id")
	turnState := stringValue(start, "turn_state")
	result = Result{RequestID: requestID, Duration: time.Since(started)}
	usage := usageFromProgress(start)
	if stringValue(start, "status") == "completed" {
		parsed, parseErr := parsePayload(start)
		if parseErr != nil {
			return Result{}, parseErr
		}
		result = mergeResult(result, parsed, usage)
		result.Duration = time.Since(started)
		if result.Text == "" {
			return Result{}, &Error{Kind: ErrorProtocol, Op: "start", Msg: "completed response did not contain output text"}
		}
		return result, nil
	}
	if requestID == "" {
		return Result{}, &Error{Kind: ErrorProtocol, Op: "start", Msg: "missing request_id"}
	}
	for i := 0; i < c.cfg.PollAttempts; i++ {
		select {
		case <-callCtx.Done():
			return Result{}, &Error{Kind: ErrorTimeout, Op: "status", Msg: "request timed out"}
		case <-time.After(c.cfg.PollInterval):
		}
		statusBody, statusErr := c.json(callCtx, http.MethodPost, "/api/llm/response_with_tools_status", map[string]any{
			"request_id": requestID,
			"turn_state": turnState,
		}, nil, "status")
		if statusErr != nil {
			return Result{}, statusErr
		}
		if value := stringValue(statusBody, "turn_state"); value != "" {
			turnState = value
		}
		if next := usageFromProgress(statusBody); next != nil {
			usage = next
		}
		switch stringValue(statusBody, "status") {
		case "completed":
			parsed, parseErr := parsePayload(statusBody)
			if parseErr != nil {
				return Result{}, parseErr
			}
			result = mergeResult(result, parsed, usage)
			result.Duration = time.Since(started)
			if result.Text == "" {
				return Result{}, &Error{Kind: ErrorProtocol, Op: "status", Msg: "completed response did not contain output text"}
			}
			return result, nil
		case "failed", "error":
			return Result{}, &Error{Kind: ErrorProtocol, Op: "status", Msg: "upstream response failed"}
		}
	}
	return Result{}, &Error{Kind: ErrorTimeout, Op: "status", Msg: "request timed out"}
}

func (c *Client) baseURL() string { return strings.TrimRight(c.cfg.BaseURL, "/") }

func (c *Client) json(ctx context.Context, method, path string, body any, extra http.Header, op string) (map[string]any, error) {
	raw, err := c.exchange(ctx, method, path, body, extra, op)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil || decoded == nil {
		return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "invalid JSON response"}
	}
	return decoded, nil
}

func (c *Client) exchange(ctx context.Context, method, requestPath string, body any, extra http.Header, op string) ([]byte, error) {
	// JoinPath escapes '?' into the path. Resolve a parsed, same-origin
	// relative reference so query parameters remain query parameters.
	relative, err := url.Parse(requestPath)
	if err != nil || relative.IsAbs() || relative.Host != "" || relative.User != nil || relative.Fragment != "" || !strings.HasPrefix(relative.Path, "/") || strings.HasPrefix(relative.Path, "//") {
		return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "invalid Prism path"}
	}
	origin, _ := url.Parse(c.baseURL())
	u := origin.ResolveReference(relative)
	if u.Scheme != "https" || u.Host != "prism.openai.com" {
		return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "invalid Prism origin"}
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "invalid request payload"}
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "failed to create request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", c.baseURL())
	req.Header.Set("Referer", c.baseURL()+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SUB Prism adapter)")
	req.Header.Set("Cookie", c.cfg.Cookie)
	for k, values := range extra {
		for _, value := range values {
			req.Header.Add(k, value)
		}
	}
	resp, err := c.cfg.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, &Error{Kind: ErrorTimeout, Op: op, Msg: "request cancelled or timed out"}
		}
		return nil, &Error{Kind: ErrorTransport, Op: op, Msg: "request transport failed"}
	}
	if resp == nil || resp.Body == nil {
		return nil, &Error{Kind: ErrorTransport, Op: op, Msg: "empty Prism response"}
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if readErr != nil {
		return nil, &Error{Kind: ErrorTransport, Op: op, Msg: "failed to read Prism response"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyHTTPError(op, resp.StatusCode)
	}
	if len(raw) > maxResponseBodyBytes {
		return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "Prism response exceeds size limit"}
	}
	return raw, nil
}

func classifyHTTPError(op string, status int) *Error {
	kind := ErrorProtocol
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = ErrorUnauthorized
	case http.StatusTooManyRequests:
		kind = ErrorRateLimited
	case http.StatusNotFound:
		kind = ErrorProject
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		kind = ErrorTimeout
	}
	return &Error{Kind: kind, Status: status, Op: op, Msg: http.StatusText(status)}
}

func (c *Client) bootstrap(ctx context.Context, method, requestPath string, body any, extra http.Header, op string) ([]byte, error) {
	raw, err := c.exchange(ctx, method, requestPath, body, extra, op)
	if err != nil {
		return nil, err
	}
	// Acknowledgements may be empty or plain text. If the endpoint supplies a
	// structured failure, do not mistake its HTTP 200 for a ready workspace.
	var status map[string]any
	if json.Unmarshal(raw, &status) == nil && status != nil {
		if reported, ok := status["error"]; ok && reported != nil && reported != "" && reported != false {
			return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "sandbox initialization failed"}
		}
		value := strings.ToLower(stringValue(status, "status"))
		if value == "error" || value == "failed" || value == "timeout" || value == "timed_out" {
			return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "sandbox initialization failed"}
		}
		if (op == "sync" || op == "render-status") && (value == "pending" || value == "running" || value == "syncing") {
			return nil, &Error{Kind: ErrorProtocol, Op: op, Msg: "sandbox initialization has not completed"}
		}
	}
	return raw, nil
}

func (c *Client) ensureSandbox(ctx context.Context) (*Sandbox, error) {
	c.mu.Lock()
	if c.sb != nil {
		sb := *c.sb
		c.mu.Unlock()
		return &sb, nil
	}
	c.mu.Unlock()
	if c.cfg.SkipSandbox {
		sb := &Sandbox{ProjectID: c.cfg.ProjectID, SandboxURL: c.baseURL() + "/s/sandboxes/proxy"}
		c.mu.Lock()
		c.sb = sb
		c.mu.Unlock()
		return sb, nil
	}
	projectID := strings.TrimSpace(c.cfg.ProjectID)
	if projectID == "" {
		var uuidErr error
		projectID, uuidErr = randomUUID()
		if uuidErr != nil {
			return nil, &Error{Kind: ErrorProtocol, Op: "project", Msg: "cannot generate project UUID"}
		}
		if _, err := c.bootstrap(ctx, http.MethodPost, "/api/projects", map[string]any{
			"project_uuid": projectID,
			"title":        "SUB Prism workspace",
			"file_uuids":   []string{},
		}, nil, "project"); err != nil {
			return nil, err
		}
	}
	ys, err := c.json(ctx, http.MethodPost, "/api/y", map[string]any{"docId": projectID, "requestContext": map[string]any{"source": "init"}}, nil, "ysweet")
	if err != nil {
		return nil, err
	}
	ysToken, ysURL := stringValue(ys, "token"), stringValue(ys, "url")
	if ysToken == "" {
		return nil, &Error{Kind: ErrorProtocol, Op: "ysweet", Msg: "missing YSweet token"}
	}
	ysBaseURL := stringValue(ys, "baseUrl")
	if ysBaseURL == "" {
		ysBaseURL = c.baseURL() + "/y/d/" + projectID
	}
	if !officialURL(ysURL, true) || !officialURL(ysBaseURL, false) {
		return nil, &Error{Kind: ErrorProtocol, Op: "ysweet", Msg: "unexpected YSweet URL"}
	}
	be, err := c.json(ctx, http.MethodPost, "/api/backend/1/new", nil, nil, "sandbox")
	if err != nil {
		return nil, err
	}
	sandboxToken, sandboxURL := stringValue(be, "token"), stringValue(be, "url")
	if sandboxToken == "" {
		return nil, &Error{Kind: ErrorProtocol, Op: "sandbox", Msg: "missing sandbox token"}
	}
	if sandboxURL == "" {
		sandboxURL = c.baseURL() + "/s/sandboxes/proxy"
	}
	if !officialURL(sandboxURL, false) {
		return nil, &Error{Kind: ErrorProtocol, Op: "sandbox", Msg: "unexpected sandbox URL"}
	}
	h := http.Header{"X-Crixet-Sandbox-Token": []string{sandboxToken}}
	cacheBust := fmt.Sprint(time.Now().UnixMilli())
	if _, err = c.bootstrap(ctx, http.MethodGet, "/s/sandboxes/proxy/heartbeat?prism_cache_bust="+cacheBust, nil, h, "heartbeat"); err != nil {
		return nil, err
	}
	res, err := c.json(ctx, http.MethodPost, "/api/projects/"+url.PathEscape(projectID)+"/sandbox/resources-token", map[string]any{"sandbox_session_id": nil, "sandbox_token": sandboxToken}, nil, "resources")
	if err != nil {
		return nil, err
	}
	resourcesToken := stringValue(res, "access_token")
	if resourcesToken == "" {
		return nil, &Error{Kind: ErrorProtocol, Op: "resources", Msg: "missing resources token"}
	}
	if _, err = c.bootstrap(ctx, http.MethodPost, "/s/sandboxes/proxy/resources-token?prism_cache_bust="+cacheBust, map[string]any{"token": resourcesToken, "baseUrl": ysBaseURL}, h, "resources-proxy"); err != nil {
		return nil, err
	}
	if _, err = c.bootstrap(ctx, http.MethodPost, "/s/sandboxes/proxy/token?prism_cache_bust="+cacheBust, map[string]any{"url": ysURL, "baseUrl": ysBaseURL, "docId": projectID, "token": ysToken, "authorization": "full"}, h, "token"); err != nil {
		return nil, err
	}
	if _, err = c.bootstrap(ctx, http.MethodGet, "/s/sandboxes/proxy/wait-for-sync?wait_ms=10000&prism_cache_bust="+cacheBust, nil, h, "sync"); err != nil {
		return nil, err
	}
	// The observed browser bootstrap triggers a render before chat. These
	// endpoints can return plain text/empty acknowledgements, unlike api/y.
	render, err := c.bootstrap(ctx, http.MethodPost, "/s/sandboxes/proxy/render?renderMode=async&renderStatusMode=json&renderResultMode=stream-v1&prism_cache_bust="+cacheBust, map[string]any{"mainDocument": "main.tex"}, h, "render")
	if err != nil {
		return nil, err
	}
	statusPath := "render-status?waitMs=10000&renderResultMode=stream-v1&renderStatusMode=json"
	var rendered map[string]any
	if json.Unmarshal(render, &rendered) == nil && stringValue(rendered, "statusPath") != "" {
		statusPath = stringValue(rendered, "statusPath")
	}
	statusURL, err := url.Parse(statusPath)
	if err != nil || statusURL.IsAbs() || statusURL.Host != "" || statusURL.User != nil || statusURL.Fragment != "" || statusURL.Path != "render-status" {
		return nil, &Error{Kind: ErrorProtocol, Op: "render", Msg: "unexpected render status path"}
	}
	query := statusURL.Query()
	query.Set("prism_cache_bust", cacheBust)
	statusURL.RawQuery = query.Encode()
	if _, err = c.bootstrap(ctx, http.MethodGet, "/s/sandboxes/proxy/"+statusURL.String(), nil, h, "render-status"); err != nil {
		return nil, err
	}
	sb := &Sandbox{ProjectID: projectID, SandboxToken: sandboxToken, SandboxURL: sandboxURL, YSweetURL: ysURL, YSweetBaseURL: ysBaseURL}
	c.mu.Lock()
	c.sb = sb
	c.mu.Unlock()
	return sb, nil
}

func parsePayload(body map[string]any) (Result, error) {
	invalid := func(message string) (Result, error) {
		return Result{}, &Error{Kind: ErrorProtocol, Op: "payload", Msg: message}
	}
	response, _ := nestedMap(body, "response")
	if response != nil && !completedPayload(response) {
		return invalid("upstream response was not successfully completed")
	}
	payload, _ := nestedMap(body, "response", "payload")
	if payload == nil {
		return invalid("completed response is missing its payload")
	}
	if !completedPayload(payload) {
		return invalid("upstream payload was not successfully completed")
	}
	result := Result{}
	output, ok := payload["output"].([]any)
	if !ok || len(output) == 0 {
		return invalid("completed payload is missing output")
	}
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item == nil || !completedPayload(item) {
			return invalid("upstream output item was not successfully completed")
		}
		switch item["type"] {
		case "message":
			if role, exists := item["role"]; exists && role != "assistant" {
				return invalid("unsupported output message role")
			}
			content, ok := item["content"].([]any)
			if !ok || len(content) == 0 {
				return invalid("output message is missing content")
			}
			for _, partRaw := range content {
				part, _ := partRaw.(map[string]any)
				if part == nil || part["type"] != "output_text" {
					return invalid("unsupported output content; only text is implemented")
				}
				text, ok := part["text"].(string)
				if !ok {
					return invalid("output text is malformed")
				}
				// Content is not metadata: preserve spaces, indentation, and
				// newlines across fragments instead of trimming each fragment.
				result.Text += text
			}
		case "reasoning":
			for _, field := range []string{"summary", "content"} {
				rawSummary, exists := item[field]
				if !exists || rawSummary == nil {
					continue
				}
				summary, ok := rawSummary.([]any)
				if !ok {
					return invalid("reasoning content is malformed")
				}
				for _, v := range summary {
					part, ok := v.(map[string]any)
					if !ok {
						return invalid("reasoning content is malformed")
					}
					if kind, exists := part["type"]; exists && kind != "summary_text" && kind != "reasoning_text" && kind != "text" {
						return invalid("unsupported reasoning content")
					}
					text, ok := part["text"].(string)
					if !ok {
						return invalid("reasoning text is malformed")
					}
					result.ReasoningText += text
				}
				// summary and content can represent the same reasoning twice.
				if len(summary) > 0 {
					break
				}
			}
		default:
			// Silently dropping function/custom/hosted tools would turn a tool
			// handoff or side effect into a fabricated final text completion.
			return invalid("unsupported upstream output item; only text and reasoning are implemented")
		}
	}
	result.ResponseID = stringValueMap(payload, "id")
	result.Model = stringValueMap(payload, "model")
	if usage, exists := payload["usage"]; exists && usage != nil {
		rawUsage, ok := usage.(map[string]any)
		if !ok {
			return invalid("upstream usage is malformed")
		}
		result.Usage = decodeUsage(rawUsage)
		if result.Usage == nil {
			return invalid("upstream usage is incomplete or invalid")
		}
		// Missing cache counters cannot be interpreted as zero: that would
		// charge cached input at the full input price. Step-level observation
		// remains separate and never upgrades this incomplete payload.
		cache, flatCache := rawUsage["cached_input_tokens"]
		details, _ := rawUsage["input_tokens_details"].(map[string]any)
		if !flatCache {
			cache = details["cached_tokens"]
		}
		if result.Usage.InputTokens > 0 {
			if _, ok := tokenCount(cache); !ok {
				return invalid("upstream usage is missing cache accounting")
			}
		}
		result.UsageSource = "payload"
	}
	return result, nil
}

func completedPayload(value map[string]any) bool {
	if state, exists := value["status"]; exists && state != "completed" {
		return false
	}
	if value["incomplete_details"] != nil {
		return false
	}
	if reported, exists := value["error"]; exists && reported != nil && reported != "" && reported != false {
		return false
	}
	return true
}

func mergeResult(base Result, parsed Result, usage *Usage) Result {
	base.ResponseID, base.Model, base.Text, base.ReasoningText = parsed.ResponseID, parsed.Model, parsed.Text, parsed.ReasoningText
	base.Usage, base.UsageSource = parsed.Usage, parsed.UsageSource
	if base.Usage == nil && usage != nil {
		base.Usage, base.UsageSource = usage, "last_token_usage"
	}
	if base.Usage == nil {
		base.UsageSource = "unknown"
	}
	return base
}

func usageFromProgress(body map[string]any) *Usage {
	live, _ := body["codex_live_progress"].(map[string]any)
	if live == nil {
		return nil
	}
	previews, _ := live["eventPreviews"].([]any)
	var latest *Usage
	for _, raw := range previews {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		payloadType, _ := item["payload_type"].(string)
		rawText, _ := item["raw"].(string)
		if payloadType != "token_count" && !strings.Contains(rawText, "last_token_usage") {
			continue
		}
		event, _ := item["raw"].(map[string]any)
		if event == nil {
			decoder := json.NewDecoder(strings.NewReader(rawText))
			decoder.UseNumber()
			if decoder.Decode(&event) != nil {
				continue
			}
		}
		payload, _ := event["payload"].(map[string]any)
		info, _ := payload["info"].(map[string]any)
		// total_token_usage is cumulative across the workspace/session. It is
		// not a request bill. last_token_usage is retained as a weaker signal:
		// it may cover only one inner tool turn, so the host must check source.
		last, _ := info["last_token_usage"].(map[string]any)
		if candidate := decodeUsage(last); candidate != nil {
			latest = candidate
		}
	}
	return latest
}

func decodeUsage(raw map[string]any) *Usage {
	if raw == nil {
		return nil
	}
	input, inputOK := tokenCount(raw["input_tokens"])
	output, outputOK := tokenCount(raw["output_tokens"])
	if !inputOK || !outputOK || input > math.MaxInt-output {
		return nil
	}
	u := &Usage{InputTokens: input, OutputTokens: output, TotalTokens: input + output}
	if total, exists := raw["total_tokens"]; exists {
		n, ok := tokenCount(total)
		if !ok || n != u.TotalTokens {
			return nil
		}
	}
	cached := raw["cached_input_tokens"]
	if cached == nil {
		if details, ok := raw["input_tokens_details"].(map[string]any); ok {
			cached = details["cached_tokens"]
		}
	}
	reasoning := raw["reasoning_output_tokens"]
	if reasoning == nil {
		if details, ok := raw["output_tokens_details"].(map[string]any); ok {
			reasoning = details["reasoning_tokens"]
		}
	}
	for _, field := range []struct {
		value  any
		target *int
		max    int
	}{{cached, &u.CachedInputTokens, input}, {reasoning, &u.ReasoningTokens, output}} {
		if field.value == nil {
			continue
		}
		n, ok := tokenCount(field.value)
		if !ok || n > field.max {
			return nil
		}
		*field.target = n
	}
	return u
}

func tokenCount(raw any) (int, bool) {
	switch n := raw.(type) {
	case json.Number:
		value, err := n.Int64()
		return int(value), err == nil && value >= 0 && uint64(value) <= uint64(math.MaxInt)
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= float64(math.MaxInt) || math.Trunc(n) != n {
			return 0, false
		}
		return int(n), true
	case int:
		return n, n >= 0
	default:
		return 0, false
	}
}

func officialURL(value string, allowWebSocket bool) bool {
	u, err := url.Parse(value)
	return err == nil && u.Host == "prism.openai.com" && u.User == nil && u.Fragment == "" && (u.Scheme == "https" || allowWebSocket && u.Scheme == "wss")
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}

func randomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func stringValue(v map[string]any, key string) string { return stringValueMap(v, key) }
func stringValueMap(v map[string]any, key string) string {
	s, _ := v[key].(string)
	return strings.TrimSpace(s)
}
func intValue(v map[string]any, key string) int {
	switch x := v[key].(type) {
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case int:
		return x
	}
	return 0
}
func nestedMap(v map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		next, ok := v[key].(map[string]any)
		if !ok {
			return nil, false
		}
		v = next
	}
	return v, true
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
