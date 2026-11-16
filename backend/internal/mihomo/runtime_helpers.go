package mihomo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

func managedManager() *Manager {
	var selected *Manager
	managers.Range(func(key, _ any) bool {
		m, ok := key.(*Manager)
		if !ok {
			return true
		}
		m.mu.Lock()
		ready := !m.closed && m.state.Running
		m.mu.Unlock()
		if ready {
			selected = m
			return false
		}
		return true
	})
	return selected
}

func waitKernelLock(ctx context.Context, attempt func() bool) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (m *Manager) acquire(ctx context.Context) error {
	select {
	case m.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *Manager) release() { <-m.gate }

func nodeDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
