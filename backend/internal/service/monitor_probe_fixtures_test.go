package service

import (
	"context"
	"sync"
	"time"
)

var groupTestNow = time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)

type groupTestGroupsFake map[int64]*Group

func (g groupTestGroupsFake) GetByID(_ context.Context, id int64) (*Group, error) {
	if group, ok := g[id]; ok {
		return group, nil
	}
	return nil, ErrGroupNotFound
}

type routeStep struct {
	account *Account
	err     error
}
type groupTestRouterFake struct {
	mu       sync.Mutex
	steps    []routeStep
	excluded [][]int64
	released []int64
}

func (r *groupTestRouterFake) route(_ context.Context, _ *Group, model string, excluded map[int64]struct{}) (*monitorGroupRoute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := []int64{}
	for id := range excluded {
		seen = append(seen, id)
	}
	r.excluded = append(r.excluded, seen)
	if len(r.steps) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	if step.err != nil {
		return nil, step.err
	}
	id := step.account.ID
	return &monitorGroupRoute{account: step.account, model: model, release: func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.released = append(r.released, id)
	}}, nil
}
func account(id int64, name string) *Account { return &Account{ID: id, Name: name} }
func newGroupTestRepoFake() *struct{}        { return &struct{}{} }
func newGroupTestService(_ *struct{}, router monitorGroupRouter,
	run func(context.Context, int64, string, *MonitorProbeConfig) (*ScheduledTestResult, error)) (*MonitorGroupProbeService, *struct{}) {
	return &MonitorGroupProbeService{
		groups: groupTestGroupsFake{4: {ID: 4, Name: "GPT PRO", Platform: PlatformOpenAI, Status: StatusActive}},
		router: router, runAccount: run,
	}, &struct{}{}
}
