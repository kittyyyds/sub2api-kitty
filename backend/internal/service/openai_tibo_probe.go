package service

import (
	"errors"
	"strings"

	"github.com/tidwall/gjson"
)

// openAITiboProbeModel is the model every Tibo probe asks; only gpt-6-astra
// reliably knows who Tibo is, so a True answer is evidence the account is
// served by astra rather than a degraded fallback model.
const openAITiboProbeModel = "gpt-6-astra"

const openAITiboProbePrompt = "Tell me whether you know who Tibo is in OpenAI based on your knowledge without searching online. Only return True or  False."
const openAITiboProbeReasoningEffort = "xhigh"
const openAITiboProbeBodyLimit = 4 << 20

var errOpenAITiboAccountUnavailable = errors.New("tibo account is unavailable")

func openAITiboProbePayload() map[string]any {
	return map[string]any{
		"model": openAITiboProbeModel, "store": false,
		"stream": true, "reasoning": map[string]any{"effort": openAITiboProbeReasoningEffort},
		"include": []string{"reasoning.encrypted_content"},
		"input":   []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": openAITiboProbePrompt}}}},
	}
}

// openAITiboIsProbePayloadRaw reports a business request that is exactly the
// Tibo probe. Its model swaps are the probe's own verdict, not passive
// evidence about the account.
func openAITiboIsProbePayloadRaw(payload []byte) bool {
	if !gjson.ValidBytes(payload) || gjson.GetBytes(payload, "model").String() != openAITiboProbeModel ||
		strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String()) != "" ||
		strings.TrimSpace(gjson.GetBytes(payload, "instructions").String()) != "" ||
		len(gjson.GetBytes(payload, "tools").Array()) != 0 {
		return false
	}
	input := gjson.GetBytes(payload, "input")
	if input.Type == gjson.String {
		return input.String() == openAITiboProbePrompt
	}
	items := input.Array()
	if len(items) != 1 || items[0].Get("role").String() != "user" {
		return false
	}
	content := items[0].Get("content")
	if content.Type == gjson.String {
		return content.String() == openAITiboProbePrompt
	}
	parts := content.Array()
	return len(parts) == 1 && parts[0].Get("type").String() == "input_text" && parts[0].Get("text").String() == openAITiboProbePrompt
}

type openAITiboHTTPObservation struct {
	delta                strings.Builder
	completed            bool
	successfulCompletion bool
	failed               bool
	trueAnswer           bool
	modelMatch           bool
	answerClass          string
}

func (o *openAITiboHTTPObservation) event(payload []byte, eventType string) {
	if !gjson.ValidBytes(payload) {
		return
	}
	if eventType == "" {
		eventType = gjson.GetBytes(payload, "type").String()
	}
	switch eventType {
	case "error", "response.failed", "response.incomplete", "response.cancelled":
		o.failed = true
	case "response.output_text.delta":
		_, _ = o.delta.WriteString(gjson.GetBytes(payload, "delta").String())
	case "response.completed":
		if o.completed {
			o.failed = true
			return
		}
		o.completed = true
		model, success := openAITiboSuccessfulCompletionModel(payload, eventType)
		o.successfulCompletion = success
		// Same rule as the passive business-response check.
		o.modelMatch = success && upstreamModelsMatchForAudit(openAITiboProbeModel, model)
		if !o.modelMatch {
			o.failed = true
			return
		}
		var output strings.Builder
		for _, item := range gjson.GetBytes(payload, "response.output").Array() {
			for _, content := range item.Get("content").Array() {
				if content.Get("type").String() == "output_text" {
					_, _ = output.WriteString(content.Get("text").String())
				}
				if content.Get("type").String() == "refusal" {
					o.failed = true
				}
			}
		}
		answer := output.String()
		if answer == "" {
			answer = o.delta.String()
		}
		o.answerClass = classifyTiboAnswer(answer)
		o.trueAnswer = o.answerClass == tiboAnswerTrue
	}
}

func observeOpenAITiboHTTPProbe(body []byte) *openAITiboHTTPObservation {
	observation := &openAITiboHTTPObservation{answerClass: "none"}
	if len(body) == 0 || len(body) > openAITiboProbeBodyLimit {
		observation.failed = true
		return observation
	}
	forEachOpenAISSEFrame(string(body), func(eventType string, payload []byte) { observation.event(payload, eventType) })
	return observation
}

// openAITiboSuccessfulCompletionModel returns the model of a successful
// response.completed event.
func openAITiboSuccessfulCompletionModel(payload []byte, eventType string) (string, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return "", false
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	}
	if eventType != "response.completed" {
		return "", false
	}
	if !openAITiboJSONValueEmpty(payload, "error") || !openAITiboJSONValueEmpty(payload, "response.error") {
		return "", false
	}
	status := strings.TrimSpace(gjson.GetBytes(payload, "response.status").String())
	if status != "" && status != "completed" {
		return "", false
	}
	model := strings.TrimSpace(gjson.GetBytes(payload, "response.model").String())
	if model == "" {
		return "", false
	}
	return model, true
}

func openAITiboJSONValueEmpty(payload []byte, path string) bool {
	value := gjson.GetBytes(payload, path)
	return !value.Exists() || value.Type == gjson.Null
}

func openAITiboAccountProxyURL(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

// isOpenAITiboAccount is an OpenAI credential that can answer the Astra Tibo
// probe from this server: ChatGPT OAuth / setup-token on the Codex lane, and
// API keys (including compatible gateways with a custom base_url).
//
// Excluded, so a probe never leaves with the wrong identity or twice:
//   - VM-bound / VM2-managed accounts: their egress (and for managed slots
//     their token) lives in the VM; a probe from the server IP would expose
//     the account and managed slots have no token here;
//   - Excel add-in OAuth clients (not a Codex identity);
//   - Agent Identity accounts (separate authentication contract);
//   - spark shadows (they borrow the parent's credentials, which are probed);
//   - proxy-pool accounts: the outbound proxy is chosen by the same
//     distributed slot reservation as a business request, which a probe
//     does not take;
//   - Ollama Cloud usage accounts (not an OpenAI model catalog);
//   - accounts whose gpt-6-astra requests are served through Excel BPS: the
//     probe would measure a route their traffic does not use;
//   - accounts configured not to serve gpt-6-astra (model mapping whitelist):
//     no business request sends them that model, so a probe would only spend
//     their quota.
func isOpenAITiboAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI {
		return false
	}
	if account.IsOpenAIVMBound() || account.IsOpenAIVMManaged() || account.IsShadow() ||
		account.IsRetiredBPSCredential() || account.IsOpenAIAgentIdentity() ||
		len(account.ProxyIDs) > 1 || IsOllamaCloudUsageAccount(account) ||
		!account.IsModelSupported(openAITiboProbeModel) {
		return false
	}
	if account.IsOpenAIOAuthLike() {
		return codexIdentityLaneAccount(account)
	}
	return account.Type == AccountTypeAPIKey
}
