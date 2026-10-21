package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrAccountTestBusy = errors.New("账号业务并发已满，测试等待空闲容量")

type testAdmissionContextKey struct{}
type testAdmissionObservation struct{ Waiting bool }

func markTestAdmissionWaiting(ctx context.Context) {
	if observation, ok := ctx.Value(testAdmissionContextKey{}).(*testAdmissionObservation); ok {
		observation.Waiting = true
	}
}

type TestAdmissionWaitError struct {
	Until  time.Time
	Reason string
}

func (e *TestAdmissionWaitError) Error() string { return e.Reason }
func (e *TestAdmissionWaitError) Unwrap() error { return ErrAccountTestBusy }

// Account/model cooldowns apply to probes too. This never changes policy or
// clears limits; it merely delays an attempt until the latest known boundary.
func accountTestCooldown(ctx context.Context, a *Account, model string, now time.Time) error {
	if a == nil {
		return errors.New("账号不存在")
	}
	until, reason := now, ""
	for _, item := range []struct {
		deadline *time.Time
		reason   string
	}{
		{a.RateLimitResetAt, "账号限流冷却尚未结束"}, {a.OverloadUntil, "账号过载冷却尚未结束"}, {a.TempUnschedulableUntil, "账号临时暂停尚未结束"},
	} {
		if item.deadline != nil && item.deadline.After(until) {
			until, reason = *item.deadline, item.reason
		}
	}
	if wait := a.GetModelRateLimitRemainingTimeWithContext(ctx, model); wait > 0 && now.Add(wait).After(until) {
		until, reason = now.Add(wait), "所选模型限流冷却尚未结束"
	}
	if reason != "" {
		return &TestAdmissionWaitError{Until: until, Reason: reason}
	}
	return nil
}

func (s *AccountTestService) acquireTestAccountSlot(ctx context.Context, a *Account) (func(), error) {
	// Production wiring supplies the shared service. Narrow standalone test
	// fixtures can omit it without creating an independent concurrency pool.
	if s.concurrencyService == nil {
		return func() {}, nil
	}
	result, err := s.concurrencyService.AcquireAccountSlot(ctx, a.ID, a.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("测试并发准入不可用: %w", err)
	}
	if !result.Acquired {
		return nil, ErrAccountTestBusy
	}
	return result.ReleaseFunc, nil
}

// Empty selections retain the established Codex test model; explicit choices are unchanged.
func resolveAccountTestModel(account *Account, configured string) string {
	model := strings.TrimSpace(configured)
	if account != nil && account.UsesOpenAICodexProtocol() && model == "" {
		return "gpt-5.3-codex"
	}
	return model
}
