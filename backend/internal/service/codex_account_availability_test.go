package service

import (
	"testing"
	"time"
)

func TestResolveCodexAccountAvailability(t *testing.T) {
	now := time.Date(2026, 9, 20, 22, 50, 0, 0, time.Local)
	at := func(d time.Duration) *time.Time {
		v := now.Add(d)
		return &v
	}
	expired := now.Add(-time.Hour)

	cases := []struct {
		name      string
		account   Account
		wantKind  string
		wantRecov time.Duration
	}{
		{name: "available", account: Account{Status: StatusActive, Schedulable: true}, wantKind: CodexAccountAvailabilityAvailable},
		{name: "rate limited while schedulable", account: Account{Status: StatusActive, Schedulable: true, RateLimitResetAt: at(2 * time.Hour)}, wantKind: CodexAccountAvailabilityRateLimited, wantRecov: 2 * time.Hour},
		{name: "rate limit expired", account: Account{Status: StatusActive, Schedulable: true, RateLimitResetAt: at(-time.Minute)}, wantKind: CodexAccountAvailabilityAvailable},
		{name: "credential error", account: Account{Status: "error", Schedulable: false}, wantKind: CodexAccountAvailabilityError},
		{name: "error beats schedulable", account: Account{Status: "error", Schedulable: true}, wantKind: CodexAccountAvailabilityError},
		{name: "manually disabled", account: Account{Status: StatusActive, Schedulable: false}, wantKind: CodexAccountAvailabilityDisabled},
		{name: "expired auto pause", account: Account{Status: StatusActive, Schedulable: true, ExpiresAt: &expired, AutoPauseOnExpired: true}, wantKind: CodexAccountAvailabilityExpired},
		{name: "expired without auto pause", account: Account{Status: StatusActive, Schedulable: true, ExpiresAt: &expired, AutoPauseOnExpired: false}, wantKind: CodexAccountAvailabilityAvailable},
		{name: "overload", account: Account{Status: StatusActive, Schedulable: true, OverloadUntil: at(15 * time.Minute)}, wantKind: CodexAccountAvailabilityOverload, wantRecov: 15 * time.Minute},
		{name: "temp unschedulable", account: Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: at(50 * time.Minute)}, wantKind: CodexAccountAvailabilityTempUnschedulable, wantRecov: 50 * time.Minute},
		{name: "latest window wins rate limit", account: Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: at(50 * time.Minute), RateLimitResetAt: at(2 * time.Hour)}, wantKind: CodexAccountAvailabilityRateLimited, wantRecov: 2 * time.Hour},
		{name: "latest window wins overload", account: Account{Status: StatusActive, Schedulable: true, RateLimitResetAt: at(10 * time.Minute), OverloadUntil: at(4 * time.Hour)}, wantKind: CodexAccountAvailabilityOverload, wantRecov: 4 * time.Hour},
		{name: "nil account", wantKind: CodexAccountAvailabilityError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ptr *Account
			if tc.name != "nil account" {
				a := tc.account
				ptr = &a
			}
			kind, recoverAt := ResolveCodexAccountAvailability(ptr, now)
			if kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", kind, tc.wantKind)
			}
			if tc.wantRecov == 0 {
				if recoverAt != nil {
					t.Fatalf("recoverAt = %v, want nil", recoverAt)
				}
				return
			}
			if recoverAt == nil || !recoverAt.Equal(now.Add(tc.wantRecov)) {
				t.Fatalf("recoverAt = %v, want %v", recoverAt, now.Add(tc.wantRecov))
			}
		})
	}
}
