package service

import (
	"context"

	coderws "github.com/coder/websocket"
)

// rejectRetiredBPSCredentialWebSocket keeps retired BPS credentials off every
// WebSocket route; they are not a Codex CLI identity.
func rejectRetiredBPSCredentialWebSocket(account *Account) error {
	if account.IsRetiredBPSCredential() {
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "Excel OAuth / BPS credentials are retired", errRetiredBPSCredential)
	}
	return nil
}

func openAIAccountProxyURL(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

// resolveOpenAIWSAccountProxy returns the account's own exit for a WebSocket
// handshake after the same IPv6 route checks the HTTP path applies.
func (s *OpenAIGatewayService) resolveOpenAIWSAccountProxy(ctx context.Context, account *Account) (string, error) {
	proxyURL := openAIAccountProxyURL(account)
	if s.openaiIPv6 != nil {
		if err := s.openaiIPv6.CheckRoute(ctx, account, proxyURL); err != nil {
			return "", err
		}
	}
	if err := validateOpenAIIPv6Route(account, proxyURL); err != nil {
		return "", err
	}
	return proxyURL, nil
}
