//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type atomicVideoUsageStub struct {
	UsageBillingRepository
	err     error
	calls   int
	lastLog *UsageLog
	lastCmd *UsageBillingCommand
	result  *UsageBillingApplyResult
}

func (r *atomicVideoUsageStub) ApplyVideoUsage(_ context.Context, cmd *UsageBillingCommand, log *UsageLog) (*UsageBillingApplyResult, error) {
	r.calls++
	r.lastCmd, r.lastLog = cmd, log
	if r.err != nil {
		return nil, r.err
	}
	if r.result != nil {
		return r.result, nil
	}
	return &UsageBillingApplyResult{Applied: false}, nil
}

type videoReceiptQuotaInvalidator struct {
	APIKeyQuotaUpdater
	key string
}

func (r *videoReceiptQuotaInvalidator) InvalidateAuthCacheByKey(_ context.Context, key string) {
	r.key = key
}

func TestVideoUsageInvalidatesExhaustedHashOnlyKey(t *testing.T) {
	key := &APIKey{ID: 6, KeyHash: strings.Repeat("a", 64), Quota: 1}
	invalidator := &videoReceiptQuotaInvalidator{}
	repo := &atomicVideoUsageStub{result: &UsageBillingApplyResult{Applied: true, APIKeyQuotaExhausted: true}}
	err := applyVideoUsageBilling(context.Background(), &UsageLog{RequestID: "grok-video:task"},
		&postUsageBillingParams{Cost: &CostBreakdown{ActualCost: 1}, User: &User{ID: 1},
			APIKey: key, Account: &Account{ID: 205}, APIKeyService: invalidator},
		&billingDeps{deferredService: &DeferredService{}}, repo)
	require.NoError(t, err)
	require.Equal(t, key.AuthCacheInvalidationKey(), invalidator.key)
	require.NotEmpty(t, invalidator.key)
}

func TestVideoUsageRequiresAtomicRepository(t *testing.T) {
	_, _, key, account, _ := newVideoTaskFixture()
	logs := &openAIRecordUsageLogRepoStub{}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(logs, userRepo, nil, nil)
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "task", Model: "exact-video", VideoCount: 1,
			VideoPriceSnapshot: &VideoPriceSnapshot{Model: "exact-video", BillingMode: BillingModePerRequest, UnitPrice: 1, FinalUnitPrice: true}},
		APIKey: key, User: key.User, Account: account,
	})
	require.ErrorContains(t, err, "atomic video usage billing is unavailable")
	require.Zero(t, logs.calls)
	require.Zero(t, userRepo.lastAmount)
}

func TestVideoUsageRecordUsesAtomicReceiptAndPropagatesFailure(t *testing.T) {
	for _, mode := range []BillingMode{BillingModeVideo, BillingModePerRequest} {
		for _, fail := range []bool{false, true} {
			t.Run(string(mode)+map[bool]string{false: "/success", true: "/retry"}[fail], func(t *testing.T) {
				_, _, key, account, _ := newVideoTaskFixture()
				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				userRepo := &openAIRecordUsageUserRepoStub{}
				svc := newOpenAIRecordUsageServiceForTest(logs, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
				atomicRepo := &atomicVideoUsageStub{}
				if fail {
					atomicRepo.err = errors.New("receipt unavailable")
				}
				svc.usageBillingRepo = atomicRepo
				err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
					Result: &OpenAIForwardResult{RequestID: "task", ResponseID: "task", Model: "exact-video", VideoCount: 1,
						VideoDurationSeconds: 5, VideoResolution: "720p", VideoPriceSnapshot: &VideoPriceSnapshot{
							Model: "exact-video", BillingMode: mode, UnitPrice: .75, FinalUnitPrice: true, DurationSeconds: 5}},
					APIKey: key, User: key.User, Account: account,
				})
				if fail {
					require.ErrorIs(t, err, atomicRepo.err)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, 1, atomicRepo.calls)
				require.Zero(t, logs.calls, "no best-effort receipt or zero-cost placeholder may follow atomic billing")
				require.Zero(t, userRepo.lastAmount, "the legacy balance path must not run")
				require.Equal(t, "grok-video:task", atomicRepo.lastCmd.RequestID)
				require.Equal(t, 1, atomicRepo.lastLog.VideoCount)
				require.Equal(t, 5, *atomicRepo.lastLog.VideoDurationSeconds)
				want := .75
				if mode == BillingModeVideo {
					want *= 5
				}
				require.Equal(t, want, atomicRepo.lastLog.ActualCost)
			})
		}
	}
}

type videoReceiptKeyRepo struct {
	APIKeyRepository
	key *APIKey
}

func (r videoReceiptKeyRepo) GetByID(context.Context, int64) (*APIKey, error) { return r.key, nil }

type videoReceiptUserRepo struct {
	UserRepository
	user *User
}

func (r videoReceiptUserRepo) GetByID(context.Context, int64) (*User, error) { return r.user, nil }

type videoReceiptGroupRepo struct {
	GroupRepository
	group *Group
}

func (r videoReceiptGroupRepo) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }

func TestVideoTaskRetriesReceiptWithoutSupplierAndRequiresMatchingFingerprint(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "repair", true: "evidence_conflict"}[conflict], func(t *testing.T) {
			_, tasks, key, account, _ := newVideoTaskFixture()
			tasks.billed = true
			svc := newOpenAIRecordUsageServiceForTest(tasks, videoReceiptUserRepo{user: key.User}, nil, nil)
			svc.accountRepo = videoTaskAccountRepo{account: account}
			svc.channelService = &ChannelService{groupRepo: videoReceiptGroupRepo{group: key.Group}}
			svc.channelService.cache.Store(&channelCache{loadedAt: time.Now()})
			atomicRepo := &atomicVideoUsageStub{}
			if conflict {
				atomicRepo.err = ErrUsageBillingRequestConflict
			}
			svc.usageBillingRepo = atomicRepo
			task := &VideoTask{ID: "local", UpstreamTaskID: "paid", UserID: key.UserID, APIKeyID: key.ID,
				GroupID: key.Group.ID, AccountID: account.ID, Status: "pending", CreatedAt: time.Now(),
				Pending: GrokVideoPendingBilling{Model: "exact-video", OriginalModel: "exact-video", BillingModel: "exact-video",
					VideoDurationSeconds: 5, PriceSnapshot: &VideoPriceSnapshot{Model: "exact-video", BillingMode: BillingModeVideo,
						UnitPrice: .75, DurationSeconds: 5, FinalUnitPrice: true}}}
			err := svc.pollVideoTask(context.Background(), task, &APIKeyService{apiKeyRepo: videoReceiptKeyRepo{key: key}})
			if conflict {
				require.ErrorIs(t, err, ErrUsageBillingRequestConflict)
				require.Equal(t, "needs_review", tasks.state)
			} else {
				require.NoError(t, err)
				require.Equal(t, "done", tasks.state)
			}
			require.Equal(t, 1, atomicRepo.calls, "only the atomic receipt path runs; no supplier client is configured")
			require.Equal(t, 3.75, atomicRepo.lastLog.ActualCost)
		})
	}
}
