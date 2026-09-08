package kittyrisk

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationScopeSnapshotDoesNotUseGroupName(t *testing.T) {
	for _, name := range []string{"GPT", "gpt-prod", "\u3000 GpT Team \t", "Claude production", "grok", "", "ChatGPT"} {
		scope := NewContentModerationScopeSnapshot(nil, name)
		require.True(t, scope.InScope, name)
	}
}

func TestExtractContentModerationFragments_AllClientControlledRolesAndReferences(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"system text"}],
		"instructions":"developer instructions",
		"messages":[
			{"role":"user","content":[{"type":"input_text","text":"user text"},{"type":"input_file","filename":"brief.txt","file_url":"https://files.example/brief.txt"}]},
			{"role":"assistant","content":"assistant text"},
			{"role":"developer","content":"developer text"},
			{"role":"tool","content":{"type":"function_call_output","output":"tool output","url":"https://tool.example/result"}}
		],
		"tools":[{"type":"function","name":"dangerous_tool","description":"tool description"}]
	}`)

	fragments := ExtractContentModerationFragments(ContentModerationProtocolOpenAIResponses, body)
	texts := make(map[string]ContentModerationFragment, len(fragments))
	for _, fragment := range fragments {
		texts[fragment.Text] = fragment
		require.Len(t, fragment.Hash, 64)
	}
	for _, text := range []string{
		"system text", "developer instructions", "user text", "brief.txt",
		"https://files.example/brief.txt", "assistant text", "developer text",
		"tool output", "https://tool.example/result", "dangerous_tool", "tool description",
	} {
		require.Contains(t, texts, text)
	}
	require.Equal(t, "system", texts["system text"].Role)
	require.Equal(t, "assistant", texts["assistant text"].Role)
	require.Equal(t, "tool", texts["tool output"].Role)
	require.Equal(t, "file", texts["brief.txt"].Kind)
	require.Equal(t, "url", texts["https://files.example/brief.txt"].Kind)
}

func TestSelectContentModerationReviewFragments_AllUserTurnsAndLatestTools(t *testing.T) {
	body := []byte(`{
		"system":"system policy",
		"messages":[
			{"role":"user","content":"old user text"},
			{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"old_call","arguments":"old tool arguments"}}]},
			{"role":"tool","content":"old tool output"},
			{"role":"user","content":[{"type":"text","text":"latest user text"},{"type":"input_file","filename":"photo.png","file_url":"https://media.example/photo.png"}]},
			{"role":"assistant","content":"assistant prose","tool_calls":[{"type":"function","function":{"name":"new_call","arguments":"related tool arguments"}}]},
			{"role":"tool","content":"related tool output"}
		],
		"tools":[{"type":"function","function":{"name":"schema_only","description":"tool schema description"}}]
	}`)

	fragments := SelectContentModerationReviewFragments(ExtractContentModerationFragments(ContentModerationProtocolOpenAIChat, body))
	texts := make(map[string]ContentModerationFragment, len(fragments))
	for _, fragment := range fragments {
		texts[fragment.Text] = fragment
	}
	// Every user-authored turn is reviewable: earlier turns are client
	// content too and must not become a smuggling channel.
	for _, expected := range []string{"old user text", "latest user text", "photo.png", "new_call", "related tool arguments", "related tool output"} {
		require.Contains(t, texts, expected)
	}
	for _, excluded := range []string{
		"system policy", "old_call", "old tool arguments", "old tool output",
		"assistant prose", "https://media.example/photo.png", "schema_only", "tool schema description",
	} {
		require.NotContains(t, texts, excluded)
	}
	require.Equal(t, "user", texts["old user text"].Role)
	require.Equal(t, "tool", texts["related tool arguments"].Role)
}

func TestSelectContentModerationReviewFragments_ToolOnlyContinuation(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call_output","call_id":"call_1","output":"current tool result"},
		{"type":"input_image","image_url":"https://media.example/output.png"}
	]}`)
	fragments := SelectContentModerationReviewFragments(ExtractContentModerationFragments(ContentModerationProtocolOpenAIResponses, body))
	require.Len(t, fragments, 1)
	require.Equal(t, "tool", fragments[0].Role)
	require.Equal(t, "current tool result", fragments[0].Text)
}

func TestContentModerationFragmentHashSeparatesRoleKindPathAndContext(t *testing.T) {
	toolText, ok := newContentModerationFragment(" Tool ", " Text ", "messages.1.content", "same text")
	require.True(t, ok)
	samePath, ok := newContentModerationFragment("tool", "text", "  messages.1.content  ", "same text")
	require.True(t, ok)
	differentPath, ok := newContentModerationFragment("tool", "text", "messages.9.content", "same text")
	require.True(t, ok)
	userText, ok := newContentModerationFragment("user", "text", "messages.1.content", "same text")
	require.True(t, ok)
	toolFile, ok := newContentModerationFragment("tool", "file", "messages.1.content", "same text")
	require.True(t, ok)

	require.Equal(t, toolText.Hash, samePath.Hash, "normalized metadata must keep a stable hash")
	require.NotEqual(t, toolText.Hash, differentPath.Hash)
	require.NotEqual(t, toolText.Hash, userText.Hash)
	require.NotEqual(t, toolText.Hash, toolFile.Hash)
}

func TestExtractContentModerationFragments_SkipsInlineBase64MediaURLs(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"user","content":[
			{"type":"input_text","text":"describe this image"},
			{"type":"input_image","image_url":"data:image/png;base64,AAAsh3llBBB"},
			{"type":"input_audio","url":"DATA:AUDIO/WAV;CHARSET=binary;BASE64,AAAsp00fBBB"},
			{"type":"input_text","text":"https://images.example/photo.png"}
		]}]
	}`)

	fragments := ExtractContentModerationFragments(ContentModerationProtocolOpenAIResponses, body)
	texts := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		texts = append(texts, fragment.Text)
	}
	require.Contains(t, texts, "describe this image")
	require.Contains(t, texts, "https://images.example/photo.png")
	require.NotContains(t, texts, "data:image/png;base64,AAAsh3llBBB")
	require.NotContains(t, texts, "DATA:AUDIO/WAV;CHARSET=binary;BASE64,AAAsp00fBBB")
}

func TestContentModerationPendingBodyBudget(t *testing.T) {
	budget := NewContentModerationPendingBodyBudget()
	first, ok := budget.TryReserve(7, 10)
	require.True(t, ok)
	require.True(t, first.Retain())
	_, ok = budget.TryReserve(4, 10)
	require.False(t, ok)
	require.EqualValues(t, 1, budget.Rejections())
	first.Release()
	require.EqualValues(t, 7, budget.InUse())
	first.Release()
	require.Zero(t, budget.InUse())
	require.EqualValues(t, 7, budget.MaxSeen())
}

func TestContentModerationRequestBodyHistogramUsesFixedBuckets(t *testing.T) {
	svc := &ContentModerationService{}
	for _, size := range []int64{0, 64 << 10, (64 << 10) + 1, (1 << 20) + 1, (16 << 20) + 1, (64 << 20) + 1, (256 << 20) + 1} {
		reservation, ok := svc.ReservePendingRequestBody(size)
		require.True(t, ok)
		reservation.Release()
	}

	histogram := svc.contentModerationBodySizeHistogram()
	require.Equal(t, []ContentModerationBodySizeBucket{
		{UpperBoundBytes: 64 << 10, Count: 2},
		{UpperBoundBytes: 1 << 20, Count: 1},
		{UpperBoundBytes: 16 << 20, Count: 1},
		{UpperBoundBytes: 64 << 20, Count: 1},
		{UpperBoundBytes: 256 << 20, Count: 1},
		{UpperBoundBytes: -1, Count: 1},
	}, histogram)
	require.EqualValues(t, (256<<20)+1, svc.observedRequestBodyMax.Load())
}

func TestContentModerationRawRequestCloneMetadataPreservesRepeatedHeaders(t *testing.T) {
	raw := ContentModerationRawRequest{Headers: http.Header{"X-Test": {"one", "two"}}, Body: []byte("raw")}
	clone := raw.CloneMetadata()
	clone.Headers.Add("X-Test", "three")
	require.Equal(t, []string{"one", "two"}, raw.Headers.Values("X-Test"))
	require.Equal(t, []byte("raw"), clone.Body)
}
