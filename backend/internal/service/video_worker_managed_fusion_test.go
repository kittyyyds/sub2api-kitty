//go:build unit

package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/stretchr/testify/require"
)

type fusionVideoScanRepo struct {
	*videoTaskMemoryRepo
	entered   chan struct{}
	resume    chan struct{}
	cancelled bool
}

func (r *fusionVideoScanRepo) LeaseVideoTasks(ctx context.Context, _ int) ([]*VideoTask, error) {
	close(r.entered)
	select {
	case <-r.resume:
	case <-ctx.Done():
		r.cancelled = true
	}
	return nil, ctx.Err()
}

func TestFusionVideoScanDrainsBeforeLeadershipRelease(t *testing.T) {
	role := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(role, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "video-fusion-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", role)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	t.Cleanup(func() { _ = os.Setenv("KITTY_MANAGED_INSTANCE", "false"); _ = managedinstance.ConfigureFromEnv() })
	managedinstance.ConfigureLeaseStore(fusionLease{})
	svc, repo, _, _, _ := newVideoTaskFixture()
	scan := &fusionVideoScanRepo{videoTaskMemoryRepo: repo, entered: make(chan struct{}), resume: make(chan struct{})}
	svc.usageLogRepo = scan
	var once sync.Once
	resume := func() { once.Do(func() { close(scan.resume) }) }
	t.Cleanup(resume)
	// Standby must not query or lease any pending tasks.
	svc.runVideoTaskCycle(t.Context(), nil)
	select {
	case <-scan.entered:
		t.Fatal("standby scanned tasks")
	default:
	}
	instance := managedinstance.Current()
	require.NoError(t, instance.SetRole("active"))
	instance.Step(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); svc.runVideoTaskCycle(t.Context(), nil) }()
	select {
	case <-scan.entered:
	case <-time.After(time.Second):
		t.Fatal("scan did not start")
	}
	require.Equal(t, int64(1), instance.Snapshot().BackgroundActive)
	instance.Drain()
	instance.Step(t.Context())
	require.True(t, instance.Snapshot().LeadershipOwned, "drain must wait for the admitted settlement cycle")
	resume()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scan did not stop")
	}
	require.False(t, scan.cancelled, "normal drain must not cancel settlement")
	require.Zero(t, instance.Snapshot().BackgroundActive)
}
