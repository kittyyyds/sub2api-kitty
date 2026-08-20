package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func billingProbeAccount(status string, rate float64, now time.Time, freshUntil time.Time) *Account {
	manual := 0.75
	return &Account{
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		RateMultiplier: &manual,
		Extra: map[string]any{
			UpstreamBillingProbeEnabledExtraKey: true,
			UpstreamBillingProbeExtraKey: map[string]any{
				"status":          status,
				"fresh_until":     freshUntil.UTC().Format(time.RFC3339Nano),
				"last_attempt_at": now.Add(-time.Minute).UTC().Format(time.RFC3339Nano),
				"next_probe_at":   now.Add(time.Hour).UTC().Format(time.RFC3339Nano),
				"data": map[string]any{
					"billing_scope":            "token",
					"resolved_rate_multiplier": rate,
					"peak_rate_enabled":        false,
				},
			},
		},
	}
}

func TestEffectiveBillingRateMultiplierAtUsesFreshProbe(t *testing.T) {
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	account := billingProbeAccount(UpstreamBillingProbeStatusOK, 0.32, now, now.Add(time.Hour))

	rate, source := account.EffectiveBillingRateMultiplierAt(now)
	require.Equal(t, 0.32, rate)
	require.Equal(t, "probe", source)
}

func TestEffectiveBillingRateMultiplierAtFallsBackToManual(t *testing.T) {
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		status  string
		freshTo time.Time
	}{
		{name: "expired", status: UpstreamBillingProbeStatusOK, freshTo: now},
		{name: "failed", status: UpstreamBillingProbeStatusFailed, freshTo: now.Add(time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := billingProbeAccount(tt.status, 0.32, now, tt.freshTo)
			rate, source := account.EffectiveBillingRateMultiplierAt(now)
			require.Equal(t, 0.75, rate)
			require.Equal(t, "manual", source)
		})
	}
}

func TestEffectiveBillingRateMultiplierAtRecomputesPeakWindow(t *testing.T) {
	now := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	account := billingProbeAccount(UpstreamBillingProbeStatusOK, 0.4, now, now.Add(24*time.Hour))
	snapshot := account.Extra[UpstreamBillingProbeExtraKey].(map[string]any)
	snapshot["data"] = map[string]any{
		"billing_scope":            "token",
		"resolved_rate_multiplier": 0.4,
		"peak_rate_enabled":        true,
		"peak_start":               "09:00",
		"peak_end":                 "18:00",
		"peak_rate_multiplier":     2.0,
		"timezone":                 "UTC",
	}

	peakRate, peakSource := account.EffectiveBillingRateMultiplierAt(now.Add(2 * time.Hour))
	offPeakRate, offPeakSource := account.EffectiveBillingRateMultiplierAt(now.Add(11 * time.Hour))
	require.Equal(t, 0.8, peakRate)
	require.Equal(t, 0.4, offPeakRate)
	require.Equal(t, "probe", peakSource)
	require.Equal(t, "probe", offPeakSource)
}
