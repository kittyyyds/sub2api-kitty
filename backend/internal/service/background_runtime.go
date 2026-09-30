package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"reflect"
)

// Global timers require both a full node and, during managed handoff, ownership
// of the active lease. Request queues and pools use their separate providers.
func startBackgroundService(cfg *config.Config, job interface{ Start() }) {
	if !cfg.RunsBackgroundJobs() {
		return
	}
	var stop func()
	if stoppable, ok := job.(interface{ Stop() }); ok {
		stop = stoppable.Stop
	}
	managedinstance.Register(reflect.TypeOf(job).String(), job.Start, stop)
}
