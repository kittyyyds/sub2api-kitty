package managedinstance

import (
	"context"
	"testing"
	"time"
)

func TestManagedPluginLifetimeSurvivesDrainButNotLostLease(t *testing.T) {
	lease := &testLease{}
	c := testController(t, "plugin", lease)
	if _, ok := c.LeadershipContext(); ok {
		t.Fatal("standby allowed")
	}
	_ = c.SetRole("active")
	c.Step(context.Background())
	ctx, ok := c.LeadershipContext()
	if !ok {
		t.Fatal("owner refused")
	}
	c.Drain()
	if _, ok = c.LeadershipContext(); ok {
		t.Fatal("drain admitted start")
	}
	if ctx.Err() != nil {
		t.Fatal("normal drain cut child before its stop hook")
	}
	lease.lost = true
	c.Step(context.Background())
	if ctx.Err() == nil {
		t.Fatal("lease loss did not cancel child")
	}
}

func TestManagedPluginLifetimeRejectsStaleLease(t *testing.T) {
	c := testController(t, "stale-plugin", &testLease{})
	_ = c.SetRole("active")
	c.Step(context.Background())
	ctx, _ := c.LeadershipContext()
	c.mu.Lock()
	c.lastRenew = time.Now().Add(-LeaseTTL)
	c.mu.Unlock()
	if _, ok := c.LeadershipContext(); ok || ctx.Err() == nil {
		t.Fatal("stale owner still allowed")
	}
}
