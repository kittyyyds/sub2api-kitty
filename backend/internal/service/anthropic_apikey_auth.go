package service

import (
	"net/http"
	"strings"
)

const (
	anthropicAPIKeyAuthSchemeExtraKey = "anthropic_apikey_auth_scheme"

	AnthropicAPIKeyAuthSchemeXAPIKey             = "x_api_key"
	AnthropicAPIKeyAuthSchemeAuthorizationBearer = "authorization_bearer"
)

// GetAnthropicAPIKeyAuthScheme returns the upstream authentication scheme for
// Anthropic API-key accounts. Missing or invalid values keep the historical
// x-api-key behavior. CN providers using their native Anthropic endpoints
// (api_protocol=anthropic) share the same override knob — Kimi/DeepSeek default
// to x-api-key, Zhipu can opt into Authorization: Bearer.
func (a *Account) GetAnthropicAPIKeyAuthScheme() string {
	if a == nil || a.Type != AccountTypeAPIKey {
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}
	if a.Platform != PlatformAnthropic && !a.IsCNProvider() {
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}

	switch strings.TrimSpace(a.GetExtraString(anthropicAPIKeyAuthSchemeExtraKey)) {
	case AnthropicAPIKeyAuthSchemeAuthorizationBearer:
		return AnthropicAPIKeyAuthSchemeAuthorizationBearer
	default:
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}
}

func setAnthropicAPIKeyAuthHeader(header http.Header, account *Account, token string) error {
	// Never accept a customer-supplied account selection credential.
	for name := range header {
		if strings.EqualFold(name, CursorKeyHeader) {
			delete(header, name)
		}
	}
	if err := ValidateCursorAccount(account); err != nil {
		return err
	}
	if account.IsCursor() {
		_, bridgeKey, err := cursorBridgeConfig()
		if err != nil {
			return err
		}
		for name := range header {
			if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "x-api-key") || strings.EqualFold(name, "x-opencodex-api-key") {
				delete(header, name)
			}
		}
		header.Set("x-api-key", bridgeKey)
		header.Set(CursorKeyHeader, strings.TrimSpace(account.GetCredential("api_key")))
		return nil
	}
	if account.GetAnthropicAPIKeyAuthScheme() == AnthropicAPIKeyAuthSchemeAuthorizationBearer {
		header.Set("Authorization", "Bearer "+token)
		return nil
	}
	header.Set("x-api-key", token)
	return nil
}
