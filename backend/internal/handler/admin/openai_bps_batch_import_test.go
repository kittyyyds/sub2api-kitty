package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBPSImportHandlersRequireFullAdminAndDisableCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountHandler{}
	for _, observer := range []bool{false, true} {
		for _, endpoint := range []gin.HandlerFunc{h.StartBPSImport, h.ListBPSImports, h.GetBPSImport, h.CancelBPSImport} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/v1/admin/accounts/bps-imports", strings.NewReader("{}"))
			if observer {
				c.Request = c.Request.WithContext(service.WithObserverScope(c.Request.Context(), []int64{123}))
			}
			endpoint(c)
			require.Equal(t, 403, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		}
	}
}
