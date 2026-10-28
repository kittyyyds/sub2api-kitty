//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type kittyRecoveryLease struct{}

func (*kittyRecoveryLease) Acquire(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (*kittyRecoveryLease) Renew(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (*kittyRecoveryLease) Release(context.Context, string) error { return nil }

type kittyRecoveryRepo struct {
	UsageBillingRepository
	calls   atomic.Int64
	entered chan struct{}
	release chan struct{}
}

func (*kittyRecoveryRepo) StageUsageBilling(context.Context, *UsageBillingCommand, *UsageLog) error {
	return nil
}
func (r *kittyRecoveryRepo) PendingUsageBilling(ctx context.Context, _ int) ([]UsageBillingEventKey, error) {
	r.calls.Add(1)
	if r.entered != nil {
		select {
		case r.entered <- struct{}{}:
		default:
		}
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, nil
}
func (*kittyRecoveryRepo) ApplyStagedUsageBilling(context.Context, UsageBillingEventKey) (*UsageBillingRecovery, error) {
	return nil, nil
}
func (*kittyRecoveryRepo) AcknowledgeUsageBillingCache(context.Context, UsageBillingEventKey) error {
	return nil
}

func TestKittyBillingRecoveryStandbyAndNaturalDrain(t *testing.T) {
	role := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(role, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "billing-recovery-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", role)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	t.Cleanup(func() {
		t.Setenv("KITTY_MANAGED_INSTANCE", "false")
		require.NoError(t, managedinstance.ConfigureFromEnv())
	})
	managedinstance.ConfigureLeaseStore(&kittyRecoveryLease{})
	c := managedinstance.Current()
	repo := &kittyRecoveryRepo{entered: make(chan struct{}, 1), release: make(chan struct{})}
	p := ProvideUsageRecordWorkerPool(nil, repo, nil, nil)
	defer p.Stop()
	c.MarkReady()
	c.Step(t.Context())
	require.Zero(t, repo.calls.Load(), "standby must not replay financial events")
	require.NoError(t, c.SetRole("active"))
	c.Step(t.Context())
	select {
	case <-repo.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery did not start")
	}
	require.EqualValues(t, 1, c.Snapshot().BackgroundActive)
	c.Drain()
	c.Step(t.Context())
	require.False(t, c.Snapshot().LeadershipReleased, "admitted accounting cycle must finish naturally")
	close(repo.release)
	require.Eventually(t, func() bool { return c.Snapshot().BackgroundActive == 0 }, time.Second, time.Millisecond)
	require.Eventually(t, func() bool { c.Step(t.Context()); return c.Snapshot().LeadershipReleased }, time.Second, time.Millisecond)
	calls := repo.calls.Load()
	p.billingRecovery.Start()
	require.Equal(t, calls, repo.calls.Load(), "terminal drain may not restart recovery")
	require.NotNil(t, p.pool)
	require.False(t, p.pool.Stopped(), "drain must retain request-owned usage workers")
}

func TestKittyBillingRecoveryStopBeforeStartAndConcurrentStop(t *testing.T) {
	for i := 0; i < 20; i++ {
		repo := &kittyRecoveryRepo{}
		w := &usageBillingRecoveryWorker{repo: repo}
		if i == 0 {
			w.Stop()
			w.Start()
			require.Zero(t, repo.calls.Load())
			continue
		}
		var g sync.WaitGroup
		g.Add(2)
		go func() { defer g.Done(); w.Start() }()
		go func() { defer g.Done(); w.Stop() }()
		g.Wait()
		w.Stop()
		w.mu.Lock()
		require.True(t, w.stopped)
		w.mu.Unlock()
	}
}
