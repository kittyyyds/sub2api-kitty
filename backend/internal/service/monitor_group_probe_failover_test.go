package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitorGroupProbeFailsOverDepletedAccountBeforeAnswer(t *testing.T) {
	router := &groupTestRouterFake{steps: []routeStep{{account: account(77, "depleted")}, {account: account(88, "ready")}}}
	calls := 0
	groups, _ := newGroupTestService(newGroupTestRepoFake(), router, func(_ context.Context, id int64, model string, cfg *MonitorProbeConfig) (*ScheduledTestResult, error) {
		calls++
		require.Equal(t, "gpt-test", model)
		require.Equal(t, "high", cfg.ReasoningEffort)
		if id == 77 {
			return &ScheduledTestResult{Status: "failed", ErrorMessage: `API returned 403: {"message":"Insufficient account balance"}`}, nil
		}
		require.Equal(t, []int64{77}, router.released, "release the first slot before selecting another")
		return &ScheduledTestResult{Status: "success", ResponseText: "21"}, nil
	})
	result := groups.runSample(context.Background(), &MonitorProbePlan{ModelID: "gpt-test", Config: &MonitorProbeConfig{ReasoningEffort: "high"}}, &Group{ID: 4})
	require.Equal(t, "success", result.Status)
	require.Equal(t, int64(88), result.AccountID)
	require.Equal(t, 2, calls)
	require.Equal(t, []int64{77}, router.excluded[1])
	require.Equal(t, []int64{77, 88}, router.released)
	require.Nil(t, result.CostUSD)
}

func TestMonitorGroupProbeNeverRetriesAnAnswerOrRequestError(t *testing.T) {
	for _, tc := range []struct{ name, status, answer, message string }{
		{"wrong completed answer", "success", "29", ""},
		{"wrong judged answer", "failed", "29", "answer_mismatch: expected 21"},
		{"partial stream", "failed", "21", "API returned 503"},
		{"invalid request", "failed", "", "API returned 400: invalid_request"},
		{"content policy", "failed", "", "API returned 403: content policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := &groupTestRouterFake{steps: []routeStep{{account: account(77, "first")}, {account: account(88, "unused")}}}
			groups, _ := newGroupTestService(newGroupTestRepoFake(), router, func(context.Context, int64, string, *MonitorProbeConfig) (*ScheduledTestResult, error) {
				return &ScheduledTestResult{Status: tc.status, ResponseText: tc.answer, ErrorMessage: tc.message}, nil
			})
			result := groups.runSample(context.Background(), &MonitorProbePlan{ModelID: "gpt-test"}, &Group{ID: 4})
			require.Equal(t, int64(77), result.AccountID)
			require.Len(t, router.excluded, 1)
			require.Equal(t, []int64{77}, router.released)
		})
	}
}

func TestMonitorGroupProbeBoundsRetriesAndRetainsFailure(t *testing.T) {
	router := &groupTestRouterFake{steps: []routeStep{{account: account(77, "one")}, {account: account(88, "two")}, {account: account(99, "three")}, {account: account(100, "unused")}}}
	groups, _ := newGroupTestService(newGroupTestRepoFake(), router, func(context.Context, int64, string, *MonitorProbeConfig) (*ScheduledTestResult, error) {
		return &ScheduledTestResult{Status: "failed", ErrorMessage: "API returned 403: Insufficient account balance"}, nil
	})
	plan := &MonitorProbePlan{ModelID: "gpt-test"}
	result := groups.runSample(context.Background(), plan, &Group{ID: 4})
	require.Equal(t, "failed", result.Status)
	require.Equal(t, int64(99), result.AccountID)
	require.Len(t, router.excluded, 3)
	require.Equal(t, []int64{77, 88, 99}, router.released)
	require.Equal(t, "quota_or_balance", monitorProbeErrorCategory(result.ErrorMessage))

	router.steps = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = groups.runSample(ctx, plan, &Group{ID: 4})
	require.Equal(t, context.Canceled.Error(), result.ErrorMessage)
	require.Len(t, router.excluded, 3, "cancellation must not select an account")
}

func TestChannelMonitorV2CandyRetainsSafeFailureCategory(t *testing.T) {
	repo := &candyRepoFake{channelMonitorV2RepoStub: channelMonitorV2RepoStub{config: candyConfigFixture()}}
	router := &groupTestRouterFake{steps: []routeStep{{account: account(77, "depleted")}}}
	groups, _ := newGroupTestService(newGroupTestRepoFake(), router, func(context.Context, int64, string, *MonitorProbeConfig) (*ScheduledTestResult, error) {
		return &ScheduledTestResult{Status: "failed", ErrorMessage: `API returned 403: Insufficient account balance; secret upstream URL`}, nil
	})
	candyServiceFixture(repo, groups).RunDue(context.Background(), groupTestNow)
	require.Len(t, repo.results, 1)
	require.Equal(t, "error", repo.results[0].Verdict)
	require.Equal(t, "quota_or_balance", repo.results[0].Reason)
	require.Empty(t, repo.results[0].AnswerPreview)
}
