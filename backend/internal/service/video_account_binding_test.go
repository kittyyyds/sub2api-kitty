//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestVideoAccountInvalidCreateNeverPersistsAccount(t *testing.T) {
	for _, platforms := range [][2]string{{PlatformVideo, PlatformGrok}, {PlatformGrok, PlatformVideo}, {PlatformVideo, PlatformComposite}} {
		t.Run(platforms[0]+"_to_"+platforms[1], func(t *testing.T) {
			for _, admin := range []bool{true, false} {
				repo := &accountRepoStubForBulkUpdate{createID: 7}
				groups := &groupRepoStubForAdmin{getByID: &Group{ID: 25, Platform: platforms[1]}}
				var err error
				if admin {
					svc := &adminServiceImpl{accountRepo: repo, groupRepo: groups}
					_, err = svc.CreateAccount(context.Background(), &CreateAccountInput{
						Name: "rejected", Platform: platforms[0], Type: AccountTypeAPIKey,
						GroupIDs: []int64{25}, SkipDefaultGroupBind: true, SkipMixedChannelCheck: true,
					})
				} else {
					svc := &AccountService{accountRepo: repo, groupRepo: groups}
					_, err = svc.Create(context.Background(), CreateAccountRequest{
						Name: "rejected", Platform: platforms[0], Type: AccountTypeAPIKey, GroupIDs: []int64{25},
					})
				}
				status, _ := infraerrors.ToHTTP(err)
				require.Equal(t, http.StatusBadRequest, status)
				requireApplicationErrorReason(t, err, "VIDEO_GROUP_PLATFORM_MISMATCH")
				require.Nil(t, repo.createAccount, "a 400 response must not leave a newly created account")
				require.Empty(t, repo.bindGroupsCalls)
			}
		})
	}
}

func TestVideoAccountInvalidUpdateNeverWritesOtherSettings(t *testing.T) {
	for _, platforms := range [][2]string{{PlatformVideo, PlatformGrok}, {PlatformGrok, PlatformVideo}} {
		t.Run(platforms[0]+"_to_"+platforms[1], func(t *testing.T) {
			repo := &accountRepoStubForBulkUpdate{getByIDAccounts: map[int64]*Account{
				7: {ID: 7, Platform: platforms[0], Type: AccountTypeAPIKey, Status: StatusActive},
			}}
			groups := &groupRepoStubForAdmin{getByID: &Group{ID: 25, Platform: platforms[1]}}
			svc := &adminServiceImpl{accountRepo: repo, groupRepo: groups}
			ids := []int64{25}
			_, err := svc.UpdateAccount(context.Background(), 7, &UpdateAccountInput{
				GroupIDs: &ids, Status: "inactive", SkipMixedChannelCheck: true,
			})
			requireApplicationErrorReason(t, err, "VIDEO_GROUP_PLATFORM_MISMATCH")
			require.Empty(t, repo.updatedAccounts)
			require.Empty(t, repo.bindGroupsCalls)
		})
	}
}

func TestVideoAccountInvalidBulkBindingNeverWritesSettings(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{
		{ID: 7, Platform: PlatformGrok, Type: AccountTypeAPIKey},
	}}
	groups := &groupRepoStubForAdmin{getByID: &Group{ID: 25, Platform: PlatformVideo}}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groups}
	ids := []int64{25}
	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{7}, GroupIDs: &ids, SkipMixedChannelCheck: true,
	})
	requireApplicationErrorReason(t, err, "VIDEO_GROUP_PLATFORM_MISMATCH")
	require.Zero(t, repo.bulkUpdateCalls)
	require.Empty(t, repo.bindGroupsCalls)
}

type videoAtomicCreateRepo struct {
	accountRepoStubForBulkUpdate
	atomicCalls int
	groups      []AccountGroup
	err         error
}

func (r *videoAtomicCreateRepo) CreateWithAccountGroups(_ context.Context, account *Account, groups []AccountGroup) error {
	r.atomicCalls++
	r.groups = groups
	if r.err != nil {
		return r.err
	}
	account.ID = 7
	return nil
}

func TestVideoAccountBoundCreateUsesAtomicWriter(t *testing.T) {
	for _, fail := range []bool{false, true} {
		repo := &videoAtomicCreateRepo{}
		if fail {
			repo.err = errors.New("group changed before transaction lock")
		}
		groups := &groupRepoStubForAdmin{getByID: &Group{ID: 25, Platform: PlatformVideo}}
		svc := &adminServiceImpl{accountRepo: repo, groupRepo: groups}
		_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
			Name: "atomic", Platform: PlatformVideo, Type: AccountTypeAPIKey,
			GroupIDs: []int64{25}, SkipDefaultGroupBind: true, SkipMixedChannelCheck: true,
		})
		if fail {
			require.ErrorIs(t, err, repo.err)
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, 1, repo.atomicCalls)
		require.Equal(t, []AccountGroup{{GroupID: 25, Priority: 1}}, repo.groups)
		require.Nil(t, repo.createAccount)
		require.Empty(t, repo.bindGroupsCalls)
	}
}
