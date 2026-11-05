package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestProviderHubAllRoutesRequireAdministrator(t *testing.T) {
	router := gin.New()
	g := router.Group("/api/v1/admin")
	g.Use(gin.HandlerFunc(middleware.NewAdminAuthMiddleware(nil, nil, nil, nil)))
	registerChannelRoutes(g, &handler.Handlers{Admin: &handler.AdminHandlers{Channel: admin.NewChannelHandler(nil, nil, nil)}})
	for _, tc := range []struct{ method, path string }{{"GET", "/operations"}, {"GET", "/provider-hub"}, {"POST", "/provider-hub/suppliers"}, {"PUT", "/provider-hub/suppliers/1"}, {"POST", "/provider-hub/suppliers/1/archive"}, {"POST", "/provider-hub/wallets"}, {"PUT", "/provider-hub/wallets/1"}, {"POST", "/provider-hub/wallets/1/archive"}, {"PUT", "/provider-hub/wallets/1/authorization"}, {"PUT", "/provider-hub/wallets/1/bindings"}, {"POST", "/provider-hub/bindings/1/unbind"}, {"POST", "/provider-hub/wallets/1/purchases"}, {"POST", "/provider-hub/purchases/1/void"}, {"POST", "/provider-hub/wallets/1/rules"}, {"POST", "/provider-hub/rules/1/close"}, {"POST", "/provider-hub/adjustments"}, {"POST", "/provider-hub/wallets/1/sync"}, {"GET", "/provider-hub/wallets/1/keys"}, {"GET", "/provider-hub/wallets/1/groups"}, {"POST", "/provider-hub/wallets/1/keys"}, {"POST", "/provider-hub/wallets/1/import"}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(tc.method, "/api/v1/admin"+tc.path, nil))
		require.Equal(t, 401, w.Code, tc.method+tc.path)
	}
}
