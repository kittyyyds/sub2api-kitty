package cyberevidence

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

func TestKittyDrainWaitsForQueuedEvidencePersistence(t *testing.T) {
	role := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(role, []byte("standby\n"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "evidence-drain-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", role)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	defer func() {
		_ = os.Setenv("KITTY_MANAGED_INSTANCE", "false")
		_ = managedinstance.ConfigureFromEnv()
	}()
	managedinstance.ConfigureLeaseStore(drainTestLease{})
	c := managedinstance.Current()
	c.MarkReady()
	m := &memoryStore{config: DefaultConfig(), gate: make(chan struct{}), started: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(m.gate) }) }
	s := New(m, testCipher{}, true)
	defer s.Close()
	defer unblock()
	s.Enqueue(1, s.Prepare([]byte(`{"input":"synthetic test request"}`), "test reason"))
	<-m.started
	c.Drain()
	c.Step(context.Background())
	require.EqualValues(t, 1, c.Snapshot().BackgroundActive)
	require.False(t, c.Snapshot().RetireReady, "container must remain alive until queued evidence has persisted")
	unblock()
	eventually(t, func() bool {
		c.Step(context.Background())
		return c.Snapshot().RetireReady
	})
	_, status := m.state()
	require.Equal(t, "saved", status)
}

// No active leader exists in this standby-to-draining fixture.
type drainTestLease struct{}

func (drainTestLease) Acquire(context.Context, string, time.Duration) (bool, error) { return true, nil }
func (drainTestLease) Renew(context.Context, string, time.Duration) (bool, error)   { return true, nil }
func (drainTestLease) Release(context.Context, string) error                        { return nil }
