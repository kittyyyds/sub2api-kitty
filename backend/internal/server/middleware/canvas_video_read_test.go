//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCanvasVideoAliasesReadAfterBalanceExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/v1/videos/task", 200},
		{"GET", "/v1/videos/task/content", 200},
		{"GET", "/v1/videos/generations/task", 200},
		{"GET", "/v1/videos/generations/task/content", 200},
		{"GET", "/v1/videos/edits/task/content", 200},
		{"GET", "/v1/videos/extensions/task/content", 200},
		{"GET", "/v1/videos/generations", 403},
		{"POST", "/v1/videos/generations", 403},
		{"POST", "/v1/videos/task/content", 403},
		{"GET", "/v1/videos/generations/task/other", 403},
		{"GET", "/v1/videos/generations/task/content/extra", 403},
		{"GET", "/v1/videos/generations/../content", 403},
		{"GET", "/v1/videos/generations//content", 403},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			key := canvasTestKey()
			key.User.Balance = 0
			cfg := &config.Config{RunMode: config.RunModeStandard}
			svc := service.NewAPIKeyService(&stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return key, nil }}, nil, nil, nil, nil, nil, cfg)
			r := gin.New()
			r.Any("/v1/videos/*rest", gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)), func(c *gin.Context) { c.Status(200) })
			req := httptest.NewRequest(test.method, test.path, nil)
			req.Header.Set("Authorization", "Bearer fixture-key")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, test.status, w.Code, w.Body.String())
		})
	}
	for _, bad := range []string{"/v1/videos/generations/", "/v1/videos/generations/%2e%2e", "/v1/videos/generations/task/other"} {
		require.False(t, isVideoTaskRead(http.MethodGet, bad), bad)
	}
}
