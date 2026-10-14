package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func enableManagedTest(t *testing.T) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(p, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "unit-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", p)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	t.Cleanup(func() { _ = os.Setenv("KITTY_MANAGED_INSTANCE", "false"); _ = managedinstance.ConfigureFromEnv() })
}
func TestManagedStartupPreservesPeerSlotsAndLongStreamsRenew(t *testing.T) {
	enableManagedTest(t)
	m := miniredis.RunT(t)
	r := redis.NewClient(&redis.Options{Addr: m.Addr()})
	defer r.Close()
	ctx := context.Background()
	a := &concurrencyCache{rdb: r, slotTTLSeconds: 10, waitQueueTTLSeconds: 10, managedSlots: map[managedSlot]struct{}{}, managedHealthy: true}
	b := &concurrencyCache{rdb: r, slotTTLSeconds: 10, waitQueueTTLSeconds: 10, managedSlots: map[managedSlot]struct{}{}, managedHealthy: true}
	now := time.Now()
	m.SetTime(now)
	ok, err := a.AcquireAccountSlot(ctx, 1, 1, "old-owner-1")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = a.AcquireUserSlot(ctx, 2, 1, "old-owner-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, b.CleanupStaleProcessSlots(ctx, "new-owner"))
	ok, err = b.AcquireAccountSlot(ctx, 1, 1, "new-owner-1")
	require.NoError(t, err)
	require.False(t, ok, "peer startup must not clear a live slot")
	for i := 0; i < 8; i++ {
		now = now.Add(4 * time.Second)
		m.SetTime(now)
		m.FastForward(4 * time.Second)
		a.renewManagedSlots(ctx)
		ok, err = b.AcquireAccountSlot(ctx, 1, 1, "new-owner-1")
		require.NoError(t, err)
		require.False(t, ok, "a stream exceeding initial TTL still holds concurrency")
	}
	require.NoError(t, a.ReleaseAccountSlot(ctx, 1, "old-owner-1"))
	a.renewManagedSlots(ctx)
	ok, err = b.AcquireAccountSlot(ctx, 1, 1, "new-owner-1")
	require.NoError(t, err)
	require.True(t, ok)
	// A crashed owner stops renewing; expiry reclaims capacity without touching healthy peers.
	now = now.Add(11 * time.Second)
	m.SetTime(now)
	m.FastForward(11 * time.Second)
	ok, err = b.AcquireUserSlot(ctx, 2, 1, "new-owner-2")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestManagedRenewNeverResurrectsReleasedMember(t *testing.T) {
	m := miniredis.RunT(t)
	r := redis.NewClient(&redis.Options{Addr: m.Addr()})
	defer r.Close()
	ctx := context.Background()
	require.NoError(t, r.ZAdd(ctx, "concurrency:account:1", redis.Z{Score: float64(time.Now().Unix()), Member: "other"}).Err())
	n, err := renewManagedSlot.Run(ctx, r, []string{"concurrency:account:1"}, "released", 60).Int()
	require.NoError(t, err)
	require.Zero(t, n)
	require.Equal(t, []string{"other"}, r.ZRange(ctx, "concurrency:account:1", 0, -1).Val())
}

func TestManagedLeaderLeaseOwnerProtectedRelease(t *testing.T) {
	m := miniredis.RunT(t)
	r := redis.NewClient(&redis.Options{Addr: m.Addr()})
	defer r.Close()
	s := managedLeaseStore{r}
	ctx := context.Background()
	ok, err := s.Acquire(ctx, "old", time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = s.Acquire(ctx, "new", time.Second)
	require.NoError(t, err)
	require.False(t, ok)
	m.FastForward(2 * time.Second)
	ok, err = s.Acquire(ctx, "new", time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, s.Release(ctx, "old"))
	require.Equal(t, "new", r.Get(ctx, managedLeaderKey).Val())
	ok, err = s.Renew(ctx, "old", time.Second)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, s.Release(ctx, "new"))
	require.Zero(t, r.Exists(ctx, managedLeaderKey).Val())
}
