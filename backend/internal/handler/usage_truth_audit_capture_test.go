package handler

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageAuditCaptureWriterCapturesWriteAndWriteString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := beginUsageAuditCapture(c)

	_, err := c.Writer.Write([]byte("data: first\n"))
	require.NoError(t, err)
	_, err = c.Writer.WriteString("data: second\n")
	require.NoError(t, err)

	snapshot, truncated := capture.snapshot()
	require.False(t, truncated)
	require.Equal(t, "data: first\ndata: second\n", string(snapshot))
	require.Equal(t, string(snapshot), recorder.Body.String())
}

func TestUsageAuditCaptureWriterRetainsTerminalUsageWhenTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := beginUsageAuditCapture(c)

	prefix := bytes.Repeat([]byte("x"), usageAuditResponseCaptureLimit+4096)
	_, err := c.Writer.Write(prefix)
	require.NoError(t, err)
	terminal := "\ndata: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":12,\"output_tokens\":8}}\n\n"
	_, err = c.Writer.WriteString(terminal)
	require.NoError(t, err)

	snapshot, truncated := capture.snapshot()
	require.True(t, truncated)
	require.LessOrEqual(t, len(snapshot), usageAuditResponseCaptureLimit+1)
	require.Contains(t, string(snapshot), terminal)
	require.Len(t, recorder.Body.Bytes(), len(prefix)+len(terminal))
}

func TestPopulateOpenAIUsageAuditUsesBoundedSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := beginUsageAuditCapture(c)
	response := `{"output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":4,"output_tokens":1}}`
	_, err := c.Writer.WriteString(response)
	require.NoError(t, err)

	result := &service.OpenAIForwardResult{
		Usage: service.OpenAIUsage{InputTokens: 4, OutputTokens: 1},
		Model: "gpt-5.4",
	}
	populateOpenAIUsageAudit(result, "/v1/responses", "", []byte(`{"model":"gpt-5.4","input":"hello"}`), capture)

	require.NotNil(t, result.UsageAudit)
	require.Equal(t, service.UsageAuditProtocolOpenAIResponses, result.UsageAudit.Protocol)
	require.JSONEq(t, `{"input_tokens":4,"output_tokens":1}`, string(result.UsageAudit.UpstreamRawUsage))
}
