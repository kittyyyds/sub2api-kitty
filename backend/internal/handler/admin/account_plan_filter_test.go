package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kittyPlanFilterAdmin struct {
	service.AdminService
	plans    []string
	pages    []int
	selected []int64
}

func (s *kittyPlanFilterAdmin) ListAccounts(ctx context.Context, page, pageSize int, platform, typ, status, search string, group int64, privacy, sort, order string) ([]service.Account, int64, error) {
	s.plans = append(s.plans, service.AccountPlanFilter(ctx))
	s.pages = append(s.pages, page)
	return []service.Account{{ID: int64(page), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}}, 2, nil
}
func (s *kittyPlanFilterAdmin) ListAccountsForSchedulerScoreFilter(ctx context.Context, platform, typ, status, search string, group int64, privacy string) ([]service.Account, error) {
	s.plans = append(s.plans, service.AccountPlanFilter(ctx))
	return nil, nil
}
func (s *kittyPlanFilterAdmin) GetAccountsByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	s.selected = append([]int64(nil), ids...)
	return []*service.Account{{ID: ids[0], Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"plan_type": "plus"}}}, nil
}

func TestKittyPlanFilterReachesListLiteBillingAndExport(t *testing.T) {
	for _, endpoint := range []string{"list", "lite", "rates", "export", "score_pool", "selected"} {
		t.Run(endpoint, func(t *testing.T) {
			svc := &kittyPlanFilterAdmin{}
			h := &AccountHandler{adminService: svc}
			router := gin.New()
			group := router.Group("/accounts", h.ApplyPlanTypeFilter)
			switch endpoint {
			case "list", "lite":
				group.GET("", h.List)
			case "rates":
				group.GET("", h.GetUpstreamBillingRates)
			default:
				group.GET("", func(c *gin.Context) {
					if endpoint == "score_pool" {
						h.listAccountSchedulerScoreFilterPool(c.Request.Context(), "openai", "oauth", "", "", 0, "")
					} else {
						var ids []int64
						if endpoint == "selected" {
							ids = []int64{81}
						}
						accounts, err := h.resolveExportAccounts(c.Request.Context(), ids, c)
						require.NoError(t, err)
						if endpoint == "selected" {
							require.Equal(t, int64(81), accounts[0].ID)
						} else {
							require.Len(t, accounts, 2)
						}
					}
					c.Status(http.StatusOK)
				})
			}
			rec := httptest.NewRecorder()
			url := "/accounts?plan_type=Pro_100&page=2&page_size=1"
			if endpoint == "lite" {
				url += "&lite=1"
			}
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			if endpoint == "selected" {
				require.Equal(t, []int64{81}, svc.selected)
				require.Empty(t, svc.plans)
			} else {
				require.NotEmpty(t, svc.plans)
				for _, plan := range svc.plans {
					require.Equal(t, "prolite", plan)
				}
				if endpoint == "export" {
					require.Equal(t, []int{1, 2}, svc.pages)
				}
			}
		})
	}
}

func TestKittyPlanFilterRejectsInvalidBeforeHandler(t *testing.T) {
	h := &AccountHandler{}
	router := gin.New()
	called := false
	router.GET("/accounts", h.ApplyPlanTypeFilter, func(c *gin.Context) { called = true; c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts?plan_type=invalid", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, called)
}
