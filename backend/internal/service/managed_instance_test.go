package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/stretchr/testify/require"
)

func TestManagedStandbyDoesNotClaimOrProbePaidJobs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(p, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "paid-worker-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", p)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	defer func() { _ = os.Setenv("KITTY_MANAGED_INSTANCE", "false"); _ = managedinstance.ConfigureFromEnv() }()
	// Deliberately absent repositories/clients would panic if any of these
	// cycles reached a claim or model call while the candidate is standby.
	(&ScheduledTestRunnerService{}).runScheduled()
	(&TokenRefreshService{}).processRefreshContext(context.Background())
	(&ChannelMonitorRunner{}).fire(context.Background(), &scheduledMonitor{id: 1})
	require.Zero(t, managedinstance.Current().Snapshot().BackgroundActive)
}
