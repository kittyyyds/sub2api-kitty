package vm2api

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/vmbridge"
	"net/http"
)

func (p *Provider) ManagedProxyCapacity(ctx context.Context, urls []string) ([]vmbridge.ProxyCapacity, error) {
	var snapshot struct {
		Config struct {
			BindLimit int `json:"bind_limit"`
		} `json:"config"`
	}
	if err := p.client.control(ctx, http.MethodGet, "/api/panel/proxies", "proxy_capacity", nil, &snapshot); err != nil {
		return nil, err
	}
	limit := snapshot.Config.BindLimit
	if limit < 1 || limit > 32 {
		return nil, &APIError{Operation: "proxy_capacity", Code: "invalid_limit"}
	}
	out := make([]vmbridge.ProxyCapacity, 0, len(urls))
	for _, raw := range urls {
		wanted, err := parseSocksProxy(raw)
		if err != nil {
			return nil, err
		}
		proxy, err := p.client.findProxy(ctx, wanted)
		if err != nil && proxy == nil {
			return nil, err
		}
		c := vmbridge.ProxyCapacity{Limit: limit, Available: true}
		if proxy != nil {
			c.Used = len(proxy.BoundVMIDs)
			c.Available = proxy.Enabled && err == nil
		}
		out = append(out, c)
	}
	return out, nil
}
