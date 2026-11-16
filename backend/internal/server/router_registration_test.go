//go:build unit

package server

import (
	"net/http"
 "encoding/json"
 "os"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Mount every optional handler, as the production dependency graph does. Tests
// of individual route modules cannot catch collisions between modules or
// duplicate registrations behind optional QQ/VM/media handler guards.
func TestPrimaryRouterRegistersAllFeaturesWithoutCollisions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &handler.Handlers{}
	fill := func(v any) {
		fields := reflect.ValueOf(v).Elem()
		for i := 0; i < fields.NumField(); i++ {
			f := fields.Field(i)
			if f.Kind() == reflect.Pointer && f.Type().Elem().Kind() == reflect.Struct {
				f.Set(reflect.New(f.Type().Elem()))
			}
		}
	}
	fill(h)
	fill(h.Admin)
	r := gin.New()
	deny := func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }
	pass := func(c *gin.Context) { c.Next() }
	cfg := &config.Config{}
	cfg.Pricing.DataDir = t.TempDir()
	require.NotPanics(t, func() {
		registerRoutes(r, h, deny, pass, deny, deny, pass, deny,
			&service.APIKeyService{}, &service.SubscriptionService{}, nil,
			&service.SettingService{}, &service.UserService{}, nil, cfg, nil)
	})
	if name := os.Getenv("CUSTOM_ROUTE_INVENTORY"); name != "" { rows:=[]map[string]string{}; for _, route := range r.Routes() { rows=append(rows,map[string]string{"method":route.Method,"path":route.Path}) }; data, err := json.Marshal(rows); require.NoError(t,err); require.NoError(t,os.WriteFile(name,data,0600)) }
	registered := map[string]bool{}
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, path := range []string{
		"POST /api/v1/qqbot/webhook",
		"GET /api/v1/user/qqbot/binding",
		"GET /api/v1/model-plaza",
		"GET /api/v1/canvas/bootstrap",
		"GET /api/v1/admin/accounts",
		"GET /api/v1/admin/usage/traffic",
		"GET /api/v1/admin/usage/traffic/keys",
		"GET /api/v1/admin/usage/traffic/health",
		"GET /api/v1/usage/traffic",
		"GET /api/v1/usage/traffic/keys",
		"GET /api/v1/admin/usage/connections",
		"GET /api/v1/admin/usage/:id/timing",
		"GET /api/v1/admin/settings/openai-response-header-timeout",
		"PUT /api/v1/admin/settings/openai-response-header-timeout",
		"GET /api/v1/admin/payment/dashboard",
		"GET /api/v1/admin/payment/orders",
		"GET /api/v1/admin/payment/coupons/templates",
		"GET /api/v1/admin/payment/invoices",
		"GET /api/v1/admin/payment/plans",
		"GET /api/v1/admin/users/cleanup/summary",
		"POST /api/v1/admin/users/cleanup/preview",
		"POST /api/v1/admin/users/cleanup/execute",
		"GET /api/v1/admin/users/:id/cleanup-guard",
		"PUT /api/v1/admin/users/:id/cleanup-guard",
	} {
		require.True(t, registered[path], "missing route: %s", path)
	}
	for _, path := range []string{
		"/api/v1/user/qqbot/binding", "/api/v1/canvas/bootstrap", "/api/v1/admin/accounts",
		"/api/v1/admin/payment/dashboard", "/api/v1/admin/payment/orders",
		"/api/v1/admin/payment/coupons/templates", "/api/v1/admin/payment/invoices", "/api/v1/admin/payment/plans",
		"/api/v1/admin/users/cleanup/summary", "/api/v1/admin/users/1/cleanup-guard",
		"/api/v1/admin/usage/traffic", "/api/v1/admin/usage/connections",
		"/api/v1/admin/usage/traffic/health", "/api/v1/admin/settings/openai-response-header-timeout",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusUnauthorized, w.Code, path)
	}
	for _, path := range []string{"/api/v1/admin/users/cleanup/preview", "/api/v1/admin/users/cleanup/execute"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		require.Equal(t, http.StatusUnauthorized, w.Code, path)
	}
	for route := range registered {
		for _, retired := range []string{"risk-control", "prompt-audit", "ip-audit", "supplier-balances", "cyber-allowlist"} {
			require.NotContains(t, route, retired, "retired route remains registered")
		}
	}
}
