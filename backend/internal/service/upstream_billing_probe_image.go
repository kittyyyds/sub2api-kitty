package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const UpstreamBillingProbeKindImageModelPrices = "image_model_prices"

// probeImageModelPrices observes the confirmed supplier's public catalog. It
// does not call SyncImagePrices: that operation also reprices customer groups.
func (s *UpstreamBillingProbeService) probeImageModelPrices(ctx context.Context, account *Account, intervalMinutes int) (*UpstreamBillingProbeSnapshot, error) {
	now := s.currentTime().UTC()
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	parsed, err := url.Parse(baseURL)
	// Match the confirmed HTTPS origin, not a suffix or an arbitrary port. A
	// different image supplier remains saveable but has no supported adapter.
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "suanliai.top") ||
		(parsed.Port() != "" && parsed.Port() != "443") || parsed.User != nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "unsupported", 0)
	}
	if s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "transport_unavailable", 0)
	}
	// Preserve the configured security allowlist for both the account route and
	// the actual public endpoint; account path/query data is never forwarded.
	if _, err := s.accountTestService.validateUpstreamBaseURL(baseURL); err != nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "invalid_base_url", 0)
	}
	if _, err := s.accountTestService.validateUpstreamBaseURL(videoModelPriceProbeURL); err != nil {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "invalid_base_url", 0)
	}
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return s.persistProbeFailure(ctx, account, intervalMinutes, now, 0, "proxy_unavailable", 0)
		}
		if account.Proxy.ID != *account.ProxyID {
			return nil, ErrUpstreamBillingProbeIdentityChanged
		}
		proxyURL = account.Proxy.URL()
	}
	result := s.fetchUpstreamProbeBody(ctx, account,
		upstreamBillingProbeTarget{proxyURL: proxyURL, publicPricing: true},
		videoModelPriceProbeURL, videoModelPriceProbeMaxBodyBytes)
	reason := result.failure
	if reason == "" {
		switch {
		case result.statusCode == http.StatusNotFound || result.statusCode == http.StatusMethodNotAllowed:
			reason = "unsupported"
		case result.statusCode != http.StatusOK:
			reason = "http_error"
		}
	}
	var models map[string]ImageSupplierPrice
	var version string
	if reason == "" {
		models, version, err = parseImageSupplierCatalog(result.body)
		if err != nil {
			reason = "invalid_response"
		}
	}
	if reason != "" {
		return s.persistProbeFailure(ctx, account, intervalMinutes, now, result.statusCode, reason, retryAfter(result.header, now))
	}
	at := now.Format(time.RFC3339)
	// Keep the shared catalog complete, but account-list snapshots must contain
	// only this account's configured models; lite account rows omit credentials.
	catalog := &ImageSupplierCatalog{Version: version, LastAttempt: at, LastSuccess: at, Models: models}
	selected := make(map[string]ImageSupplierPrice)
	for _, upstream := range account.GetModelMapping() {
		if price, ok := models[upstream]; ok {
			selected[upstream] = price
		}
	}
	snapshot := &UpstreamBillingProbeSnapshot{
		Kind: UpstreamBillingProbeKindImageModelPrices,
		ImagePricing: &ImageSupplierCatalog{
			Version: version, LastAttempt: at, LastSuccess: at,
			Models: selected,
		},
		Status:        UpstreamBillingProbeStatusOK,
		ReceivedAt:    probeTimePtr(now),
		FreshUntil:    probeTimePtr(now.Add(2 * time.Duration(intervalMinutes) * time.Minute)),
		LastAttemptAt: now,
		NextProbeAt:   now.Add(nextProbeDelay(intervalMinutes, 0)),
		HTTPStatus:    result.statusCode,
	}
	if err := s.updateSnapshot(ctx, account, snapshot, nil); err != nil {
		return nil, err
	}
	// Publish only after the account CAS succeeds. Catalog maintenance is best
	// effort: an unavailable settings store must not undo a saved observation.
	if err := s.refreshSharedImageCatalog(ctx, catalog); err != nil {
		slog.Warn("image_model_price_catalog_update_failed", "account_id", account.ID)
	}
	return snapshot, nil
}

// refreshSharedImageCatalog makes the confirmed public CNY prices available to
// the group editor without changing group pricing or its price-change history.
// It shares the existing process lock with image price synchronization; a busy
// lock or a failed settings write can be retried on the next account probe.
func (s *UpstreamBillingProbeService) refreshSharedImageCatalog(ctx context.Context, catalog *ImageSupplierCatalog) error {
	if s.settingService == nil || s.settingService.settingRepo == nil || catalog == nil {
		return nil
	}
	if !imagePriceSyncMu.TryLock() {
		return nil
	}
	defer imagePriceSyncMu.Unlock()
	current, err := s.settingService.GetImageCatalog(ctx)
	if err != nil {
		return err
	}
	// Different accounts can finish probing the same supplier out of order.
	previousAt, previousErr := time.Parse(time.RFC3339, current.LastSuccess)
	observedAt, observedErr := time.Parse(time.RFC3339, catalog.LastSuccess)
	if previousErr == nil && observedErr == nil && previousAt.After(observedAt) {
		return nil
	}
	current.Models = catalog.Models
	current.Version = catalog.Version
	current.LastSuccess = catalog.LastSuccess
	current.LastAttempt = catalog.LastAttempt
	current.LastError = catalog.LastError
	// Preserve LastSyncAttempt: observing costs must not postpone group repricing.
	raw, err := json.Marshal(current)
	if err != nil {
		return err
	}
	return s.settingService.settingRepo.Set(ctx, imageCatalogSettingKey, string(raw))
}

func isModelPriceProbeAccount(account *Account) bool {
	return account != nil && (account.Platform == PlatformVideo || isSuanliImageAccount(account))
}
