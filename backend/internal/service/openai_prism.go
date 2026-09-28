package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/prism"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const prismUpstreamEndpoint = "/api/llm/response_with_tools_start"

// This bounded local pilot serializes a GPT account across tenants without
// retaining credentials or sharing a sandbox. Multiple SUB replicas require a
// distributed account lease before this experimental protocol is deployed.
var prismTurnSlots sync.Map

// An accepted turn can keep running after our HTTP timeout. A fresh Client
// must not erase that uncertainty and overlap another job on this account.
// This local pilot requires operator reconciliation before process restart;
// production needs durable reconciliation state rather than this process map.
var prismUncertainAccounts sync.Map

type prismSlot struct {
	busy chan struct{}
	refs int
}

var prismSlotMu sync.Mutex

func acquirePrismTurn(ctx context.Context, accountID int64) (func(), error) {
	prismSlotMu.Lock()
	raw, _ := prismTurnSlots.LoadOrStore(accountID, &prismSlot{busy: make(chan struct{}, 1)})
	slot := raw.(*prismSlot)
	slot.refs++
	prismSlotMu.Unlock()
	drop := func() {
		prismSlotMu.Lock()
		slot.refs--
		if slot.refs == 0 {
			prismTurnSlots.Delete(accountID)
		}
		prismSlotMu.Unlock()
	}
	select {
	case slot.busy <- struct{}{}:
		return func() { <-slot.busy; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

// prismFailure commits exactly one sanitized response. Once a Prism start
// might have been sent we never return UpstreamFailoverError: retrying an
// unknown outcome can execute and charge twice on the provider account.
func prismFailure(c *gin.Context, status int, code, message string) (*OpenAIForwardResult, error) {
	committed := StopOpenAICompactSSEKeepaliveCommitted(c)
	MarkResponseCommitted(c)
	if committed {
		writeOpenAICompactSSEFailureMessage(c, status, code, message)
	} else {
		c.JSON(status, gin.H{"error": gin.H{"type": "prism_error", "code": code, "message": message}})
	}
	return nil, fmt.Errorf("prism: %s", code)
}

// forwardPrism reuses SUB's selected account, proxy, policies and usage record
// contract. It owns no account pool and never reads client-supplied upstream
// credentials. Only a separately bound Prism session authenticates this hop.
func (s *OpenAIGatewayService) forwardPrism(ctx context.Context, c *gin.Context, account *Account, body []byte, chat bool, started time.Time) (*OpenAIForwardResult, error) {
	if !PrismFeatureEnabled() {
		return prismFailure(c, 503, "prism_disabled", "Prism local pilot is disabled")
	}
	// Preserve the administrator's Fast policy before validating this pilot's
	// narrower protocol. An explicit filter may remove a tier; a required tier
	// must never silently become a default-tier Prism request.
	policyModel := account.GetMappedModel(strings.TrimSpace(gjson.GetBytes(body, "model").String()))
	policyBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, policyModel, body)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
			MarkResponseCommitted(c)
			return nil, blocked
		}
		return prismFailure(c, 400, "prism_policy_invalid", "Prism could not apply the request policy")
	}
	body = policyBody
	if tier := gjson.GetBytes(body, "service_tier"); tier.Exists() && tier.Type != gjson.Null {
		return prismFailure(c, 400, "prism_service_tier_unsupported", "Prism pilot cannot apply the effective service_tier policy")
	}
	request, stream, err := parsePrismTextRequest(body, chat)
	if err != nil {
		return prismFailure(c, 400, "prism_request_unsupported", err.Error())
	}
	originalModel := request.Model
	request.Model = account.GetMappedModel(originalModel)
	if request.Model == "" || strings.EqualFold(request.Model, "auto") || strings.EqualFold(request.Model, "default") {
		return prismFailure(c, 400, "prism_explicit_model_required", "Select an explicit upstream model")
	}
	if account.IsExcelBPSEnabledForModel(originalModel) {
		return prismFailure(c, 400, "prism_protocol_conflict", "Prism and Excel cannot own the same model on this account")
	}
	if isOpenAIResponsesCompactPath(c) {
		return prismFailure(c, 400, "prism_compact_unsupported", "Prism pilot requires full text history; compact is unsupported")
	}
	if s.httpUpstream == nil {
		return prismFailure(c, 503, "prism_transport_unavailable", "Prism transport is unavailable")
	}
	callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	release, err := acquirePrismTurn(callCtx, account.ID)
	if err != nil {
		return prismFailure(c, 504, "prism_queue_timeout", "Prism account wait expired before dispatch")
	}
	defer release()
	if _, uncertain := prismUncertainAccounts.Load(account.ID); uncertain {
		return prismFailure(c, 409, "prism_outcome_unknown", "A previous Prism operation needs reconciliation before this account can send again")
	}
	// The account may have been edited/disabled while this request queued.
	latest, err := s.admitOpenAITurn(callCtx, c, account, originalModel)
	if err != nil {
		return nil, err
	}
	account = latest
	if !account.IsPrismEnabledForModel(originalModel) {
		return prismFailure(c, 409, "prism_account_changed", "Account protocol changed before dispatch")
	}
	cookie, err := DecryptPrismCookie(account, s.cfg)
	if err != nil {
		return prismFailure(c, 503, "prism_session_unavailable", "Bind a valid Prism session to this GPT account first")
	}
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := prism.New(prism.Config{
		Cookie: cookie, UserID: account.GetPrismUserID(), RequestTTL: 120 * time.Second,
		// A new workspace per full-history request isolates customers even when
		// they use the same account and caller-provided conversation identifier.
		Do: func(req *http.Request) (*http.Response, error) {
			if req.URL.Scheme != "https" || req.URL.Host != "prism.openai.com" || req.URL.User != nil {
				return nil, errors.New("prism origin rejected")
			}
			upCtx := WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileLongStream))
			return s.httpUpstream.Do(req.WithContext(upCtx), proxyURL, account.ID, 1)
		},
	})
	if err != nil {
		return prismFailure(c, 503, "prism_session_invalid", "Prism session configuration is invalid")
	}
	SetActualOpenAIUpstreamEndpoint(c, prismUpstreamEndpoint)
	SetOpsUpstreamModel(c, request.Model)
	output, err := client.Run(callCtx, request)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		status, code := http.StatusBadGateway, "prism_upstream_failed"
		var pe *prism.Error
		if errors.As(err, &pe) {
			if pe.RequestStarted {
				prismUncertainAccounts.Store(account.ID, time.Now().UTC())
			}
			switch pe.Kind {
			case prism.ErrorUnauthorized:
				status, code = 502, "prism_session_rejected"
			case prism.ErrorRateLimited:
				status, code = 429, "prism_rate_limited"
			case prism.ErrorTimeout:
				status, code = 504, "prism_timeout"
			}
			setOpsUpstreamError(c, pe.Status, code, "")
		}
		return prismFailure(c, status, code, "Prism request failed; it was not replayed")
	}
	// Do not label an echo/default as observed identity, or charge a session's
	// cumulative total as if it were this customer turn's usage.
	if output.Model == "" || output.Model != request.Model {
		return prismFailure(c, 502, "prism_model_unverified", "Prism did not confirm the requested model; no customer charge")
	}
	if output.Usage == nil || output.UsageSource != "payload" {
		return prismFailure(c, 502, "prism_usage_unverified", "Prism per-response usage is unavailable; no customer charge")
	}
	u := output.Usage
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.CachedInputTokens < 0 || u.CachedInputTokens > u.InputTokens || u.TotalTokens != u.InputTokens+u.OutputTokens {
		return prismFailure(c, 502, "prism_usage_invalid", "Prism returned inconsistent usage; no customer charge")
	}
	result := &OpenAIForwardResult{
		RequestID: output.RequestID, ResponseID: output.ResponseID, Model: originalModel,
		UpstreamModel: request.Model, BillingModel: request.Model, UpstreamResponseModel: output.Model,
		UpstreamEndpoint: prismUpstreamEndpoint, Stream: stream, ReasoningEffort: &request.ReasoningEffort,
		RequestedReasoningEffort: coalesceRequestedReasoningEffort(RequestedReasoningEffortFromContext(ctx), &request.ReasoningEffort),
		Usage:                    OpenAIUsage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, CacheReadInputTokens: u.CachedInputTokens},
		UpstreamTerminalEvent:    "response.completed", Duration: time.Since(started),
	}
	if result.ResponseID == "" {
		return prismFailure(c, 502, "prism_response_invalid", "Prism response identifier is missing")
	}
	c.Header("X-Sub2API-Upstream", "prism")
	c.Header("X-Sub2API-Prism-Stream", "buffered")
	c.Header("X-Sub2API-Usage-Source", "prism_response_payload")
	// This is completion-buffered SSE, not fabricated incremental generation.
	ms := int(time.Since(started).Milliseconds())
	result.FirstTokenMs = &ms
	if err := writePrismTextResponse(c, output, originalModel, chat, stream); err != nil {
		result.ClientDisconnect = true
		return result, err
	}
	return result, nil
}

func parsePrismTextRequest(body []byte, chat bool) (prism.Request, bool, error) {
	var raw map[string]json.RawMessage
	bad := func() (prism.Request, bool, error) {
		return prism.Request{}, false, errors.New("Prism pilot accepts explicit-model full-history text requests only")
	}
	if json.Unmarshal(body, &raw) != nil || raw == nil {
		return bad()
	}
	allowed := map[string]bool{"model": true, "stream": true, "metadata": true, "user": true, "prompt_cache_key": true, "store": true, "tools": true, "tool_choice": true, "parallel_tool_calls": true}
	if chat {
		allowed["messages"] = true
		allowed["reasoning_effort"] = true
		allowed["stream_options"] = true
		allowed["n"] = true
	} else {
		allowed["input"] = true
		allowed["instructions"] = true
		allowed["reasoning"] = true
	}
	for key, value := range raw {
		if !allowed[key] && string(value) != "null" {
			return bad()
		}
	}
	var req prism.Request
	if json.Unmarshal(raw["model"], &req.Model) != nil || strings.TrimSpace(req.Model) == "" {
		return bad()
	}
	req.Model = strings.TrimSpace(req.Model)
	var stream bool
	if value, ok := raw["stream"]; ok && json.Unmarshal(value, &stream) != nil {
		return bad()
	}
	if value, ok := raw["store"]; ok && string(value) != "false" && string(value) != "null" {
		return bad()
	}
	if value, ok := raw["n"]; ok && string(value) != "1" {
		return bad()
	}
	if value, ok := raw["tools"]; ok && string(value) != "null" {
		var tools []json.RawMessage
		if json.Unmarshal(value, &tools) != nil || len(tools) > 0 {
			return bad()
		}
	}
	if value, ok := raw["tool_choice"]; ok && string(value) != "null" && string(value) != `"none"` && string(value) != `"auto"` {
		return bad()
	}
	if value, ok := raw["parallel_tool_calls"]; ok && string(value) != "false" && string(value) != "null" {
		return bad()
	}
	if value, ok := raw["stream_options"]; ok && string(value) != "null" {
		var opts map[string]json.RawMessage
		if json.Unmarshal(value, &opts) != nil {
			return bad()
		}
		for key, v := range opts {
			var b bool
			if key != "include_usage" || json.Unmarshal(v, &b) != nil {
				return bad()
			}
		}
	}
	req.ReasoningEffort = "medium"
	if value, ok := raw["reasoning_effort"]; ok {
		if json.Unmarshal(value, &req.ReasoningEffort) != nil {
			return bad()
		}
	}
	if value, ok := raw["reasoning"]; ok && string(value) != "null" {
		var reasoning map[string]json.RawMessage
		if json.Unmarshal(value, &reasoning) != nil {
			return bad()
		}
		for key, v := range reasoning {
			if key != "effort" || json.Unmarshal(v, &req.ReasoningEffort) != nil {
				return bad()
			}
		}
	}
	switch req.ReasoningEffort {
	case "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return bad()
	}
	if value, ok := raw["instructions"]; ok && string(value) != "null" {
		var text string
		if json.Unmarshal(value, &text) != nil {
			return bad()
		}
		if text != "" {
			req.Input = append(req.Input, prism.InputMessage{Role: "developer", Content: []prism.ContentPart{{Type: "input_text", Text: text}}})
		}
	}
	input := raw["input"]
	if chat {
		input = raw["messages"]
	}
	var text string
	if !chat && json.Unmarshal(input, &text) == nil {
		if text == "" {
			return bad()
		}
		req.Input = append(req.Input, prism.InputMessage{Role: "user", Content: []prism.ContentPart{{Type: "input_text", Text: text}}})
		return req, stream, nil
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(input, &messages) != nil || len(messages) == 0 {
		return bad()
	}
	for _, rawMsg := range messages {
		// Unmarshalling JSON null into a Go string leaves its previous value.
		// Reject it explicitly; never reuse another message's content.
		if string(rawMsg["content"]) == "null" {
			return bad()
		}
		var text string
		for key := range rawMsg {
			if key != "role" && key != "content" && key != "type" {
				return bad()
			}
		}
		if kind, ok := rawMsg["type"]; ok && string(kind) != `"message"` {
			return bad()
		}
		var msg prism.InputMessage
		if json.Unmarshal(rawMsg["role"], &msg.Role) != nil {
			return bad()
		}
		switch msg.Role {
		case "system", "developer", "user", "assistant":
		default:
			return bad()
		}
		if json.Unmarshal(rawMsg["content"], &text) == nil {
			msg.Content = []prism.ContentPart{{Type: "input_text", Text: text}}
		} else {
			var parts []map[string]json.RawMessage
			if json.Unmarshal(rawMsg["content"], &parts) != nil || len(parts) == 0 {
				return bad()
			}
			for _, part := range parts {
				for key := range part {
					if key != "type" && key != "text" {
						return bad()
					}
				}
				var kind, content string
				if json.Unmarshal(part["type"], &kind) != nil || json.Unmarshal(part["text"], &content) != nil {
					return bad()
				}
				if kind != "text" && kind != "input_text" && kind != "output_text" {
					return bad()
				}
				msg.Content = append(msg.Content, prism.ContentPart{Type: "input_text", Text: content})
			}
		}
		if msg.Role == "assistant" {
			for i := range msg.Content {
				msg.Content[i].Type = "output_text"
			}
		}
		req.Input = append(req.Input, msg)
	}
	return req, stream, nil
}

func writePrismTextResponse(c *gin.Context, output prism.Result, publicModel string, chat, stream bool) error {
	u := output.Usage
	usage := gin.H{"input_tokens": u.InputTokens, "output_tokens": u.OutputTokens, "total_tokens": u.TotalTokens, "input_tokens_details": gin.H{"cached_tokens": u.CachedInputTokens}, "output_tokens_details": gin.H{"reasoning_tokens": u.ReasoningTokens}}
	created := time.Now().Unix()
	messageID := output.ResponseID + "_message"
	part := gin.H{"type": "output_text", "text": output.Text, "annotations": []any{}}
	message := gin.H{"id": messageID, "type": "message", "role": "assistant", "status": "completed", "content": []any{part}}
	response := gin.H{"id": output.ResponseID, "object": "response", "created_at": created, "status": "completed", "model": publicModel, "output": []any{message}, "usage": usage, "error": nil, "incomplete_details": nil}
	if chat {
		usage = gin.H{"prompt_tokens": u.InputTokens, "completion_tokens": u.OutputTokens, "total_tokens": u.TotalTokens, "prompt_tokens_details": gin.H{"cached_tokens": u.CachedInputTokens}, "completion_tokens_details": gin.H{"reasoning_tokens": u.ReasoningTokens}}
		response = gin.H{"id": output.ResponseID, "object": "chat.completion", "created": created, "model": publicModel, "choices": []any{gin.H{"index": 0, "message": gin.H{"role": "assistant", "content": output.Text}, "finish_reason": "stop"}}, "usage": usage}
	}
	MarkResponseCommitted(c)
	if !stream {
		c.JSON(200, response)
		return nil
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	seq := 0
	emit := func(kind string, payload gin.H) error {
		if !chat {
			payload["type"] = kind
			payload["sequence_number"] = seq
			seq++
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		prefix := ""
		if kind != "" {
			prefix = "event: " + kind + "\n"
		}
		_, err = c.Writer.WriteString(prefix + "data: " + string(raw) + "\n\n")
		c.Writer.Flush()
		return err
	}
	if chat {
		for _, delta := range []gin.H{{"role": "assistant", "content": ""}, {"content": output.Text}} {
			if err := emit("", gin.H{"id": output.ResponseID, "object": "chat.completion.chunk", "created": created, "model": publicModel, "choices": []any{gin.H{"index": 0, "delta": delta, "finish_reason": nil}}}); err != nil {
				return err
			}
		}
		if err := emit("", gin.H{"id": output.ResponseID, "object": "chat.completion.chunk", "created": created, "model": publicModel, "choices": []any{gin.H{"index": 0, "delta": gin.H{}, "finish_reason": "stop"}}}); err != nil {
			return err
		}
		if err := emit("", gin.H{"id": output.ResponseID, "object": "chat.completion.chunk", "created": created, "model": publicModel, "choices": []any{}, "usage": usage}); err != nil {
			return err
		}
		_, err := c.Writer.WriteString("data: [DONE]\n\n")
		c.Writer.Flush()
		return err
	}
	events := []struct {
		kind string
		data gin.H
	}{
		{"response.created", gin.H{"response": gin.H{"id": output.ResponseID, "object": "response", "created_at": created, "status": "in_progress", "model": publicModel, "output": []any{}, "usage": nil}}},
		{"response.output_item.added", gin.H{"output_index": 0, "item": gin.H{"id": messageID, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}}},
		{"response.content_part.added", gin.H{"output_index": 0, "content_index": 0, "item_id": messageID, "part": gin.H{"type": "output_text", "text": "", "annotations": []any{}}}},
		{"response.output_text.delta", gin.H{"output_index": 0, "content_index": 0, "item_id": messageID, "delta": output.Text}},
		{"response.output_text.done", gin.H{"output_index": 0, "content_index": 0, "item_id": messageID, "text": output.Text}},
		{"response.content_part.done", gin.H{"output_index": 0, "content_index": 0, "item_id": messageID, "part": part}},
		{"response.output_item.done", gin.H{"output_index": 0, "item": message}},
		{"response.completed", gin.H{"response": response}},
	}
	for _, event := range events {
		if err := emit(event.kind, event.data); err != nil {
			return err
		}
	}
	return nil
}
