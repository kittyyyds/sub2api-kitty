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

func TestFinanceRoutesRequireAdministrator(t *testing.T) {
	r := gin.New()
	g := r.Group("/api/v1/admin")
	g.Use(gin.HandlerFunc(middleware.NewAdminAuthMiddleware(nil, nil, nil, nil)))
	registerChannelRoutes(g, &handler.Handlers{Admin: &handler.AdminHandlers{Channel: admin.NewChannelHandler(nil, nil, nil)}})
	for _, tc := range []struct{ method, path string }{{"GET", "/supplier-balances"}, {"PUT", "/supplier-balances/thresholds"}, {"GET", "/finance-overview"}, {"GET", "/permanent-usage"}, {"POST", "/permanent-usage/backfill"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, "/api/v1/admin"+tc.path, nil))
		require.Equal(t, 401, w.Code, tc.path)
	}
}
