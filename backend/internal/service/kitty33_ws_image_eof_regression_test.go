//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Uses only APIs already present in exact Kitty .32, so the same file is a
// runnable red/green oracle rather than a baseline that merely fails to compile.
func TestKitty33WSImageEOFHasExactlyOneFailureTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	conn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_image_red","model":"gpt-5.1"}}`),
		[]byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens":11,"output_tokens":3}}}`),
		[]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"image_generation_call","id":"ig_red","status":"completed","result":"c3ludGhldGlj","size":"1024x1024"}}`),
	}}
	svc := wsSSETestService(t, &wsSSETestDialer{conn: conn}, &httpUpstreamRecorder{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	result, err := forwardSemanticWSAdapter(t, svc, c)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.False(t, result.SucceededForScheduling())
	require.Len(t, conn.writes, 1, "no second inference after image progress")
	wire := recorder.Body.String()
	require.Equal(t, 1, strings.Count(wire, `"type":"response.failed"`), "client receives one explicit failed terminal after image EOF")
	require.Equal(t, 1, strings.Count(wire, `"type":"image_generation_call"`), "the image is preserved once")
	require.Contains(t, wire, `"id":"resp_image_red"`)
	require.True(t, IsResponseCommitted(c))
}
