package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadOpenAITiboRouteDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	got := cfg.Gateway.OpenAITiboRoute
	require.Equal(t, 10*time.Minute, got.Interval)
	require.Equal(t, 5*time.Minute, got.Spread)
	require.Equal(t, 3, got.ConfirmSamples)
	require.Equal(t, 20*time.Second, got.ConfirmSpacing)
	require.Equal(t, 10*time.Minute, got.MinDegradedDwell)
	require.Equal(t, 45*time.Minute, got.MaxStale)
	require.Equal(t, 20, got.MaxProbesPerHour)
	require.Equal(t, 4, got.ProbeConcurrency)
	require.False(t, got.SchedulerPreferHealthyRoute)
	// Probing ships off in this fork; when enabled, API-key and paused OAuth
	// accounts are included as in the source fork.
	require.NotNil(t, got.Enabled)
	require.False(t, *got.Enabled)
	require.Equal(t, 6*time.Second, got.TickInterval)
	require.NotNil(t, got.ProbeAPIKeyAccounts)
	require.True(t, *got.ProbeAPIKeyAccounts)
	require.NotNil(t, got.ProbePausedAccounts)
	require.True(t, *got.ProbePausedAccounts)
}

func TestLoadOpenAITiboRouteFromFileAndEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("gateway:\n  openai_tibo_route:\n    confirm_spacing: 45s\n    probe_api_key_accounts: false\n"), 0o600))
	t.Setenv("CONFIG_FILE", configFile)
	t.Setenv("GATEWAY_OPENAI_TIBO_ROUTE_SCHEDULER_PREFER_HEALTHY_ROUTE", "true")
	t.Setenv("GATEWAY_OPENAI_TIBO_ROUTE_MAX_STALE", "20m")
	cfg, err := Load()
	require.NoError(t, err)
	got := cfg.Gateway.OpenAITiboRoute
	require.Equal(t, 45*time.Second, got.ConfirmSpacing)
	require.NotNil(t, got.ProbeAPIKeyAccounts)
	require.False(t, *got.ProbeAPIKeyAccounts)
	require.True(t, got.SchedulerPreferHealthyRoute)
	require.Equal(t, 20*time.Minute, got.MaxStale)
}

func TestValidateOpenAITiboRoute(t *testing.T) {
	require.NoError(t, validateOpenAITiboRoute(OpenAITiboRouteConfig{}))
	require.NoError(t, validateOpenAITiboRoute(OpenAITiboRouteConfig{ConfirmSamples: 3, MinDegradedDwell: -time.Second}))
	for name, cfg := range map[string]OpenAITiboRouteConfig{
		"tick":     {TickInterval: -time.Second},
		"interval": {Interval: -time.Second},
		"spread":   {Spread: -time.Second},
		"samples":  {ConfirmSamples: 10},
		"budget":   {MaxProbesPerHour: -1},
	} {
		require.Error(t, validateOpenAITiboRoute(cfg), name)
	}
}
