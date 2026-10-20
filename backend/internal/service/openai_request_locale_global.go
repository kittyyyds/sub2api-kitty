package service

import "context"

// Compatibility aliases for older internal integrations; both use one parser.
func normalizeUpstreamOpenAIRequestLocale(ctx context.Context, account *Account, body []byte, transport string) []byte {
	return normalizeOpenAIRequestLocale(ctx, account, body, transport)
}
func normalizeUpstreamOpenAIRequestLocaleTimezone(ctx context.Context, account *Account, body []byte, transport, timezone string) []byte {
	return normalizeOpenAIRequestLocaleTimezone(ctx, account, body, transport, timezone)
}
