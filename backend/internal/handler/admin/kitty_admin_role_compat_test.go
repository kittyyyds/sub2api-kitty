package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestKittyAuditAdministratorRoleCompatibility(t *testing.T) {
	for _, role := range []string{"", "user", "observer", "admin", "super_admin"} {
		t.Run(role, func(t *testing.T) {
			w := auditAdminRequest(t, func(c *gin.Context) {
				if ipAuditAdministrator(c) {
					c.Status(http.StatusNoContent)
				}
			}, "GET", "/test", role, "")
			want := http.StatusForbidden
			if role == "admin" || role == "super_admin" {
				want = http.StatusNoContent
			}
			require.Equal(t, want, w.Code)
		})
	}
}
