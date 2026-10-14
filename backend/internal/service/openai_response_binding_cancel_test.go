package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type responseBindingTraceKey struct{}

// Model a remote cache that honors cancellation, unlike the general cache stub.
// A new state store below forces reads through it instead of the writer's memory.
type responseBindingCancelCache struct {
	GatewayCache
	t             *testing.T
	bindings      map[string]int64
	cancelParent  context.CancelFunc
	blockWrites   bool
	writes        int
	traceExpected string
	canceledReads int
}

func (c *responseBindingCancelCache) SetSessionAccountID(ctx context.Context, group int64, key string, id int64, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	require.True(c.t, ok, "persistence must have a finite timeout")
	require.Positive(c.t, time.Until(deadline))
	require.LessOrEqual(c.t, time.Until(deadline), openAIWSStateStoreRedisTimeout)
	if c.traceExpected != "" {
		require.Equal(c.t, c.traceExpected, ctx.Value(responseBindingTraceKey{}))
	}
	if c.cancelParent != nil {
		c.cancelParent()
		c.cancelParent = nil
	}
	if c.blockWrites {
		<-ctx.Done()
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.bindings[fmt.Sprintf("%d:%s", group, key)] = id
	c.writes++
	return nil
}

func (c *responseBindingCancelCache) GetSessionAccountID(ctx context.Context, group int64, key string) (int64, error) {
	if err := ctx.Err(); err != nil {
		c.canceledReads++
		return 0, err
	}
	return c.bindings[fmt.Sprintf("%d:%s", group, key)], nil
}

func TestOpenAIWSStateStore_ResponseBindingsSurviveRequestCancellation(t *testing.T) {
	for _, mode := range []string{"already_canceled", "deadline_expired", "canceled_during_write"} {
		t.Run(mode, func(t *testing.T) {
			parent := context.WithValue(context.Background(), responseBindingTraceKey{}, "trace-for-binding")
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			cache := &responseBindingCancelCache{t: t, bindings: map[string]int64{}, traceExpected: "trace-for-binding"}
			switch mode {
			case "already_canceled":
				cancel()
			case "deadline_expired":
				var expireCancel context.CancelFunc
				ctx, expireCancel = context.WithDeadline(parent, time.Now().Add(-time.Second))
				defer expireCancel()
			case "canceled_during_write":
				cache.cancelParent = cancel
			}
			writer := NewOpenAIWSStateStore(cache)
			require.NoError(t, writer.BindResponseAccount(ctx, 15, "resp_completed", 111, time.Minute))
			require.NoError(t, writer.BindHTTPResponseOwner(ctx, 15, "resp_completed", 1, 4, time.Minute))
			require.Error(t, ctx.Err(), "the original request must remain canceled")
			require.Equal(t, 3, cache.writes)

			reader := NewOpenAIWSStateStore(cache)
			accountID, err := reader.GetResponseAccount(context.Background(), 15, "resp_completed")
			require.NoError(t, err)
			require.Equal(t, int64(111), accountID)
			userID, keyID, found, err := reader.GetHTTPResponseOwner(context.Background(), 15, "resp_completed")
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, int64(1), userID)
			require.Equal(t, int64(4), keyID)
			accountID, err = reader.GetResponseAccount(context.Background(), 16, "resp_completed")
			require.NoError(t, err)
			require.Zero(t, accountID)
			_, _, found, err = reader.GetHTTPResponseOwner(context.Background(), 16, "resp_completed")
			require.NoError(t, err)
			require.False(t, found, "ownership remains isolated by group")
		})
	}
}

func TestOpenAIWSStateStore_CanceledRequestPersistenceStillTimesOut(t *testing.T) {
	for _, owner := range []bool{false, true} {
		t.Run(fmt.Sprintf("owner_%t", owner), func(t *testing.T) {
			t.Parallel()
			cache := &responseBindingCancelCache{t: t, blockWrites: true}
			store := NewOpenAIWSStateStore(cache)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			started := time.Now()
			var err error
			if owner {
				err = store.BindHTTPResponseOwner(ctx, 15, "resp_timeout", 1, 4, time.Minute)
			} else {
				err = store.BindResponseAccount(ctx, 15, "resp_timeout", 111, time.Minute)
			}
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Less(t, time.Since(started), openAIWSStateStoreRedisTimeout+2*time.Second)
		})
	}
}

func TestOpenAIWSStateStore_ResponseBindingReadsStillHonorCancellation(t *testing.T) {
	cache := &responseBindingCancelCache{t: t}
	store := NewOpenAIWSStateStore(cache)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	accountID, err := store.GetResponseAccount(ctx, 15, "resp_not_cached")
	require.NoError(t, err) // Existing account lookup treats cache errors as a miss.
	require.Zero(t, accountID)
	_, _, found, err := store.GetHTTPResponseOwner(ctx, 15, "resp_not_cached")
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, found)
	require.Equal(t, 2, cache.canceledReads)
}
