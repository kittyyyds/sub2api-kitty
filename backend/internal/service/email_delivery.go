package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Conditional deletion prevents a delayed SMTP failure from deleting a newer
// verification code written by another request. Redis implements this atomically.
type conditionalEmailCodeCache interface {
	DeleteUndeliveredVerificationCode(context.Context, string, string) error
	DeleteUndeliveredNotifyCode(context.Context, string, string) error
}

func clearUndeliveredEmailCode(ctx context.Context, cache EmailCache, email, code string, notify bool) {
	// Delivery often fails because the request context expired; cleanup must still run.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	conditional, ok := cache.(conditionalEmailCodeCache)
	if !ok {
		// Older/test adapters cannot delete conditionally. Leaving the short
		// cooldown is safer than deleting a potentially newer code.
		return
	}
	var err error
	if notify {
		err = conditional.DeleteUndeliveredNotifyCode(cleanupCtx, email, code)
	} else {
		err = conditional.DeleteUndeliveredVerificationCode(cleanupCtx, email, code)
	}
	if err != nil {
		slog.Warn("failed to clear undelivered email code", "recipient_hash", notificationEmailHash(email), "error", err)
	}
}

// Reserve before delivery so concurrent requests cannot all pass a read-only
// count check. The existing limit and window remain unchanged.
func reserveEmailCodeUserRate(ctx context.Context, cache EmailCache, userID int64) error {
	if cache == nil {
		return ErrServiceUnavailable
	}
	count, err := cache.IncrNotifyCodeUserRate(ctx, userID, notifyCodeUserRateWindow)
	if err != nil {
		return fmt.Errorf("reserve email verification rate limit: %w", err)
	}
	if count > notifyCodeUserRateLimit {
		return ErrNotifyCodeUserRateLimit
	}
	return nil
}
