//go:build unit

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func performAuthHandlerJSON(t *testing.T, path string, body any, handle func(*gin.Context)) response.Response {
	t.Helper()
	gin.SetMode(gin.TestMode)
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	ginCtx.Request = req

	handle(ginCtx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusBadRequest, resp.Code)
	return resp
}

func TestLogin2FA_ExpiredSessionReturnsReasonCode(t *testing.T) {
	h := &AuthHandler{
		totpService: service.NewTotpService(nil, nil, &oauthPendingFlowTotpCacheStub{}, nil, nil, nil),
	}

	resp := performAuthHandlerJSON(t, "/api/v1/auth/login/2fa", map[string]string{
		"temp_token": "expired-or-unknown-temp-token",
		"totp_code":  "123456",
	}, h.Login2FA)

	require.Equal(t, "TOTP_LOGIN_SESSION_EXPIRED", resp.Reason)
	require.Equal(t, "Invalid or expired 2FA session", resp.Message)
}

// 密码超长必须在人机验证（一次性票据）之前就被拒绝；这里 authService 为 nil，
// 若处理器继续走到验证码校验就会 panic。
func TestRegister_PasswordTooLongRejectedBeforeCaptcha(t *testing.T) {
	h := &AuthHandler{}

	resp := performAuthHandlerJSON(t, "/api/v1/auth/register", map[string]string{
		"email":       "user@example.com",
		"password":    strings.Repeat("a", 73),
		"verify_code": "123456",
	}, h.Register)

	require.Equal(t, "PASSWORD_TOO_LONG", resp.Reason)
}

func TestCreatePendingOAuthAccount_PasswordTooLongRejectedBeforeCaptcha(t *testing.T) {
	h := &AuthHandler{}

	resp := performAuthHandlerJSON(t, "/api/v1/auth/oauth/pending/create-account", map[string]string{
		"email":       "user@example.com",
		"password":    strings.Repeat("a", 73),
		"verify_code": "123456",
	}, func(c *gin.Context) { h.createPendingOAuthAccount(c, "oidc") })

	require.Equal(t, "PASSWORD_TOO_LONG", resp.Reason)
}
