package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type promptTransformFixture struct {
	err   error
	calls int
	input []byte
}

func (f *promptTransformFixture) HasGatewayPrompt() bool { return true }
func (f *promptTransformFixture) TransformGatewayRequest(_ context.Context, _, _ int64, _, _ string, body []byte) ([]byte, error) {
	f.calls++
	f.input = body
	return body, f.err
}

func TestGatewayPromptMiddleware_InputErrorsDoNotBecome503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, encoding string
		body           []byte
		err            error
		status         int
		code           string
		calls          int
	}{
		{"temporary", "", []byte(`{"model":"m","input":"hi"}`), errors.New("插件临时不可用"), 503, "prompt_plugin_unavailable", 1},
		{"typed-invalid", "", []byte(`{"model":"m"}`), fmt.Errorf("wrapped: %w", service.ErrGatewayPromptInvalidRequest), 400, "invalid_request_body", 1},
		{"typed-limit", "", []byte(`{"model":"m"}`), fmt.Errorf("wrapped: %w", &http.MaxBytesError{Limit: service.GatewayPromptMaxBody}), 413, "request_body_too_large", 1},
		{"corrupt-gzip", "gzip", []byte("broken gzip"), nil, 400, "invalid_request_body", 0},
		{"corrupt-zstd", "zstd", []byte("broken zstd"), nil, 400, "invalid_request_body", 0},
		{"unsupported", "br", []byte("body"), nil, 400, "invalid_request_body", 0},
		{"malformed-json", "", []byte(`{"broken"`), nil, 400, "protocol_validation", 0},
		{"plain-oversize", "", bytes.Repeat([]byte{'a'}, service.GatewayPromptMaxBody+1), nil, 413, "request_body_too_large", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &promptTransformFixture{err: tc.err}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				gid := int64(2)
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, GroupID: &gid})
			})
			r.Use(gatewayPromptMiddleware(f))
			r.POST("/v1/responses", func(c *gin.Context) { c.JSON(400, gin.H{"error": gin.H{"code": "protocol_validation"}}) })
			req := httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(tc.body))
			req.Header.Set("Content-Encoding", tc.encoding)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.code, gjson.Get(w.Body.String(), "error.code").String())
			require.Equal(t, tc.calls, f.calls)
			require.NotContains(t, w.Body.String(), "broken gzip")
		})
	}
}

func TestGatewayPromptMiddleware_PreservesCompressedLongConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Above gRPC's default 4 MiB threshold; keep tools, history and instructions exact.
	body := []byte(`{"model":"m","instructions":"original","input":[{"role":"user","content":"` + strings.Repeat("history ", 1<<20) + `"}],"tools":[{"type":"function","name":"keep"}],"stream":true}`)
	var wire bytes.Buffer
	w := gzip.NewWriter(&wire)
	_, err := w.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	f := &promptTransformFixture{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		gid := int64(2)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, GroupID: &gid})
	})
	r.Use(gatewayPromptMiddleware(f))
	r.POST("/v1/responses/compact", func(c *gin.Context) {
		got, err := pkghttputil.ReadRequestBodyWithPreallocLimit(c.Request, service.GatewayPromptMaxBody)
		require.NoError(t, err)
		require.Equal(t, body, got)
		require.Empty(t, c.GetHeader("Content-Encoding"))
		require.Equal(t, int64(len(body)), c.Request.ContentLength)
		c.Status(204)
	})
	req := httptest.NewRequest("POST", "/v1/responses/compact", io.NopCloser(&wire))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, 204, rec.Code)
	require.Equal(t, 1, f.calls)
	require.Equal(t, body, f.input)
}
