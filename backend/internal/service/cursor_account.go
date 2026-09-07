package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const CursorAuthMode = "user_api_key"
const CursorKeyHeader = "X-Kitty-Cursor-Api-Key"

var cursorUserKeyPattern = regexp.MustCompile(`^crsr_[A-Za-z0-9_-]{8,512}$`)

func (a *Account) IsCursor() bool {
	return a != nil && a.Platform == PlatformAnthropic && a.Type == AccountTypeAPIKey && a.GetCredential("cursor_auth_mode") == CursorAuthMode
}

// The bridge destination is operator-controlled. Account forms cannot redirect
// Cursor credentials or the bridge admission secret to a supplied URL.
func cursorBridgeConfig() (string, string, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_URL")), "/")
	key := strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_API_KEY"))
	u, err := url.Parse(base)
	if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || key == "" {
		return "", "", infraerrors.ServiceUnavailable("CURSOR_BRIDGE_NOT_CONFIGURED", "Cursor 网关尚未配置，请联系站点维护者完成首次连接")
	}
	return base, key, nil
}

func ValidateCursorAccount(a *Account) error {
	if a == nil || a.Credentials == nil {
		return nil
	}
	if _, present := a.Credentials["cursor_auth_mode"]; !present {
		if strings.HasPrefix(strings.TrimSpace(a.GetCredential("api_key")), "crsr_") {
			return infraerrors.BadRequest("CURSOR_ACCOUNT_TYPE_REQUIRED", "Cursor 用户 Key 请通过 Anthropic 下的 Cursor 账号类型添加")
		}
		return nil
	}
	if !a.IsCursor() {
		return infraerrors.BadRequest("INVALID_CURSOR_ACCOUNT_TYPE", "Cursor 账号仅支持 Anthropic 下的用户 API Key 接入")
	}
	if !cursorUserKeyPattern.MatchString(strings.TrimSpace(a.GetCredential("api_key"))) {
		return infraerrors.BadRequest("INVALID_CURSOR_API_KEY", "请填写 crsr_ 开头的 Cursor 用户 API Key，不能填写密码或中转站 Key")
	}
	if strings.TrimSpace(a.GetCredential("base_url")) != "" {
		return infraerrors.BadRequest("CURSOR_BASE_URL_MANAGED", "Cursor 网关地址由站点统一管理，无需填写 Base URL")
	}
	if a.ProxyID != nil || len(a.GetProxyPool()) > 0 {
		return infraerrors.BadRequest("CURSOR_PROXY_UNSUPPORTED", "当前 Cursor 请求使用网关服务器出口，不支持账号级代理")
	}
	return nil
}

var cursorBridgeHTTPClient = &http.Client{
	Timeout:       20 * time.Second,
	Transport:     &http.Transport{Proxy: nil, MaxIdleConns: 16, IdleConnTimeout: 60 * time.Second},
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

type CursorBridgeStatus struct {
	Available bool   `json:"available"`
	Message   string `json:"message"`
}

// Only status is returned. Upstream response bodies and credentials are never
// reflected into administrator errors or request logs.
func requestCursorBridge(ctx context.Context, path, userKey string) (bool, int, error) {
	base, key, err := cursorBridgeConfig()
	if err != nil {
		return false, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, nil)
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if userKey != "" {
		req.Header.Set(CursorKeyHeader, userKey)
	}
	res, err := cursorBridgeHTTPClient.Do(req)
	if err != nil {
		return false, 0, infraerrors.ServiceUnavailable("CURSOR_BRIDGE_UNREACHABLE", "暂时无法连接 Cursor 网关，请稍后重试")
	}
	defer res.Body.Close()
	var body struct {
		MultiAccount  bool `json:"multi_account"`
		Authenticated bool `json:"authenticated"`
	}
	if res.StatusCode != http.StatusOK {
		return false, res.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&body); err != nil {
		return false, res.StatusCode, infraerrors.ServiceUnavailable("CURSOR_BRIDGE_UPGRADE_REQUIRED", "Cursor 网关需要升级至支持可视化账号管理的版本")
	}
	if userKey != "" {
		return body.Authenticated, res.StatusCode, nil
	}
	return body.MultiAccount, res.StatusCode, nil
}

func GetCursorBridgeStatus(ctx context.Context) CursorBridgeStatus {
	ok, _, err := requestCursorBridge(ctx, "/v1/cursor/status", "")
	if err != nil {
		return CursorBridgeStatus{Message: err.Error()}
	}
	if !ok {
		return CursorBridgeStatus{Message: "Cursor 网关暂不可用或尚未升级，请联系站点维护者"}
	}
	return CursorBridgeStatus{Available: true, Message: "网关已就绪，可添加 Cursor 用户 API Key"}
}

func VerifyCursorUserKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if !cursorUserKeyPattern.MatchString(key) {
		return infraerrors.BadRequest("INVALID_CURSOR_API_KEY", "请填写有效的 crsr_ 开头的 Cursor 用户 API Key")
	}
	ok, status, err := requestCursorBridge(ctx, "/v1/cursor/verify", key)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return infraerrors.BadRequest("CURSOR_AUTH_REJECTED", "Cursor 认证未通过，请检查 Key 是否有效以及账号权限")
	}
	if !ok {
		return infraerrors.ServiceUnavailable("CURSOR_AUTH_UNAVAILABLE", "Cursor 认证暂未成功，请检查网关状态后重试")
	}
	return nil
}
