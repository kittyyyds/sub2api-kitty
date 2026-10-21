package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

type accountEgressSettings interface {
	GetValue(context.Context, string) (string, error)
}

func accountEgressSyncEnabled(ctx context.Context, settings accountEgressSettings) bool {
	if settings == nil {
		return false
	}
	for _, key := range []string{SettingKeyOpenAIRequestTimezoneEnabled, SettingKeyOpenAIRequestTimezoneAuto} {
		value, err := settings.GetValue(ctx, key)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			return false
		}
		if strings.TrimSpace(value) != "" && value != "true" {
			return false
		}
	}
	return true
}
func (s *OpenAIGatewayService) strictAccountEgress(ctx context.Context, a *Account) bool {
	return s != nil && s.settingService != nil && a != nil && a.IsOpenAI() && a.ProxyID != nil && *a.ProxyID > 0 && s.settingService.GetOpenAIRequestTimezoneEnabled(ctx, false) && s.settingService.GetOpenAIRequestTimezoneAuto(ctx, false)
}
func (s *OpenAIGatewayService) validateAccountEgress(ctx context.Context, a *Account, proxy string) error {
	if err := ValidateConfiguredAccountProxy(a, proxy); err != nil && (a.Proxy == nil || s.strictAccountEgress(ctx, a)) {
		return err
	}
	if s.strictAccountEgress(ctx, a) && (a.Proxy == nil || a.Proxy.URL() != proxy) {
		return errors.New("全局请求跟随已开启，本次出口与账号绑定代理不一致；未切换其他代理或直连")
	}
	return nil
}

type accountLivenessProbeKey struct{}

func (s *OpenAIGatewayService) ProbeOpenAIModels(ctx context.Context, a *Account) (*OpenAIModelsResponse, error) {
	return s.FetchOpenAIModelsList(context.WithValue(ctx, accountLivenessProbeKey{}, true), a)
}
func classifyAccountProbeError(err error) (string, string) {
	if err == nil {
		return AccountTokenGuardProbeOK, "credential accepted"
	}
	var upstream *codexModelsManifestUpstreamError
	if errors.As(err, &upstream) && upstream.statusCode == http.StatusUnauthorized {
		return AccountTokenGuardProbeAuth, "credential rejected (401)"
	}
	// Network errors and proxy/WAF 403 responses are not evidence of revoked credentials.
	return AccountTokenGuardProbeTransient, "inspection failed; credentials were not classified as revoked"
}
func (s *AccountTokenGuardService) probeUsingAccountEgress(ctx context.Context, cfg AccountTokenGuardConfig, a *Account) AccountTokenGuardProbeResult {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ProbeTimeoutSeconds)*time.Second)
	defer cancel()
	_, err := s.gateway.ProbeOpenAIModels(ctx, a)
	state, detail := classifyAccountProbeError(err)
	return AccountTokenGuardProbeResult{State: state, Detail: detail, LatencyMS: int(time.Since(started).Milliseconds()), Diagnostic: AccountTokenGuardDiagnostic{Code: state, EndedAt: guardTimePtr(time.Now())}}
}
func (s *AccountTokenGuardService) reloginUsingAccountEgress(ctx context.Context, cfg AccountTokenGuardConfig, a *Account, entry AccountTokenGuardReloginAccount) (map[string]any, error) {
	if s.gateway != nil && s.gateway.strictAccountEgress(ctx, a) {
		if s.reauth == nil || a.ProxyID == nil {
			return nil, errors.New("账号代理重登运行时不可用，未转交外部服务")
		}
		login, err := s.reauth.TwoFAProxyLogin(ctx, entry, *a.ProxyID)
		if err != nil {
			return nil, err
		}
		return login(ctx)
	}
	return s.relogin(ctx, cfg, entry)
}
func (s *OpenAIOAuthReauthService) useAccountEgress(ctx context.Context, a *Account) bool {
	return a != nil && a.IsOpenAI() && a.ProxyID != nil && *a.ProxyID > 0 && accountEgressSyncEnabled(ctx, s.settings)
}

// Keep this check at the boundary, after every route has selected its actual exit.
func validateBoundTimezoneEgress(ctx context.Context, b *requestTimezoneBinding, proxy string) error {
	if b == nil || b.service == nil {
		return nil
	}
	return b.service.validateAccountEgress(ctx, b.account, strings.TrimSpace(proxy))
}
