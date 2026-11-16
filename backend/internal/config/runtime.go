package config

import (
	"fmt"
	"time"
)

const (
	RuntimeRoleFull    = "full"
	RuntimeRoleGateway = "gateway"
)

type RuntimeConfig struct {
	Role string `mapstructure:"role"`
}

func (c *Config) RunsBackgroundJobs() bool { return c == nil || c.Runtime.Role != RuntimeRoleGateway }
func (c *Config) validateRuntime() error {
	switch c.Runtime.Role {
	case "", RuntimeRoleFull, RuntimeRoleGateway:
	default:
		return fmt.Errorf("runtime.role must be full or gateway")
	}
	if c.Server.GracefulShutdownTimeout < 0 || c.Server.GracefulShutdownTimeout > 3600 {
		return fmt.Errorf("invalid shutdown timeout")
	}
	if c.Server.ShutdownDrainDelay < 0 || c.Server.ShutdownDrainDelay > 300 {
		return fmt.Errorf("invalid drain delay")
	}
	return nil
}
func (c ServerConfig) ShutdownTimeout() time.Duration {
	if c.GracefulShutdownTimeout <= 0 {
		return 5 * time.Second
	}
	return time.Duration(c.GracefulShutdownTimeout) * time.Second
}
