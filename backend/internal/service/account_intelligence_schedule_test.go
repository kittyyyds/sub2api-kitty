//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestAccountIntelligenceScheduleValidation(t *testing.T) {
	now := time.Date(2026, 11, 16, 11, 1, 0, 0, time.UTC)
	for _, tc := range []struct {
		cron  string
		valid bool
	}{
		{"*/30 * * * *", true}, {"*/5 * * * *", true}, {"0 0 * * *", true},
		{"*/30:每30分钟运行一次", false}, {"* * * * *", true}, {"0,2 * * * *", true},
		{"0/5 * * * *", true}, {"0 0 31 2 *", false}, {"* * 29 2 *", true},
		{"* * * * * *", false}, {"@every 30s", false}, {"*/0 * * * *", false},
	} {
		t.Run(tc.cron, func(t *testing.T) {
			p := defaultIntelligenceSchedule(1)
			p.Enabled = true
			p.Model = "gpt-6-astra"
			p.CronExpression = tc.cron
			err := validateIntelligenceSchedule(p, now, time.UTC)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, p.NextRunAt)
				require.True(t, p.NextRunAt.After(now))
				require.Equal(t, "UTC", p.Timezone)
			} else {
				require.Error(t, err)
			}
		})
	}
	p := defaultIntelligenceSchedule(1)
	p.Model = "gpt-6-astra"
	require.NoError(t, validateIntelligenceSchedule(p, now, time.UTC))
	require.Nil(t, p.NextRunAt)
	p.MaxResults = 0
	require.Error(t, validateIntelligenceSchedule(p, now, time.UTC))
}

type intelligenceScheduleStub struct {
	*manualIQRepo
	scheduleMu sync.Mutex
	plan       *AccountIntelligenceSchedule
	claims     int
}

func (r *intelligenceScheduleStub) IntelligenceScheduleGet(context.Context, int64) (*AccountIntelligenceSchedule, error) {
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	if r.plan == nil {
		return nil, nil
	}
	copy := *r.plan
	return &copy, nil
}
func (r *intelligenceScheduleStub) IntelligenceScheduleSave(_ context.Context, p *AccountIntelligenceSchedule, revision int64) (bool, error) {
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	if r.plan != nil && r.plan.Revision != revision {
		return false, nil
	}
	copy := *p
	r.plan = &copy
	return true, nil
}
func (r *intelligenceScheduleStub) IntelligenceSchedules(context.Context) ([]AccountIntelligenceSchedule, error) {
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	if r.plan == nil {
		return nil, nil
	}
	return []AccountIntelligenceSchedule{*r.plan}, nil
}
func (r *intelligenceScheduleStub) IntelligenceScheduleClaim(_ context.Context, p *AccountIntelligenceSchedule, now, next time.Time) (bool, error) {
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	if r.plan == nil || !r.plan.Enabled || r.plan.Revision != p.Revision || !r.plan.NextRunAt.Equal(*p.NextRunAt) {
		return false, nil
	}
	r.plan.NextRunAt = &next
	r.plan.LastAttemptAt = &now
	r.claims++
	return true, nil
}
func (*intelligenceScheduleStub) IntelligenceScheduleError(context.Context, *AccountIntelligenceSchedule, time.Time, string) error {
	return nil
}
func (*intelligenceScheduleStub) IntelligencePruneScheduled(context.Context, int64, int) error {
	return nil
}

func TestAccountIntelligenceScheduleSavesWithoutRunningAndDetectsConflict(t *testing.T) {
	r := &intelligenceScheduleStub{manualIQRepo: &manualIQRepo{a: &Account{ID: 27, Platform: PlatformOpenAI}, items: map[string]*AccountIntelligenceTest{}}}
	s := &AccountTestService{accountRepo: r}
	p, err := s.GetIntelligenceSchedule(context.Background(), 27)
	require.NoError(t, err)
	require.False(t, p.Enabled)
	require.Zero(t, p.Revision)
	p.Model = "gpt-6-astra"
	p.Enabled = true
	p.CronExpression = "* * * * *"
	saved, err := s.SaveIntelligenceSchedule(context.Background(), 27, *p)
	require.NoError(t, err)
	require.EqualValues(t, 1, saved.Revision)
	require.NotNil(t, saved.NextRunAt)
	require.Equal(t, "* * * * *", saved.CronExpression)
	require.Empty(t, r.items)
	_, err = s.SaveIntelligenceSchedule(context.Background(), 27, *p)
	require.Error(t, err)
	saved.Enabled = false
	saved, err = s.SaveIntelligenceSchedule(context.Background(), 27, *saved)
	require.NoError(t, err)
	require.Nil(t, saved.NextRunAt)
}

func TestAccountIntelligenceScheduleEveryMinuteConsecutiveTicks(t *testing.T) {
	now := time.Date(2026, 11, 16, 11, 1, 0, 0, time.UTC)
	due := now.Add(-time.Second)
	r := &intelligenceScheduleStub{manualIQRepo: &manualIQRepo{a: &Account{ID: 27, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"synthetic_ui_test": true}}, items: map[string]*AccountIntelligenceTest{}, done: make(chan *AccountIntelligenceTest, 2)}, plan: &AccountIntelligenceSchedule{AccountID: 27, Revision: 1, Enabled: true, Kind: "candy", Model: "gpt-6-astra", CronExpression: "* * * * *", Timezone: "UTC", MaxResults: 12, NextRunAt: &due}}
	runner := NewScheduledTestRunnerService(nil, nil, &AccountTestService{accountRepo: r}, r, nil, nil)
	defer runner.Stop()
	ids := map[string]bool{}
	for i := 0; i < 2; i++ {
		tick := now.Add(time.Duration(i) * time.Minute)
		runner.runAccountIntelligenceSchedules(context.Background(), tick)
		runner.runAccountIntelligenceSchedules(context.Background(), tick)
		select {
		case result := <-r.done:
			require.Equal(t, "scheduled", result.Source)
			require.False(t, ids[result.RequestID], "consecutive minutes must use distinct request IDs")
			ids[result.RequestID] = true
		case <-time.After(5 * time.Second):
			t.Fatal("one-minute scheduled test did not finish")
		}
		r.scheduleMu.Lock()
		claims, next := r.claims, *r.plan.NextRunAt
		r.scheduleMu.Unlock()
		require.Equal(t, i+1, claims, "a duplicate sweep must not create another occurrence")
		require.Equal(t, tick.Add(time.Minute), next)
	}
}

func TestAccountIntelligenceScheduleUsesManagedTickAndSharedHistory(t *testing.T) {
	now := time.Now()
	due := now.Add(-time.Minute)
	r := &intelligenceScheduleStub{manualIQRepo: &manualIQRepo{a: &Account{ID: 27, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"synthetic_ui_test": true}}, items: map[string]*AccountIntelligenceTest{}, done: make(chan *AccountIntelligenceTest, 2)}, plan: &AccountIntelligenceSchedule{AccountID: 27, Revision: 1, Enabled: true, Kind: "candy", Model: "gpt-6-astra", CronExpression: "*/30 * * * *", Timezone: "UTC", MaxResults: 12, NextRunAt: &due}}
	runner := NewScheduledTestRunnerService(nil, nil, &AccountTestService{accountRepo: r}, r, nil, nil)
	runner.runAccountIntelligenceSchedules(context.Background(), now)
	runner.runAccountIntelligenceSchedules(context.Background(), now)
	select {
	case result := <-r.done:
		require.Equal(t, "scheduled", result.Source)
		require.EqualValues(t, 27, result.AccountID)
		require.NotEmpty(t, result.Prompt)
		require.Equal(t, "21", result.ExpectedAnswer)
	case <-time.After(5 * time.Second):
		t.Fatal("automatic test did not finish")
	}
	runner.Stop()
	r.scheduleMu.Lock()
	require.Equal(t, 1, r.claims)
	r.plan.NextRunAt = &due
	r.scheduleMu.Unlock()
	runner.runAccountIntelligenceSchedules(context.Background(), now)
	r.scheduleMu.Lock()
	require.Equal(t, 1, r.claims)
	r.scheduleMu.Unlock()
}
