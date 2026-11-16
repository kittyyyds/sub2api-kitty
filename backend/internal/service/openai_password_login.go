package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

type OpenAIPasswordLoginInput struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	MFAMode   string `json:"mfa_mode"`
	MFASecret string `json:"mfa_secret"`
	ProxyID   *int64 `json:"proxy_id"`
	Remember  bool   `json:"remember"`
}

type OpenAIPasswordLoginJob struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Stage     string `json:"stage"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

type OpenAIPasswordLoginResult struct {
	TokenInfo      *OpenAITokenInfo `json:"token_info"`
	ProxyID        *int64           `json:"proxy_id"`
	RememberJobID  string           `json:"remember_job_id,omitempty"`
	RememberFailed bool             `json:"remember_failed,omitempty"`
}

type openAIPasswordLoginClient struct {
	endpoint string
	secret   string
	client   *http.Client
}

func newOpenAIPasswordLoginClient() *openAIPasswordLoginClient {
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_PASSWORD_LOGIN_URL")), "/")
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil
	}
	secret, err := os.ReadFile(os.Getenv("OPENAI_PASSWORD_LOGIN_SECRET_FILE"))
	if err != nil || len(strings.TrimSpace(string(secret))) < 32 {
		return nil
	}
	return &openAIPasswordLoginClient{endpoint: endpoint, secret: strings.TrimSpace(string(secret)), client: &http.Client{
		Timeout:       12 * time.Second,
		Transport:     &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 4, ResponseHeaderTimeout: 10 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func passwordLoginError(status int, code string) error {
	messages := map[string]string{
		"runtime_unavailable":         "账号登录组件暂时不可用，请稍后重试或使用授权链接。",
		"invalid_input":               "请检查邮箱、密码和 2FA 输入格式。",
		"proxy_required":              "所选登录代理不可用，未回退到直连。",
		"login_busy":                  "当前管理员已有登录任务，请等待完成或取消。",
		"capacity":                    "登录任务已满，请稍后重试。",
		"job_missing":                 "登录任务或授权结果已过期，请重新登录。",
		"code_not_expected":           "当前任务未等待验证码。",
		"password_rejected":           "上游拒绝了账号密码，请核对或改用授权链接。",
		"totp_rejected":               "2FA 验证失败，请检查密钥或使用新的验证码。",
		"preflight_blocked":           "登录出口预检被拒绝，尚未验证账号密码和 2FA，请检查出口或使用授权链接。",
		"access_denied":               "登录请求被上游拒绝访问，暂不能确认是否需要额外验证，请使用授权链接。",
		"email_verification_required": "上游另要求邮箱验证码，请使用授权链接完成邮箱验证。",
		"security_check":              "上游要求额外安全验证，请使用授权链接在官方页面完成。",
		"interaction_required":        "上游要求邮箱验证码或其他交互，请改用授权链接。",
		"phone_verification_required": "上游要求手机号验证，请在官方页面完成。",
		"account_deactivated":         "上游报告账号已停用或删除。",
		"rate_limited":                "上游限制了登录频率，请稍后重试。",
		"transport_error":             "登录连接失败，请检查所选出口。",
		"proxy_timeout":               "登录出口连接超时。",
		"timeout":                     "登录超时，请重新开始。",
		"canceled":                    "登录已取消。",
		"credentials_incomplete":      "登录未取得完整的 Codex OAuth 授权。",
		"identity_mismatch":           "登录邮箱与授权结果不一致，未保存账号。",
		"workspace_mismatch":          "登录工作区与授权结果不一致。",
		"callback_invalid":            "OAuth 回调校验失败。",
		"protocol_error":              "上游登录流程未完成，可重试或改用授权链接。",
		"saved_login_unavailable":     "保存的登录资料暂时不可用，请重新填写或稍后重试。",
		"saved_login_missing":         "此账号尚未保存登录资料，请先填写并勾选记住账号。",
	}
	message, ok := messages[code]
	if !ok {
		code = "runtime_unavailable"
		message = messages[code]
		status = http.StatusServiceUnavailable
	}
	return infraerrors.New(status, "OPENAI_PASSWORD_"+strings.ToUpper(code), message)
}

func (p *openAIPasswordLoginClient) call(ctx context.Context, body any, result any) error {
	if p == nil {
		return passwordLoginError(503, "runtime_unavailable")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return passwordLoginError(400, "invalid_input")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/v1/login", bytes.NewReader(data))
	if err != nil {
		return passwordLoginError(503, "runtime_unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+p.secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return passwordLoginError(503, "runtime_unavailable")
	}
	defer res.Body.Close()
	data, err = io.ReadAll(io.LimitReader(res.Body, 256*1024+1))
	if err != nil || len(data) > 256*1024 {
		return passwordLoginError(503, "runtime_unavailable")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var failure struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(data, &failure) != nil {
			return passwordLoginError(503, "runtime_unavailable")
		}
		status := res.StatusCode
		if status == 401 || status == 403 || status >= 500 {
			status = 503
		}
		return passwordLoginError(status, failure.Code)
	}
	if json.Unmarshal(data, result) != nil {
		return passwordLoginError(503, "runtime_unavailable")
	}
	return nil
}

var passwordLoginJobID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func (s *OpenAIOAuthService) PasswordLoginAvailable(ctx context.Context) bool {
	p := s.passwordLogin
	if p == nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+p.secret)
	res, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var result struct {
		Available bool `json:"available"`
	}
	return res.StatusCode == 200 && json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&result) == nil && result.Available
}

func (s *OpenAIOAuthService) StartPasswordLogin(ctx context.Context, ownerID int64, input OpenAIPasswordLoginInput) (*OpenAIPasswordLoginJob, error) {
	if ownerID <= 0 || len(input.Email) > 320 || len(input.Password) > 4096 || len(input.MFASecret) > 2048 {
		return nil, passwordLoginError(400, "invalid_input")
	}
	proxyURL := ""
	if input.ProxyID != nil {
		if *input.ProxyID <= 0 || s.proxyRepo == nil {
			return nil, passwordLoginError(400, "proxy_required")
		}
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err != nil || proxy == nil || !proxy.IsActive() || proxy.IsExpired(time.Now()) {
			return nil, passwordLoginError(400, "proxy_required")
		}
		proxyURL = proxy.URL()
		if proxyURL == "" {
			return nil, passwordLoginError(400, "proxy_required")
		}
	}
	var result OpenAIPasswordLoginJob
	err := s.passwordLogin.call(ctx, map[string]any{"action": "start", "owner_id": ownerID, "input": input, "proxy_id": input.ProxyID, "proxy_url": proxyURL}, &result)
	if err == nil && input.Remember {
		savedOpenAILoginMu.Lock()
		store := openAISavedLoginStore()
		store.prunePending()
		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		saveErr := store.write(savedOpenAILoginRecord{Scope: pendingOpenAILoginScope(ownerID, result.ID), Input: input, ExpiresAt: time.Now().Add(15 * time.Minute).Unix()})
		savedOpenAILoginMu.Unlock()
		if saveErr != nil {
			_, _ = s.PasswordLoginAction(ctx, ownerID, result.ID, "cancel", "")
			return nil, passwordLoginError(503, "saved_login_unavailable")
		}
	}
	return &result, err
}

func (s *OpenAIOAuthService) PasswordLoginAction(ctx context.Context, ownerID int64, jobID, action, code string) (*OpenAIPasswordLoginJob, error) {
	if ownerID <= 0 || !passwordLoginJobID.MatchString(jobID) || !slices.Contains([]string{"status", "cancel", "code"}, action) || (action == "code" && !regexp.MustCompile(`^\d{6}$`).MatchString(code)) {
		return nil, passwordLoginError(400, "invalid_input")
	}
	var result OpenAIPasswordLoginJob
	err := s.passwordLogin.call(ctx, map[string]any{"action": action, "owner_id": ownerID, "job_id": jobID, "code": code}, &result)
	if action == "cancel" || (err == nil && (result.Status == "failed" || result.Status == "canceled")) {
		savedOpenAILoginMu.Lock()
		_ = os.Remove(openAISavedLoginStore().file(pendingOpenAILoginScope(ownerID, jobID)))
		savedOpenAILoginMu.Unlock()
	}
	return &result, err
}

func (s *OpenAIOAuthService) TakePasswordLoginResult(ctx context.Context, ownerID int64, jobID string) (*OpenAIPasswordLoginResult, error) {
	if ownerID <= 0 || !passwordLoginJobID.MatchString(jobID) {
		return nil, passwordLoginError(400, "invalid_input")
	}
	var result struct {
		ProxyID    *int64 `json:"proxy_id"`
		Credential struct {
			AccessToken      string `json:"access_token"`
			RefreshToken     string `json:"refresh_token"`
			IDToken          string `json:"id_token"`
			Email            string `json:"email"`
			ChatGPTAccountID string `json:"chatgpt_account_id"`
			ExpiresAt        int64  `json:"expires_at"`
		} `json:"credential"`
	}
	if err := s.passwordLogin.call(ctx, map[string]any{"action": "result", "owner_id": ownerID, "job_id": jobID}, &result); err != nil {
		return nil, err
	}
	c := result.Credential
	claims, err := openai.ParseIDToken(c.IDToken)
	if err != nil || c.AccessToken == "" || c.RefreshToken == "" || c.ExpiresAt/1000 <= time.Now().Unix() || !slices.Contains(claims.Aud, openai.ClientID) {
		return nil, passwordLoginError(400, "credentials_incomplete")
	}
	u := claims.GetUserInfo()
	if !strings.EqualFold(u.Email, c.Email) || u.ChatGPTAccountID == "" || u.ChatGPTAccountID != c.ChatGPTAccountID {
		return nil, passwordLoginError(400, "identity_mismatch")
	}
	rememberJob := ""
	rememberFailed := false
	savedOpenAILoginMu.Lock()
	store := openAISavedLoginStore()
	pending, pendingErr := store.read(pendingOpenAILoginScope(ownerID, jobID))
	if pendingErr == nil && pending != nil && strings.EqualFold(pending.Input.Email, u.Email) {
		pending.Authorized = true
		pending.WorkspaceID = u.ChatGPTAccountID
		if store.write(*pending) == nil {
			rememberJob = jobID
		} else {
			rememberFailed = true
		}
	} else if pendingErr != nil || pending != nil {
		rememberFailed = true
	}
	savedOpenAILoginMu.Unlock()
	return &OpenAIPasswordLoginResult{RememberJobID: rememberJob, RememberFailed: rememberFailed, ProxyID: result.ProxyID, TokenInfo: &OpenAITokenInfo{
		AccessToken: c.AccessToken, RefreshToken: c.RefreshToken, IDToken: c.IDToken,
		ExpiresAt: c.ExpiresAt / 1000, ExpiresIn: c.ExpiresAt/1000 - time.Now().Unix(), ClientID: openai.ClientID,
		Email: u.Email, ChatGPTAccountID: u.ChatGPTAccountID, ChatGPTUserID: u.ChatGPTUserID, OrganizationID: u.OrganizationID, PlanType: u.PlanType,
	}}, nil
}
