package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
)

// NewOpenAIOAuthClient creates a new OpenAI OAuth client
func NewOpenAIOAuthClient() service.OpenAIOAuthClient {
	return &openaiOAuthService{tokenURL: openai.TokenURL}
}

type openaiOAuthService struct {
	tokenURL string
}

func (s *openaiOAuthService) tokenURLForClient(clientID string) string {
	if clientID != openai.ExcelClientID {
		return s.tokenURL
	}
	u, err := url.Parse(s.tokenURL)
	if err != nil {
		return s.tokenURL
	}
	query := u.Query()
	query.Set("unified", "true")
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *openaiOAuthService) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}

	client, err := createOpenAICodeExchangeClient(proxyURL, clientID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	var tokenResp openai.TokenResponse

	request := client.R().
		SetContext(ctx).
		SetSuccessResult(&tokenResp)
	if clientID != openai.ExcelClientID {
		// codex-login's OAuthClient keeps this vector order when form-encoding.
		// QueryEscape differs from WHATWG form encoding for '*' and '~'.
		formValue := func(value string) string {
			encoded := url.QueryEscape(value)
			encoded = strings.ReplaceAll(encoded, "~", "%7E")
			return strings.ReplaceAll(encoded, "%2A", "*")
		}
		formBody := "grant_type=authorization_code&client_id=" + formValue(clientID) +
			"&code=" + formValue(code) + "&redirect_uri=" + formValue(redirectURI) +
			"&code_verifier=" + formValue(codeVerifier)
		// The official login client (codex-login OAuthClient) sends no
		// User-Agent/originator on the authorization-code grant; the
		// auth-code device lane (newCodexAuthCodeReqClient) matches it.
		request.SetHeader("Content-Type", "application/x-www-form-urlencoded").
			SetBodyString(formBody)
	} else {
		formData := url.Values{}
		formData.Set("grant_type", "authorization_code")
		formData.Set("client_id", clientID)
		formData.Set("code", code)
		formData.Set("redirect_uri", redirectURI)
		formData.Set("code_verifier", codeVerifier)
		request.SetFormDataFromValues(formData)
	}
	resp, err := request.Post(s.tokenURLForClient(clientID))

	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}

	if !resp.IsSuccessState() {
		if clientID == openai.ExcelClientID {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_EXCHANGE_FAILED", "Excel OAuth token exchange failed: status %d", resp.StatusCode)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_EXCHANGE_FAILED", "token exchange failed: status %d, body: %s", resp.StatusCode, resp.String())
	}

	return &tokenResp, nil
}

func (s *openaiOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
}

func (s *openaiOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	// 调用方应始终传入正确的 client_id；为兼容旧数据，未指定时默认使用 OpenAI ClientID
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}
	return s.refreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
}

func (s *openaiOAuthService) refreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL, clientID string) (*openai.TokenResponse, error) {
	client, err := createOpenAIRefreshClient(proxyURL, clientID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	var tokenResp openai.TokenResponse

	request := client.R().
		SetContext(ctx).
		SetSuccessResult(&tokenResp)
	if clientID != openai.ExcelClientID {
		// Codex 0.166 managed auth posts a JSON RefreshRequest in serde field
		// order (client_id, grant_type, refresh_reason, refresh_token) with the
		// account's device identity; there is no scope parameter.
		refreshBody, marshalErr := marshalCodexRefreshRequest(ctx, refreshToken, clientID)
		if marshalErr != nil {
			return nil, marshalErr
		}
		authUA, authOriginator := service.CodexAuthIdentityForContext(ctx)
		request.SetHeader("User-Agent", authUA).
			SetHeader("originator", authOriginator).
			SetHeader("Content-Type", "application/json").
			SetBodyBytes(refreshBody)
	} else {
		// The Excel add-in client keeps its form-encoded refresh contract.
		formData := url.Values{}
		formData.Set("grant_type", "refresh_token")
		formData.Set("refresh_token", refreshToken)
		formData.Set("client_id", clientID)
		request.SetFormDataFromValues(formData)
	}
	resp, err := request.Post(s.tokenURLForClient(clientID))

	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}

	if !resp.IsSuccessState() {
		return nil, service.NewCredentialOperationRefreshError(resp.StatusCode, []byte(resp.String()))
	}

	return &tokenResp, nil
}

// marshalCodexRefreshRequest encodes the official RefreshRequest. The refresh
// reason comes from the caller's trigger (openai.WithCodexRefreshReason);
// callers without one are explicit/manual refreshes ("forced").
func marshalCodexRefreshRequest(ctx context.Context, refreshToken, clientID string) ([]byte, error) {
	refreshReason := openai.CodexRefreshReasonFromContext(ctx)
	if refreshReason == "" {
		refreshReason = "forced"
	}
	return json.Marshal(struct {
		ClientID      string `json:"client_id"`
		GrantType     string `json:"grant_type"`
		RefreshReason string `json:"refresh_reason"`
		RefreshToken  string `json:"refresh_token"`
	}{ClientID: clientID, GrantType: "refresh_token", RefreshReason: refreshReason, RefreshToken: refreshToken})
}

// createOpenAICodeExchangeClient 为 Codex 登录换 token 选择客户端：Codex
// client_id 走官方登录客户端的 auth-code 设备通道（OpenSSL ClientHello、h1、
// 无 User-Agent/originator，与 codex-login OAuthClient 一致）；Excel client_id
// 不是 Codex CLI，保持原有共享 req 客户端。
func createOpenAICodeExchangeClient(proxyURL, clientID string) (*req.Client, error) {
	if clientID != openai.ExcelClientID && !codexDeviceClientDisabled() {
		return newCodexAuthCodeReqClient(proxyURL, 120*time.Second)
	}
	return createOpenAIReqClient(proxyURL)
}

// createOpenAIRefreshClient 为 refresh_token 调用选择客户端，规则同上。
func createOpenAIRefreshClient(proxyURL, clientID string) (*req.Client, error) {
	if clientID != openai.ExcelClientID && !codexDeviceClientDisabled() {
		// auth.openai.com token calls use the Codex CLI device lane.
		return newCodexDeviceReqClientWithAuthCA(proxyURL, 120*time.Second, true)
	}
	return createOpenAIReqClient(proxyURL)
}

func createOpenAIReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
	})
}

func shouldReturnOpenAINoProxyHint(ctx context.Context, proxyURL string, err error) bool {
	if strings.TrimSpace(proxyURL) != "" || err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return !errors.Is(err, context.Canceled)
}

func newOpenAINoProxyHintError(cause error) error {
	return infraerrors.New(
		http.StatusBadGateway,
		"OPENAI_OAUTH_PROXY_REQUIRED",
		"OpenAI OAuth request failed: no proxy is configured and this server could not reach OpenAI directly. Select a proxy that can access OpenAI, then retry; if the authorization code has expired, regenerate the authorization URL.",
	).WithCause(cause)
}

func (s *openaiOAuthService) RetireAuthClient(ctx context.Context, refreshToken, clientID string) {
	if id := openai.CodexAuthAccountFromContext(ctx); id > 0 {
		// A successful reload replaces the account's auth client, including
		// snapshots loaded concurrently with a different token or device.
		codexAuthH2Pool.Retire(fmt.Sprintf("auth-account-%d", id))
		return
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}
	body, _ := json.Marshal(map[string]string{"client_id": clientID, "grant_type": "refresh_token", "refresh_token": refreshToken})
	ua, originator := service.CodexAuthIdentityForContext(ctx)
	codexAuthH2Pool.Retire(codexAuthPoolKey(ctx, s.tokenURL, http.Header{"User-Agent": {ua}, "Originator": {originator}}, body))
}
