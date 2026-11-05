package service

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
)

func TestSupplierBalanceFreshnessScopeAndNoSecrets(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	limit := 5.0
	b := UpstreamBalanceSnapshot{Status: "ok", ReceivedAt: &past, FreshUntil: &future, Data: map[string]any{"unit": "USD", "wallet_balance": 4.0, "remaining": 99.0, "is_valid": true, "private": "must-not-leak"}}
	build := func() SupplierBalanceRow {
		raw, _ := json.Marshal(b)
		return BuildSupplierBalanceRow(1, "fixture", "openai", "https://secret:credential@provider.example/private/token?key=secret", raw, SupplierBalanceThresholds{"USD": {Wallet: &limit, Remaining: &limit}}, now)
	}
	r := build()
	require.Equal(t, "provider.example", r.Host)
	require.Equal(t, []string{"wallet"}, r.Alerts)
	require.Equal(t, 4.0, *r.Wallet)
	require.Equal(t, 99.0, *r.Remaining)
	raw, _ := json.Marshal(r)
	require.NotContains(t, string(raw), "secret")
	require.NotContains(t, string(raw), "must-not-leak")
	b.FreshUntil = &past
	r = build()
	require.Equal(t, "stale", r.Status)
	require.Empty(t, r.Alerts)
	require.NotNil(t, r.Wallet, "last observation is retained but labeled stale")
	b.FreshUntil = &future
	b.Status = "failed"
	r = build()
	require.Equal(t, "failed", r.Status)
	require.Empty(t, r.Alerts)
	b.Status = "ok"
	b.Data["unit"] = "CNY"
	require.Empty(t, build().Alerts, "USD threshold must not compare CNY")
	b.Data["unit"] = "USD"
	b.Data["unlimited"] = true
	b.Data["remaining"] = -1.0
	r = build()
	require.Nil(t, r.Remaining)
	require.True(t, r.Unlimited)
	require.Equal(t, []string{"wallet"}, r.Alerts)
	b.Data["is_valid"] = false
	require.Equal(t, "invalid", build().Status)
	require.Empty(t, build().Alerts)
}
func TestSupplierBalanceUnknownNotZeroAndThresholdValidation(t *testing.T) {
	r := BuildSupplierBalanceRow(1, "x", "openai", "", nil, nil, time.Now())
	require.Nil(t, r.Wallet)
	require.Nil(t, r.Remaining)
	require.Equal(t, "unknown", r.Status)
	r = BuildSupplierBalanceRow(1, "x", "openai", "", []byte(`{"invalid`), nil, time.Now())
	require.Equal(t, "invalid", r.Status)
	for _, n := range []float64{-1, math.Inf(1), math.NaN(), 1e13} {
		require.Error(t, ValidateSupplierBalanceThresholds(SupplierBalanceThresholds{"USD": {Wallet: &n}}))
	}
	require.Error(t, ValidateSupplierBalanceThresholds(SupplierBalanceThresholds{"usd": {}}))
	require.NoError(t, ValidateSupplierBalanceThresholds(SupplierBalanceThresholds{}))
}
