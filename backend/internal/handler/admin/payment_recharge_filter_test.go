package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRechargeActivityFilterRejectsInvalidBoundsAndSelectors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{
		"status=unknown", "channel=unknown", "start_time=bad", "end_time=bad",
		"start_time=2026-11-11T00:00:00Z&end_time=2026-11-10T00:00:00Z",
	} {
		t.Run(query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest("GET", "/?"+query, nil)
			_, ok := rechargeActivityFilter(ctx)
			require.False(t, ok)
			require.Equal(t, 400, rec.Code)
		})
	}
}

func TestRechargeActivityFilterPreservesExplicitInstants(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/?channel=manual&email=%20Alice%20&status=unpaid&start_time=2026-11-10T16:00:00Z&end_time=2026-11-11T16:00:00Z", nil)
	filter, ok := rechargeActivityFilter(ctx)
	require.True(t, ok)
	require.Equal(t, "Alice", filter.Email)
	require.Equal(t, "manual", filter.Channel)
	require.Equal(t, "unpaid", filter.Status)
	require.Equal(t, int64(86400), int64(filter.EndTime.Sub(filter.StartTime).Seconds()))
}
