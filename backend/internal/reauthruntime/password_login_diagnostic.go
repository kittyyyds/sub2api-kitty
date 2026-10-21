package reauthruntime

import (
	"strings"
	"sync"
)

// Keep a bounded tail in memory only. Raw runtime output can contain session
// URLs, cookies and credentials; it must never be logged or sent to the client.
type passwordLoginDiagnostic struct {
	mu   sync.Mutex
	tail []byte
}

const passwordDiagnosticLimit = 32 << 10

func (d *passwordLoginDiagnostic) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(p)
	if len(p) >= passwordDiagnosticLimit {
		d.tail = append(d.tail[:0], p[len(p)-passwordDiagnosticLimit:]...)
	} else {
		if excess := len(d.tail) + len(p) - passwordDiagnosticLimit; excess > 0 {
			copy(d.tail, d.tail[excess:])
			d.tail = d.tail[:len(d.tail)-excess]
		}
		d.tail = append(d.tail, p...)
	}
	return n, nil
}

func (d *passwordLoginDiagnostic) failure() *LoginError {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := LoginProgress{Status: "failed", Stage: "protocol_login", Code: "protocol_login_failed"}
	for _, line := range strings.Split(string(d.tail), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "[proxy] 代理检测通过"):
			p.Stage = "navigation"
		case line == "[3/5] Password login page reached.":
			p.Stage = "password"
		case line == "[mfa] TOTP 2FA challenge reached.":
			p.Stage = "totp"
		case line == "[2/5] Start Codex OAuth flow":
			p.Stage = "authorization"
		case line == "[6/6] Convert OAuth callback to sub2api import":
			p.Stage = "token_exchange"
		case line == "[warn] Password was rejected. Enter it again, or q to quit.":
			p.Stage, p.Code = "password", "protocol_password_rejected"
		case line == "[warn] The code generated from the configured 2FA key was rejected; enter a current code manually.":
			p.Stage, p.Code = "totp", "protocol_totp_rejected"
		case strings.HasPrefix(line, "[3/5] Email OTP page reached."):
			p.Stage, p.Code = "email", "protocol_email_verification_required"
		}
		// Only recognize the reviewed worker's error markers. Never return a
		// copied error line, URL, response body or purported provider explanation.
		if strings.HasPrefix(line, "[error] ") || strings.HasPrefix(line, "[proxy] 代理连接失败：") {
			switch {
			case strings.Contains(line, "PROXY_SECURITY_CHECK_REQUIRES_INTERACTION"):
				p.Stage, p.Code = "security_verification", "protocol_security_verification_required"
			case strings.Contains(line, "Python curl_cffi"), strings.Contains(line, "Cannot find module"), strings.Contains(line, "ERR_MODULE_NOT_FOUND"):
				p.Code = "login_runtime_dependency_failed"
			case strings.Contains(line, "curl: (60)"), strings.Contains(line, "curl: (35)"):
				p.Code = "proxy_tls_failed"
			case strings.Contains(line, "curl: (5)"), strings.Contains(line, "curl: (6)"), strings.Contains(line, "curl: (7)"), strings.Contains(line, "curl: (28)"), strings.Contains(line, "CONNECT tunnel failed, response 407"):
				p.Code = "proxy_connect_failed"
			case strings.HasPrefix(line, "[error] UNEXPECTED_LOGIN_PAGE:"):
				p.Code = "protocol_login_page_unexpected"
			case strings.HasPrefix(line, "[error] Missing csrfToken"), strings.HasPrefix(line, "[error] Missing __Host-next-auth.csrf-token"), strings.HasPrefix(line, "[error] signin/openai did not return url"):
				p.Stage, p.Code = "navigation", "protocol_login_initialization_failed"
			}
		}
	}
	clear(d.tail)
	d.tail = nil
	return &LoginError{Progress: SafeLoginProgress(p)}
}
