package service

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/stretchr/testify/require"
)

type fusionLease struct{}

func (fusionLease) Acquire(context.Context, string, time.Duration) (bool, error) { return true, nil }
func (fusionLease) Renew(context.Context, string, time.Duration) (bool, error)   { return true, nil }
func (fusionLease) Release(context.Context, string) error                        { return nil }

type fusionPeriodicJob struct{ starts, stops atomic.Int32 }

func (j *fusionPeriodicJob) Start() { j.starts.Add(1) }
func (j *fusionPeriodicJob) Stop()  { j.stops.Add(1) }

func TestFusionBackgroundRequiresFullRoleAndManagedLease(t *testing.T) {
	role := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(role, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "fusion-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", role)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	t.Cleanup(func() { _ = os.Setenv("KITTY_MANAGED_INSTANCE", "false"); _ = managedinstance.ConfigureFromEnv() })
	managedinstance.ConfigureLeaseStore(fusionLease{})
	full, gateway := &fusionPeriodicJob{}, &fusionPeriodicJob{}
	startBackgroundService(&config.Config{}, full)
	startBackgroundService(&config.Config{Runtime: config.RuntimeConfig{Role: config.RuntimeRoleGateway}}, gateway)
	instance := managedinstance.Current()
	instance.Step(t.Context())
	require.Zero(t, full.starts.Load())
	require.NoError(t, instance.SetRole("active"))
	instance.Step(t.Context())
	require.Equal(t, int32(1), full.starts.Load())
	require.Zero(t, gateway.starts.Load())
	instance.Drain()
	instance.Step(t.Context())
	require.Eventually(t, func() bool { return full.stops.Load() == 1 }, time.Second, time.Millisecond)
	require.Zero(t, gateway.stops.Load())
}
