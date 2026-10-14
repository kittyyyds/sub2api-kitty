//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVideoFinalUnitPriceLocksAndBillsWithoutMultipliers(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mode        BillingMode
		price, want float64
		independent bool
	}{
		{"per request", BillingModePerRequest, 1.3, 1.3, false},
		{"per second", BillingModeVideo, .25, 7.5, false},
		{"independent media rate", BillingModeVideo, .25, 7.5, true},
		{"explicit zero", BillingModePerRequest, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			submission, _, key, account, body := newVideoTaskFixture()
			key.Group.ModelPricing[0].BillingMode = tc.mode
			*key.Group.ModelPricing[0].PerRequestPrice = tc.price
			key.Group.RateMultiplier = 3
			key.Group.VideoRateIndependent = tc.independent
			key.Group.VideoRateMultiplier = .2
			userRate := 7.0
			rateRepo := &openAIUserGroupRateRepoStub{rate: &userRate}
			submission.userGroupRateResolver = newUserGroupRateResolver(rateRepo, nil, time.Minute, nil, "video.final-price.test")

			task, err := submission.BeginVideoTask(context.Background(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
			require.NoError(t, err)
			require.Zero(t, rateRepo.calls, "final unit pricing does not depend on the user's group discount")
			require.True(t, task.Pending.PriceSnapshot.FinalUnitPrice)
			require.Equal(t, 1.0, task.Pending.PriceSnapshot.RateMultiplier, "old binaries must also charge the final price after rollback")
			// Simulate persistence/restart before the completion is received.
			raw, err := json.Marshal(task.Pending)
			require.NoError(t, err)
			var pending GrokVideoPendingBilling
			require.NoError(t, json.Unmarshal(raw, &pending))
			require.True(t, pending.PriceSnapshot.FinalUnitPrice)
			// Neither later edits nor the query's reported duration may reprice a task.
			*key.Group.ModelPricing[0].PerRequestPrice = 999
			key.Group.RateMultiplier = 24
			key.Group.VideoRateMultiplier = 11
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			billing := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, rateRepo)
			atomicRepo := &atomicVideoUsageStub{}
			billing.usageBillingRepo = atomicRepo
			result := &OpenAIForwardResult{RequestID: "video-final-price", ResponseID: "video-final-price",
				Model: pending.Model, BillingModel: pending.BillingModel, VideoCount: 1,
				VideoDurationSeconds: 120, VideoPriceSnapshot: pending.PriceSnapshot, Duration: time.Second}
			require.NoError(t, billing.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: result, APIKey: key, User: key.User, Account: account,
			}))
			require.NotNil(t, atomicRepo.lastLog)
			require.InDelta(t, tc.want, atomicRepo.lastLog.TotalCost, 1e-12)
			require.InDelta(t, tc.want, atomicRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, tc.want, atomicRepo.lastCmd.BalanceCost, 1e-12)
			require.Equal(t, 1.0, atomicRepo.lastLog.RateMultiplier)
			require.Equal(t, 30, *atomicRepo.lastLog.VideoDurationSeconds)
			require.Equal(t, string(tc.mode), *atomicRepo.lastLog.BillingMode)
		})
	}
}

func TestVideoOldPriceSnapshotRetainsOriginalCharge(t *testing.T) {
	for _, tc := range []struct {
		name, raw        string
		want, multiplier float64
	}{
		{"old per second", `{"model":"exact-video","billing_mode":"video","unit_price":0.25,"rate_multiplier":2,"duration_seconds":30}`, 15, 2},
		{"old per request", `{"model":"exact-video","billing_mode":"per_request","unit_price":1.3,"rate_multiplier":2,"duration_seconds":30}`, 2.6, 2},
		{"old free task", `{"model":"exact-video","billing_mode":"per_request","unit_price":1.3,"rate_multiplier":0,"duration_seconds":30}`, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var snapshot VideoPriceSnapshot
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &snapshot))
			require.False(t, snapshot.FinalUnitPrice)
			group := videoTestGroup(BillingModePerRequest, 999)
			group.RateMultiplier, group.VideoRateMultiplier, group.VideoRateIndependent = 24, 11, true
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			billing := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			atomicRepo := &atomicVideoUsageStub{}
			billing.usageBillingRepo = atomicRepo
			require.NoError(t, billing.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{RequestID: "old-video-price", Model: "exact-video", VideoCount: 1,
					VideoDurationSeconds: 120, VideoPriceSnapshot: &snapshot, Duration: time.Second},
				APIKey: &APIKey{ID: 6, GroupID: &group.ID, Group: group}, User: &User{ID: 1},
				Account: &Account{ID: 205, Platform: PlatformVideo, Type: AccountTypeAPIKey},
			}))
			require.InDelta(t, tc.want, atomicRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, tc.want, atomicRepo.lastCmd.BalanceCost, 1e-12)
			require.Equal(t, tc.multiplier, atomicRepo.lastLog.RateMultiplier)
			require.Equal(t, 30, *atomicRepo.lastLog.VideoDurationSeconds)
		})
	}
}
