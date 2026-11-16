//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateUser_WithAdminRole(t *testing.T) {
	repo := &userRepoStub{nextID: 30}
	svc := &adminServiceImpl{userRepo: repo}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	user, err := svc.CreateUser(context.Background(), &CreateUserInput{ActorAdminID: 90001,
		Email:    "admin@test.com",
		Password: "strong-pass",
		Role:     RoleAdmin,
	})
	require.NoError(t, err)
	require.Equal(t, RoleAdmin, user.Role)
}

func TestAdminService_CreateUser_DefaultsToUserRole(t *testing.T) {
	repo := &userRepoStub{nextID: 31}
	svc := &adminServiceImpl{userRepo: repo}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	user, err := svc.CreateUser(context.Background(), &CreateUserInput{ActorAdminID: 90001,
		Email:    "plain@test.com",
		Password: "strong-pass",
	})
	require.NoError(t, err)
	require.Equal(t, RoleUser, user.Role)
}

func TestAdminService_CreateUser_InvalidRoleRejected(t *testing.T) {
	repo := &userRepoStub{nextID: 32}
	svc := &adminServiceImpl{userRepo: repo}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	_, err := svc.CreateUser(context.Background(), &CreateUserInput{ActorAdminID: 90001,
		Email:    "bad@test.com",
		Password: "strong-pass",
		Role:     "superuser",
	})
	require.Error(t, err)
	require.Empty(t, repo.created, "非法角色不应写入用户")
}

func TestAdminService_UpdateUser_PromoteToAdmin(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUser}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{},
		authCacheInvalidator: invalidator,
	}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{ActorAdminID: 90001, Role: RoleAdmin})
	require.NoError(t, err)
	require.Equal(t, RoleAdmin, updated.Role)
	require.Equal(t, []int64{42}, invalidator.userIDs, "角色变更应失效认证缓存")
}

func TestAdminService_UpdateUser_RoleOmittedKeepsExisting(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleAdmin}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{}}

	newName := "renamed"
	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{ActorAdminID: 90001, Username: &newName})
	require.NoError(t, err)
	require.Equal(t, RoleAdmin, updated.Role, "未提供 role 时不应改变现有角色")
}

func TestAdminService_UpdateUser_InvalidRoleRejected(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUser}}
	repo := &rpmUserRepoStub{userRepoStub: base}
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{}}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{ActorAdminID: 90001, Role: "root"})
	require.Error(t, err)
	require.Nil(t, repo.lastUpdated, "非法角色不应触发持久化")
}

// roleGuardUserRepoStub 在 rpmUserRepoStub 之上提供可控的管理员计数，
// 用于测试"最后一个管理员不可降级"守卫。
type roleGuardUserRepoStub struct {
	*rpmUserRepoStub
	adminTotal int64
	superTotal int64
	listCalls  int
}

func (s *roleGuardUserRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	s.listCalls++
	if filters.Role == RoleSuperAdmin {
		return nil, &pagination.PaginationResult{Total: s.superTotal}, nil
	}
	return nil, &pagination.PaginationResult{Total: s.adminTotal}, nil
}

func TestAdminService_EnsureNotLastAdmin_RejectsLastLegacyAdmin(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "a@example.com", Role: RoleAdmin}}
	repo := &roleGuardUserRepoStub{rpmUserRepoStub: &rpmUserRepoStub{userRepoStub: base}, adminTotal: 1}
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{}}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	err := svc.ensureNotLastAdmin(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "last admin")
	require.Nil(t, repo.lastUpdated, "最后一个管理员不应被降级持久化")
	require.Equal(t, 2, repo.listCalls, "降级路径应触发管理员计数")
}

func TestAdminService_UpdateUser_DemoteAdminAllowedWhenOthersExist(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "a@example.com", Role: RoleAdmin}}
	repo := &roleGuardUserRepoStub{rpmUserRepoStub: &rpmUserRepoStub{userRepoStub: base}, adminTotal: 2}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{},
		authCacheInvalidator: invalidator,
	}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{ActorAdminID: 90001, Role: RoleUser})
	require.NoError(t, err)
	require.Equal(t, RoleUser, updated.Role)
	require.NotNil(t, repo.lastUpdated)
	require.Equal(t, RoleUser, repo.lastUpdated.Role, "存在其他管理员时允许降级")
}

func TestAdminService_UpdateUser_PromoteDoesNotCountAdmins(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "u@example.com", Role: RoleUser}}
	repo := &roleGuardUserRepoStub{rpmUserRepoStub: &rpmUserRepoStub{userRepoStub: base}, adminTotal: 1}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{},
		authCacheInvalidator: &authCacheInvalidatorStub{},
	}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{ActorAdminID: 90001, Role: RoleAdmin})
	require.NoError(t, err)
	require.Equal(t, RoleAdmin, updated.Role)
	require.Equal(t, 0, repo.listCalls, "升级路径不应触发管理员计数")
}

func TestAdminService_UpdateUser_AdminGuardsAreClientErrors(t *testing.T) {
	base := &userRepoStub{user: &User{ID: 42, Email: "a@example.com", Role: RoleAdmin}}
	repo := &roleGuardUserRepoStub{rpmUserRepoStub: &rpmUserRepoStub{userRepoStub: base}, adminTotal: 1}
	svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: &redeemRepoStub{}}

	svc.userRepo = roleActorTestRepo{UserRepository: svc.userRepo}
	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{ActorAdminID: 90001, Status: StatusDisabled})
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "ADMIN_DISABLE_FORBIDDEN", infraerrors.Reason(err))

	err = svc.ensureNotLastAdmin(context.Background())
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "LAST_ADMIN_DEMOTE_FORBIDDEN", infraerrors.Reason(err))
	require.Nil(t, repo.lastUpdated)
}

// Production passes the authenticated actor. Keep the existing target-user
// stubs while giving these role-management tests a distinct live super admin.
type roleActorTestRepo struct{ UserRepository }

func (r roleActorTestRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	if id == 90001 {
		return &User{ID: id, Role: RoleSuperAdmin, Status: StatusActive}, nil
	}
	return r.UserRepository.GetByID(ctx, id)
}
