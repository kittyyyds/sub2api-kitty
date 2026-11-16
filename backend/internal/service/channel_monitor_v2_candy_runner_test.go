package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type candyJobFunc func(context.Context, time.Time)

func (f candyJobFunc) RunDue(ctx context.Context, now time.Time) { f(ctx, now) }

func waitCandyRunner(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal(description)
	}
}

func TestChannelMonitorV2CandyRunnerTicksWithoutPageRequests(t *testing.T) {
	calls := make(chan struct{}, 10)
	r := newChannelMonitorV2CandyRunner(candyJobFunc(func(context.Context, time.Time) { calls <- struct{}{} }))
	r.interval = 10 * time.Millisecond
	t.Cleanup(r.Stop)
	r.Start()
	r.Start()
	waitCandyRunner(t, calls, "initial background check did not run")
	waitCandyRunner(t, calls, "periodic background check did not run")
	r.Stop()
	waitCandyRunner(t, r.done, "runner did not exit")
}

func TestChannelMonitorV2CandyRunnerCancelsInFlightAndDoesNotOverlap(t *testing.T) {
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	r := newChannelMonitorV2CandyRunner(candyJobFunc(func(ctx context.Context, _ time.Time) {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done()
	}))
	r.interval = time.Millisecond
	t.Cleanup(r.Stop)
	r.Start()
	waitCandyRunner(t, started, "check did not start")
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, int32(1), calls.Load(), "slow checks must not overlap")
	r.Stop()
	r.Stop()
	waitCandyRunner(t, r.done, "in-flight request was not cancelled")
	require.Equal(t, int32(1), calls.Load())
}

func TestChannelMonitorV2CandyRunnerStoppedBeforeStart(t *testing.T) {
	var calls atomic.Int32
	r := newChannelMonitorV2CandyRunner(candyJobFunc(func(context.Context, time.Time) { calls.Add(1) }))
	r.Stop()
	r.Start()
	waitCandyRunner(t, r.done, "stopped runner did not exit")
	require.Zero(t, calls.Load())
}

func TestProvideChannelMonitorV2CandyRunnerStartsConfiguredProbe(t *testing.T) {
	t.Setenv("KITTY_DEPLOYMENT_HANDOFF", "false")
	repo := &candyRepoFake{channelMonitorV2RepoStub: channelMonitorV2RepoStub{config: candyConfigFixture()}}
	groups, _ := newGroupTestService(newGroupTestRepoFake(), &groupTestRouterFake{steps: []routeStep{{account: account(77, "probe")}}}, func(context.Context, int64, string, *MonitorProbeConfig) (*ScheduledTestResult, error) {
		return &ScheduledTestResult{Status: "success", ResponseText: "21"}, nil
	})
	svc := &ChannelMonitorV2Service{candy: candyServiceFixture(repo, groups)}
	r := ProvideChannelMonitorV2CandyRunner(svc, &config.Config{})
	require.NotNil(t, r)
	t.Cleanup(r.Stop)
	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.results) == 1 && repo.results[0].Verdict == "correct"
	}, 3*time.Second, 10*time.Millisecond)
	r.Stop()
	waitCandyRunner(t, r.done, "provider-owned runner did not stop")
}

func TestProvideChannelMonitorV2CandyRunnerHonorsDeploymentAndGatewayGates(t *testing.T) {
	for _, tc := range []struct{ name, handoff, role string }{
		{"handoff", "true", config.RuntimeRoleFull},
		{"gateway", "false", config.RuntimeRoleGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KITTY_DEPLOYMENT_HANDOFF", tc.handoff)
			// An uninitialized service would panic if a gated provider ran it.
			r := ProvideChannelMonitorV2CandyRunner(&ChannelMonitorV2Service{candy: &ChannelMonitorV2CandyService{}}, &config.Config{Runtime: config.RuntimeConfig{Role: tc.role}})
			require.NotNil(t, r)
			select {
			case <-r.done:
				t.Fatal("gated runner unexpectedly ran")
			case <-time.After(20 * time.Millisecond):
			}
			r.Stop()
		})
	}
}
