package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskFiltersKeepCustomerScopeAndValidateRange(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, query, role    string
		admin, authed, valid bool
		user                 int64
		code                 int
	}{
		{"owner ignores forged user", "?user_id=999&group_id=7&model=video-1", "user", false, true, true, 42, 200},
		{"admin scoped user", "?user_id=17", "admin", true, true, true, 17, 200},
		{"admin all users", "", "admin", true, true, true, 0, 200},
		{"nonadmin denied", "", "user", true, true, false, 0, 403},
		{"unauthenticated", "", "", false, false, false, 0, 401},
		{"range too long", "?start_at=2025-01-01T00:00:00Z", "user", false, true, false, 0, 400},
		{"range reversed", "?start_at=2026-10-01T00:00:00Z", "user", false, true, false, 0, 400},
		{"invalid group", "?group_id=-2", "user", false, true, false, 0, 400},
		{"invalid status", "?status=other", "user", false, true, false, 0, 400},
		{"missing timezone", "?start_at=2026-09-01", "user", false, true, false, 0, 400},
		{"historical interval", "?start_at=2024-01-01T00:00:00Z&end_at=2024-02-01T00:00:00Z", "user", false, true, true, 42, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/video-tasks/stats"+tc.query, nil)
			if tc.authed {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			}
			c.Set(string(middleware.ContextKeyUserRole), tc.role)
			f, ok := videoTaskFilter(c, tc.admin, now)
			require.Equal(t, tc.valid, ok)
			require.Equal(t, tc.code, w.Code)
			if ok {
				require.Equal(t, tc.user, f.UserID)
				require.NoError(t, f.ValidateScope())
			}
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
}

func TestVideoTaskPaginationRejectsOverflow(t *testing.T) {
	for _, query := range []string{"?page=999999999999999999999999", "?page=10002", "?page_size=0", "?page_size=101", "?page=-1"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/video-tasks"+query, nil)
		_, _, err := videoTaskPagination(c)
		require.Error(t, err, query)
	}
}
