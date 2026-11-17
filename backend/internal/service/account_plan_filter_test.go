package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type kittyPlanFilterRepo struct {
	AccountRepository
	plans []string
	pages []int
}

func (r *kittyPlanFilterRepo) ListWithFilters(ctx context.Context, p pagination.PaginationParams, platform, typ, status, search string, group int64, privacy string) ([]Account, *pagination.PaginationResult, error) {
	r.plans = append(r.plans, AccountPlanFilter(ctx))
	r.pages = append(r.pages, p.Page)
	return []Account{{ID: int64(p.Page)}}, &pagination.PaginationResult{Total: 2}, nil
}

func TestKittyPlanFilterAliasesAndBulkPagination(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"", ""}, {"Plus", "plus"}, {"Pro_100", "prolite"}, {"pro-200", "pro"}, {"ChatGPT Pro", "pro"}, {"pro500", "promax"}, {"Free", "free"}} {
		t.Run(tc.input, func(t *testing.T) {
			ctx, err := WithAccountPlanFilter(context.Background(), tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, AccountPlanFilter(ctx))
			repo := &kittyPlanFilterRepo{}
			svc := &adminServiceImpl{accountRepo: repo}
			ids, err := svc.resolveBulkUpdateTargetIDs(context.Background(), &BulkUpdateAccountFilters{PlanType: tc.input})
			require.NoError(t, err)
			require.Equal(t, []int64{1, 2}, ids)
			require.Equal(t, []int{1, 2}, repo.pages)
			require.Equal(t, []string{tc.want, tc.want}, repo.plans)
		})
	}
	repo := &kittyPlanFilterRepo{}
	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.resolveBulkUpdateTargetIDs(context.Background(), &BulkUpdateAccountFilters{PlanType: "invalid OR true"})
	require.Error(t, err)
	require.Empty(t, repo.plans)
}
