// Package managedinstance coordinates one-way, host-controlled application handoff.
// It never changes customer settings or exposes an HTTP control endpoint.
package managedinstance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const LeaseTTL = 30 * time.Second

type LeaseStore interface {
	Acquire(context.Context, string, time.Duration) (bool, error)
	Renew(context.Context, string, time.Duration) (bool, error)
	Release(context.Context, string) error
}

type worker struct {
	name        string
	start, stop func()
}

type Status struct {
	InstanceID         string    `json:"instance_id"`
	Role               string    `json:"role"`
	Ready              bool      `json:"ready"`
	ActiveHTTP         int64     `json:"active_http"`
	ActiveSSE          int64     `json:"active_sse"`
	ActiveWS           int64     `json:"active_ws"`
	ActiveRequests     int64     `json:"active_requests"`
	BackgroundActive   int64     `json:"background_active"`
	BackgroundStopped  bool      `json:"background_stopped"`
	LeadershipReleased bool      `json:"leadership_released"`
	LeadershipOwned    bool      `json:"leadership_owned"`
	RetireReady        bool      `json:"retire_ready"`
	AccountingReady    bool      `json:"accounting_ready"`
	LeaseHealthy       bool      `json:"lease_healthy"`
	UpdatedAt          time.Time `json:"updated_at"`
	Error              string    `json:"error,omitempty"`
}

type Controller struct {
	mu                                                sync.Mutex
	id, roleFile, role                                string
	ready, owned, healthy, started, stopping, stopped bool
	lastRenew                                         time.Time
	workers                                           []worker
	startedWorkers                                    []worker
	accounting                                        []func() bool
	store                                             LeaseStore
	http, sse, ws, background                         int64
	workContext                                       context.Context
	cancelWork                                        context.CancelFunc
	errCode                                           string
}

var globalMu sync.RWMutex
var global *Controller

func Current() *Controller { globalMu.RLock(); defer globalMu.RUnlock(); return global }
func Enabled() bool        { return Current() != nil }

func ConfigureFromEnv() error {
	if os.Getenv("KITTY_MANAGED_INSTANCE") != "true" {
		globalMu.Lock()
		global = nil
		globalMu.Unlock()
		return nil
	}
	id, path := os.Getenv("KITTY_INSTANCE_ID"), os.Getenv("KITTY_INSTANCE_ROLE_FILE")
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,95}$`).MatchString(id) || !filepath.IsAbs(path) {
		return errors.New("managed instance requires a unique valid KITTY_INSTANCE_ID and absolute role file")
	}
	c := New(id, path)
	if err := c.ReadRole(); err != nil {
		return err
	}
	globalMu.Lock()
	global = c
	globalMu.Unlock()
	return nil
}

func New(id, roleFile string) *Controller {
	ctx, cancel := context.WithCancel(context.Background())
	return &Controller{id: id, roleFile: roleFile, role: "standby", healthy: true, workContext: ctx, cancelWork: cancel}
}

func ConfigureLeaseStore(store LeaseStore) {
	if c := Current(); c != nil {
		c.mu.Lock()
		c.store = store
		c.mu.Unlock()
	}
}
func Register(name string, start, stop func()) {
	if c := Current(); c != nil {
		c.mu.Lock()
		c.workers = append(c.workers, worker{name, start, stop})
		c.mu.Unlock()
	} else {
		start()
	}
}
func RegisterAccounting(check func() bool) {
	if c := Current(); c != nil {
		c.mu.Lock()
		c.accounting = append(c.accounting, check)
		c.mu.Unlock()
	}
}

// Begin admits a background cycle only on the elected active instance. A normal
// drain leaves the context alive. Lease loss cancels it and forbids new work.
func Begin(ctx context.Context) (context.Context, func(), bool) {
	if c := Current(); c != nil {
		return c.Begin(ctx)
	}
	return ctx, func() {}, true
}
func (c *Controller) Begin(ctx context.Context) (context.Context, func(), bool) {
	c.mu.Lock()
	if c.owned && time.Since(c.lastRenew) >= LeaseTTL/2 {
		c.healthy = false
		c.role = "draining"
		c.errCode = "leadership_renewal_overdue"
		c.cancelWork()
	}
	if c.role != "active" || !c.owned || !c.healthy || c.stopping || c.stopped {
		c.mu.Unlock()
		return ctx, func() {}, false
	}
	c.background++
	workCtx := c.workContext
	c.mu.Unlock()
	job, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(workCtx, cancel)
	var once sync.Once
	return job, func() { once.Do(func() { stop(); cancel(); c.mu.Lock(); c.background--; c.mu.Unlock() }) }, true
}

// LeadershipContext permits a child process only on the active lease owner.
// Normal drain leaves the context alive for the registered stop hook; lease
// loss cancels it immediately. Process lifetime is not a background cycle.
func (c *Controller) LeadershipContext() (context.Context, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owned && time.Since(c.lastRenew) >= LeaseTTL/2 {
		c.healthy = false
		c.role = "draining"
		c.errCode = "leadership_renewal_overdue"
		c.cancelWork()
	}
	return c.workContext, c.role == "active" && c.owned && c.healthy && !c.stopping && !c.stopped
}

// Track is for request-owned asynchronous work. It remains allowed while the
// old frontend drains and therefore does not depend on background leadership.
func Track() func() {
	c := Current()
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	c.background++
	c.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { c.mu.Lock(); c.background--; c.mu.Unlock() }) }
}

func (c *Controller) ReadRole() error {
	b, err := os.ReadFile(c.roleFile)
	if err != nil {
		c.Drain()
		c.mu.Lock()
		c.errCode = "role_file_unavailable"
		c.mu.Unlock()
		return errors.New("managed role file unavailable")
	}
	return c.SetRole(strings.TrimSpace(string(b)))
}
func (c *Controller) SetRole(role string) error {
	if role != "standby" && role != "active" && role != "draining" {
		c.Drain()
		return errors.New("invalid managed instance role")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.role == "draining" && role != "draining" {
		c.errCode = "draining_is_terminal"
		return errors.New("draining instance cannot be reactivated")
	}
	if c.started && role == "standby" {
		c.role = "draining"
		c.errCode = "active_cannot_return_to_standby"
		return errors.New("active instance cannot return to standby")
	}
	c.role = role
	return nil
}
func (c *Controller) Drain()     { c.mu.Lock(); c.role = "draining"; c.mu.Unlock() }
func (c *Controller) MarkReady() { c.mu.Lock(); c.ready = true; c.mu.Unlock() }

// Step is serialized by Run. No worker stop runs until admitted cycles have
// completed; Stop functions may cancel idle timers, but never an admitted job.
func (c *Controller) Step(ctx context.Context) {
	c.mu.Lock()
	role, owned, started, stopping, bg, store := c.role, c.owned, c.started, c.stopping, c.background, c.store
	c.mu.Unlock()
	if store == nil {
		c.mu.Lock()
		c.healthy = false
		c.errCode = "lease_store_unavailable"
		c.mu.Unlock()
		return
	}
	leaseCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if role == "active" && !owned && !started {
		ok, err := store.Acquire(leaseCtx, c.id, LeaseTTL)
		c.mu.Lock()
		c.healthy = err == nil
		if err != nil {
			c.errCode = "leadership_unavailable"
		}
		if ok && err == nil {
			c.owned = true
			c.started = true
			c.lastRenew = time.Now()
			c.errCode = ""
		}
		workers := append([]worker(nil), c.workers...)
		c.mu.Unlock()
		if ok && err == nil {
			for _, w := range workers {
				// Start hooks can load persisted schedules; keep the owner lease
				// fresh between hooks instead of letting a slow startup expire it.
				rctx, rcancel := context.WithTimeout(ctx, 2*time.Second)
				confirmed, renewErr := store.Renew(rctx, c.id, LeaseTTL)
				rcancel()
				c.mu.Lock()
				if !confirmed || renewErr != nil {
					c.healthy = false
					c.role = "draining"
					c.errCode = "leadership_lost"
					c.cancelWork()
				} else {
					c.lastRenew = time.Now()
				}
				canStart := c.role == "active" && c.healthy
				c.mu.Unlock()
				if !canStart {
					break
				}
				w.start()
				c.mu.Lock()
				c.startedWorkers = append(c.startedWorkers, w)
				c.mu.Unlock()
			}
		}
		return
	}
	if owned {
		ok, err := store.Renew(leaseCtx, c.id, LeaseTTL)
		c.mu.Lock()
		c.healthy = ok && err == nil
		if c.healthy {
			c.lastRenew = time.Now()
		} else {
			c.errCode = "leadership_lost"
			c.role = "draining"
			c.cancelWork()
		}
		c.mu.Unlock()
	}
	if role == "draining" && !stopping && bg == 0 {
		c.mu.Lock()
		if c.background != 0 || c.stopping {
			c.mu.Unlock()
			return
		}
		c.stopping = true
		workers := append([]worker(nil), c.startedWorkers...)
		wasStarted := c.started
		c.mu.Unlock()
		go func() {
			if wasStarted {
				var wg sync.WaitGroup
				for _, w := range workers {
					if w.stop != nil {
						wg.Add(1)
						go func(w worker) { defer wg.Done(); w.stop() }(w)
					}
				}
				wg.Wait()
			}
			c.mu.Lock()
			c.stopped = true
			c.mu.Unlock()
		}()
	}
	c.mu.Lock()
	stopped := c.stopped
	c.mu.Unlock()
	if role == "draining" && stopped && owned {
		if err := store.Release(leaseCtx, c.id); err == nil {
			c.mu.Lock()
			c.owned = false
			c.mu.Unlock()
		}
	}
}

func (c *Controller) Snapshot() Status {
	c.mu.Lock()
	s := Status{InstanceID: c.id, Role: c.role, Ready: c.ready, ActiveHTTP: c.http, ActiveSSE: c.sse, ActiveWS: c.ws, ActiveRequests: c.http + c.ws, BackgroundActive: c.background, BackgroundStopped: c.stopped, LeadershipReleased: !c.owned && c.stopped, LeadershipOwned: c.owned, LeaseHealthy: c.healthy, UpdatedAt: time.Now().UTC(), Error: c.errCode}
	checks := append([]func() bool(nil), c.accounting...)
	c.mu.Unlock()
	s.AccountingReady = true
	for _, check := range checks {
		if !check() {
			s.AccountingReady = false
		}
	}
	s.RetireReady = s.Role == "draining" && s.ActiveRequests == 0 && s.ActiveSSE == 0 && s.BackgroundActive == 0 && s.LeadershipReleased && s.AccountingReady
	return s
}

func (c *Controller) Publish() error {
	s := c.Snapshot()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(c.roleFile), "status.json")
	tmp := path + "." + c.id + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func (c *Controller) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_ = c.ReadRole()
		c.Step(ctx)
		if err := c.Publish(); err != nil {
			c.mu.Lock()
			c.errCode = "status_write_failed"
			c.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (c *Controller) WaitDrained(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if c.Snapshot().RetireReady {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("managed instance remains draining: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
