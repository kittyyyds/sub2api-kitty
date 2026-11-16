//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type canvasHandlerKeys struct {
	key         *service.APIKey
	lookupUsers []int64
}

func (s *canvasHandlerKeys) GetOwnedForCanvas(_ context.Context, userID, keyID int64) (*service.APIKey, error) {
	s.lookupUsers = append(s.lookupUsers, userID)
	if s.key.ID != keyID || s.key.UserID != userID {
		return nil, service.ErrAPIKeyNotFound
	}
	return s.key, nil
}
func (s *canvasHandlerKeys) List(_ context.Context, userID int64, _ pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	s.lookupUsers = append(s.lookupUsers, userID)
	return []service.APIKey{*s.key}, &pagination.PaginationResult{Total: 1}, nil
}

type canvasHandlerUsers struct{}

func (canvasHandlerUsers) GetByID(context.Context, int64) (*service.User, error) {
	return &service.User{ID: 42, Username: "canvas-user", Email: "fixture@example.test", Balance: 12, PasswordHash: "never-return"}, nil
}

func newCanvasHandlerTestRouter(t *testing.T, gateway http.Handler) (*gin.Engine, *canvasHandlerKeys) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := &service.Group{ID: 3, Name: "group", Platform: service.PlatformOpenAI}
	keys := &canvasHandlerKeys{key: &service.APIKey{ID: 7, UserID: 42, Key: "sk-fixture-never-return-full-key", KeyHash: "never-return-hash", Status: service.StatusActive, Name: "my-key", GroupID: &g.ID, Group: g}}
	h := &CanvasHandler{apiKeyService: keys, userService: canvasHandlerUsers{}, gateway: gateway}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "Bearer fixture-session" {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		} else {
			c.AbortWithStatus(401)
		}
	})
	r.GET("/api/v1/canvas/bootstrap", h.Bootstrap)
	r.GET("/api/v1/canvas/keys/:id/models", h.Models)
	r.Any("/canvas-api/v1/*path", h.Gateway)
	return r, keys
}

func canvasHandlerRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer fixture-session")
	req.Header.Set("X-Canvas-Key-ID", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCanvasBootstrapOnlyReturnsWhitelistedMetadata(t *testing.T) {
	r, keys := newCanvasHandlerTestRouter(t, nil)
	w := canvasHandlerRequest(r, "GET", "/api/v1/canvas/bootstrap")
	require.Equal(t, 200, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.NotContains(t, w.Body.String(), keys.key.Key)
	require.NotContains(t, w.Body.String(), keys.key.KeyHash)
	require.NotContains(t, w.Body.String(), "never-return")
	var payload struct {
		Data struct {
			User        map[string]any   `json:"user"`
			Keys        []map[string]any `json:"keys"`
			StorageMode string           `json:"storage_mode"`
		}
		Code int
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "local", payload.Data.StorageMode)
	require.Len(t, payload.Data.User, 4)
	require.Len(t, payload.Data.Keys, 1)
	require.NotContains(t, payload.Data.Keys[0], "key")
	require.Equal(t, []int64{42}, keys.lookupUsers)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/canvas/bootstrap", nil))
	require.Equal(t, 401, w.Code)
}

func TestCanvasModelsNormalizesCatalogAndKeepsSecretsServerSide(t *testing.T) {
	called := 0
	r, keys := newCanvasHandlerTestRouter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("X-Canvas-Key-ID"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[{"id":"gpt-test"},{"id":"gpt-test"},{"id":""}]}`))
	}))
	w := canvasHandlerRequest(r, "GET", "/api/v1/canvas/keys/7/models?client_version=fixture")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, 1, called)
	require.NotContains(t, w.Body.String(), keys.key.Key)
	var payload struct {
		Data struct {
			Models []struct {
				ID           string `json:"id"`
				Capabilities any    `json:"capabilities"`
			} `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Models, 1)
	require.Equal(t, "gpt-test", payload.Data.Models[0].ID)
	keys.key.UserID = 43
	w = canvasHandlerRequest(r, "GET", "/api/v1/canvas/keys/7/models")
	require.Equal(t, 404, w.Code)
	require.Equal(t, 1, called)
}

func TestCanvasModelsPreservesGatewayDenial(t *testing.T) {
	r, _ := newCanvasHandlerTestRouter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":{"message":"expired"}}`))
	}))
	w := canvasHandlerRequest(r, "GET", "/api/v1/canvas/keys/7/models")
	require.Equal(t, 403, w.Code)
}

func TestCanvasGatewayDeniesUnlistedEndpointBeforeLookup(t *testing.T) {
	r, keys := newCanvasHandlerTestRouter(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unlisted relay") }))
	for _, endpoint := range []string{"/canvas-api/v1/admin/users", "/canvas-api/v1/audio/speech", "/canvas-api/v1/images/batches", "/canvas-api/v1/responses/compact"} {
		w := canvasHandlerRequest(r, "POST", endpoint)
		require.Equal(t, 404, w.Code, endpoint)
	}
	require.Empty(t, keys.lookupUsers)
}

func TestCanvasModelRecorderBoundsPayload(t *testing.T) {
	r := &canvasModelRecorder{}
	_, err := r.Write([]byte(strings.Repeat("x", 4<<20+1)))
	require.Error(t, err)
	require.Zero(t, r.body.Len())
}
