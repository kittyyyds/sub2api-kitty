package reauthruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Progress contains allowlisted metadata only. BrowserID never enters the public job.
type LoginProgress struct {
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	Code       string `json:"code,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	BrowserID  string `json:"-"`
}
type progressKey struct{}

func WithLoginProgress(ctx context.Context, report func(LoginProgress)) context.Context {
	return context.WithValue(ctx, progressKey{}, report)
}
func ReportLoginProgress(ctx context.Context, p LoginProgress) {
	if f, ok := ctx.Value(progressKey{}).(func(LoginProgress)); ok {
		f(p)
	}
}

var loginCodes = map[string]string{
	"protocol_login_failed":                   "后台登录进程未完成，尚未确定具体原因；这不代表密码或 2FA 密钥错误，请联系管理员核查。",
	"protocol_security_verification_required": "所选代理访问官方登录页面时遇到安全验证，后台登录已停止；这不代表密码或 2FA 错误。请通过官方登录流程完成验证后再试，避免连续重试。",
	"protocol_password_rejected":              "官方密码验证未通过，请核对密码后再试。",
	"protocol_totp_rejected":                  "根据所填 2FA 密钥生成的验证码未通过，请核对密钥及验证码时间后再试。",
	"protocol_email_verification_required":    "官方要求邮箱验证码，当前后台 2FA 导入无法自动完成此步骤，请使用交互式 OAuth 授权。",
	"protocol_login_page_unexpected":          "官方返回了后台登录程序未识别的页面，请使用交互式 OAuth 授权或由管理员检查登录流程兼容性。",
	"protocol_login_initialization_failed":    "登录初始化未取得必要的会话信息，尚未验证密码和 2FA，请由管理员检查登录流程。",
	"login_runtime_dependency_failed":         "服务器后台登录运行环境缺少必要依赖，请由管理员修复；无需修改账号密码。",
	"proxy_tls_failed":                        "所选代理的 TLS 连接或证书验证失败，未改用直连；请检查代理连接和证书配置。",
	"authorization_rejected":                  "官方授权页面拒绝访问，请查看登录窗口与 HTTP 状态。",
	"login_failed":                            "登录未完成，请查看失败阶段。",
	"browser_unavailable":                     "服务器登录浏览器不可用，请稍后重试。",
	"browser_busy":                            "登录浏览器繁忙，请稍后重试。",
	"proxy_connect_failed":                    "所选代理连接失败，未回退直连。",
	"security_verification_required":          "官方要求安全验证，请打开验证窗口完成。",
	"interaction_required":                    "官方页面需要你确认，请打开登录窗口。",
	"timezone_probe_failed":                   "无法确定所选代理时区，尚未提交登录凭据。",
	"invalid_timezone":                        "登录时区无效，请重新选择。",
	"timezone_requires_proxy":                 "登录时区设置需要选择账号代理。",
	"login_timeout":                           "登录会话已超时，请重新开始。",
	"login_canceled":                          "登录已取消。",
	"password_rejected":                       "密码验证未通过，请在登录窗口核对。",
	"totp_rejected":                           "两步验证码未通过，请在登录窗口核对。",
	"token_exchange_failed":                   "授权凭证交换失败，请重新授权。",
	"account_identity_mismatch":               "授权账号与待导入账号不一致，未导入。",
	"invalid_oauth_callback":                  "授权回调校验失败，未导入。",
	"credential_save_failed":                  "登录完成，但账号凭据保存失败。",
}
var loginStages = map[string]bool{"queued": true, "protocol_login": true, "timezone_probe": true, "browser_starting": true, "navigation": true, "security_verification": true, "email": true, "password": true, "totp": true, "authorization": true, "token_exchange": true, "succeeded": true, "failed": true}

func SafeLoginProgress(p LoginProgress) LoginProgress {
	if !loginStages[p.Stage] {
		p.Stage = "failed"
	}
	if _, ok := loginCodes[p.Code]; !ok && p.Code != "" {
		p.Code = "login_failed"
	}
	if p.HTTPStatus < 100 || p.HTTPStatus > 599 {
		p.HTTPStatus = 0
	}
	if p.Status != "running" && p.Status != "waiting_interaction" && p.Status != "succeeded" {
		p.Status = "failed"
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil || len(p.Timezone) > 80 {
		p.Timezone = ""
	}
	if p.Status == "failed" && p.Code == "" {
		p.Code = "login_failed"
	}
	return p
}
func LoginMessage(code string) string {
	if s, ok := loginCodes[code]; ok {
		return s
	}
	return loginCodes["login_failed"]
}

type LoginError struct{ Progress LoginProgress }

func (e *LoginError) Error() string { return LoginMessage(e.Progress.Code) }
func LoginFailure(err error) LoginProgress {
	var e *LoginError
	if errors.As(err, &e) {
		return SafeLoginProgress(e.Progress)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return LoginProgress{Status: "failed", Code: "login_timeout"}
	}
	if errors.Is(err, context.Canceled) {
		return LoginProgress{Status: "failed", Code: "login_canceled"}
	}
	return LoginProgress{Status: "failed", Code: "login_failed"}
}

// This URL is server configuration, never a value accepted from a login form.
type browserClient struct {
	base, token string
	client      *http.Client
}

func newBrowserClient() (*browserClient, error) {
	base := strings.TrimRight(os.Getenv("KITTY_LOGIN_BROWSER_URL"), "/")
	token := os.Getenv("KITTY_LOGIN_BROWSER_TOKEN")
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 {
		return nil, &LoginError{Progress: LoginProgress{Status: "failed", Code: "browser_unavailable"}}
	}
	return &browserClient{base: base, token: token, client: &http.Client{Timeout: 40 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (b *browserClient) call(ctx context.Context, method, path string, data any) ([]byte, int, error) {
	var body io.Reader
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, &LoginError{Progress: LoginProgress{Status: "failed", Code: "browser_unavailable"}}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, resp.StatusCode, &LoginError{Progress: LoginProgress{Status: "failed", Code: "browser_unavailable"}}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code := "browser_unavailable"
		if resp.StatusCode == 429 {
			code = "browser_busy"
		}
		return nil, resp.StatusCode, &LoginError{Progress: LoginProgress{Status: "failed", Code: code}}
	}
	return raw, resp.StatusCode, nil
}

var browserIDPattern = regexp.MustCompile(`^[a-f0-9]{48}$`)

func BrowserAction(ctx context.Context, id, action string, input any) ([]byte, error) {
	if !browserIDPattern.MatchString(id) || (action != "frame" && action != "input" && action != "resume") {
		return nil, errors.New("登录窗口不存在或已结束")
	}
	b, err := newBrowserClient()
	if err != nil {
		return nil, err
	}
	method := http.MethodPost
	if action == "frame" {
		method = http.MethodGet
	}
	raw, _, err := b.call(ctx, method, "/sessions/"+id+"/"+action, input)
	return raw, err
}
func BrowserPasswordLogin(ctx context.Context, email, password, totp, proxy, timezone string) (map[string]any, error) {
	b, err := newBrowserClient()
	if err != nil {
		return nil, err
	}
	ReportLoginProgress(ctx, LoginProgress{Status: "running", Stage: "browser_starting", Timezone: timezone})
	raw, _, err := b.call(ctx, http.MethodPost, "/sessions", map[string]string{"email": email, "password": password, "totp": totp, "proxy": proxy, "timezone": timezone})
	if err != nil {
		return nil, err
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &created) != nil || !browserIDPattern.MatchString(created.ID) {
		return nil, &LoginError{Progress: LoginProgress{Status: "failed", Code: "browser_unavailable"}}
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _, _ = b.call(c, http.MethodDelete, "/sessions/"+created.ID, nil)
	}()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		raw, _, err = b.call(ctx, http.MethodGet, "/sessions/"+created.ID, nil)
		if err != nil {
			return nil, err
		}
		var status struct {
			LoginProgress
			Credential map[string]any `json:"credential"`
		}
		if json.Unmarshal(raw, &status) != nil {
			return nil, &LoginError{Progress: LoginProgress{Status: "failed", Code: "browser_unavailable"}}
		}
		p := SafeLoginProgress(status.LoginProgress)
		p.BrowserID = created.ID
		p.Timezone = timezone
		ReportLoginProgress(ctx, p)
		switch p.Status {
		case "succeeded":
			for _, key := range []string{"access_token", "refresh_token", "id_token"} {
				s, _ := status.Credential[key].(string)
				if s == "" {
					return nil, &LoginError{Progress: LoginProgress{Status: "failed", Stage: "token_exchange", Code: "token_exchange_failed"}}
				}
			}
			returnedEmail, _ := status.Credential["email"].(string)
			if !strings.EqualFold(returnedEmail, email) {
				return nil, &LoginError{Progress: LoginProgress{Status: "failed", Stage: "token_exchange", Code: "account_identity_mismatch"}}
			}
			return status.Credential, nil
		case "failed":
			return nil, &LoginError{Progress: p}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
