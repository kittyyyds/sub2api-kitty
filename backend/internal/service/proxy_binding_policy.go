package service

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync"
	"time"
)

type ProxyBindingPolicy struct {
	Enabled      bool     `json:"enabled"`
	OnlyTwoFA    bool     `json:"only_twofa"`
	Platforms    []string `json:"platforms"`
	HistoryLimit int      `json:"history_limit"`
	AutoAssign   bool     `json:"auto_assign"`
}

func (p ProxyBindingPolicy) Matches(platform, kind, source string) bool {
	return p.Enabled && kind == AccountTypeOAuth && slices.Contains(p.Platforms, platform) && (!p.OnlyTwoFA || source == "twofa")
}
func (p ProxyBindingPolicy) Validate() error {
	if p.HistoryLimit < 1 || p.HistoryLimit > 10000 {
		return errors.New("IP 历史绑定上限须为 1–10000")
	}
	if len(p.Platforms) == 0 {
		return errors.New("请选择参与规则的 OAuth 平台")
	}
	for _, v := range p.Platforms {
		if !slices.Contains([]string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok}, v) {
			return errors.New("不支持的 OAuth 平台")
		}
	}
	return nil
}

type ProxyBindingSummary struct {
	ProxyID      int64  `json:"proxy_id"`
	History      int64  `json:"history"`
	Current      int64  `json:"current"`
	Reserved     int64  `json:"reserved"`
	Limit        int    `json:"limit"`
	ExtraAllowed bool   `json:"extra_allowed"`
	ExitVerified bool   `json:"exit_verified"`
	ExitIP       string `json:"exit_ip,omitempty"`
}
type ProxyBindingStore interface {
	GetProxyBindingPolicy(context.Context) (ProxyBindingPolicy, error)
	SaveProxyBindingPolicy(context.Context, ProxyBindingPolicy) error
	ProxyBindingSummaries(context.Context) ([]ProxyBindingSummary, error)
	AllowOneProxyBinding(context.Context, int64) error
	RememberProxyBindingExit(context.Context, int64, string, time.Time) error
}
type ProxyBindingAdmin interface {
	GetProxyBindingPolicy(context.Context) (ProxyBindingPolicy, error)
	SaveProxyBindingPolicy(context.Context, ProxyBindingPolicy) error
	ProxyBindingSummaries(context.Context) ([]ProxyBindingSummary, error)
	AllowOneProxyBinding(context.Context, int64) error
}

func (s *adminServiceImpl) proxyBindingStore() (ProxyBindingStore, error) {
	r, ok := s.accountRepo.(ProxyBindingStore)
	if !ok {
		return nil, errors.New("IP 绑定规则仓储未就绪")
	}
	return r, nil
}
func (s *adminServiceImpl) GetProxyBindingPolicy(ctx context.Context) (ProxyBindingPolicy, error) {
	r, e := s.proxyBindingStore()
	if e != nil {
		return ProxyBindingPolicy{}, e
	}
	return r.GetProxyBindingPolicy(ctx)
}
func (s *adminServiceImpl) SaveProxyBindingPolicy(ctx context.Context, p ProxyBindingPolicy) error {
	if e := p.Validate(); e != nil {
		return e
	}
	r, e := s.proxyBindingStore()
	if e != nil {
		return e
	}
	return r.SaveProxyBindingPolicy(ctx, p)
}
func (s *adminServiceImpl) ProxyBindingSummaries(ctx context.Context) ([]ProxyBindingSummary, error) {
	r, e := s.proxyBindingStore()
	if e != nil {
		return nil, e
	}
	return r.ProxyBindingSummaries(ctx)
}
func (s *adminServiceImpl) AllowOneProxyBinding(ctx context.Context, id int64) error {
	r, e := s.proxyBindingStore()
	if e != nil {
		return e
	}
	return r.AllowOneProxyBinding(ctx, id)
}

// Probe actual exits before automatic allocation so multiple proxy entries
// sharing one address do not become multiple independent slots.
func (s *adminServiceImpl) refreshProxyBindingExits(ctx context.Context) error {
	r, e := s.proxyBindingStore()
	if e != nil {
		return e
	}
	proxies, e := s.proxyRepo.ListActive(ctx)
	if e != nil {
		return e
	}
	summaries, e := r.ProxyBindingSummaries(ctx)
	if e != nil {
		return e
	}
	verified := map[int64]bool{}
	for _, row := range summaries {
		verified[row.ProxyID] = row.ExitVerified
	}
	if s.proxyProber == nil {
		return errors.New("代理出口探测不可用，未分配代理")
	}
	// Reuse the verified ten-minute snapshot. Bound parallelism keeps large pools
	// from serially consuming eight seconds per record or flooding proxy servers.
	sem := make(chan struct{}, 4)
	failures := make(chan error, len(proxies))
	var wg sync.WaitGroup
	for _, p := range proxies {
		if verified[p.ID] || p.IsExpired(time.Now()) {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		}
		wg.Add(1)
		go func(p Proxy) {
			defer wg.Done()
			defer func() { <-sem }()
			probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			exit, _, err := s.proxyProber.ProbeProxy(probeCtx, p.URL())
			cancel()
			ip := ""
			if err == nil && exit != nil && net.ParseIP(exit.IP) != nil {
				ip = net.ParseIP(exit.IP).String()
			}
			if err = r.RememberProxyBindingExit(ctx, p.ID, ip, p.UpdatedAt); err != nil {
				failures <- err
			}
		}(p)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		return err
	}
	return ctx.Err()
}
