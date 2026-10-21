package reauthruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// PasswordLogin uses the already verified, release-pinned login runtime. Secrets
// are passed via the child's private environment, never command line or logs.
// A selected proxy is mandatory here; failure never falls back to direct login.
func (m *Manager) PasswordLogin(ctx context.Context, email, password, totp, proxy string) (map[string]any, error) {
	u, err := url.Parse(proxy)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
		return nil, errors.New("invalid login proxy")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	select {
	case m.loginSlots <- struct{}{}:
		defer func() { <-m.loginSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m.Ensure()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for m.Status().State != "running" {
		if m.Status().State == "unavailable" || m.Status().State == "stopped" {
			return nil, errors.New("login runtime unavailable")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
	dir := filepath.Join(m.root, m.version+"-linux-"+runtime.GOARCH)
	return passwordLoginProcess(ctx, dir, email, password, totp, proxy)
}

func passwordLoginCommand(ctx context.Context, dir, tmp, email, password, totp, proxy string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "node"), filepath.Join(tmp, "src", "protocol-login.mjs"), "--email", email, "--output-mode", "sub2api", "--sub2api-out", filepath.Join(tmp, "oauth.json"), "--sub2api-name", "twofa-import")
	cmd.Dir = tmp
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "PYTHONDONTWRITEBYTECODE=1", "HOME=" + tmp, "TMPDIR=" + tmp,
		"NODE_PATH=" + filepath.Join(dir, "tosub2", "node_modules"),
		"TOSUB2_PYTHON=" + filepath.Join(dir, "python"), "NODE_EXECUTABLE=" + filepath.Join(dir, "node"),
		"CHATGPT_LOGIN_PASSWORD=" + password, "CHATGPT_TOTP_SECRET=" + totp, "CHATGPT_PROXY_URL=" + proxy, "CHATGPT_PROXY_MAX_ATTEMPTS=1"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	configureProcess(cmd)
	return cmd
}
func passwordLoginProcess(ctx context.Context, dir, email, password, totp, proxy string) (map[string]any, error) {
	tmp, err := os.MkdirTemp("", "sub2api-twofa-")
	if err != nil {
		return nil, errors.New("login temporary directory unavailable")
	}
	defer os.RemoveAll(tmp)
	if err := preparePinnedLoginSource(dir, tmp); err != nil {
		return nil, errors.New("verified fixed-proxy login source unavailable")
	}
	cmd := passwordLoginCommand(ctx, dir, tmp, email, password, totp, proxy)
	diagnostic := &passwordLoginDiagnostic{}
	cmd.Stdout = diagnostic
	cmd.Stderr = diagnostic
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, diagnostic.failure()
	}
	f, err := os.Open(filepath.Join(tmp, "oauth.json"))
	if err != nil {
		return nil, errors.New("login result unavailable")
	}
	defer f.Close()
	var result struct {
		Accounts []struct {
			Credentials map[string]any `json:"credentials"`
		} `json:"accounts"`
	}
	if err = json.NewDecoder(io.LimitReader(f, 2<<20)).Decode(&result); err != nil || len(result.Accounts) != 1 {
		return nil, errors.New("invalid login result")
	}
	creds := result.Accounts[0].Credentials
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		value, _ := creds[key].(string)
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("incomplete login result")
		}
	}
	return creds, nil
}
