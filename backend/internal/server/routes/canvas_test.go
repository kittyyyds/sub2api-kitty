package routes

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type canvasRouteKeys struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (s *canvasRouteKeys) GetByID(context.Context, int64) (*service.APIKey, error) { return s.key, nil }
func (s *canvasRouteKeys) UpdateLastUsed(context.Context, int64, time.Time) error  { return nil }

type canvasRouteUsers struct{ service.UserRepository }

func (*canvasRouteUsers) GetByID(context.Context, int64) (*service.User, error) {
	return &service.User{ID: 42, Status: service.StatusActive, Balance: 1}, nil
}

func TestCanvasReentersOriginalGatewayModelAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	cfg := &config.Config{RunMode: config.RunModeStandard, Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024}}
	g := &service.Group{ID: 3, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true, ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"allowed-model"}}}
	keys := service.NewAPIKeyService(&canvasRouteKeys{key: &service.APIKey{ID: 7, UserID: 42, KeyHash: strings.Repeat("a", 64), Status: service.StatusActive, GroupID: &g.ID, Group: g}}, &canvasRouteUsers{}, nil, nil, nil, nil, cfg)
	RegisterGatewayRoutes(r, &handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil)}, middleware.NewAPIKeyAuthMiddleware(keys, nil, cfg), keys, nil, nil, nil, nil, cfg)
	RegisterCanvasRoutes(r, r.Group("/api/v1"), middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	}), keys, nil, nil, nil)
	req := httptest.NewRequest("POST", "/canvas-api/v1/chat/completions", strings.NewReader(`{"model":"denied-model","messages":[]}`))
	req.Header.Set("Authorization", "Bearer panel-session")
	req.Header.Set("X-Canvas-Key-ID", "7")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 404, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "not available for this group")
}

func TestCanvasRoutesRequireMainSiteJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterCanvasRoutes(r, r.Group("/api/v1"), middleware.JWTAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(401) }), nil, nil, nil, nil)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/canvas/bootstrap"}, {"GET", "/api/v1/canvas/keys/7/models"}, {"POST", "/api/v1/canvas/keys/7/quote"},
		{"POST", "/canvas-api/v1/chat/completions"}, {"GET", "/canvas-api/v1/videos/task/content"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
		require.Equal(t, 401, w.Code, route.path)
	}
}

func TestCanvasRoutesBlockUnlistedGatewayWithAuthenticatedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterCanvasRoutes(r, r.Group("/api/v1"), middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	}), nil, nil, nil, nil)
	for _, route := range []struct{ method, path string }{
		{"GET", "/canvas-api/v1/admin/users"}, {"DELETE", "/canvas-api/v1/videos/task"}, {"POST", "/canvas-api/v1/messages"},
		{"GET", "/canvas-api/v1/responses"}, {"POST", "/canvas-api/v1/audio/speech"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
		require.Equal(t, 404, w.Code, route.path)
	}
}
