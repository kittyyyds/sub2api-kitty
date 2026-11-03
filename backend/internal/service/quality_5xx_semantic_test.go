package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/qualityqueue"
	"github.com/Wei-Shaw/sub2api/internal/pkg/qualityqueue/queuetest"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func qualitySignalTestLimiter(t *testing.T) (*RateLimitService, *miniredis.Miniredis) {
	t.Helper()
	r := miniredis.RunT(t)
	client := queuetest.NewClient(r.Addr())
	t.Cleanup(func() { _ = client.Close() })
	queue := qualityqueue.NewRedis(client)
	t.Cleanup(func() { _ = queue.Close() })
	return &RateLimitService{qualityTrigger: &quality5xxTrigger{queue: queue}}, r
}

func quality5xxFixtureConfig() *PelicanTestConfig {
	return &PelicanTestConfig{QuestionKind: "candy", Prompt: CandyPrompt, ReasoningEffort: "high", ParallelCount: 1,
		Quality: &QualityPolicy{ExpectedAnswer: "21", Judge: &QualityJudgeConfig{GroupID: 1, ModelID: "judge", Prompt: "grade"}, Action: "disable_scheduling"}}
}

func TestQuality5xxSemanticStreams(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, tc := range []struct {
			name, payload string
			want          bool
		}{
			{"overload", `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"overloaded"}}}`, true},
			{"request_policy_400", `{"type":"response.failed","response":{"error":{"code":"invalid_prompt","message":"Invalid prompt: policy rejection"}}}`, false},
			{"request_permission", `{"type":"response.failed","response":{"error":{"type":"permission_error","message":"forbidden content"}}}`, false},
		} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", tc.name, passthrough), func(t *testing.T) {
				limiter, client := qualitySignalTestLimiter(t)
				svc := openAIClientToolsTestService(nil)
				svc.rateLimitService = limiter
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				account := &Account{ID: 417, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: " + tc.payload + "\n\n"))}
				if passthrough {
					_, _ = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
				} else {
					_, _ = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
				}
				require.Equal(t, tc.want, client.Exists(quality5xxPendingKey))
			})
		}
	}
}

func TestQuality5xxSemanticProbeDoesNotRecurse(t *testing.T) {
	limiter, client := qualitySignalTestLimiter(t)
	svc := &OpenAIGatewayService{rateLimitService: limiter}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(context.Background(), qualityProbeContextKey{}, true))
	svc.recordOpenAIStreamUpstreamError(c, &Account{ID: 417, Type: AccountTypeOAuth}, false, "", "stream_failed", []byte(`{"type":"response.failed"}`), "failed")
	require.Zero(t, len(client.Keys()))
}

func TestQuality5xxWebsocketSemanticFailure(t *testing.T) {
	for _, event := range []string{"error", "response.failed"} {
		t.Run(event, func(t *testing.T) {
			limiter, client := qualitySignalTestLimiter(t)
			svc := &OpenAIGatewayService{rateLimitService: limiter}
			account := &Account{ID: 417, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			payload := []byte(`{"type":"error","error":{"code":"invalid_prompt","message":"policy rejection"}}`)
			if event == "error" {
				svc.handleOpenAIWSErrorEventTransientFailure(context.Background(), account, "gpt-5.5", nil, payload)
			} else {
				svc.handleOpenAIWSFailureAccountSideEffects(context.Background(), account, "gpt-5.5", nil, payload)
			}
			require.Equal(t, []string{"417"}, qualityTestMembers(t, client))
		})
	}
}

func TestQuality5xxChoosesAnswerCheckWithoutChangingRule(t *testing.T) {
	plans, results := &qualityTriggerPlans{}, &pelicanResults{}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}
	plan := pelicanPlan()
	plan.TriggerSource = quality5xxSource
	plan.PelicanConfig = quality5xxFixtureConfig()
	plan.PelicanConfig.Quality.TriggerOnUpstream5xx = true
	original := plan.PelicanConfig
	runner := &ScheduledTestRunnerService{planRepo: plans, scheduledSvc: NewScheduledTestService(plans, results), accountTestSvc: &AccountTestService{accountRepo: &qualitySupportAccounts{account: account}}}
	calls := 0
	runner.judgeQuality = func(context.Context, int64, *PelicanTestConfig, string) *QualityJudgment {
		return &QualityJudgment{Verdict: "correct"}
	}
	runner.runPelican = func(_ context.Context, _ int64, _ string, cfg *PelicanTestConfig) (*ScheduledTestResult, error) {
		calls++
		require.Equal(t, "candy", cfg.QuestionKind)
		return &ScheduledTestResult{Status: "success", ResponseText: "21", PelicanConfig: cfg}, nil
	}
	require.True(t, runner.runOnePlan(context.Background(), plan))
	require.Equal(t, 1, calls)
	require.Len(t, results.results, 1)
	require.Equal(t, "success", results.results[0].Status)
	require.Equal(t, "candy", original.QuestionKind)
	require.Equal(t, "21", original.Quality.ExpectedAnswer)
	require.Equal(t, 1, plans.outcomes)
}

type qualityRecentPlanRepo struct {
	qualityTriggerPlans
	plan *ScheduledTestPlan
}

func (r *qualityRecentPlanRepo) ListByAccountID(context.Context, int64) ([]*ScheduledTestPlan, error) {
	return []*ScheduledTestPlan{r.plan}, nil
}

func TestQuality5xxDoesNotSkipRecentlyCompletedPlan(t *testing.T) {
	plan := pelicanPlan()
	recent, future := time.Now().Add(-time.Second), time.Now().Add(time.Hour)
	plan.LastRunAt, plan.NextRunAt = &recent, &future
	plan.PelicanConfig = quality5xxFixtureConfig()
	plan.PelicanConfig.Quality.TriggerOnUpstream5xx = true
	plans, results := &qualityRecentPlanRepo{plan: plan}, &pelicanResults{}
	runner := &ScheduledTestRunnerService{planRepo: plans, scheduledSvc: NewScheduledTestService(plans, results)}
	runner.runPelican = func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error) {
		return &ScheduledTestResult{Status: "success", QualityJudgment: &QualityJudgment{Verdict: "correct"}}, nil
	}
	require.NoError(t, runner.runQualityTriggeredAccount(context.Background(), plan.AccountID, time.Now()))
	require.Len(t, results.results, 1)
	require.True(t, future.Equal(plans.next), "event test must keep the future cron")
}

func TestQuality5xxCoalescesWithOverlappingCron(t *testing.T) {
	plan := pelicanPlan()
	plan.PelicanConfig = quality5xxFixtureConfig()
	plan.PelicanConfig.Quality.TriggerOnUpstream5xx = true
	observed, until := time.Now(), time.Now().Add(time.Minute)
	plan.RunningUntil = &until
	plans := &qualityRecentPlanRepo{plan: plan}
	runner := &ScheduledTestRunnerService{planRepo: plans}
	require.Error(t, runner.runQualityTriggeredAccount(context.Background(), plan.AccountID, observed))
	finished := observed.Add(time.Second)
	plan.LastRunAt, plan.RunningUntil = &finished, nil
	require.NoError(t, runner.runQualityTriggeredAccount(context.Background(), plan.AccountID, observed))
	require.False(t, plans.claimed, "cron completion must consume the old signal without a second test")
}

func qualityTestMembers(t *testing.T, server *miniredis.Miniredis) []string {
	t.Helper()
	members, err := server.ZMembers(quality5xxPendingKey)
	require.NoError(t, err)
	return members
}
