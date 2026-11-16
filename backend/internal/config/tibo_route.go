package config

import (
	"fmt"
	"time"
)

// OpenAITiboRouteConfig tunes Tibo HTTP probing, vote-confirmed verdict
// flips and scheduler tiering. Zero durations/counts fall back to the
// built-in defaults; see OpenAIGatewayService.openAITiboRouteConfig.
// Each probe is one real gpt-6-astra request (reasoning effort xhigh) on the
// probed account and consumes quota of that account.
type OpenAITiboRouteConfig struct {
	// Enabled turns on Tibo route probing and tiered routing. The admin
	// account list shows the astra health dots only while it is on. This fork
	// ships it off (the source fork defaults to on): every probe is a real
	// gpt-6-astra request on the account's quota that the official Codex CLI
	// never sends, so operators enable it explicitly once the deployment is
	// stable. nil (struct built without viper, e.g. unit fixtures) is off too.
	Enabled *bool `mapstructure:"enabled"`
	// TickInterval is how often the background loop checks which enrolled
	// accounts are due (the source fork drove this from the Codex ticket
	// harvester at 6s). It is not the per-account probe cadence.
	TickInterval time.Duration `mapstructure:"tick_interval"`
	// ProbeAPIKeyAccounts also probes OpenAI API-key accounts (including
	// compatible gateways with a custom base_url) at their own Responses URL.
	// ProbePausedAccounts keeps probing manually paused (unschedulable) OAuth
	// accounts while their quota allows it. Both default to true, as in the
	// source fork; nil (struct built without viper) is also true.
	ProbeAPIKeyAccounts *bool `mapstructure:"probe_api_key_accounts"`
	ProbePausedAccounts *bool `mapstructure:"probe_paused_accounts"`
	// Interval is the base delay between regular probes. Spread is the
	// absolute ± window (uniform). Regular probes land in
	// [Interval-Spread, Interval+Spread]. ConfirmSpacing is not spread.
	Interval time.Duration `mapstructure:"interval"`
	Spread   time.Duration `mapstructure:"spread"`
	// ConfirmSamples is the vote window for a verdict flip; a majority of it
	// must agree. ConfirmSpacing separates the confirmation probes.
	ConfirmSamples int           `mapstructure:"confirm_samples"`
	ConfirmSpacing time.Duration `mapstructure:"confirm_spacing"`
	// MinDegradedDwell delays degraded -> healthy after a flip to degraded.
	// Negative disables the dwell.
	MinDegradedDwell time.Duration `mapstructure:"min_degraded_dwell"`
	// MaxStale keeps the last confirmed verdict while samples are unknown or
	// missing; after it the effective state is unknown.
	MaxStale time.Duration `mapstructure:"max_stale"`
	// MaxProbesPerHour caps probes per account and route (including
	// confirmation probes). ProbeConcurrency caps probes process-wide.
	MaxProbesPerHour int `mapstructure:"max_probes_per_hour"`
	ProbeConcurrency int `mapstructure:"probe_concurrency"`
	// SchedulerPreferHealthyRoute soft-orders scheduler candidates by route
	// health tier (healthy > unknown > degraded) and releases degraded sticky
	// sessions when a healthy account exists.
	SchedulerPreferHealthyRoute bool `mapstructure:"scheduler_prefer_healthy_route"`
}

func validateOpenAITiboRoute(cfg OpenAITiboRouteConfig) error {
	for name, value := range map[string]time.Duration{
		"tick_interval": cfg.TickInterval, "interval": cfg.Interval, "spread": cfg.Spread,
		"confirm_spacing": cfg.ConfirmSpacing, "max_stale": cfg.MaxStale,
	} {
		if value < 0 {
			return fmt.Errorf("gateway.openai_tibo_route.%s must be non-negative", name)
		}
	}
	if cfg.ConfirmSamples < 0 || cfg.ConfirmSamples > 9 {
		return fmt.Errorf("gateway.openai_tibo_route.confirm_samples must be within [0,9]")
	}
	if cfg.MaxProbesPerHour < 0 || cfg.ProbeConcurrency < 0 {
		return fmt.Errorf("gateway.openai_tibo_route probe limits must be non-negative")
	}
	return nil
}
