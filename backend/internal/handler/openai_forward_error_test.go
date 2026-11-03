package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIForwardErrorUpstream struct {
	service.HTTPUpstream
	status int
	body   string
}

func (u *openAIForwardErrorUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{StatusCode: u.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(u.body))}, nil
}

func TestOpenAIForwardErrorPreservesCommittedResponseOrWritesFallback(t *testing.T) {
	for _, tt := range []struct {
		name, input, body string
		status            int
		stream            bool
	}{
		{"upstream_stream", `"hello"`, `{"error":{"message":"bad input"}}`, 400, true},
		{"upstream_json", `"hello"`, `{"error":{"message":"bad input"}}`, 400, false},
		{"model_denied", `"hello"`, `{"error":{"code":"model_not_available"}}`, 403, true},
		{"local_validation", `[{"type":"function_call_output","call_id":"missing","output":"x"}]`, "", 400, true},
		{"incomplete_stream", `"hello"`, "", 200, true},
		{"incomplete_json", `"hello"`, "", 200, false},
		{"failed_stream", `"hello"`, "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"message\":\"rejected\"}}}\n\n", 200, true},
		{"failed_json", `"hello"`, "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"message\":\"rejected\"}}}\n\n", 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			upstream := &openAIForwardErrorUpstream{status: tt.status, body: tt.body}
			cfg := &config.Config{}
			account := &service.Account{ID: 300, Status: service.StatusActive, Schedulable: true, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}, Extra: map[string]any{}}
			repo := prismAdmissionAccountRepo{account: account}
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
			body := `{"model":"gpt-6-astra","stream":` + map[bool]string{true: "true", false: "false"}[tt.stream] + `,"input":` + tt.input + `}`
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
			before := c.Writer.Size()
			_, err := gateway.Forward(context.Background(), c, account, []byte(body))
			require.Error(t, err)
			response := rec.Body.String()
			h := &OpenAIGatewayHandler{}
			if response == "" {
				require.False(t, openAIForwardErrorAlreadyCommunicated(c, before, err))
				require.True(t, h.ensureForwardErrorResponse(c, false))
				response = rec.Body.String()
				require.True(t, json.Valid([]byte(response)), response)
				require.Equal(t, http.StatusBadGateway, rec.Code)
				return
			}
			if !openAIForwardErrorAlreadyCommunicated(c, before, err) {
				require.False(t, h.ensureForwardErrorResponse(c, false))
			}
			require.Equal(t, response, rec.Body.String())
			if tt.name == "incomplete_stream" || tt.name == "failed_stream" {
				require.Equal(t, 1, strings.Count(response, "event: response.failed"))
			} else {
				require.True(t, json.Valid([]byte(response)), response)
				require.NotContains(t, response, "event:")
			}
		})
	}
}
