package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIQIntervalUsesCompletionDelayAndFixedQuestion(t *testing.T) {
	now := time.Now()
	p := &ScheduledTestPlan{ModelID: "gpt-6-astra", MaxResults: 99, AutoRecover: true, PelicanConfig: &PelicanTestConfig{ScheduleMode: IQCompletionInterval, IntervalSeconds: 17, QuestionKind: "pelican", Prompt: "ignored", ReasoningEffort: "medium", ParallelCount: 8}}
	next, err := nextPlanRun(p, now)
	require.NoError(t, err)
	require.Equal(t, now.Add(17*time.Second), next)
	require.Equal(t, 1, p.PelicanConfig.ParallelCount)
	require.Equal(t, CandyPrompt, p.PelicanConfig.Prompt)
	require.Equal(t, 20, p.MaxResults)
	require.False(t, p.AutoRecover)
	p.PelicanConfig.IntervalSeconds = 0
	_, err = nextPlanRun(p, now)
	require.Error(t, err)
}
func TestIQVerdictsKeepFailuresSeparate(t *testing.T) {
	require.Equal(t, "correct", IQAnswerVerdict("success", "21"))
	require.Equal(t, "other", IQAnswerVerdict("success", "29"))
	require.Equal(t, "other", IQAnswerVerdict("success", "32"))
	require.Equal(t, "error", IQAnswerVerdict("failed", "21"))
	require.Equal(t, "other", IQAnswerVerdict("success", "21 or 29"))
}

func TestIQIntervalPersistsVerdictAfterRunnerSnapshot(t *testing.T) {
	for _, answer := range []string{"21", "29", "32"} {
		plans := &pelicanPlanRepo{}
		results := &pelicanResults{}
		runner := &ScheduledTestRunnerService{planRepo: plans, scheduledSvc: NewScheduledTestService(plans, results)}
		runner.runPelican = func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error) {
			return &ScheduledTestResult{Status: "success", ResponseText: answer}, nil
		}
		p := pelicanPlan()
		p.PelicanConfig.ScheduleMode = IQCompletionInterval
		p.PelicanConfig.IntervalSeconds = 60
		runner.runOnePlan(context.Background(), p)
		require.Len(t, results.results, 1)
		require.Equal(t, 20, results.pruned)
		require.Equal(t, IQAnswerVerdict("success", answer), results.results[0].PelicanConfig.IQVerdict)
		require.Equal(t, p.ModelID, results.results[0].PelicanConfig.ModelID)
	}
}

func TestIQAnswerMismatchIsCompletedAnswer(t *testing.T) {
	for _, answer := range []string{"29", "32", "21 or 29"} {
		message := intelligenceTestOutputError(&PelicanTestConfig{QuestionKind: "candy"}, answer)
		require.Equal(t, "answer_mismatch: expected 21", message)
		require.Equal(t, "other", IQAnswerVerdict("failed", answer, message))
	}
	require.Equal(t, "error", IQAnswerVerdict("failed", "29", "API returned 403"))
	require.Equal(t, "error", IQAnswerVerdict("failed", "", "answer_mismatch: expected 21"))
	require.Equal(t, "error", IQAnswerVerdict("failed", "21", "timeout"))
}

func TestIQIntervalPersistsRealMismatchStatus(t *testing.T) {
	plans := &pelicanPlanRepo{}
	results := &pelicanResults{}
	runner := &ScheduledTestRunnerService{planRepo: plans, scheduledSvc: NewScheduledTestService(plans, results)}
	runner.runPelican = func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error) {
		return &ScheduledTestResult{Status: "failed", ResponseText: "29", ErrorMessage: intelligenceTestOutputError(&PelicanTestConfig{QuestionKind: "candy"}, "29")}, nil
	}
	p := pelicanPlan()
	p.PelicanConfig.ScheduleMode = IQCompletionInterval
	p.PelicanConfig.IntervalSeconds = 60
	runner.runOnePlan(context.Background(), p)
	require.Len(t, results.results, 1)
	require.Equal(t, "other", results.results[0].PelicanConfig.IQVerdict)
	require.Equal(t, "29", results.results[0].ResponseText)
}
