package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRequestTimezoneSettingsValidationAndPreservation(t *testing.T) {
	for _, zone := range OpenAIRequestTimezoneOptions() {
		require.NoError(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: zone}))
	}
	require.Contains(t, OpenAIRequestTimezoneOptions(), "Asia/Shanghai")
	require.Contains(t, OpenAIRequestTimezoneOptions(), "UTC")
	for _, invalid := range []map[string]any{
		{openAIRequestTimezoneExtraKey: "Local"}, {openAIRequestTimezoneExtraKey: "../etc/passwd"},
		{openAIRequestTimezoneExtraKey: 8}, {openAIRequestTimezoneEnabledExtraKey: "true"},
		{openAIRequestTimezoneSyncSearchExtraKey: 1},
	} {
		require.Error(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, invalid))
	}
	require.Error(t, ValidateOpenAIRequestTimezoneExtra(PlatformGrok, map[string]any{openAIRequestTimezoneEnabledExtraKey: true}))
	a := &Account{Platform: PlatformOpenAI, Extra: map[string]any{
		"tiered_routing_enabled": true, "pool": "premium", "quota_daily_limit": 50,
		"custom_feature": "keep", openAIRequestTimezoneEnabledExtraKey: true,
		openAIRequestTimezoneExtraKey: "America/New_York", openAIRequestTimezoneSyncSearchExtraKey: true,
	}}
	patch := map[string]any{openAIRequestTimezoneEnabledExtraKey: false}
	merged := preserveOpenAIRequestTimezoneExtra(a, patch)
	require.Equal(t, false, merged[openAIRequestTimezoneEnabledExtraKey])
	for key, value := range a.Extra {
		if key != openAIRequestTimezoneEnabledExtraKey {
			require.Equal(t, value, merged[key])
		}
	}
	require.Equal(t, true, a.Extra[openAIRequestTimezoneEnabledExtraKey])
	require.Len(t, patch, 1)
	otherEdit := preserveOpenAIRequestTimezoneExtra(a, map[string]any{"pool": "standard"})
	require.Equal(t, "America/New_York", otherEdit[openAIRequestTimezoneExtraKey])
	require.Equal(t, true, otherEdit[openAIRequestTimezoneEnabledExtraKey])
	require.False(t, (&Account{Platform: PlatformOpenAI}).OpenAIRequestTimezoneEnabled())
}

func TestRequestTimezoneDisabledAndNonOpenAIKeepRequestBytes(t *testing.T) {
	body := localeTestBody([]string{"<environment_context><timezone>Asia/Shanghai</timezone><current_date>old</current_date></environment_context>"}, []string{"environments.environment_context"})
	for _, a := range []*Account{nil, {Platform: PlatformOpenAI},
		{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneExtraKey: "Asia/Tokyo"}},
		{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: false, openAIRequestTimezoneSyncSearchExtraKey: true}},
		{Platform: PlatformGrok, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true}},
		{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "invalid"}},
	} {
		svc := timezoneAutoService(false, true, nil)
		out := svc.normalizeRequestTimezone(context.Background(), a, body, "http")
		require.Equal(t, body, out)
	}
}

func TestRequestTimezonePreservesRoutingBillingAndFailoverInput(t *testing.T) {
	body := []byte(`{"model":"gpt-6-sol","reasoning":{"effort":"max"},"service_tier":"priority","prompt_cache_key":"customer-session","previous_response_id":"resp_continue","client_metadata":{"session_id":"s1"},"input":[{"role":"user","content":[{"type":"input_text","text":"<environment_context><timezone>Asia/Shanghai</timezone><current_date>old</current_date><cwd>D:/project</cwd></environment_context>"},{"type":"input_text","text":"按北京时间安排任务"},{"type":"input_image","image_url":"https://example.com/image.png"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["environments.environment_context","user.text","image"]}},{"type":"function_call_output","call_id":"call_1","output":"unchanged"}],"tools":[{"type":"web_search","user_location":{"timezone":"Asia/Shanghai","country":"CN","city":"Shanghai"}}]}`)
	original := bytes.Clone(body)
	a := &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "America/New_York"}}
	b := &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "Asia/Tokyo"}}
	now := time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC)
	out := normalizeOpenAIRequestLocale(context.Background(), a, body, "http", now)
	for _, path := range []string{"model", "reasoning", "service_tier", "prompt_cache_key", "previous_response_id", "client_metadata", "input.0.content.1", "input.0.content.2", "input.1", "tools.0.user_location.country", "tools.0.user_location.city"} {
		require.Equal(t, gjson.GetBytes(original, path).Raw, gjson.GetBytes(out, path).Raw, path)
	}
	require.Contains(t, gjson.GetBytes(out, "input.0.content.0.text").String(), "<current_date>old</current_date>")
	require.Contains(t, gjson.GetBytes(out, "input.0.content.0.text").String(), "D:/project")
	require.Equal(t, original, body, "account retry must retain the original request")
	next := normalizeOpenAIRequestLocale(context.Background(), b, body, "ws", now)
	require.Contains(t, gjson.GetBytes(next, "input.0.content.0.text").String(), DefaultOpenAIRequestTimezone)
	require.Contains(t, gjson.GetBytes(next, "input.0.content.0.text").String(), "<current_date>old</current_date>")
	a.Extra[openAIRequestTimezoneSyncSearchExtraKey] = true
	search := normalizeOpenAIRequestLocale(context.Background(), a, body, "ws", now)
	require.Equal(t, DefaultOpenAIRequestTimezone, gjson.GetBytes(search, "tools.0.user_location.timezone").String())
	require.Equal(t, "CN", gjson.GetBytes(search, "tools.0.user_location.country").String())
	require.Equal(t, "Shanghai", gjson.GetBytes(search, "tools.0.user_location.city").String())
}

func TestRequestTimezoneMalformedBlocksArePreserved(t *testing.T) {
	for _, text := range []string{
		"<environment_context><timezone>Asia/Shanghai</timezone></environment_context><environment_context><cwd>/app</cwd></environment_context>",
		"<environment_context><timezone>Asia/Shanghai</timezone><timezone>Asia/Tokyo</timezone></environment_context>",
		"<environment_context><timezone><nested/></timezone></environment_context>",
		"<environment_context><timezone/></environment_context>",
	} {
		out, _, reason := rewriteOpenAIRequestEnvironment(text, "UTC")
		require.Equal(t, "invalid_environment_context", reason)
		require.Equal(t, text, out)
	}
}

func TestRequestTimezoneLooseEnvironmentPreservesDatesAndOtherBytes(t *testing.T) {
	text := "<environment_context>\n<cwd>D:/A&B</cwd><current_date/><timezone> Asia/Shanghai </timezone><subagents>A < B & C</subagents><current_time>10:12</current_time>\n</environment_context>"
	a := &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "UTC"}}
	for _, content := range []any{text, []map[string]any{{"type": "input_text", "text": text}}} {
		body, _ := json.Marshal(map[string]any{"input": []map[string]any{{"role": "user", "content": content}}})
		out := normalizeOpenAIRequestLocale(context.Background(), a, body, "http", time.Now())
		expected := bytes.Replace(body, []byte("Asia/Shanghai"), []byte(DefaultOpenAIRequestTimezone), 1)
		require.JSONEq(t, string(expected), string(out))
	}
	ordinary := []byte(`{"input":[{"role":"user","content":"Please explain <environment_context><cwd>/app</cwd><timezone>Asia/Shanghai</timezone></environment_context>"}]}`)
	require.Equal(t, ordinary, normalizeOpenAIRequestLocale(context.Background(), a, ordinary, "ws", time.Now()))
}
