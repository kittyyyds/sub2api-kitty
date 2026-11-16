package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type deliveryEmptySettings struct {
	service.SettingRepository
	reads atomic.Int32
}

func (s *deliveryEmptySettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	s.reads.Add(1)
	return map[string]string{}, nil
}

func TestEmailDelivery_MissingCodesAreDistinctFromCacheFailure(t *testing.T) {
	cache, mr, rdb := newMiniredisEmailCache(t)
	ctx := context.Background()
	_, err := cache.GetVerificationCode(ctx, "new@example.com")
	require.ErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
	require.ErrorIs(t, err, redis.Nil)
	_, err = cache.GetNotifyVerifyCode(ctx, "new@example.com")
	require.ErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
	_, err = cache.GetPasswordResetToken(ctx, "new@example.com")
	require.ErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
	require.NoError(t, mr.Set(verifyCodeKey("broken@example.com"), "not-json"))
	_, err = cache.GetVerificationCode(ctx, "broken@example.com")
	require.Error(t, err)
	require.NotErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
	require.NoError(t, rdb.Close())
	_, err = cache.GetNotifyVerifyCode(ctx, "new@example.com")
	require.Error(t, err)
	require.NotErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
}

func TestEmailDelivery_FailureClearsOrdinaryAndNotificationCodes(t *testing.T) {
	cache, _, _ := newMiniredisEmailCache(t)
	settings := &deliveryEmptySettings{}
	sender := service.NewEmailService(settings, cache)
	users := service.NewUserService(nil, nil, nil, nil)
	ctx := context.Background()
	// No network is used. Missing SMTP config fails after a code is stored.
	for i := 0; i < 2; i++ {
		err := sender.SendVerifyCode(ctx, "ordinary@example.com", "Kitty")
		require.ErrorIs(t, err, service.ErrEmailNotConfigured)
		_, err = cache.GetVerificationCode(ctx, "ordinary@example.com")
		require.ErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
		err = users.SendNotifyEmailCode(ctx, 42, "notify@example.com", sender, cache)
		require.ErrorIs(t, err, service.ErrEmailNotConfigured)
		_, err = cache.GetNotifyVerifyCode(ctx, "notify@example.com")
		require.ErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
	}
	require.Equal(t, int32(4), settings.reads.Load())
}

func TestEmailDelivery_CacheFailureStopsBeforeSending(t *testing.T) {
	cache, _, rdb := newMiniredisEmailCache(t)
	settings := &deliveryEmptySettings{}
	sender := service.NewEmailService(settings, cache)
	users := service.NewUserService(nil, nil, nil, nil)
	require.NoError(t, rdb.Close())
	require.Error(t, sender.SendVerifyCode(context.Background(), "ordinary@example.com", "Kitty"))
	require.Error(t, users.SendNotifyEmailCode(context.Background(), 1, "notify@example.com", sender, cache))
	require.Zero(t, settings.reads.Load(), "cache failure must not reach SMTP configuration or transport")
}

func TestEmailDelivery_LateFailurePreservesNewerCodeAndAttempts(t *testing.T) {
	cache, _, _ := newMiniredisEmailCache(t)
	ctx := context.Background()
	concrete := cache.(*emailCache)
	for _, notify := range []bool{false, true} {
		set, get, incr, clear := cache.SetVerificationCode, cache.GetVerificationCode, cache.IncrVerificationCodeAttempts, concrete.DeleteUndeliveredVerificationCode
		if notify {
			set, get, incr, clear = cache.SetNotifyVerifyCode, cache.GetNotifyVerifyCode, cache.IncrNotifyVerifyCodeAttempts, concrete.DeleteUndeliveredNotifyCode
		}
		require.NoError(t, set(ctx, "race@example.com", &service.VerificationCodeData{Code: "new-code"}, time.Minute))
		_, err := incr(ctx, "race@example.com")
		require.NoError(t, err)
		require.NoError(t, clear(ctx, "race@example.com", "old-code"))
		data, err := get(ctx, "race@example.com")
		require.NoError(t, err)
		require.Equal(t, "new-code", data.Code)
		require.Equal(t, 1, data.Attempts)
		require.NoError(t, clear(ctx, "race@example.com", "new-code"))
		_, err = get(ctx, "race@example.com")
		require.ErrorIs(t, err, service.ErrEmailCacheEntryNotFound)
	}
}

func TestEmailDelivery_ConcurrentUserRequestsReserveBeforeSMTP(t *testing.T) {
	cache, _, _ := newMiniredisEmailCache(t)
	settings := &deliveryEmptySettings{}
	sender := service.NewEmailService(settings, cache)
	users := service.NewUserService(nil, nil, nil, nil)
	var deliveredAttempts, limited atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := users.SendNotifyEmailCode(context.Background(), 10, fmt.Sprintf("n%d@example.com", i), sender, cache)
			switch {
			case errors.Is(err, service.ErrEmailNotConfigured):
				deliveredAttempts.Add(1)
			case errors.Is(err, service.ErrNotifyCodeUserRateLimit):
				limited.Add(1)
			default:
				t.Errorf("unexpected send result: %v", err)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, int32(5), deliveredAttempts.Load(), "preserve the existing five-per-user limit")
	require.Equal(t, int32(20), limited.Load())
	require.Equal(t, int32(5), settings.reads.Load())
}
