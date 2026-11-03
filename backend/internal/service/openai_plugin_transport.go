package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (result *http.Response, resultErr error) {
	if err := ValidateConfiguredAccountProxy(account, proxyURL); err != nil {
		return nil, err
	}
	if s.openAIOAuthReauth != nil {
		if err := s.openAIOAuthReauth.validateAuthConnectionPolicy(request.Context(), account); err != nil {
			return nil, err
		}
	}
	if account != nil && account.AuthConnectionID != "" {
		if s.openAIOAuthReauth == nil {
			return nil, fmt.Errorf("authorization connection service unavailable")
		}
		if err := s.openAIOAuthReauth.ValidateAuthConnectionSend(request.Context(), account); err != nil {
			return nil, err
		}
	}
	var phaseGuard *openAIHTTPPhaseGuard
	request, phaseGuard = s.guardOpenAIHTTPRequest(request, account)
	request, finishHeaderTimeout := guardOpenAIResponseHeaderTimeout(request, account)
	defer func() {
		if result != nil && result.Request == nil {
			result.Request = request
		}
		result, resultErr = phaseGuard.finish(result, resultErr)
		resultErr = finishHeaderTimeout(result, resultErr)
	}()
	if phaseGuard != nil && phaseGuard.ctx.Err() != nil {
		return nil, context.Cause(phaseGuard.ctx)
	}
	request, safetyInfo := s.applySafetyPromptToRequest(request, account)
	defer func() {
		if result != nil {
			result = s.wrapSafetyPromptResponse(result, safetyInfo)
		}
	}()
	if account.IsSIWCEnabled() {
		if request.URL == nil || request.URL.Scheme != "https" || request.URL.Host != "api.openai.com" || request.URL.Path != "/v1/responses" {
			return nil, fmt.Errorf("SIWC request must use the official Responses endpoint")
		}
		// Compatibility adapters can add internal headers after building a request.
		// Enforce SIWC headers at the final send boundary as well.
		authorization := request.Header.Get("Authorization")
		applySIWCRequestHeaders(request)
		request.Host = ""
		if err := s.priorityLatencyBeforeSend(request.Context(), account); err != nil {
			return nil, err
		}
		if err := s.fenceAuthConnectionEgress(request.Context(), request, account); err != nil {
			return nil, err
		}
		observeAttempt := s.observePriorityAttempt(request, account)
		resp, err := s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
		observeAttempt(resp, err)
		if err == nil && resp != nil && resp.StatusCode == http.StatusUnauthorized && s.openAIOAuthReauth != nil {
			var cache OpenAITokenCache
			if s.openAITokenProvider != nil {
				cache = s.openAITokenProvider.tokenCache
			}
			s.openAIOAuthReauth.rejectSIWCAccessToken(request.Context(), account, strings.TrimPrefix(authorization, "Bearer "), cache)
		}
		return resp, err
	}
	return s.doUpstreamWithProxyFallback(request.Context(), request, account, proxyURL)
}

// doOpenAIAccountTestUpstream applies the same transport lifecycle as production forwarding.
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
	_ []byte,
) (*http.Response, error) {
	if err := ValidateConfiguredAccountProxy(account, proxyURL); err != nil {
		return nil, err
	}
	gateway := s.openaiGatewayService
	if gateway != nil {
		request = request.WithContext(gateway.withRequestTimezoneEgress(request.Context(), account))
		var err error
		request, err = BindOpenAIRequestTimezone(request, proxyURL)
		if err != nil {
			return nil, err
		}
	}
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback {
		return timedAccountTestUpstream(s.httpUpstream).DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return timedAccountTestUpstream(s.httpUpstream).Do(request, proxyURL, account.ID, account.Concurrency)
}
