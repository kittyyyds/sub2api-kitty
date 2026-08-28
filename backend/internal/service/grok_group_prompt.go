package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	GrokPromptModePrepend = "prepend"
	GrokPromptModeAppend  = "append"

	maxGrokPromptBytes = 36 * 1024
	grokPromptMarker   = "[SUB2API_GROK_ENHANCEMENT_V1]"
)

// DefaultGrokEnhancementPrompt is deliberately client-neutral. Local clients
// retain ownership of their tools, permissions and project instructions.
const DefaultGrokEnhancementPrompt = `Operate as a high-agency engineering and research agent. Pursue the user's actual objective through completion while preserving the current task state and all confirmed prior results.

Execution rules:
- Understand the repository, runtime, constraints and existing conventions before changing anything.
- Break substantial work into concrete steps, then execute them without repeatedly asking for confirmation when the next action is clear.
- Use whichever tools the current client exposes for search, inspection, editing, execution and verification. Never claim a tool action or result that was not observed.
- Reuse existing artifacts, traces, files and completed work. On failure, diagnose the specific cause, correct the command or implementation and continue.
- Keep edits focused, preserve unrelated user changes and prefer established local patterns over unnecessary new abstractions.
- For code changes, handle implementation, edge cases, tests, build checks and a concise handoff. For investigation, distinguish evidence from inference and produce actionable findings.
- Treat client, project and user instructions as additive context. Preserve their requirements and do not replace them with generic workflow text.
- Communicate directly and precisely. Surface blockers, assumptions and tradeoffs, but keep momentum and finish feasible work end to end.`

func NormalizeGrokPromptSettings(platform string, enabled bool, mode, content string) (bool, string, string, error) {
	if platform != PlatformGrok {
		return false, GrokPromptModePrepend, "", nil
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = GrokPromptModePrepend
	}
	if mode != GrokPromptModePrepend && mode != GrokPromptModeAppend {
		return false, "", "", errors.New("grok_prompt_mode must be prepend or append")
	}
	content = strings.TrimSpace(content)
	if len(content) > maxGrokPromptBytes {
		return false, "", "", fmt.Errorf("grok_prompt_content exceeds %d bytes", maxGrokPromptBytes)
	}
	return enabled, mode, content, nil
}

func grokPromptSettingsFromContext(c *gin.Context) (string, string, bool) {
	if c == nil {
		return "", "", false
	}
	raw, ok := c.Get("api_key")
	if !ok {
		return "", "", false
	}
	apiKey, ok := raw.(*APIKey)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != PlatformGrok || !apiKey.Group.GrokPromptEnabled {
		return "", "", false
	}
	prompt := strings.TrimSpace(apiKey.Group.GrokPromptContent)
	if prompt == "" {
		prompt = DefaultGrokEnhancementPrompt
	}
	mode := apiKey.Group.GrokPromptMode
	if mode != GrokPromptModeAppend {
		mode = GrokPromptModePrepend
	}
	return grokPromptMarker + "\n" + prompt, mode, true
}

func mergeGrokPromptText(existing, prompt, mode string) string {
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return prompt
	}
	if mode == GrokPromptModeAppend {
		return existing + "\n\n" + prompt
	}
	return prompt + "\n\n" + existing
}

func applyGrokGroupPromptToResponses(c *gin.Context, body []byte) ([]byte, error) {
	prompt, mode, enabled := grokPromptSettingsFromContext(c)
	if !enabled || bytes.Contains(body, []byte(grokPromptMarker)) {
		return body, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	existing := ""
	if raw, ok := root["instructions"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &existing); err != nil {
			return nil, errors.New("responses instructions must be a string")
		}
	}
	merged, _ := json.Marshal(mergeGrokPromptText(existing, prompt, mode))
	root["instructions"] = merged
	return json.Marshal(root)
}

func applyGrokGroupPromptToChat(c *gin.Context, body []byte) ([]byte, error) {
	prompt, mode, enabled := grokPromptSettingsFromContext(c)
	if !enabled || bytes.Contains(body, []byte(grokPromptMarker)) {
		return body, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(root["messages"], &messages); err != nil {
		return nil, errors.New("chat messages must be an array")
	}
	injected, _ := json.Marshal(map[string]string{"role": "system", "content": prompt})
	insertAt := 0
	if mode == GrokPromptModeAppend {
		for insertAt < len(messages) {
			var header struct {
				Role string `json:"role"`
			}
			if json.Unmarshal(messages[insertAt], &header) != nil || (header.Role != "system" && header.Role != "developer") {
				break
			}
			insertAt++
		}
	}
	messages = append(messages, nil)
	copy(messages[insertAt+1:], messages[insertAt:])
	messages[insertAt] = injected
	root["messages"], _ = json.Marshal(messages)
	return json.Marshal(root)
}

func applyGrokGroupPromptToMessages(c *gin.Context, body []byte) ([]byte, error) {
	prompt, mode, enabled := grokPromptSettingsFromContext(c)
	if !enabled || bytes.Contains(body, []byte(grokPromptMarker)) {
		return body, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	raw, exists := root["system"]
	if !exists || string(raw) == "null" {
		root["system"], _ = json.Marshal(prompt)
		return json.Marshal(root)
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		root["system"], _ = json.Marshal(mergeGrokPromptText(text, prompt, mode))
		return json.Marshal(root)
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, errors.New("messages system must be a string or content block array")
	}
	block, _ := json.Marshal(map[string]string{"type": "text", "text": prompt})
	if mode == GrokPromptModeAppend {
		blocks = append(blocks, block)
	} else {
		blocks = append([]json.RawMessage{block}, blocks...)
	}
	root["system"], _ = json.Marshal(blocks)
	return json.Marshal(root)
}
