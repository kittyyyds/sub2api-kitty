package routes

import (
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKittyUpgradeRegistersCompleteAdminRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &handler.Handlers{}
	fill := func(value any) {
		v := reflect.ValueOf(value).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.CanSet() && f.Kind() == reflect.Pointer && f.Type().Elem().Kind() == reflect.Struct {
				f.Set(reflect.New(f.Type().Elem()))
			}
		}
	}
	fill(h)
	fill(h.Admin)
	e := gin.New()
	noop := func(c *gin.Context) { c.Next() }
	require.NotPanics(t, func() {
		RegisterAdminRoutes(e.Group("/api/v1"), h, noop, noop, noop, nil, &middleware.PanelRateLimiter{}, nil)
	})
	paths := map[string]bool{}
	for _, route := range e.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	for _, p := range []string{
		"GET /api/v1/admin/accounts/openai-request-timezones",
		"POST /api/v1/admin/accounts/openai-request-timezones/proxy/:proxy_id",
		"GET /api/v1/admin/support-tickets",
		"GET /api/v1/admin/channels/context-rules",
	} {
		require.True(t, paths[p], p)
	}
}
