package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/codexwire"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/klauspost/compress/zstd"
)

// doOpenAIUpstream 按 IPv6 路由校验 → VM 桥 → VM 代理校验 → 智能 WS → codexwire/
// httpUpstream 的顺序选择出站；响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if request == nil || account == nil {
		return nil, errors.New("OpenAI request or account is missing")
	}
	// Restored native reasoning state is bound to the credential principal it
	// was produced under; refuse to send it under any other principal.
	if err := checkNativeReasoningRequestCredential(request); err != nil {
		return nil, err
	}
	if attempt, _ := request.Context().Value(groupSmartWSAttemptKey{}).(*groupSmartWSAttempt); attempt != nil && attempt.sent.Load() {
		return nil, finishGroupSmartWSAttempt(attempt, errSmartWSAlreadyDispatched)
	}
	if s.openaiIPv6 != nil {
		if err := s.openaiIPv6.CheckRoute(request.Context(), account, proxyURL); err != nil {
			return nil, err
		}
	}
	if err := validateOpenAIIPv6Route(account, proxyURL); err != nil {
		return nil, err
	}

	if s.openaiVM != nil {
		if response, handled, err := s.openaiVM.RoundTrip(request, account); handled {
			return response, err
		}
	}
	if err := validateOpenAIVMProxy(account, account.Proxy); err != nil {
		return nil, err
	}
	if response, handled, err := s.tryGroupSmartWS(request, proxyURL, account); handled {
		return response, err
	}
	if profile, ok, err := resolveCodexWireHTTPProfile(s.cfg, request, account); ok {
		if err != nil {
			return nil, err
		}
		request = prepareCodexWireHTTPRequest(request, account)
		return s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, profile)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}

// codexWireHTTPEligible 判断一次已经走到最终直连出站的请求能否改走 codexwire。
// 调用方必须已经依次排除 VM 桥和智能 WS；这里再兜底排除：
//   - 未开启 gateway.openai_ws.exact_fidelity_enabled；
//   - 非 openai 平台或非 OAuth/SetupToken 账号（API Key 保持 net/http）；
//   - VM 绑定/托管账号（VM 服务未注入时它们仍走原 Do 和 VM 出口）；
//   - Excel 客户端账号（不是 Codex CLI 身份）。
func codexWireHTTPEligible(cfg *config.Config, request *http.Request, account *Account) bool {
	if cfg == nil || !cfg.Gateway.OpenAIWS.ExactFidelityEnabled || request == nil || request.URL == nil {
		return false
	}
	if !account.usesOpenAICodexWire() || account.IsOpenAIVMBound() || account.IsOpenAIVMManaged() {
		return false
	}
	if strings.TrimSpace(account.GetCredential("client_id")) == openai.ExcelClientID {
		return false
	}
	return true
}

// resolveCodexWireHTTPProfile 返回 codexwire 出站使用的 TLS profile。
// ok=false 表示保持原 httpUpstream.Do；ok=true 且 err!=nil 表示自定义 CA 配置
// 无法加载，按官方客户端的行为直接失败而不是静默回退。
func resolveCodexWireHTTPProfile(cfg *config.Config, request *http.Request, account *Account) (*tlsfingerprint.Profile, bool, error) {
	if !codexWireHTTPEligible(cfg, request, account) {
		return nil, false, nil
	}
	profile := resolveCodexHTTPSTLSProfile(cfg, account)
	if profile == nil {
		// 账号显式 codex_tls_profile=none：保留 Go 默认栈。
		return nil, false, nil
	}
	if isCodexOAuthAccount(account) {
		withCA, err := codexwire.WithCodexCustomCA(profile)
		if err != nil {
			return nil, true, err
		}
		profile = withCA
	}
	return profile, true, nil
}

// prepareCodexWireHTTPRequest 标记 business 客户端（h2 时每请求独立连接）并
// 按官方 CLI 压缩 Responses 请求体。请求头内容仍由现有构建逻辑负责。
func prepareCodexWireHTTPRequest(request *http.Request, account *Account) *http.Request {
	// Each OAuth API attempt builds a fresh official HTTP client. Scope the
	// marker to this clone, including explicitly clearing it on other accounts.
	if business := codexBusinessHTTPAccount(account); business || CodexBusinessHTTPFromContext(request.Context()) {
		request = request.Clone(WithCodexBusinessHTTP(request.Context(), business))
	}
	compressCodexResponsesRequestBody(request)
	return request
}

// resolveCodexHTTPSTLSProfile 为 codex lane 的 HTTP 请求解析 TLS 指纹。
//
// 官方 Codex CLI（Linux）的 HTTP 出站走 reqwest default-tls → native-tls →
// 静态链接的 OpenSSL 3.6：不发 ALPN、走 HTTP/1.1、每个请求新建连接
// （codexgolden 实抓，testdata/golden/0.163.0）。因此默认档是 openssl36
// （codexwire 原始 h1 客户端），可用 SUB2API_CODEX_NATIVE_TLS_PROFILE 换成
// openssl35-gnu。
//
// 只有配置了自定义 CA（CODEX_CA_CERTIFICATE / SSL_CERT_FILE）时官方才切到
// rustls + ALPN [h2, http/1.1]；codex_http_h2_enabled=true 模拟这种部署
// （rustls-h2 + codexwire h2 多路复用）。账号 extra 的 codex_tls_profile /
// tls_fingerprint_builtin 优先；显式 "none" 返回 nil 走 Go 默认栈。
func resolveCodexHTTPSTLSProfile(cfg *config.Config, account *Account) *tlsfingerprint.Profile {
	if account != nil {
		if name := strings.TrimSpace(firstNonEmpty(account.GetExtraString("codex_tls_profile"), account.GetExtraString("tls_fingerprint_builtin"))); name != "" {
			if name == "none" {
				return nil
			}
			if profile := tlsfingerprint.BuiltinProfile(name); profile != nil {
				return profile
			}
		}
	}
	if (cfg != nil && cfg.Gateway.OpenAIWS.CodexHTTPH2Enabled) || (isCodexOAuthAccount(account) && codexwire.CodexCustomCAConfigured()) {
		return tlsfingerprint.BuiltinProfile("rustls-h2")
	}
	return codexwire.NativeHTTPProfile()
}

// compressCodexResponsesRequestBody 复刻官方 Codex CLI 的 HTTP 请求压缩：
// enable_request_compression 默认开启（codex-rs features/lib.rs:1449），
// ChatGPT OAuth + OpenAI provider 的 Responses 请求体用 zstd level 3 压缩并
// 设置 content-encoding: zstd（http-client/request.rs:209）。
// 压缩后重写 Body/GetBody，重试克隆请求可复放同样的压缩字节。
// 只在 codexwire 出站上调用：VM 桥和智能 WS 拿到的仍是原始请求体。
func compressCodexResponsesRequestBody(request *http.Request) {
	// 官方只在 Responses HTTP executor 压缩请求体（responses_request_compression）。
	if request == nil || request.URL == nil || !strings.HasSuffix(strings.TrimRight(request.URL.Path, "/"), "/responses") {
		return
	}
	if request.Body == nil || request.Body == http.NoBody {
		return
	}
	if strings.TrimSpace(request.Header.Get("Content-Encoding")) != "" {
		return
	}
	raw, err := io.ReadAll(request.Body)
	_ = request.Body.Close()
	restore := func(body []byte) {
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
		request.ContentLength = int64(len(body))
	}
	if err != nil || len(raw) == 0 {
		request.Body = http.NoBody
		request.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
		request.ContentLength = 0
		return
	}
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		restore(raw)
		return
	}
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		restore(raw)
		return
	}
	if err := zw.Close(); err != nil {
		restore(raw)
		return
	}
	restore(buf.Bytes())
	request.Header.Set("Content-Encoding", "zstd")
}

// doOpenAIAccountTestUpstream 让 OpenAI OAuth 账号测试与真实转发使用同一出站路径。
// API Key 账号保持各自原有的 HTTPUpstream 行为。
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	if s.openaiGatewayService != nil && s.openaiGatewayService.openaiIPv6 != nil {
		if err := s.openaiGatewayService.openaiIPv6.CheckRoute(request.Context(), account, proxyURL); err != nil {
			return nil, err
		}
	}
	if s.openaiVM != nil {
		started := time.Now().UTC()
		if response, handled, err := s.openaiVM.RoundTrip(request, account); handled {
			return observeAccountTestResponse(request.Context(), started, response, err)
		}
	}
	if err := validateOpenAIVMProxy(account, account.Proxy); err != nil {
		return nil, err
	}
	if err := validateOpenAIIPv6Route(account, proxyURL); err != nil {
		return nil, err
	}
	if profile, ok, err := resolveCodexWireHTTPProfile(s.cfg, request, account); ok {
		if err != nil {
			return nil, err
		}
		// 压缩放在预算登记之后：检测预算按原始请求体长度估算。
		return executeQualityTestHTTP(request, account.ID, func(r *http.Request) (*http.Response, error) {
			return s.httpUpstream.DoWithTLS(prepareCodexWireHTTPRequest(r, account), proxyURL, account.ID, account.Concurrency, profile)
		})
	}
	if useTLSFallback {
		return s.doAccountTestHTTPWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}

	return s.doAccountTestHTTP(request, proxyURL, account.ID, account.Concurrency)
}
