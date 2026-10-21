package reauthruntime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPasswordLoginDiagnosticSeparatesCausesWithoutLeakingOutput(t *testing.T) {
	for _, tc := range []struct{ output, stage, code string }{
		{"[proxy] 代理检测通过（第 1 次）\n[error] 代理检测失败：PROXY_SECURITY_CHECK_REQUIRES_INTERACTION secret-token", "security_verification", "protocol_security_verification_required"},
		{"[3/5] Password login page reached.\n[warn] Password was rejected. Enter it again, or q to quit.\n[error] readline was closed", "password", "protocol_password_rejected"},
		{"[mfa] TOTP 2FA challenge reached.\n[warn] The code generated from the configured 2FA key was rejected; enter a current code manually.", "totp", "protocol_totp_rejected"},
		{"[3/5] Email OTP page reached. Enter code, or type r to resend.\n[error] readline was closed", "email", "protocol_email_verification_required"},
		{"[error] Missing csrfToken", "navigation", "protocol_login_initialization_failed"},
		{"[error] 代理检测失败：curl: (60) certificate secret-token", "protocol_login", "proxy_tls_failed"},
		{"[error] 未找到可用的 Python curl_cffi 环境 secret-token", "protocol_login", "login_runtime_dependency_failed"},
		{"[error] curl: (7) Failed to connect secret-token", "protocol_login", "proxy_connect_failed"},
		{"[error] UNEXPECTED_LOGIN_PAGE: private-login-url", "protocol_login", "protocol_login_page_unexpected"},
		{"[6/6] Convert OAuth callback to sub2api import\n[error] unknown secret-token", "token_exchange", "protocol_login_failed"},
		{"arbitrary secret-token password-rejected http://secret-token", "protocol_login", "protocol_login_failed"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			d := &passwordLoginDiagnostic{}
			// Exercise process pipes splitting markers across writes.
			for _, b := range []byte(tc.output) {
				_, err := d.Write([]byte{b})
				require.NoError(t, err)
			}
			err := d.failure()
			require.Equal(t, tc.code, err.Progress.Code)
			require.Equal(t, tc.stage, err.Progress.Stage)
			require.NotContains(t, err.Error(), "secret-token")
			require.NotContains(t, err.Error(), "private-login-url")
			require.Empty(t, d.tail)
		})
	}
}

func TestPasswordLoginDiagnosticBoundsMemoryAndKeepsLastError(t *testing.T) {
	d := &passwordLoginDiagnostic{}
	raw := strings.Repeat("sensitive", passwordDiagnosticLimit)
	n, err := d.Write([]byte(raw))
	require.NoError(t, err)
	require.Equal(t, len(raw), n)
	require.LessOrEqual(t, len(d.tail), passwordDiagnosticLimit)
	d.Write([]byte("\n[error] PROXY_SECURITY_CHECK_REQUIRES_INTERACTION\n"))
	require.LessOrEqual(t, len(d.tail), passwordDiagnosticLimit)
	errLogin := d.failure()
	require.Equal(t, "protocol_security_verification_required", errLogin.Progress.Code)
	require.Zero(t, errLogin.Progress.HTTPStatus) // No guessed HTTP status.
	require.NotContains(t, errLogin.Error(), "sensitive")
}
