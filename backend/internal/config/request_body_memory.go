package config

// Opt-in sizing follows the upstream quarter-of-process-memory policy. Explicit
// byte settings, including zero (disabled), always take precedence.
func applyRequestBodyMemoryBudget(cfg *Config, explicitConfig bool, envValue string) {
	if cfg == nil || !cfg.Gateway.RequestBodyMemoryBudgetAuto || explicitConfig || envValue != "" {
		return
	}
	cfg.Gateway.RequestBodyMemoryBudgetBytes = autoRequestBodyMemoryBudget(detectMemoryLimitBytes())
}

