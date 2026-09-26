//go:build unit

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSchedulerFullAccountBatchRedis(t *testing.T) {
	ctx := context.Background()
	cache, mr := newSchedulerCacheUnitWithRedis(t)
	cache.mgetChunkSize = 3 // Split account/last-used pairs across MGET chunks.
	bucket := service.SchedulerBucket{GroupID: 2, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	accounts := []service.Account{
		{
			ID: 101, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
			Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{2},
			Credentials: map[string]any{"email": "fixture@example.invalid", "chatgpt_account_id": "synthetic", "plan_type": "pro"},
			Extra:       map[string]any{"private_fixture": map[string]any{"state": "synthetic-value", "identity": "synthetic-fingerprint"}},
		},
		{ID: 102, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
	}
	token, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, token, accounts))
	meta, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, meta, 2)
	for _, account := range meta {
		require.NotContains(t, account.Credentials, "email")
		require.NotContains(t, account.Credentials, "chatgpt_account_id")
		require.NotContains(t, account.Extra, "private_fixture")
	}
	lastUsed := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{101: lastUsed}))
	full, err := cache.GetAccounts(ctx, []int64{101, 102, 101, 999, -1})
	require.NoError(t, err)
	require.Len(t, full, 2)
	require.Equal(t, "synthetic", full[101].GetCredential("chatgpt_account_id"))
	require.Contains(t, full[101].Extra, "private_fixture")
	require.Equal(t, lastUsed, *full[101].LastUsedAt)
	// Warm the real Redis snapshot through the service, without touching the DB.
	snapshot := service.NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	loaded, err := snapshot.GetAccounts(ctx, []int64{101, 102})
	require.NoError(t, err)
	require.Len(t, loaded, 2)
	require.Contains(t, loaded[101].Extra, "private_fixture")

	// A corrupt full snapshot cannot silently turn into an empty account payload.
	mr.Set(schedulerAccountKey("101"), `{"ID":999}`)
	_, err = cache.GetAccounts(ctx, []int64{101})
	require.ErrorContains(t, err, "ID mismatch")
	mr.Set(schedulerAccountKey("101"), `{invalid`)
	_, err = cache.GetAccounts(ctx, []int64{101})
	require.Error(t, err)
}
