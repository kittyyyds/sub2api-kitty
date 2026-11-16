package service

import (
	"bytes"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Ported from the source fork for the in-request encrypted-content recovery
// (openai_messages_encrypted_recovery.go). Only the replay boundary is kept;
// the source's partial-usage results for compat streams are not ported.
//
// Upstream work and downstream delivery are independent replay boundaries.
// Keep only a monotonic bit: no payload is retained, and a later sparse/zero
// terminal cannot erase already observed work or metered usage.
type openAIResponseReplayGuard struct{ progressed bool }

func (g *openAIResponseReplayGuard) Observe(payload []byte, eventType string) {
	if g == nil || g.progressed {
		return
	}
	if trimmed := bytes.TrimSpace(payload); len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) {
		return
	}
	// Malformed fragments and transport heartbeats are not evidence of model
	// work. Downstream commitment is still checked separately by each caller.
	if !gjson.ValidBytes(payload) {
		return
	}
	eventType = strings.TrimSpace(eventType)
	if eventType != "ping" && eventType != "error" && !openAIStreamEventTypeIsTerminal(eventType) &&
		(openAIStreamDataStartsClientOutput(string(payload), eventType) || openAIStreamDataStartsVisibleOutput(string(payload), eventType)) {
		g.progressed = true
		return
	}
	if bytes.Contains(payload, []byte(`"usage"`)) {
		for _, path := range []string{"usage", "response.usage", "data.usage", "data.response.usage"} {
			if usage, ok := openAIUsageFromGJSON(gjson.GetBytes(payload, path)); ok && openAIUsageHasTokens(&usage) {
				g.progressed = true
				return
			}
		}
	}
	// Buffered terminals can contain output that was never sent as deltas.
	if bytes.Contains(payload, []byte(`"output"`)) {
		for _, path := range []string{"response.output", "output"} {
			if output := gjson.GetBytes(payload, path); output.IsArray() && len(output.Array()) > 0 {
				g.progressed = true
				return
			}
		}
	}
	if bytes.Contains(payload, []byte(`"tool_usage"`)) {
		for _, path := range []string{"response.tool_usage", "tool_usage"} {
			if usage := gjson.GetBytes(payload, path); usage.IsObject() && len(usage.Map()) > 0 {
				g.progressed = true
				return
			}
		}
	}
}

func (g *openAIResponseReplayGuard) CanReplay(c *gin.Context) bool {
	if (g != nil && g.progressed) || GetOpsCyberPolicy(c) != nil {
		return false
	}
	return c == nil || c.Request == nil || c.Request.Context().Err() == nil
}
