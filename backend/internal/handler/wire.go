package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
)

// ProvideAdminHandlers creates the AdminHandlers struct
func ProvideAdminHandlers(
	performance *service.AccountPerformanceService,
	dashboardHandler *admin.DashboardHandler,
	userHandler *admin.UserHandler,
	userCleanupHandler *admin.UserCleanupHandler,
	groupHandler *admin.GroupHandler,
	accountHandler *admin.AccountHandler,
	announcementHandler *admin.AnnouncementHandler,
	dataManagementHandler *admin.DataManagementHandler,
	backupHandler *admin.BackupHandler,
	oauthHandler *admin.OAuthHandler,
	openaiOAuthHandler *admin.OpenAIOAuthHandler,
	openaiReauthHandler *admin.OpenAIOAuthReauthHandler,
	geminiOAuthHandler *admin.GeminiOAuthHandler,
	antigravityOAuthHandler *admin.AntigravityOAuthHandler,
	grokOAuthHandler *admin.GrokOAuthHandler,
	cnProviderHandler *admin.CNProviderHandler,
	proxyHandler *admin.ProxyHandler,
	redeemHandler *admin.RedeemHandler,
	promoHandler *admin.PromoHandler,
	settingHandler *admin.SettingHandler,
	opsHandler *admin.OpsHandler,
	systemHandler *admin.SystemHandler,
	subscriptionHandler *admin.SubscriptionHandler,
	usageHandler *admin.UsageHandler,
	userAttributeHandler *admin.UserAttributeHandler,
	errorPassthroughHandler *admin.ErrorPassthroughHandler,
	tlsFingerprintProfileHandler *admin.TLSFingerprintProfileHandler,
	pluginHandler *admin.PluginHandler,
	openaiVMHandler *admin.OpenAIVMHandler,
	openaiVMManagedHandler *admin.OpenAIVMManagedHandler,
	openaiIPv6Handler *admin.OpenAIIPv6Handler,
	apiKeyHandler *admin.AdminAPIKeyHandler,
	scheduledTestHandler *admin.ScheduledTestHandler,
	channelQualityHandler *admin.ChannelQualityHandler,
	channelHandler *admin.ChannelHandler,
	channelMonitorHandler *admin.ChannelMonitorHandler,
	channelMonitorTemplateHandler *admin.ChannelMonitorRequestTemplateHandler,

	billingExportHandler *admin.BillingExportHandler,

	paymentHandler *admin.PaymentHandler,
	affiliateHandler *admin.AffiliateHandler,
	complianceHandler *admin.ComplianceHandler,
	auditLogHandler *admin.AuditLogHandler,
	upstreamBillingProbe *service.UpstreamBillingProbeService,
	ollamaCloudUsage *service.OllamaCloudUsageService,
	settingService *service.SettingService,
	opencodeGoUsage *service.OpenCodeGoUsageService,
	openAIGatewayService *service.OpenAIGatewayService,
	claudeResetCredits *service.ClaudeResetCreditService,
) *AdminHandlers {
	accountHandler.SetUpstreamBillingProbeService(upstreamBillingProbe)
	channelHandler.ConfigureCS1Providers(accountHandler, upstreamBillingProbe)
	accountHandler.SetOllamaCloudUsageService(ollamaCloudUsage)
	accountHandler.SetOpenAIGatewayService(openAIGatewayService)
	accountHandler.SetOpenCodeGoUsageService(opencodeGoUsage)
	accountHandler.SetClaudeResetCreditService(claudeResetCredits)
	return &AdminHandlers{
		ChannelQuality:         channelQualityHandler,
		Dashboard:              dashboardHandler,
		User:                   userHandler,
		UserCleanup:            userCleanupHandler,
		Group:                  groupHandler,
		Account:                accountHandler,
		Announcement:           announcementHandler,
		DataManagement:         dataManagementHandler,
		Backup:                 backupHandler,
		OAuth:                  oauthHandler,
		OpenAIOAuth:            openaiOAuthHandler,
		OpenAIReauth:           openaiReauthHandler,
		GeminiOAuth:            geminiOAuthHandler,
		AntigravityOAuth:       antigravityOAuthHandler,
		GrokOAuth:              grokOAuthHandler,
		CNProvider:             cnProviderHandler,
		Proxy:                  proxyHandler,
		Redeem:                 redeemHandler,
		Promo:                  promoHandler,
		Setting:                settingHandler,
		Ops:                    opsHandler,
		System:                 systemHandler,
		Subscription:           subscriptionHandler,
		Usage:                  usageHandler,
		UserAttribute:          userAttributeHandler,
		ErrorPassthrough:       errorPassthroughHandler,
		TLSFingerprintProfile:  tlsFingerprintProfileHandler,
		Plugin:                 pluginHandler,
		OpenAIVM:               openaiVMHandler,
		OpenAIVMManaged:        openaiVMManagedHandler,
		OpenAIIPv6:             openaiIPv6Handler,
		APIKey:                 apiKeyHandler,
		ScheduledTest:          scheduledTestHandler,
		Channel:                channelHandler,
		ChannelMonitor:         channelMonitorHandler,
		ChannelMonitorTemplate: channelMonitorTemplateHandler,

		BillingExport: billingExportHandler,

		Payment:    paymentHandler,
		Affiliate:  affiliateHandler,
		Compliance: complianceHandler,
		AuditLog:   auditLogHandler,
	}
}

func ProvideGatewayHandler(
	gatewayService *service.GatewayService,
	openAIGatewayService *service.OpenAIGatewayService,
	geminiCompatService *service.GeminiMessagesCompatService,
	antigravityGatewayService *service.AntigravityGatewayService,
	userService *service.UserService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	usageService *service.UsageService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	errorPassthroughService *service.ErrorPassthroughService,

	userMsgQueueService *service.UserMessageQueueService,
	cfg *config.Config,
	settingService *service.SettingService,

	geminiCachedContentService *service.GeminiCachedContentService,
) *GatewayHandler {
	h := NewGatewayHandler(gatewayService, openAIGatewayService, geminiCompatService, antigravityGatewayService,
		userService, concurrencyService, billingCacheService, usageService, apiKeyService, usageRecordWorkerPool,
		errorPassthroughService, userMsgQueueService, cfg, settingService)

	h.geminiCachedContentService = geminiCachedContentService
	return h
}

func ProvideOpenAIGatewayHandler(
	gatewayService *service.OpenAIGatewayService,
	pluginManager *service.PluginManager,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	errorPassthroughService *service.ErrorPassthroughService,

	opsService *service.OpsService,
	grokQuotaService *service.GrokQuotaService,
	cfg *config.Config,

	compositeResolver *service.CompositeRouteResolver,
) *OpenAIGatewayHandler {
	service.ConfigureMediaTaskRuntime(gatewayService, apiKeyService, cfg)
	service.ConfigureOpenAITiboProbeRuntime(gatewayService, cfg)
	h := NewOpenAIGatewayHandler(gatewayService, concurrencyService, billingCacheService, apiKeyService,
		usageRecordWorkerPool, errorPassthroughService, opsService, cfg)
	h.compositeResolver = compositeResolver
	h.pluginManager = pluginManager

	h.grokMediaEligibilityProber = grokQuotaService
	return h
}

func ProvideBatchImageHandler(
	batchService *service.BatchImagePublicService,
	download *service.BatchImageDownloadService,
	cleanup *service.BatchImageCleanupService,
	openAI *OpenAIGatewayHandler,
) *BatchImageHandler {
	h := NewBatchImageHandler(batchService, download, cleanup)
	h.openAI = openAI
	return h
}

// ProvideSystemHandler creates admin.SystemHandler with UpdateService
func ProvideSystemHandler(updateService *service.UpdateService, lockService *service.SystemOperationLockService, adminService service.AdminService) *admin.SystemHandler {
	h := admin.NewSystemHandler(updateService, lockService)
	if checker, ok := adminService.(service.MihomoNodeAdminService); ok {
		h.SetMihomoNodeChecker(checker)
	}
	return h
}

// ProvideSettingHandler creates SettingHandler with version from BuildInfo
func ProvideSettingHandler(settingService *service.SettingService, buildInfo BuildInfo, notificationEmailService *service.NotificationEmailService) *SettingHandler {
	h := NewSettingHandler(settingService, buildInfo.Version)
	h.SetNotificationEmailService(notificationEmailService)
	return h
}

// ProvideAdminSettingHandler creates admin.SettingHandler with notification template APIs.
func ProvideAdminSettingHandler(settingService *service.SettingService, emailService *service.EmailService, turnstileService *service.TurnstileService, aliyunCaptchaService *service.AliyunCaptchaService, opsService *service.OpsService, paymentConfigService *service.PaymentConfigService, paymentService *service.PaymentService, userAttributeService *service.UserAttributeService, notificationEmailService *service.NotificationEmailService, totpService *service.TotpService, userService *service.UserService) *admin.SettingHandler {
	h := admin.NewSettingHandler(settingService, emailService, turnstileService, opsService, paymentConfigService, paymentService, userAttributeService)
	h.SetNotificationEmailService(notificationEmailService)
	h.SetAliyunCaptchaService(aliyunCaptchaService)
	h.SetStepUpDeps(totpService, userService)
	return h
}

// ProvideHandlers creates the Handlers struct
func ProvideHandlers(
	gptDirectHandler *GPTDirectHandler,
	qqBotHandler *QQBotHandler,
	retainedMediaHandler *RetainedMediaHandler,
	authHandler *AuthHandler,
	userHandler *UserHandler,
	mcpOAuthHandler *MCPOAuthHandler,
	apiKeyHandler *APIKeyHandler,
	usageHandler *UsageHandler,
	redeemHandler *RedeemHandler,
	subscriptionHandler *SubscriptionHandler,
	announcementHandler *AnnouncementHandler,
	channelMonitorUserHandler *ChannelMonitorUserHandler,
	channelMonitorV2Handler *ChannelMonitorV2Handler,
	channelMonitorV3Handler *ChannelMonitorV3Handler,
	adminHandlers *AdminHandlers,
	gatewayHandler *GatewayHandler,
	openaiGatewayHandler *OpenAIGatewayHandler,
	settingHandler *SettingHandler,
	totpHandler *TotpHandler,
	passkeyHandler *PasskeyHandler,
	paymentHandler *PaymentHandler,
	paymentWebhookHandler *PaymentWebhookHandler,
	availableChannelHandler *AvailableChannelHandler,
	billingPreviewHandler *BillingPreviewHandler,
	supportTicketHandler *SupportTicketHandler,
	modelPlazaHandler *ModelPlazaHandler,
	asyncImageHandler *AsyncImageHandler,
	batchImageHandler *BatchImageHandler,
	_ *service.IdempotencyCoordinator,
	_ *service.IdempotencyCleanupService,
	_ *service.OpenAIQuotaAutoResetService,
) *Handlers {
	return &Handlers{
		GPTDirect:        gptDirectHandler,
		QQBot:            qqBotHandler,
		MCPOAuth:         mcpOAuthHandler,
		RetainedMedia:    retainedMediaHandler,
		Auth:             authHandler,
		User:             userHandler,
		APIKey:           apiKeyHandler,
		Usage:            usageHandler,
		Redeem:           redeemHandler,
		Subscription:     subscriptionHandler,
		Announcement:     announcementHandler,
		ChannelMonitor:   channelMonitorUserHandler,
		ChannelMonitorV2: channelMonitorV2Handler,
		ChannelMonitorV3: channelMonitorV3Handler,
		Admin:            adminHandlers,
		Gateway:          gatewayHandler,
		OpenAIGateway:    openaiGatewayHandler,
		Setting:          settingHandler,
		Totp:             totpHandler,
		Passkey:          passkeyHandler,
		Payment:          paymentHandler,
		PaymentWebhook:   paymentWebhookHandler,
		AvailableChannel: availableChannelHandler,
		BillingPreview:   billingPreviewHandler,
		ModelPlaza:       modelPlazaHandler,
		AsyncImage:       asyncImageHandler,
		BatchImage:       batchImageHandler,
		SupportTicket:    supportTicketHandler,
	}
}

// ProvideUserHandler keeps the user-facing scheduled test results wired when
// regenerating the application's dependency graph.
func ProvideUserHandler(
	userService *service.UserService,
	authService *service.AuthService,
	emailService *service.EmailService,
	emailCache service.EmailCache,
	affiliateService *service.AffiliateService,
	userPlatformQuotaRepo service.UserPlatformQuotaRepository,
	scheduledTestService *service.ScheduledTestService,
) *UserHandler {
	h := NewUserHandler(userService, authService, emailService, emailCache, affiliateService, userPlatformQuotaRepo)
	h.SetScheduledTestService(scheduledTestService)
	return h
}

// ProviderSet is the Wire provider set for all handlers
var ProviderSet = wire.NewSet(
	admin.NewOpenAIIPv6Handler,
	NewQQBotHandler,
	NewRetainedMediaHandler,
	admin.NewUserCleanupHandler,
	// Top-level handlers
	NewAuthHandler,
	ProvideUserHandler,
	NewMCPOAuthHandler,
	NewAPIKeyHandler,
	NewUsageHandler,
	NewGPTDirectHandler,
	NewRedeemHandler,
	NewSubscriptionHandler,
	NewAnnouncementHandler,
	NewChannelMonitorUserHandler,
	NewChannelMonitorV2Handler,
	NewChannelMonitorV3Handler,
	ProvideGatewayHandler,
	ProvideOpenAIGatewayHandler,
	NewTotpHandler,
	NewPasskeyHandler,
	ProvideSettingHandler,
	NewPaymentHandler,
	NewPaymentWebhookHandler,
	NewAvailableChannelHandler,
	NewBillingPreviewHandler,
	NewSupportTicketHandler,
	NewModelPlazaHandler,
	NewAsyncImageHandler,
	ProvideBatchImageHandler,

	// Admin handlers
	admin.NewDashboardHandler,
	admin.NewUserHandler,
	admin.NewGroupHandlerWithConfig,
	admin.ProvideAccountHandler,
	admin.NewAnnouncementHandler,
	admin.NewDataManagementHandler,
	admin.NewBackupHandler,
	admin.NewOAuthHandler,
	admin.NewOpenAIOAuthHandler,
	admin.NewOpenAIOAuthReauthHandler,
	admin.NewGeminiOAuthHandler,
	admin.NewAntigravityOAuthHandler,
	admin.NewGrokOAuthHandler,
	admin.NewCNProviderHandler,
	admin.NewProxyHandler,
	admin.NewRedeemHandler,
	admin.NewPromoHandler,
	ProvideAdminSettingHandler,
	admin.NewOpsHandler,
	ProvideSystemHandler,
	admin.NewSubscriptionHandler,
	admin.NewUsageHandler,
	admin.NewUserAttributeHandler,
	admin.NewErrorPassthroughHandler,
	admin.NewTLSFingerprintProfileHandler,
	admin.NewPluginHandler,
	admin.NewOpenAIVMHandler,
	admin.NewOpenAIVMManagedHandler,
	admin.NewAdminAPIKeyHandler,
	admin.NewScheduledTestHandler,
	admin.NewChannelQualityHandler,
	ProvideProviderHubChannelHandler,
	admin.NewChannelMonitorHandler,
	admin.NewChannelMonitorRequestTemplateHandler,
	admin.NewBillingExportHandler,
	admin.NewPaymentHandler,
	admin.NewAffiliateHandler,
	admin.NewComplianceHandler,
	admin.NewAuditLogHandler,

	// AdminHandlers and Handlers constructors
	ProvideAdminHandlers,
	ProvideHandlers,
)
