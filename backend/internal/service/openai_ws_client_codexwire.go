package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/codexwire"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
)

// codexWireClientConn wraps codexwire.WSConn to satisfy openAIWSClientConn and
// the optional pool interfaces used by the v2 forwarder. Error mapping:
// ErrClosedByPeer surfaces as coderws.CloseError so the existing close-status
// classifiers keep working.
type codexWireClientConn struct {
	conn *codexwire.WSConn
}

var _ openAIWSClientConn = (*codexWireClientConn)(nil)
var _ openaiwsv2.FrameConn = (*codexWireClientConn)(nil)

func (c *codexWireClientConn) WriteJSON(ctx context.Context, value any) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.WriteJSON(ctx, value)
}

func (c *codexWireClientConn) ReadMessage(ctx context.Context) ([]byte, error) {
	if c == nil || c.conn == nil {
		return nil, errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := c.conn.ReadMessage(ctx)
	if err != nil {
		return nil, mapCodexWireReadError(err)
	}
	return payload, nil
}

func (c *codexWireClientConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.conn == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opcode, payload, err := c.conn.ReadFrame(ctx)
	if err != nil {
		return coderws.MessageText, nil, mapCodexWireReadError(err)
	}
	if opcode == 2 {
		return coderws.MessageBinary, payload, nil
	}
	return coderws.MessageText, payload, nil
}

func (c *codexWireClientConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if msgType == coderws.MessageBinary {
		return c.conn.WriteMessageBinary(ctx, payload)
	}
	return c.conn.WriteMessage(ctx, payload)
}

func (c *codexWireClientConn) Ping(ctx context.Context) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.Ping(ctx)
}

// RequiresReaderLoop: control frames are only consumed inside ReadMessage,
// same contract as the coder/websocket conn.
func (*codexWireClientConn) RequiresReaderLoop() bool { return true }

func (*codexWireClientConn) SupportsIdlePingWithoutReader() bool { return false }

func (c *codexWireClientConn) UpstreamPingCount() int64 {
	if c == nil || c.conn == nil {
		return 0
	}
	return c.conn.UpstreamPingCount()
}

func (c *codexWireClientConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	// Codex Responses drops its WebSocket owner without sending Close.
	// Live has a separate adapter that explicitly sends code 1000.
	_ = c.conn.CloseNow()
	return nil
}

func (c *codexWireClientConn) CloseNow() error {
	if c == nil || c.conn == nil {
		return nil
	}
	_ = c.conn.CloseNow()
	return nil
}

func mapCodexWireReadError(err error) error {
	var peerErr *codexwire.ErrClosedByPeer
	if errors.As(err, &peerErr) && peerErr != nil {
		return coderws.CloseError{
			Code:   coderws.StatusCode(peerErr.Code),
			Reason: peerErr.Reason,
		}
	}
	if errors.Is(err, codexwire.ErrClosed) {
		return errOpenAIWSConnClosed
	}
	return err
}

// codexWireTLSProfileName resolves the ClientHello profile for the codex lane.
// Account extras may override ("codex_tls_profile": "rustls"|"nodejs24"|"none").
func codexWireTLSProfileName(account *Account) string {
	if account != nil {
		if name := account.GetExtraString("codex_tls_profile"); name != "" {
			return name
		}
	}
	return "rustls"
}

// codexWireWSEligible 只放行走通用上游 WS 拨号的 openai 平台 OAuth/SetupToken
// 账号。VM 路由（上下文里带 VM HTTP 客户端，或账号已绑定/托管 VM）、Excel
// 客户端账号以及显式明文代理路由保持原 coder/websocket 拨号。
func codexWireWSEligible(ctx context.Context, cfg *config.Config, req openAIWSAcquireRequest) bool {
	if cfg == nil || !cfg.Gateway.OpenAIWS.ExactFidelityEnabled {
		return false
	}
	account := req.Account
	if !account.usesOpenAICodexWire() || account.IsOpenAIVMBound() || account.IsOpenAIVMManaged() {
		return false
	}
	if strings.TrimSpace(account.GetCredential("client_id")) == openai.ExcelClientID {
		return false
	}
	if openAIVMWSHTTPClientFromContext(ctx) != nil {
		return false
	}
	target, _ := url.Parse(req.WSURL)
	// The raw lane tunnels TLS only. Keep explicit plaintext proxy routes on
	// the existing dialer rather than silently bypassing their configured proxy.
	plaintextProxy := target != nil && (target.Scheme == "ws" || target.Scheme == "http") && strings.TrimSpace(req.ProxyURL) != ""
	return !plaintextProxy
}

// dialOpenAIWSForAccount shares the exact Codex lane across pool and passthrough.
// Returns conn + status + handshake response headers, matching the signature of
// openAIWSClientDialer.Dial. The proxy is always the account's own proxy URL
// (including IPv6 lease proxies); codexwire tunnels http/https/socks5 itself.
func dialOpenAIWSForAccount(ctx context.Context, cfg *config.Config, req openAIWSAcquireRequest, headers http.Header, fallback openAIWSClientDialer) (openAIWSClientConn, int, http.Header, error) {
	if !codexWireWSEligible(ctx, cfg, req) {
		return fallback.Dial(ctx, req.WSURL, headers, req.ProxyURL)
	}
	ordered, leftover := codexwire.SplitCodexWSHeaders(headers)
	// 未命中模板头（cookie/attestation 等模板外注入头）按出现顺序附在官方头之后。
	ordered = append(ordered, leftover...)
	if openAIWSCompressionDisabled(ctx) {
		// This private context is set only by the Live sideband entry. Its
		// handshake has a distinct header contract from Responses.
		ordered = codexwire.OrderCodexLiveWSHeaders(headers)
	}

	var profile *tlsfingerprint.Profile
	if name := codexWireTLSProfileName(req.Account); name != "none" {
		profile = tlsfingerprint.BuiltinProfile(name)
		if profile == nil {
			profile = tlsfingerprint.BuiltinProfile("rustls")
		}
	}
	if isCodexOAuthAccount(req.Account) {
		var err error
		profile, err = codexwire.WithCodexCustomCA(profile)
		if err != nil {
			return nil, 0, nil, err
		}
	}
	timeout := 15 * time.Second
	if cfg.Gateway.OpenAIWS.DialTimeoutSeconds > 0 {
		timeout = time.Duration(cfg.Gateway.OpenAIWS.DialTimeoutSeconds) * time.Second
	}
	wsConn, res, err := codexwire.DialWS(ctx, req.WSURL, ordered, req.ProxyURL, codexwire.DialOptions{
		DisableCompression: openAIWSCompressionDisabled(ctx),
		TLSProfile:         profile,
		HandshakeTimeout:   timeout,
	})
	if err != nil {
		status := 0
		respHeaders := http.Header(nil)
		var body []byte
		if res != nil {
			status = res.StatusCode
			respHeaders = res.Headers
			body = res.Body
		}
		return nil, status, respHeaders, &openAIWSHandshakeError{Body: body, Err: err}
	}
	return &codexWireClientConn{conn: wsConn}, 0, res.Headers, nil
}

// Realtime uses default WebSocket extensions; Responses explicitly opts into
// permessage-deflate. Keep that protocol choice scoped to the dial context.
type openAIWSCompressionDisabledKey struct{}

func withoutOpenAIWSCompression(ctx context.Context) context.Context {
	return context.WithValue(ctx, openAIWSCompressionDisabledKey{}, true)
}

func openAIWSCompressionDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	disabled, _ := ctx.Value(openAIWSCompressionDisabledKey{}).(bool)
	return disabled
}
