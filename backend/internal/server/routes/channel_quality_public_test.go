package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelQualityPublicRoutesBlockOnlyUserResults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &channelMonitorRouteSettingRepoStub{values: map[string]string{service.SettingKeyChannelQualityPublicEnabled: "false"}}
	svc := service.NewSettingService(repo, &config.Config{})
	router := gin.New()
	registerChannelQualityPublicRoutes(router.Group("/user"), &handler.UserHandler{}, svc)
	router.GET("/admin/test-plans", func(c *gin.Context) { c.Status(http.StatusOK) })
	for _, path := range []string{"/user/test-results", "/user/test-results?group_view=all", "/user/test-results/1/history", "/user/test-votes", "/user/test-results/1/vote"} {
		method := http.MethodGet
		if path == "/user/test-results/1/vote" {
			method = http.MethodPost
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, path, nil))
		require.Equal(t, http.StatusForbidden, rec.Code, path)
		require.Contains(t, rec.Body.String(), "CHANNEL_QUALITY_PUBLIC_DISABLED")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/test-plans", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	// The gate reads the current value; enabling takes effect without restart.
	repo.values[service.SettingKeyChannelQualityPublicEnabled] = "true"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/user/test-results", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code, "request reaches the normal authenticated handler")
}
