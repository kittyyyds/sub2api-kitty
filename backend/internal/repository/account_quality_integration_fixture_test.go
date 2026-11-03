//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type qualityIntegrationFixture struct {
	t       *testing.T
	plans   service.ScheduledTestPlanRepository
	account int64
	plan    *service.ScheduledTestPlan
	until   time.Time
}

func newQualityIntegrationFixture(t *testing.T, extra string, policy *service.QualityPolicy) *qualityIntegrationFixture {
	t.Helper()
	ctx := context.Background()
	f := &qualityIntegrationFixture{t: t, plans: NewScheduledTestPlanRepository(integrationDB)}
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type,status,schedulable,extra) VALUES('quality-integration','openai','oauth','active',true,$1::jsonb) RETURNING id`, extra).Scan(&f.account))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM scheduler_outbox WHERE account_id=$1`, f.account)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM scheduled_test_plans WHERE account_id=$1`, f.account)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id=$1`, f.account)
	})
	svc := service.NewScheduledTestService(f.plans, NewScheduledTestResultRepository(integrationDB))
	quality := &service.QualityPolicy{Action: service.QualityActionObserveOnly}
	if policy != nil {
		copy := *policy
		quality = &copy
	}
	quality.ExpectedAnswer = "21"
	quality.Judge = &service.QualityJudgeConfig{GroupID: 1, ModelID: "judge", Prompt: "grade"}
	plan, err := svc.CreatePlan(ctx, &service.ScheduledTestPlan{
		AccountID: f.account, ModelID: "gpt-6-astra", CronExpression: "*/30 * * * *", Enabled: true, MaxResults: 100,
		PelicanConfig: &service.PelicanTestConfig{QuestionKind: "candy", Prompt: service.CandyPrompt, ReasoningEffort: "medium", ParallelCount: 1, Quality: quality},
	})
	require.NoError(t, err)
	f.plan = plan
	f.claim()
	return f
}

func (f *qualityIntegrationFixture) claim() {
	f.t.Helper()
	ctx := context.Background()
	require.NoError(f.t, f.plans.TriggerQuality(ctx, f.plan.ID))
	plan, err := f.plans.GetByID(ctx, f.plan.ID)
	require.NoError(f.t, err)
	now := time.Now().Add(time.Second).Truncate(time.Microsecond)
	f.until = now.Add(15 * time.Minute)
	ok, err := f.plans.ClaimPelican(ctx, plan, now, f.until, now.Add(30*time.Minute))
	require.NoError(f.t, err)
	require.True(f.t, ok)
	f.plan = plan
}

func (f *qualityIntegrationFixture) apply(outcome string) string {
	f.t.Helper()
	got, err := f.plans.ApplyQualityOutcome(context.Background(), f.plan, f.until, outcome)
	require.NoError(f.t, err)
	return got
}

func (f *qualityIntegrationFixture) exec(query string, args ...any) {
	f.t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), query, append([]any{f.account}, args...)...)
	require.NoError(f.t, err)
}

func (f *qualityIntegrationFixture) extra() map[string]any {
	f.t.Helper()
	var raw []byte
	require.NoError(f.t, integrationDB.QueryRowContext(context.Background(), `SELECT extra FROM accounts WHERE id=$1`, f.account).Scan(&raw))
	extra := map[string]any{}
	require.NoError(f.t, json.Unmarshal(raw, &extra))
	return extra
}

func (f *qualityIntegrationFixture) count(query string) int {
	f.t.Helper()
	var n int
	require.NoError(f.t, integrationDB.QueryRowContext(context.Background(), query, f.account).Scan(&n))
	return n
}

func (f *qualityIntegrationFixture) events() int {
	f.t.Helper()
	n := f.count(`SELECT count(*) FROM scheduler_outbox WHERE account_id=$1 AND event_type='account_changed'`)
	f.exec(`DELETE FROM scheduler_outbox WHERE account_id=$1`)
	return n
}
