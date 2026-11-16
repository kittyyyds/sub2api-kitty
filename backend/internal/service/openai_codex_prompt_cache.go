package service

import (
	"bytes"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Codex PromptCacheOptions carries only the host-selected TTL. Platform cache
// modes and content breakpoints are a separate contract (codex e54a5713,
// codex-api/src/common.rs and protocol/src/turn_input.rs).
func validCodexPromptCacheTTL(value string) bool {
	return value == "30m" || value == "12h"
}

func validCodexPromptCacheOptions(value any) bool {
	options, ok := value.(map[string]any)
	if !ok || len(options) != 1 {
		return false
	}
	ttl, ok := options["ttl"].(string)
	return ok && validCodexPromptCacheTTL(ttl)
}

func filterCodexPromptCacheConfiguration(body []byte) ([]byte, bool, error) {
	// A cache key alone needs no filtering. Escaped JSON member names must still
	// take the parser path, including otherwise absent literal option names.
	escaped := bytes.Contains(body, []byte(`\u`))
	hasOptions := bytes.Contains(body, []byte("prompt_cache_options")) || escaped
	hasBreakpoints := bytes.Contains(body, []byte("prompt_cache_breakpoint")) || escaped
	if len(body) == 0 || !hasOptions && !hasBreakpoints {
		return body, false, nil
	}
	if !gjson.ValidBytes(body) {
		return nil, false, fmt.Errorf("invalid JSON request")
	}
	options := gjson.GetBytes(body, "prompt_cache_options")
	ttl := options.Get("ttl")
	validOptions := options.IsObject() && len(options.Map()) == 1 && ttl.Type == gjson.String && validCodexPromptCacheTTL(ttl.String())

	// Remove unsupported Platform breakpoints without re-encoding user content,
	// tool arguments or large integer IDs while retaining the native TTL.
	var paths []string
	if options.Exists() && !validOptions {
		paths = append(paths, "prompt_cache_options")
	}
	if hasBreakpoints {
		root := gjson.ParseBytes(body)
		if root.Get("prompt_cache_breakpoint").Exists() {
			paths = append(paths, "prompt_cache_breakpoint")
		}
		input := root.Get("input")
		if input.IsArray() {
			input.ForEach(func(index, item gjson.Result) bool {
				if item.Get("prompt_cache_breakpoint").Exists() {
					paths = append(paths, fmt.Sprintf("input.%d.prompt_cache_breakpoint", index.Int()))
				}
				content := item.Get("content")
				if content.IsArray() {
					content.ForEach(func(partIndex, part gjson.Result) bool {
						if part.Get("prompt_cache_breakpoint").Exists() {
							paths = append(paths, fmt.Sprintf("input.%d.content.%d.prompt_cache_breakpoint", index.Int(), partIndex.Int()))
						}
						return true
					})
				}
				return true
			})
		}
	}
	for _, path := range paths {
		var err error
		body, err = sjson.DeleteBytes(body, path)
		if err != nil {
			return nil, false, err
		}
	}
	return body, len(paths) > 0, nil
}

// deleteOpenAIPromptCacheBreakpoints removes Platform-only breakpoints from a
// decoded request. Codex rejects them ("prompt_cache_breakpoint is not supported").
func deleteOpenAIPromptCacheBreakpoints(request map[string]any) bool {
	modified := false
	if _, ok := request["prompt_cache_breakpoint"]; ok {
		delete(request, "prompt_cache_breakpoint")
		modified = true
	}
	input, _ := request["input"].([]any)
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := item["prompt_cache_breakpoint"]; ok {
			delete(item, "prompt_cache_breakpoint")
			modified = true
		}
		parts, _ := item["content"].([]any)
		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := part["prompt_cache_breakpoint"]; ok {
				delete(part, "prompt_cache_breakpoint")
				modified = true
			}
		}
	}
	return modified
}

// filterOpenAIResponsesPromptCacheConfiguration keeps the Codex identity lane on
// the native PromptCacheOptions contract (TTL-only options retained, Platform
// modes and content breakpoints removed). Other accounts keep their existing
// field policy elsewhere in the forwarding pipeline and are returned unchanged.
func filterOpenAIResponsesPromptCacheConfiguration(account *Account, _ string, body []byte) ([]byte, bool, error) {
	if !codexIdentityLaneAccount(account) || !bytes.Contains(body, []byte("prompt_cache_")) && !bytes.Contains(body, []byte(`\u`)) {
		return body, false, nil
	}
	filtered, changed, err := filterCodexPromptCacheConfiguration(body)
	if err != nil {
		return nil, false, fmt.Errorf("filter prompt cache configuration: %w", err)
	}
	return filtered, changed, nil
}
