package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const proxyTimezoneTTL = 15 * time.Minute
const proxyTimezoneFailureTTL = 30 * time.Second

// ProxyTimezoneResult contains only exit metadata, never proxy credentials.
type ProxyTimezoneResult struct {
	Success   bool      `json:"success"`
	Reason    string    `json:"reason"`
	Timezone  string    `json:"timezone,omitempty"`
	IPAddress string    `json:"ip_address,omitempty"`
	Country   string    `json:"country,omitempty"`
	City      string    `json:"city,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Cached    bool      `json:"cached"`
}

type proxyTimezoneResolver struct {
	prober  ProxyExitInfoProber
	mu      sync.Mutex
	cache   map[string]ProxyTimezoneResult
	flights singleflight.Group
	slots   chan struct{}
	now     func() time.Time
}

func newProxyTimezoneResolver(prober ProxyExitInfoProber) *proxyTimezoneResolver {
	return &proxyTimezoneResolver{prober: prober, cache: make(map[string]ProxyTimezoneResult), slots: make(chan struct{}, 4), now: time.Now}
}

func (r *proxyTimezoneResolver) cached(key string) (ProxyTimezoneResult, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.cache[key]
	if ok && r.now().Before(v.ExpiresAt) {
		v.Cached = true
		return v, true
	}
	return ProxyTimezoneResult{}, false
}

func (r *proxyTimezoneResolver) resolve(ctx context.Context, proxy *Proxy, force bool) ProxyTimezoneResult {
	if proxy == nil {
		return ProxyTimezoneResult{Reason: "no_proxy"}
	}
	if r == nil || r.prober == nil {
		return ProxyTimezoneResult{Reason: "probe_unavailable"}
	}
	if !proxy.IsActive() || proxy.IsExpired(r.now()) {
		return ProxyTimezoneResult{Reason: "proxy_unavailable"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// The full URL fingerprint includes authentication/session selectors. An
	// edited proxy or a new rotation username must never reuse the old result.
	raw := proxy.URL()
	sum := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(sum[:])
	if !force {
		if v, ok := r.cached(key); ok {
			return v
		}
	}
	result := r.flights.DoChan(key, func() (any, error) {
		if !force {
			if v, ok := r.cached(key); ok {
				return v, nil
			}
		}
		// Shared probes survive a canceled first caller, but are always bounded.
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		select {
		case r.slots <- struct{}{}:
			defer func() { <-r.slots }()
		case <-probeCtx.Done():
			return ProxyTimezoneResult{Reason: "probe_busy"}, nil
		}
		info, _, err := r.prober.ProbeProxy(probeCtx, raw)
		now := r.now()
		v := ProxyTimezoneResult{Reason: "probe_failed", CheckedAt: now, ExpiresAt: now.Add(proxyTimezoneFailureTTL)}
		if err == nil && info != nil {
			if net.ParseIP(info.IP) != nil && openAIRequestTimezoneAllowed[info.Timezone] {
				if _, loadErr := time.LoadLocation(info.Timezone); loadErr == nil {
					v.Success = true
					v.Reason = "detected"
					v.Timezone = info.Timezone
					v.IPAddress = info.IP
					v.Country = info.Country
					v.City = info.City
					v.ExpiresAt = now.Add(proxyTimezoneTTL)
				}
			}
			if !v.Success {
				v.Reason = "timezone_unknown"
			}
		}
		r.mu.Lock()
		// Keep memory bounded even when many distinct rotating credentials exist.
		if len(r.cache) >= 1024 {
			for k, old := range r.cache {
				if !now.Before(old.ExpiresAt) {
					delete(r.cache, k)
				}
			}
			if len(r.cache) >= 1024 {
				for k := range r.cache {
					delete(r.cache, k)
					break
				}
			}
		}
		r.cache[key] = v
		r.mu.Unlock()
		return v, nil
	})
	select {
	case <-ctx.Done():
		return ProxyTimezoneResult{Reason: "request_canceled"}
	case result := <-result:
		return result.Val.(ProxyTimezoneResult)
	}
}

// ProbeOpenAIProxyTimezone is used by the admin preview and the forwarding path.
func (s *OpenAIGatewayService) ProbeOpenAIProxyTimezone(ctx context.Context, proxy *Proxy, force bool) ProxyTimezoneResult {
	if s == nil {
		return ProxyTimezoneResult{Reason: "probe_unavailable"}
	}
	return s.proxyTimezone.resolve(ctx, proxy, force)
}

func (s *OpenAIGatewayService) normalizeOpenAIRequestLocale(ctx context.Context, account *Account, body []byte, transport string, now time.Time) []byte {
	if account == nil || !account.OpenAIRequestTimezoneEnabled() || account.Extra[openAIRequestTimezoneModeExtraKey] != "proxy" {
		return normalizeOpenAIRequestLocale(ctx, account, body, transport, now)
	}
	// Use the selected account's proxy for each attempt, never a global/default
	// proxy or the client's IP. Missing/failed detection preserves client data.
	if account.ProxyID == nil || account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
		return body
	}
	result := s.ProbeOpenAIProxyTimezone(ctx, account.Proxy, false)
	if !result.Success {
		return body
	}
	// Scheduler accounts are shared between requests. Never mutate their Extra
	// or persist a detected timezone over the user's saved manual preference.
	copyAccount := *account
	copyAccount.Extra = make(map[string]any, len(account.Extra))
	for k, v := range account.Extra {
		copyAccount.Extra[k] = v
	}
	copyAccount.Extra[openAIRequestTimezoneExtraKey] = result.Timezone
	copyAccount.Extra[openAIRequestTimezoneModeExtraKey] = "manual"
	return normalizeOpenAIRequestLocale(ctx, &copyAccount, body, transport, now)
}
