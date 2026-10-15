package service

import (
	"bytes"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	upstreamResponseModelObserverContextKey = "upstream_response_model_observer"
	upstreamResponseModelMaxLength          = 200
	upstreamResponseModelRedacted           = "[redacted]"
	upstreamResponseModelSecretPrefixMin    = 16
)

// upstreamResponseModelObserver tracks one forwarding attempt (or one WS turn).
// A terminal declaration wins over an earlier declaration; otherwise the first
// declaration is retained. Observation never affects the forwarding path.
//
// Billing normally ignores the observed model as well; the only exception is a
// channel explicitly configured with billing_model_source = response_model,
// where a conflict flag makes billing fall back to the baseline model
// (see responseModelBillingDeclaration).
//
// The same observer also records the service tier the upstream reports having
// used (OpenAI service_tier, Anthropic usage.speed). The observed tier stays
// separate from the final outbound request tier until usage recording resolves
// the billable tier for the selected credential protocol.
type upstreamResponseModelObserver struct {
	astraCompleted bool
	astraHasText   bool
	first          string
	terminal       string
	conflict       bool

	// firstTier holds the first non-terminal tier declaration; it is discarded
	// when later non-terminal declarations disagree. terminalTier comes from a
	// terminal event and always wins.
	firstTier         string
	firstTierConflict bool
	terminalTier      string
}

func (o *upstreamResponseModelObserver) Observe(model string, terminal bool) {
	model = normalizeObservedUpstreamResponseModel(model)
	if model == "" {
		return
	}
	current := o.Model()
	if current != "" && !strings.EqualFold(current, model) {
		o.conflict = true
	}
	if terminal {
		o.terminal = model
		return
	}
	if o.first == "" {
		o.first = model
	}
}

func normalizeObservedUpstreamResponseModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	runes := []rune(model)
	if len(runes) > upstreamResponseModelMaxLength {
		model = string(runes[:upstreamResponseModelMaxLength])
	}
	return model
}

// redactUpstreamResponseModelSecrets returns the value that is safe to persist
// or log. Billing and mismatch checks must continue to use the original
// response model before calling this helper.
//
// The observer bounds model declarations to 200 runes. Include the identically
// bounded form of each secret so a long value echoed as response.model cannot
// leak its first 200 runes after observation truncation.
func redactUpstreamResponseModelSecrets(account *Account, responseModel string) string {
	responseModel = strings.TrimSpace(responseModel)
	if responseModel == "" || account == nil {
		return responseModel
	}
	for _, key := range SensitiveCredentialKeys {
		if upstreamResponseModelContainsCredential(responseModel, account.GetCredential(key)) {
			return upstreamResponseModelRedacted
		}
	}
	for _, value := range account.GetHeaderOverrides() {
		if upstreamResponseModelContainsCredential(responseModel, value) {
			return upstreamResponseModelRedacted
		}
	}
	return responseModel
}

func upstreamResponseModelContainsCredential(responseModel, credential string) bool {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return false
	}
	if strings.Contains(responseModel, credential) {
		return true
	}
	bounded := normalizeObservedUpstreamResponseModel(credential)
	if bounded != credential && strings.Contains(responseModel, bounded) {
		return true
	}
	if strings.HasPrefix(strings.ToLower(credential), "bearer ") {
		token := strings.TrimSpace(credential[len("bearer "):])
		return token != "" && upstreamResponseModelContainsCredential(responseModel, token)
	}

	// If an upstream prefixes an echoed long credential (for example
	// "relay:"+secret), the observer's 200-rune bound leaves only a suffix that
	// matches the beginning of the secret. Redact any meaningful such prefix;
	// shorter coincidences are ignored to preserve useful model names.
	modelRunes := []rune(responseModel)
	credentialRunes := []rune(credential)
	if len(credentialRunes) <= len(modelRunes) {
		return false
	}
	for start := 0; start+upstreamResponseModelSecretPrefixMin <= len(modelRunes); start++ {
		fragment := modelRunes[start:]
		if len(fragment) > len(credentialRunes) {
			continue
		}
		if string(fragment) == string(credentialRunes[:len(fragment)]) {
			return true
		}
	}
	return false
}

func (o *upstreamResponseModelObserver) ObserveOpenAI(payload []byte, eventType string) {
	switch eventType {
	case "response.output_text.delta":
		o.astraHasText = o.astraHasText || strings.TrimSpace(gjson.GetBytes(payload, "delta").String()) != ""
	case "response.completed", "response.done":
		r := gjson.ParseBytes(payload).Get("response")
		o.astraCompleted = codexAstraCompleted(r) || (o.astraHasText && r.Get("model").String() == "gpt-6-astra" && r.Get("status").String() == "completed" && !r.Get("error").IsObject())
	case "response.failed", "response.incomplete", "error":
		o.astraCompleted = false
	}
	model := firstValidTrimmedGJSONString(payload, "response.model", "model")
	terminal := isUpstreamResponseModelTerminalEvent(eventType)
	o.Observe(model, terminal)
	// Every payload that declares a service tier also declares a model, so
	// model-free delta frames skip the extra lookups entirely.
	if model == "" {
		return
	}
	// Non-terminal Responses API events echo the requested tier rather than the
	// tier actually used. Only terminal events and untyped payloads (chat
	// completions chunks, non-streaming bodies) report the processing tier.
	if !terminal && strings.TrimSpace(eventType) != "" {
		return
	}
	tier := normalizeObservedOpenAIServiceTier(firstValidTrimmedGJSONString(payload, "response.service_tier", "service_tier"))
	o.ObserveServiceTier(tier, terminal)
}

func (o *upstreamResponseModelObserver) ObserveAnthropic(payload []byte) {
	model := firstValidTrimmedGJSONString(payload, "message.model", "model")
	o.Observe(model, false)
	// usage.speed travels with the message object (message_start in streams,
	// the top-level body otherwise), i.e. only in payloads that declare a model.
	if model == "" {
		return
	}
	tier := normalizeObservedAnthropicSpeed(firstValidTrimmedGJSONString(payload, "message.usage.speed", "usage.speed"))
	o.ObserveServiceTier(tier, false)
}

// ObserveServiceTier records a tier declared by the upstream response. A
// terminal declaration always wins; non-terminal declarations are only trusted
// when they agree with each other.
func (o *upstreamResponseModelObserver) ObserveServiceTier(tier string, terminal bool) {
	if o == nil || tier == "" {
		return
	}
	if terminal {
		o.terminalTier = tier
		return
	}
	if o.firstTier == "" {
		o.firstTier = tier
		return
	}
	if o.firstTier != tier {
		o.firstTierConflict = true
	}
}

// ServiceTier returns the tier the upstream reports having used, or "" when the
// response never declared one unambiguously.
func (o *upstreamResponseModelObserver) ServiceTier() string {
	if o == nil {
		return ""
	}
	if o.terminalTier != "" {
		return o.terminalTier
	}
	if o.firstTierConflict {
		return ""
	}
	return o.firstTier
}

// normalizeObservedOpenAIServiceTier maps a tier reported by an OpenAI response
// onto the billing vocabulary. "auto" never describes a processing tier and
// unknown values are ignored rather than guessed at.
func normalizeObservedOpenAIServiceTier(raw string) string {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "priority", "fast":
		return OpenAIFastTierPriority
	case "default", "flex", "scale":
		return value
	default:
		return ""
	}
}

// normalizeObservedAnthropicSpeed maps Anthropic usage.speed onto the billing
// vocabulary: "fast" is the billable fast-mode tier, "standard" the base rate.
func normalizeObservedAnthropicSpeed(raw string) string {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "fast", "standard":
		return value
	default:
		return ""
	}
}

func (o *upstreamResponseModelObserver) ObserveGemini(payload []byte) {
	model := firstValidTrimmedGJSONString(
		payload,
		"modelVersion",
		"response.modelVersion",
		"response.response.modelVersion",
	)
	// Gemini streaming has no universal terminal event carrying modelVersion;
	// treating each declaration as terminal retains the latest chunk.
	o.Observe(model, true)
}

func (o *upstreamResponseModelObserver) Model() string {
	if o == nil {
		return ""
	}
	if o.terminal != "" {
		return o.terminal
	}
	return o.first
}

func (o *upstreamResponseModelObserver) Conflict() bool {
	return o != nil && o.conflict
}

func beginUpstreamResponseModelObservation(c *gin.Context) *upstreamResponseModelObserver {
	observer := &upstreamResponseModelObserver{}
	if c != nil {
		c.Set(upstreamResponseModelObserverContextKey, observer)
	}
	return observer
}

func upstreamResponseModelObserverFromContext(c *gin.Context) *upstreamResponseModelObserver {
	if c == nil {
		return nil
	}
	value, ok := c.Get(upstreamResponseModelObserverContextKey)
	if !ok {
		return nil
	}
	observer, _ := value.(*upstreamResponseModelObserver)
	return observer
}

func observedUpstreamResponseModel(c *gin.Context) string {
	return upstreamResponseModelObserverFromContext(c).Model()
}

// observeAnthropicUpstreamResponseModel records the model declared by a raw
// Anthropic response body or SSE event. Protocol converters must call this on
// the upstream payload before they rewrite the client-facing model.
func observeAnthropicUpstreamResponseModel(c *gin.Context, payload []byte) {
	if c == nil || len(bytes.TrimSpace(payload)) == 0 {
		return
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	observer.ObserveAnthropic(payload)
}

func applyObservedUpstreamResponseModel(c *gin.Context, result *OpenAIForwardResult) {
	if result == nil {
		return
	}
	result.UpstreamResponseModel = observedUpstreamResponseModel(c)
	result.UpstreamResponseModelConflict = observedUpstreamResponseModelConflict(c)
}

func applyObservedGatewayResponseModel(c *gin.Context, result *ForwardResult) {
	if result == nil {
		return
	}
	result.UpstreamResponseModel = observedUpstreamResponseModel(c)
	result.UpstreamResponseModelConflict = observedUpstreamResponseModelConflict(c)
}

func observedUpstreamResponseModelConflict(c *gin.Context) bool {
	return upstreamResponseModelObserverFromContext(c).Conflict()
}

func observedUpstreamResponseServiceTier(c *gin.Context) string {
	return upstreamResponseModelObserverFromContext(c).ServiceTier()
}

// resolvedOpenAIUpstreamServiceTierFromObserver preserves the final outbound
// request tier. The observed response tier remains separate on
// OpenAIForwardResult.UpstreamResponseServiceTier and is reconciled once, at
// usage time, where the account protocol is available. In particular, the
// private ChatGPT Codex backend commonly reports default even for effective
// Fast turns, while public API response tiers remain authoritative.
func resolvedOpenAIUpstreamServiceTierFromObserver(_ *upstreamResponseModelObserver, outboundBodyTier *string) *string {
	return outboundBodyTier
}

func resolvedOpenAIUpstreamServiceTier(c *gin.Context, outboundBodyTier *string) *string {
	return resolvedOpenAIUpstreamServiceTierFromObserver(upstreamResponseModelObserverFromContext(c), outboundBodyTier)
}

func observeOpenAISSEBody(observer *upstreamResponseModelObserver, body string) {
	if observer == nil || strings.TrimSpace(body) == "" {
		return
	}
	forEachOpenAISSEFrame(body, func(eventType string, payload []byte) {
		observer.ObserveOpenAI(payload, eventType)
	})
}

func firstValidTrimmedGJSONString(payload []byte, paths ...string) string {
	if len(payload) == 0 {
		return ""
	}
	for _, path := range paths {
		value := gjson.GetBytes(payload, path)
		if !value.Exists() || value.Type != gjson.String {
			continue
		}
		if text := strings.TrimSpace(value.String()); text != "" {
			// Validate only after finding a candidate. This avoids a full validation
			// pass on the common model-free delta path while still rejecting malformed
			// payloads that appear to declare a value.
			if !gjson.ValidBytes(payload) {
				return ""
			}
			return text
		}
	}
	return ""
}

func isUpstreamResponseModelTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

func upstreamModelMismatch(sentModel, responseModel string) *bool {
	responseModel = strings.TrimSpace(responseModel)
	if responseModel == "" {
		return nil
	}
	sentModel = strings.TrimSpace(sentModel)
	mismatch := sentModel == "" || !upstreamModelsMatchForAudit(sentModel, responseModel)
	return &mismatch
}

func upstreamModelsMatchForAudit(sentModel, responseModel string) bool {
	if strings.EqualFold(sentModel, responseModel) {
		return true
	}
	if openAIModelSnapshotMatches(sentModel, responseModel) {
		return true
	}

	// xAI reports the runtime build ID for these supported public aliases.
	// Canonicalize only for mismatch auditing; keep the raw response model for
	// observability and for the separate response-model billing safeguards.
	sentGrokModel := canonicalGrokBuildRuntimeModel(sentModel)
	return sentGrokModel != "" && sentGrokModel == canonicalGrokBuildRuntimeModel(responseModel)
}

func openAIModelSnapshotMatches(sentModel, responseModel string) bool {
	sentModel = strings.ToLower(strings.TrimSpace(sentModel))
	responseModel = strings.ToLower(strings.TrimSpace(responseModel))
	aliases := []string{sentModel}
	if base, ok := strings.CutSuffix(sentModel, "-latest"); ok {
		aliases = append(aliases, base)
	}
	for _, alias := range aliases {
		if !isOpenAIModelFamily(alias) {
			continue
		}
		if responseModel == alias || isOpenAIModelSnapshotSuffix(strings.TrimPrefix(responseModel, alias+"-"), strings.HasPrefix(responseModel, alias+"-")) {
			return true
		}
	}
	return false
}

func isOpenAIModelFamily(model string) bool {
	return strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "codex-") ||
		strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4")
}

func isOpenAIModelSnapshotSuffix(suffix string, hasPrefix bool) bool {
	if !hasPrefix {
		return false
	}
	if len(suffix) == len("2006-01-02") {
		_, err := time.Parse("2006-01-02", suffix)
		return err == nil
	}
	if len(suffix) == len("20060102") {
		_, err := time.Parse("20060102", suffix)
		return err == nil
	}
	if len(suffix) == len("0102") {
		_, err := time.Parse("0102", suffix)
		return err == nil
	}
	return false
}

func canonicalGrokBuildRuntimeModel(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "grok-4.5", "grok-4.5-latest", "grok-4.5-build":
		return "grok-4.5-build"
	case "grok-4.6", "grok-4.6-latest", "grok-4.6-build":
		return "grok-4.6-build"
	case "grok-4.7", "grok-4.7-latest", "grok-4.7-build":
		return "grok-4.7-build"
	default:
		return ""
	}
}

func upstreamSentModel(requestedModel, upstreamModel string) string {
	sentModel := strings.TrimSpace(upstreamModel)
	if sentModel == "" {
		sentModel = strings.TrimSpace(requestedModel)
	}
	return sentModel
}
