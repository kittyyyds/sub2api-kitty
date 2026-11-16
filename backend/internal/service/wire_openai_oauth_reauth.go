package service

import "github.com/Wei-Shaw/sub2api/internal/config"

// The re-login worker only served Excel authorization for the removed Excel
// BPS route; the service keeps its configuration, credential-encryption and
// worker endpoints but no longer queues tasks (see CreateTaskForProfile).
func ProvideOpenAIOAuthReauthService(
	repo OpenAIOAuthReauthRepository,
	adminService AdminService,
	oauth *OpenAIOAuthService,
	encryptor OpenAICredentialEncryptor,
	cfg *config.Config,
	tokenCacheInvalidator TokenCacheInvalidator,
	runtimeBlocker AccountRuntimeBlocker,
	buildInfo BuildInfo,
	settings SettingRepository,
) *OpenAIOAuthReauthService {
	configured := cfg != nil && cfg.Totp.EncryptionKeyConfigured
	svc := NewOpenAIOAuthReauthService(repo, adminService, nil, oauth, encryptor, configured, tokenCacheInvalidator, runtimeBlocker)
	svc.settings = settings
	svc.configureWorker(cfg, buildInfo)
	return svc
}
