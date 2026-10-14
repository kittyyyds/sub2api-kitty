package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/reauthruntime"
	"strings"
	"time"
)

type TwoFALoginEnvironment struct {
	Enabled  bool   `json:"enabled"`
	Mode     string `json:"mode"`
	Timezone string `json:"timezone"`
}

// Resolve the exact proxy once. Detection and login share this immutable URL.
func (s *OpenAIOAuthReauthService) TwoFAImportLogin(ctx context.Context, entry AccountTokenGuardReloginAccount, proxyID int64, environment *TwoFALoginEnvironment, prober ProxyExitInfoProber) (func(context.Context) (map[string]any, error), error) {
	if environment == nil {
		return s.TwoFAProxyLogin(ctx, entry, proxyID)
	} // Older clients keep the existing protocol.
	if err := ValidateOpenAITwoFALogin(entry); err != nil {
		return nil, err
	}
	if s == nil || s.accounts == nil {
		return nil, errors.New("代理服务不可用")
	}
	proxyURL, err := s.reauthProxyURL(ctx, &proxyID)
	if err != nil || proxyURL == "" {
		return nil, errors.New("所选代理不可用；尚未提交登录凭据")
	}
	snapshot := *environment
	if snapshot.Enabled && (snapshot.Mode != "" && snapshot.Mode != "manual" && snapshot.Mode != "proxy") {
		return nil, errors.New("登录时区模式无效")
	}
	if snapshot.Enabled && snapshot.Mode != "proxy" && !openAIRequestTimezoneAllowed[snapshot.Timezone] {
		return nil, errors.New("登录时区无效")
	}
	return func(c context.Context) (map[string]any, error) {
		timezone := "UTC"
		if snapshot.Enabled {
			timezone = snapshot.Timezone
			if snapshot.Mode == "proxy" {
				reauthruntime.ReportLoginProgress(c, reauthruntime.LoginProgress{Status: "running", Stage: "timezone_probe"})
				if prober == nil {
					return nil, &reauthruntime.LoginError{Progress: reauthruntime.LoginProgress{Status: "failed", Stage: "timezone_probe", Code: "timezone_probe_failed"}}
				}
				pc, cancel := context.WithTimeout(c, 15*time.Second)
				info, _, probeErr := prober.ProbeProxy(pc, proxyURL)
				cancel()
				if probeErr != nil || info == nil || !openAIRequestTimezoneAllowed[info.Timezone] {
					return nil, &reauthruntime.LoginError{Progress: reauthruntime.LoginProgress{Status: "failed", Stage: "timezone_probe", Code: "timezone_probe_failed"}}
				}
				timezone = info.Timezone
			}
		}
		return reauthruntime.BrowserPasswordLogin(c, strings.ToLower(strings.TrimSpace(entry.Email)), entry.Password, strings.TrimSpace(entry.MFASecret), proxyURL, timezone)
	}, nil
}
