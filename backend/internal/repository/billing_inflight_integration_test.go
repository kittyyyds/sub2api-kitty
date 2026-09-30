//go:build integration

package repository

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFusionInflightRealRedisAtomicityAndVideoLifetime(t *testing.T) {
	cache := &billingCache{rdb: testRedis(t)}
	ctx := t.Context()
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ok, _, err := cache.ReserveInflightBalance(ctx, 71, strconv.Itoa(id), 1, 5, time.Minute)
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			if ok {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, int32(5), allowed.Load(), "atomic reservations must admit only the available concurrent budget")
	ok, _, err := cache.ReserveInflightBalance(ctx, 72, "video", 1.3, 10, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	alive, err := cache.RenewInflightBalance(ctx, 72, "video", 7*24*time.Hour)
	require.NoError(t, err)
	require.True(t, alive)
	ok, _, err = cache.ReserveInflightBalance(ctx, 72, "text", .1, 10, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	zkey, hkey := billingInflightKeys(72)
	for _, key := range []string{zkey, hkey} {
		ttl, err := cache.rdb.PTTL(ctx, key).Result()
		require.NoError(t, err)
		require.Greater(t, ttl, 6*24*time.Hour)
	}
	require.NoError(t, cache.ReleaseInflightBalance(ctx, 72, "video"))
	require.NoError(t, cache.ReleaseInflightBalance(ctx, 72, "video"))
	n, err := cache.rdb.ZCard(ctx, zkey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "idempotent release must leave the other request intact")
}
