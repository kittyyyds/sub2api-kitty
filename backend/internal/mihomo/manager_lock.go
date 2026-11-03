package mihomo

import "context"

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

func (m *Manager) acquire(ctx context.Context) error {
	select {
	case m.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *Manager) release() { <-m.gate }
