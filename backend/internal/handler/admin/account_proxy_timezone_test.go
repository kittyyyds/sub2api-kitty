package admin

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type timezoneAdminStub struct {
	service.AdminService
	reads int
}

func (s *timezoneAdminStub) GetProxy(_ context.Context, id int64) (*service.Proxy, error) {
	s.reads++
	if id != 7 {
		return nil, errors.New("not found")
	}
	return &service.Proxy{ID: 7, Host: "fixture.invalid", Username: "secret-user", Password: "secret-password"}, nil
}

func TestProxyTimezoneHandlerOnlyReadsConfiguredProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		id, body      string
		status, reads int
	}{
		{"0", `{}`, 400, 0}, {"bad", `{}`, 400, 0}, {"7", `{"force":"yes"}`, 400, 0}, {"7", `{`, 400, 0}, {"8", `{}`, 404, 1}, {"7", `{"force":true}`, 200, 1},
	} {
		t.Run(tc.id+tc.body, func(t *testing.T) {
			stub := &timezoneAdminStub{}
			h := &AccountHandler{adminService: stub}
			r := gin.New()
			r.POST("/timezone/:proxy_id", h.ProbeOpenAIProxyTimezone)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/timezone/"+tc.id, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.reads, stub.reads)
			require.NotContains(t, w.Body.String(), "secret-")
			if tc.status == 200 {
				require.Contains(t, w.Body.String(), "probe_unavailable")
			}
		})
	}
}
