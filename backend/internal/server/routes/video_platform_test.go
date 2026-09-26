//go:build unit

package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoPlatformEndpointGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/v1/videos/generations", 204},
		{http.MethodPost, "/v1/videos", 204},
		{http.MethodGet, "/v1/videos/task/content", 204},
		{http.MethodGet, "/models", 204},
		{http.MethodGet, "/v1/usage", 204},
		{http.MethodGet, "/v1/sub2api/billing", 204},
		{http.MethodPost, "/v1/sub2api/billing", 404},
		{http.MethodPost, "/v1/chat/completions", 404},
		{http.MethodPost, "/v1/images/generations", 404},
		{http.MethodGet, "/v1/responses", 404},
		{http.MethodPost, "/v1/messages", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("api_key", &service.APIKey{Group: &service.Group{Platform: service.PlatformVideo}})
			})
			router.Use(videoPlatformEndpointGuard)
			router.Handle(tc.method, tc.path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			require.Equal(t, tc.want, response.Code)
		})
	}
}
