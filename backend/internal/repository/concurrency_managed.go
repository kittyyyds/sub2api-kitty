package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/redis/go-redis/v9"
)

type managedSlot struct{ key, member string }

// Only members acquired by this cache are renewed. XX prevents a delayed
// heartbeat from resurrecting a released request. Crashed processes stop
// renewing; the existing Redis-time TTL reaper eventually reclaims them.
const renewManagedSlotSource = `
redis.replicate_commands()
if redis.call('ZSCORE',KEYS[1],ARGV[1]) == false then return 0 end
local now=redis.call('TIME')
redis.call('ZADD',KEYS[1],'XX',now[1],ARGV[1])
redis.call('EXPIRE',KEYS[1],ARGV[2])
return 1
`

var renewManagedSlot = redis.NewScript(renewManagedSlotSource)

func (c *concurrencyCache) startManagedRenewals() {
	c.managedSlots = make(map[managedSlot]struct{})
	c.managedHealthy = true
	managedinstance.RegisterAccounting(func() bool { c.managedMu.Lock(); defer c.managedMu.Unlock(); return c.managedHealthy })
	interval := time.Duration(c.slotTTLSeconds) * time.Second / 4
	if interval > 10*time.Second {
		interval = 10 * time.Second
	}
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			c.renewManagedSlots(ctx)
			cancel()
		}
	}()
}
func (c *concurrencyCache) trackManagedSlot(key, member string) {
	c.managedMu.Lock()
	defer c.managedMu.Unlock()
	if c.managedSlots != nil {
		c.managedSlots[managedSlot{key, member}] = struct{}{}
	}
}
func (c *concurrencyCache) untrackManagedSlot(key, member string) {
	c.managedMu.Lock()
	defer c.managedMu.Unlock()
	delete(c.managedSlots, managedSlot{key, member})
}
func (c *concurrencyCache) renewManagedSlots(ctx context.Context) {
	c.managedMu.Lock()
	slots := make([]managedSlot, 0, len(c.managedSlots))
	for s := range c.managedSlots {
		slots = append(slots, s)
	}
	c.managedMu.Unlock()
	healthy := true
	// Pipelined bounded batches avoid an unbounded Lua script on a shared Redis.
	for start := 0; start < len(slots); start += 200 {
		end := start + 200
		if end > len(slots) {
			end = len(slots)
		}
		pipe := c.rdb.Pipeline()
		commands := make([]*redis.Cmd, 0, end-start)
		for _, s := range slots[start:end] {
			commands = append(commands, pipe.Eval(ctx, renewManagedSlotSource, []string{s.key}, s.member, c.slotTTLSeconds))
		}
		_, err := pipe.Exec(ctx)
		if err != nil {
			healthy = false
			continue
		}
		for i, cmd := range commands {
			n, err := cmd.Int()
			if err != nil || n != 1 {
				c.managedMu.Lock()
				_, stillActive := c.managedSlots[slots[start+i]]
				c.managedMu.Unlock()
				if stillActive {
					healthy = false
				}
			}
		}
	}
	c.managedMu.Lock()
	c.managedHealthy = healthy
	c.managedMu.Unlock()
}
