//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type canvasKeyReaderFunc func(context.Context, int64, int64) (*service.APIKey, error)

func (f canvasKeyReaderFunc) GetOwnedForCanvas(ctx context.Context, u, k int64) (*service.APIKey, error) {
	return f(ctx, u, k)
}

func canvasTestKey() *service.APIKey {
	g := &service.Group{ID: 3, Name: "group", Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true}
	return &service.APIKey{ID: 7, UserID: 42, KeyHash: strings.Repeat("a", 64), Status: service.StatusActive, GroupID: &g.ID, Group: g, User: &service.User{ID: 42, Status: service.StatusActive, Balance: 10}}
}

func TestCanvasGatewayKeepsOriginalKeyEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		change func(*service.APIKey)
		status int
	}{
		{"hash_only", func(*service.APIKey) {}, 200},
		{"owner", func(k *service.APIKey) { k.UserID = 43 }, 404},
		{"disabled", func(k *service.APIKey) { k.Status = service.StatusAPIKeyDisabled }, 401},
		{"expired", func(k *service.APIKey) { p := time.Now().Add(-time.Hour); k.ExpiresAt = &p }, 403},
		{"quota", func(k *service.APIKey) { k.Quota = 1; k.QuotaUsed = 1 }, 429},
		{"balance", func(k *service.APIKey) { k.User.Balance = 0 }, 403},
		{"user_disabled", func(k *service.APIKey) { k.User.Status = "disabled" }, 401},
		{"group_disabled", func(k *service.APIKey) { k.Group.Status = "disabled" }, 403},
		{"exclusive_group", func(k *service.APIKey) { k.Group.IsExclusive = true }, 403},
		{"ip_acl", func(k *service.APIKey) {
			k.IPWhitelist = []string{"203.0.113.0/24"}
			k.CompiledIPWhitelist = ip.CompileIPRules(k.IPWhitelist)
		}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := canvasTestKey()
			tc.change(key)
			cfg := &config.Config{RunMode: config.RunModeStandard}
			svc := service.NewAPIKeyService(&stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				t.Fatal("must not look up the panel JWT as an API key")
				return nil, nil
			}}, nil, nil, nil, nil, nil, cfg)
			r := gin.New()
			r.POST("/v1/chat/completions", gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)), func(c *gin.Context) {
				resolved, ok := GetAPIKeyFromContext(c)
				require.True(t, ok)
				require.Equal(t, int64(7), resolved.ID)
				require.Empty(t, c.GetHeader("Authorization"))
				require.Empty(t, c.GetHeader("Cookie"))
				require.Equal(t, "198.51.100.8:4321", c.Request.RemoteAddr)
				c.Header("Content-Type", "text/event-stream")
				c.Writer.WriteString("data: first\n\n")
				c.Writer.Flush()
				c.Writer.WriteString("data: [DONE]\n\n")
			})
			r.POST("/canvas-api/v1/chat/completions", func(c *gin.Context) {
				c.Set(string(ContextKeyUser), AuthSubject{UserID: 42})
				id, ok := CanvasKeyID(c)
				if !ok {
					return
				}
				req, ok := CanvasGatewayRequest(c, canvasKeyReaderFunc(func(_ context.Context, u, k int64) (*service.APIKey, error) {
					require.Equal(t, int64(42), u)
					require.Equal(t, int64(7), k)
					return key, nil
				}), id, "/v1/chat/completions")
				if ok {
					r.ServeHTTP(c.Writer, req)
				}
			})
			request := httptest.NewRequest(http.MethodPost, "/canvas-api/v1/chat/completions", strings.NewReader(`{"model":"sample","stream":true}`))
			request.RemoteAddr = "198.51.100.8:4321"
			request.Header.Set("Authorization", "Bearer panel-session")
			request.Header.Set("X-Canvas-Key-ID", "7")
			request.Header.Set("Cookie", "panel-secret=test")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, request)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == 200 {
				require.True(t, w.Flushed)
				require.Equal(t, "data: first\n\ndata: [DONE]\n\n", w.Body.String())
			}
		})
	}
}

func TestCanvasKeyHeadersDoNotAuthenticateOrdinaryGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeStandard}
	svc := service.NewAPIKeyService(&stubApiKeyRepo{}, nil, nil, nil, nil, nil, cfg)
	r := gin.New()
	r.GET("/v1/models", gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)), func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Canvas-Key-ID", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 401, w.Code)
}

func TestCanvasRequiresPanelIdentityAndSinglePositiveKeyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, value := range []string{"", "0", "-1", "7,8", "7.0", "99999999999999999999999999"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/canvas-api/v1/models", nil)
		c.Request.Header.Set("X-Canvas-Key-ID", value)
		_, ok := CanvasKeyID(c)
		require.False(t, ok, value)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/canvas-api/v1/models", nil)
	_, ok := CanvasGatewayRequest(c, canvasKeyReaderFunc(func(context.Context, int64, int64) (*service.APIKey, error) {
		t.Fatal("unauthenticated lookup")
		return nil, nil
	}), 7, "/v1/models")
	require.False(t, ok)
}

func TestCanvasGatewayRouteAllowlist(t *testing.T) {
	for _, path := range []string{"/v1/models", "/v1/images/tasks/task-1", "/v1/videos/task-1", "/v1/videos/task-1/content", "/v1/videos/generations/task-1/content"} {
		require.True(t, CanvasGatewayPathAllowed("GET", path), path)
	}
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1/images/generations", "/v1/images/edits/async", "/v1/videos"} {
		require.True(t, CanvasGatewayPathAllowed("POST", path), path)
	}
	for _, path := range []string{"https://example.test/v1/models", "/api/v1/admin/users", "/v1/messages", "/v1/audio/speech", "/v1/videos/../models", "/v1/videos/%2e%2e", "/v1/videos//content", "/v1/images/batches", "/v1/videos/task/content/extra", "/v1/videos/edits/task"} {
		for _, method := range []string{"GET", "POST", "DELETE", "PATCH"} {
			require.False(t, CanvasGatewayPathAllowed(method, path), method+" "+path)
		}
	}
	require.False(t, CanvasGatewayPathAllowed("GET", "/v1/responses"))
}
