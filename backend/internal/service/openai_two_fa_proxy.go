package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/reauthruntime"
	"strings"
)

// Resolve the selected proxy before starting a job; never substitute a direct
// connection or the legacy external service when resolution/login fails.
func (s *OpenAIOAuthReauthService) TwoFAProxyLogin(ctx context.Context, entry AccountTokenGuardReloginAccount, proxyID int64) (func(context.Context) (map[string]any, error), error) {
	if err := ValidateOpenAITwoFALogin(entry); err != nil {
		return nil, err
	}
	if s == nil || s.accounts == nil || s.worker == nil {
		return nil, errors.New("代理登录需要内置登录运行时；未切换为服务器直连")
	}
	proxyURL, err := s.reauthProxyURL(ctx, &proxyID)
	if err != nil || proxyURL == "" {
		return nil, errors.New("所选代理不可用；未创建直连登录任务")
	}
	s.EnsureWorker()
	return func(c context.Context) (map[string]any, error) {
		reauthruntime.ReportLoginProgress(c, reauthruntime.LoginProgress{Status: "running", Stage: "protocol_login"})
		return s.worker.PasswordLogin(c, strings.ToLower(strings.TrimSpace(entry.Email)), entry.Password, strings.TrimSpace(entry.MFASecret), proxyURL)
	}, nil
}

func (s *AccountTokenGuardService) StartTwoFAProxyLogin(ctx context.Context, entry AccountTokenGuardReloginAccount, login func(context.Context) (map[string]any, error)) (*OpenAITwoFALoginJob, error) {
	if login == nil {
		return nil, errors.New("代理登录服务不可用")
	}
	return s.startTwoFALogin(ctx, entry, false, login)
}
