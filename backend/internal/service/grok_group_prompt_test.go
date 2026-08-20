package service

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func grokPromptTestContext(mode, content string) *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	c.Set("api_key", &APIKey{Group: &Group{
		Platform:          PlatformGrok,
		GrokPromptEnabled: true,
		GrokPromptMode:    mode,
		GrokPromptContent: content,
	}})
	return c
}

func TestApplyGrokGroupPromptToResponsesPreservesClientInstructions(t *testing.T) {
	body := []byte(`{"model":"grok-4.5","instructions":"client rules","input":"hello"}`)
	got, err := applyGrokGroupPromptToResponses(grokPromptTestContext(GrokPromptModePrepend, "server rules"), body)
	require.NoError(t, err)
	require.Equal(t, grokPromptMarker+"\nserver rules\n\nclient rules", gjson.GetBytes(got, "instructions").String())

	again, err := applyGrokGroupPromptToResponses(grokPromptTestContext(GrokPromptModePrepend, "server rules"), got)
	require.NoError(t, err)
	require.JSONEq(t, string(got), string(again))
}

func TestApplyGrokGroupPromptToChatAppendFollowsInstructionHeaders(t *testing.T) {
	body := []byte(`{"model":"grok-4.5","messages":[{"role":"system","content":"s"},{"role":"developer","content":"d"},{"role":"user","content":"u"}]}`)
	got, err := applyGrokGroupPromptToChat(grokPromptTestContext(GrokPromptModeAppend, "server rules"), body)
	require.NoError(t, err)
	require.Equal(t, "system", gjson.GetBytes(got, "messages.2.role").String())
	require.Contains(t, gjson.GetBytes(got, "messages.2.content").String(), grokPromptMarker)
	require.Equal(t, "user", gjson.GetBytes(got, "messages.3.role").String())
}

func TestApplyGrokGroupPromptToMessagesSupportsStringAndBlocks(t *testing.T) {
	ctx := grokPromptTestContext(GrokPromptModeAppend, "server rules")
	stringBody := []byte(`{"model":"grok-4.5","system":"client rules","messages":[{"role":"user","content":"u"}]}`)
	got, err := applyGrokGroupPromptToMessages(ctx, stringBody)
	require.NoError(t, err)
	require.Equal(t, "client rules\n\n"+grokPromptMarker+"\nserver rules", gjson.GetBytes(got, "system").String())

	blockBody := []byte(`{"model":"grok-4.5","system":[{"type":"text","text":"client rules"}],"messages":[{"role":"user","content":"u"}]}`)
	got, err = applyGrokGroupPromptToMessages(grokPromptTestContext(GrokPromptModePrepend, "server rules"), blockBody)
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(got, "system.0.text").String(), grokPromptMarker)
	require.Equal(t, "client rules", gjson.GetBytes(got, "system.1.text").String())
}

func TestNormalizeGrokPromptSettingsClearsOtherPlatforms(t *testing.T) {
	enabled, mode, content, err := NormalizeGrokPromptSettings(PlatformOpenAI, true, GrokPromptModeAppend, "secret")
	require.NoError(t, err)
	require.False(t, enabled)
	require.Equal(t, GrokPromptModePrepend, mode)
	require.Empty(t, content)
}
