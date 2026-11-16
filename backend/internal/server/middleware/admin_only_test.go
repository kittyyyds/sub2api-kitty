package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminOnlyRecognizesBothAdministrativeRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		role   string
		status int
	}{
		{"admin", http.StatusOK}, {"super_admin", http.StatusOK},
		{"user", http.StatusForbidden}, {"unknown", http.StatusForbidden},
		{"", http.StatusUnauthorized},
	} {
		t.Run(tc.role, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				if tc.role != "" {
					c.Set(string(ContextKeyUserRole), tc.role)
				}
				c.Next()
			})
			r.Use(AdminOnly())
			r.GET("/admin", func(c *gin.Context) { c.Status(http.StatusOK) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin", nil))
			require.Equal(t, tc.status, w.Code)
		})
	}
}
