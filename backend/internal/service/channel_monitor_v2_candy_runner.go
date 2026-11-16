package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
)

type channelMonitorV2CandyJob interface {
	RunDue(context.Context, time.Time)
}

// ChannelMonitorV2CandyRunner owns the active candy checks independently of the
// passive traffic aggregator and of requests to the monitor page. Database
// claims in RunDue enforce the configured per-probe interval across instances.
type ChannelMonitorV2CandyRunner struct {
	job       channelMonitorV2CandyJob
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func newChannelMonitorV2CandyRunner(job channelMonitorV2CandyJob) *ChannelMonitorV2CandyRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChannelMonitorV2CandyRunner{
		job: job, interval: 15 * time.Second, ctx: ctx, cancel: cancel, done: make(chan struct{}),
	}
}

func (r *ChannelMonitorV2CandyRunner) Start() {
	if r == nil || r.job == nil {
		return
	}
	r.startOnce.Do(func() { go r.loop() })
}

func (r *ChannelMonitorV2CandyRunner) Stop() {
	if r != nil {
		r.stopOnce.Do(r.cancel)
	}
}

func (r *ChannelMonitorV2CandyRunner) loop() {
	defer close(r.done)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		if r.ctx.Err() != nil {
			return
		}
		// Lease loss cancels an in-flight check and prevents standby instances
		// from initiating new upstream traffic.
		ctx, finish, allowed := managedinstance.Begin(r.ctx)
		if allowed {
			r.job.RunDue(ctx, time.Now())
			finish()
		}
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
