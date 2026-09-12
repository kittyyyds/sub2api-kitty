package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const openAIClientProfileEgressKey = "openai_client_profile_egress"

// This is a point-in-time observation through the configured route, not an
// attestation that a model request used this IP or that a timezone was sent.
type OpenAIClientProfileEgress struct {
	AccountID    int64     `json:"account_id"`
	ProxyID      *int64    `json:"proxy_id"`
	ProxyName    string    `json:"proxy_name"`
	Source       string    `json:"source"`
	IP           string    `json:"ip"`
	City         string    `json:"city"`
	Region       string    `json:"region"`
	Country      string    `json:"country"`
	CountryCode  string    `json:"country_code"`
	Timezone     string    `json:"timezone"`
	TimezoneSent bool      `json:"timezone_sent"`
	ObservedAt   time.Time `json:"observed_at"`
	LatencyMS    int64     `json:"latency_ms"`
	Binding      string    `json:"binding"`
}

func (s *adminServiceImpl) resolveClientProfileEgressRoute(ctx context.Context, id int64, selected *int64) (*Account, *Proxy, string, error) {
	a, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, "", err
	}
	if a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth || a.IsCredentialShadow() {
		return nil, nil, "", infraerrors.BadRequest("CLIENT_PROFILE_EGRESS_TARGET", "仅支持独立的 GPT OAuth 账号")
	}
	pool := a.GetProxyPool()
	var p *Proxy
	if len(pool) > 0 {
		chosen := pool[0]
		if selected != nil {
			chosen = *selected
		} else if len(pool) > 1 {
			return nil, nil, "", infraerrors.BadRequest("CLIENT_PROFILE_EGRESS_SELECT", "此账号有多个代理，请选择要检测的代理")
		}
		found := false
		for _, proxyID := range pool {
			if proxyID == chosen {
				found = true
				break
			}
		}
		if !found {
			return nil, nil, "", infraerrors.BadRequest("CLIENT_PROFILE_EGRESS_BINDING", "只能检测当前账号绑定的代理")
		}
		p, err = s.proxyRepo.GetByID(ctx, chosen)
		if err != nil {
			return nil, nil, "", err
		}
		if !p.IsActive() || p.IsExpired(time.Now()) {
			return nil, nil, "", infraerrors.BadRequest("CLIENT_PROFILE_EGRESS_UNAVAILABLE", "代理已停用或过期；检测不会自动改为直连")
		}
	} else if selected != nil && *selected != 0 {
		return nil, nil, "", infraerrors.BadRequest("CLIENT_PROFILE_EGRESS_BINDING", "此账号当前未绑定代理")
	}
	return a, p, clientProfileEgressBinding(a, p), nil
}

func clientProfileEgressBinding(a *Account, p *Proxy) string {
	// Include proxy configuration to expire an observation when a proxy keeps
	// its ID but its host/authentication changes. Only the digest is exposed.
	material := fmt.Sprintf("%d|%v|direct", a.ID, a.GetProxyPool())
	if p != nil {
		material += fmt.Sprintf("|%d|%s|%s", p.ID, p.URL(), p.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	h := sha256.Sum256([]byte(material))
	return hex.EncodeToString(h[:])
}

func (s *adminServiceImpl) GetOpenAIClientProfileEgress(ctx context.Context, id int64) (*OpenAIClientProfileEgress, error) {
	a, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, exists := a.Extra[openAIClientProfileEgressKey]
	if !exists {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, nil
	}
	var snapshot OpenAIClientProfileEgress
	if json.Unmarshal(data, &snapshot) != nil || snapshot.AccountID != id {
		return nil, nil
	}
	_, _, binding, err := s.resolveClientProfileEgressRoute(ctx, id, snapshot.ProxyID)
	if err != nil || binding != snapshot.Binding {
		return nil, nil
	}
	return &snapshot, nil
}

func (s *adminServiceImpl) ProbeOpenAIClientProfileEgress(ctx context.Context, id int64, selected *int64) (*OpenAIClientProfileEgress, error) {
	a, p, binding, err := s.resolveClientProfileEgressRoute(ctx, id, selected)
	if err != nil {
		return nil, err
	}
	if s.proxyProber == nil {
		return nil, infraerrors.InternalServer("CLIENT_PROFILE_EGRESS_UNAVAILABLE", "出口检测服务未配置")
	}
	proxyURL := ""
	result := &OpenAIClientProfileEgress{AccountID: a.ID, Source: "direct", ProxyName: "服务器直连", Binding: binding}
	if p != nil {
		proxyURL = p.URL()
		proxyID := p.ID
		result.ProxyID = &proxyID
		result.ProxyName = p.Name
		result.Source = "proxy"
	}
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	info, latency, err := s.proxyProber.ProbeProxy(probeCtx, proxyURL)
	if err != nil || info == nil || net.ParseIP(info.IP) == nil {
		// Probe errors can contain an authenticated proxy URL. Never echo them.
		return nil, infraerrors.BadRequest("CLIENT_PROFILE_EGRESS_FAILED", "出口检测失败，请检查代理及检测服务连通性；未回退直连")
	}
	_, _, currentBinding, err := s.resolveClientProfileEgressRoute(ctx, id, result.ProxyID)
	if err != nil || currentBinding != binding {
		return nil, infraerrors.BadRequest("CLIENT_PROFILE_EGRESS_CHANGED", "检测期间代理配置已改变，请重新检测")
	}
	result.IP = info.IP
	result.City = info.City
	result.Region = info.Region
	result.Country = info.Country
	result.CountryCode = info.CountryCode
	zone := strings.TrimSpace(info.Timezone)
	if zone != "" && zone != "Local" {
		if _, err := time.LoadLocation(zone); err == nil {
			result.Timezone = zone
		}
	}
	result.ObservedAt = time.Now().UTC()
	result.LatencyMS = latency
	if err = s.accountRepo.UpdateExtra(ctx, id, map[string]any{openAIClientProfileEgressKey: result}); err != nil {
		return nil, err
	}
	return result, nil
}
