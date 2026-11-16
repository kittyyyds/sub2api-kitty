package service

import (
	"context"
	"database/sql"
	"math"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/qqbot"
)

func ProvideQQBotService(db *sql.DB, encryptor SecretEncryptor, cfg *config.Config, invalidator APIKeyAuthCacheInvalidator, billing *BillingCacheService, email *EmailService, settings *SettingService, users UserRepository, monitor *ChannelMonitorV2Service, groups GroupRepository, channels ChannelRepository, channelService *ChannelService, priceResolver *ModelPricingResolver, billingService *BillingService, rates UserGroupRateRepository, keys *APIKeyService, accounts AccountRepository, concurrency *ConcurrencyService, balanceNotify *BalanceNotifyService) *qqbot.Service {
	s := qqbot.New(db, encryptor, cfg.Totp.EncryptionKeyConfigured, func(ctx context.Context, user int64) {
		if invalidator != nil {
			invalidator.InvalidateAuthCacheByUserID(ctx, user)
		}
		if billing != nil {
			_ = billing.InvalidateUserBalance(ctx, user)
		}
	})
	s.SetUserConcurrencyReader(func(ctx context.Context, userID int64, limit int) (*int, error) {
		if limit <= 0 || concurrency == nil {
			return nil, nil
		}
		loads, err := concurrency.GetUsersLoadBatch(ctx, []UserWithConcurrency{{ID: userID, MaxConcurrency: limit}})
		if err != nil {
			return nil, err
		}
		if load := loads[userID]; load != nil {
			n := load.CurrentConcurrency
			return &n, nil
		}
		return nil, nil
	})
	if balanceNotify != nil {
		balanceNotify.balanceAlertSink = s.EnqueueBalanceAlert
		s.SetBalanceAlertValidator(func(ctx context.Context, userID int64, expected float64) (bool, error) {
			u, err := users.GetByID(ctx, userID)
			if err != nil {
				return false, err
			}
			if u == nil || !u.IsActive() || u.DeletedAt != nil || !u.BalanceNotifyEnabled {
				return false, nil
			}
			threshold, _, enabled := balanceNotify.resolveUserEffectiveThreshold(ctx, u)
			return enabled && math.Abs(threshold-expected) <= 1e-8 && u.Balance < threshold, nil
		})
	}
	s.SetTransferBalancePolicy(qqBotTransferableBalance)
	s.SetBindingEmailDelivery(&qqBotBindingEmail{email: email, settings: settings, users: users})
	status := &qqBotChannelStatus{monitor: monitor, settings: settings, groups: groups}
	s.SetChannelStatusProvider(status)
	s.SetHealthSnapshotProvider(status)
	s.SetModelPriceProvider(&qqBotModelPrices{groups: groups, users: users, rates: rates, channels: channelService, billing: billingService, resolver: priceResolver})
	s.SetPriceSnapshotProvider(&qqBotPriceSource{channels: channels, groups: groups})
	s.SetSiteAIProvider(&qqBotSiteAI{keys: keys, groups: groups, users: users, accounts: accounts, settings: settings, email: email, baseURL: qqBotInternalBaseURL(cfg.Server.Port)})
	startBackgroundService(cfg, s)
	return s
}

type qqBotBindingEmail struct {
	email    *EmailService
	settings *SettingService
	users    UserRepository
}

func (d *qqBotBindingEmail) FrontendURL(ctx context.Context) string {
	return d.settings.GetFrontendURL(ctx)
}

func (d *qqBotBindingEmail) LookupAccount(ctx context.Context, address string) (int64, string, error) {
	u, err := d.users.GetByEmail(ctx, address)
	if err != nil {
		return 0, "", err
	}
	if u == nil || !u.IsActive() || u.DeletedAt != nil {
		return 0, "", ErrUserNotFound
	}
	return u.ID, u.Email, nil
}

func (d *qqBotBindingEmail) SendEmail(ctx context.Context, to, subject, body string) error {
	return d.email.SendEmail(ctx, to, subject, body)
}
