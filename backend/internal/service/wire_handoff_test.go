package service

import (
	"context"
	"testing"
	"time"
)

type handoffMonitorRepository struct {
	ChannelMonitorRepository
	loads int
}

func (r *handoffMonitorRepository) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	r.loads++
	return nil, nil
}

type handoffScheduledPlanRepository struct {
	ScheduledTestPlanRepository
}

func (*handoffScheduledPlanRepository) ListDue(context.Context, time.Time) ([]*ScheduledTestPlan, error) {
	return nil, nil
}

// Exercise the real providers: startup must neither load monitor plans nor
// register paid scheduled tests in the temporary traffic-serving process.
func TestDeploymentHandoffProviderStartup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		started bool
	}{
		{name: "default", started: true},
		{name: "explicit_false", value: "false", started: true},
		{name: "temporary_handoff", value: "true", started: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KITTY_DEPLOYMENT_HANDOFF", tc.value)
			repo := &handoffMonitorRepository{}
			svc := NewChannelMonitorService(repo, nil)
			monitorRunner := ProvideChannelMonitorRunner(svc, nil, nil, nil)
			t.Cleanup(monitorRunner.Stop)
			planRepo := &handoffScheduledPlanRepository{}
			testRunner := ProvideScheduledTestRunnerService(planRepo, nil, nil, nil, nil, nil, nil, &ChannelMonitorV2Service{})
			t.Cleanup(testRunner.Stop)

			if tc.started {
				if repo.loads != 1 {
					t.Fatalf("normal startup must load monitor plans once; got %d", repo.loads)
				}
				if testRunner.cron == nil || len(testRunner.cron.Entries()) != 1 {
					t.Fatal("normal startup must register the scheduled test cron")
				}
				return
			}
			if repo.loads != 0 {
				t.Fatalf("handoff must not load monitor plans; got %d loads", repo.loads)
			}
			if testRunner.cron != nil {
				t.Fatal("handoff must not register the scheduled test cron")
			}
			// CRUD callbacks must not activate a skipped monitor runner either.
			svc.scheduler.Schedule(&ChannelMonitor{ID: 1, Enabled: true, IntervalSeconds: 60})
			monitorRunner.mu.Lock()
			active := monitorRunner.started || len(monitorRunner.tasks) != 0
			monitorRunner.mu.Unlock()
			if active {
				t.Fatal("monitor CRUD must not start paid probes during handoff")
			}
		})
	}
}
