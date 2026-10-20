package service

import "context"

// Deprecated input retained for older administrative clients. Account timezone
// preferences no longer select the login engine or override global policy.
type TwoFALoginEnvironment struct {
	Enabled  bool   `json:"enabled"`
	Mode     string `json:"mode"`
	Timezone string `json:"timezone"`
}

// The ordinary GPT 2FA importer always uses the pinned background protocol worker.
// It resolves and pins the selected proxy without a direct-network fallback.
func (s *OpenAIOAuthReauthService) TwoFAImportLogin(ctx context.Context, entry AccountTokenGuardReloginAccount, proxyID int64, _ *TwoFALoginEnvironment, _ ProxyExitInfoProber) (func(context.Context) (map[string]any, error), error) {
	return s.TwoFAProxyLogin(ctx, entry, proxyID)
}
