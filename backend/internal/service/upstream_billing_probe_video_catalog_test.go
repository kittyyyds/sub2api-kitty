package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type videoProbeCatalogSettingsRepo struct {
	*upstreamBillingProbeSettingRepo
	readErr     error
	writeErr    error
	beforeWrite func()
	writes      int
}

func (r *videoProbeCatalogSettingsRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.upstreamBillingProbeSettingRepo.GetMultiple(ctx, keys)
}

func (r *videoProbeCatalogSettingsRepo) Set(ctx context.Context, key, value string) error {
	if key == videoCatalogSettingKey {
		r.writes++
		if r.beforeWrite != nil {
			r.beforeWrite()
		}
		if r.writeErr != nil {
			return r.writeErr
		}
	}
	return r.upstreamBillingProbeSettingRepo.Set(ctx, key, value)
}

type videoProbeRejectedCASRepo struct {
	*upstreamBillingProbeAccountRepo
}

func (r *videoProbeRejectedCASRepo) UpdateUpstreamBillingProbeSnapshot(context.Context, *Account, *UpstreamBillingProbeSnapshot, *float64) error {
	return ErrUpstreamBillingProbeIdentityChanged
}

func TestUpstreamBillingProbeVideoPublishesCatalogOnlyAfterCAS(t *testing.T) {
	account := videoProbeFixtureAccount()
	upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
	svc, _ := videoProbeFixtureService(account, upstream)
	changes := []VideoPriceChange{{At: "2026-10-08T00:00:00Z", GroupID: 12, Model: videoProbeFixtureModel, Old: 0.3, New: 0.4}}
	previous := VideoSupplierCatalog{
		Version: "old", LastError: "old failure", LastSuccess: "2026-10-08T00:00:00Z", Changes: changes,
		LastPriceSyncAttempt: "2026-10-08T00:00:00Z",
	}
	raw, err := json.Marshal(previous)
	require.NoError(t, err)
	settings := &videoProbeCatalogSettingsRepo{upstreamBillingProbeSettingRepo: &upstreamBillingProbeSettingRepo{
		values: map[string]string{videoCatalogSettingKey: string(raw)},
	}}
	settings.beforeWrite = func() {
		stored := decodeUpstreamBillingProbeSnapshot(account.Extra)
		require.NotNil(t, stored, "catalog publication must follow a successful account CAS")
		require.Equal(t, UpstreamBillingProbeStatusOK, stored.Status)
	}
	svc.settingService.settingRepo = settings

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 1, settings.writes)
	// This is the same service read used by videoUpstreamAPI.get in the group editor.
	gateway := &OpenAIGatewayService{settingService: svc.settingService}
	_, catalog, err := gateway.VideoAdminSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, snapshot.VideoPricing.Version, catalog.Version)
	require.Equal(t, snapshot.VideoPricing.LastAttempt, catalog.LastAttempt)
	require.Equal(t, previous.LastPriceSyncAttempt, catalog.LastPriceSyncAttempt, "account probes must not postpone periodic group repricing")
	require.NotEqual(t, catalog.LastAttempt, catalog.LastPriceSyncAttempt)
	require.Equal(t, snapshot.VideoPricing.LastSuccess, catalog.LastSuccess)
	require.Empty(t, catalog.LastError)
	require.Equal(t, changes, catalog.Changes, "reading supplier prices must preserve pricing audit history")
	require.Equal(t, 0.2, catalog.Models[videoProbeFixtureModel].UnitPrice, "keep upstream CNY values unchanged")
	require.Equal(t, 0.1, catalog.Models[videoProbeFixtureModel].ReferencePerSecond)
	require.Equal(t, 0.3, catalog.Models[videoProbeFixtureModel].ResolutionPrices["720p"])
	require.Equal(t, 0.7, *account.RateMultiplier)
	require.Equal(t, 0.4, account.Extra[AccountCostMultiplierExtraKey])
}

func TestUpstreamBillingProbeVideoCatalogUnavailableDoesNotFailSnapshot(t *testing.T) {
	for _, scenario := range []string{"read failure", "write failure", "busy lock", "missing settings"} {
		t.Run(scenario, func(t *testing.T) {
			account := videoProbeFixtureAccount()
			upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
			svc, _ := videoProbeFixtureService(account, upstream)
			settings := &videoProbeCatalogSettingsRepo{upstreamBillingProbeSettingRepo: &upstreamBillingProbeSettingRepo{}}
			svc.settingService.settingRepo = settings
			switch scenario {
			case "read failure":
				settings.readErr = errors.New("fixture read failure")
			case "write failure":
				settings.writeErr = errors.New("fixture write failure")
			case "missing settings":
				svc.settingService = nil
			case "busy lock":
				videoPriceSyncMu.Lock()
			}
			snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
			if scenario == "busy lock" {
				videoPriceSyncMu.Unlock()
			}
			require.NoError(t, err)
			require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
			require.NotNil(t, decodeUpstreamBillingProbeSnapshot(account.Extra).VideoPricing)
			require.Empty(t, settings.values[videoCatalogSettingKey])

			// A later probe retries catalog publication; no rollback or group write
			// is required to recover from a settings-store or lock failure.
			settings.readErr, settings.writeErr = nil, nil
			if svc.settingService == nil {
				svc.settingService = NewSettingService(settings, nil)
			}
			upstream.resp = videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)
			_, err = svc.ProbeAccount(context.Background(), account.ID)
			require.NoError(t, err)
			require.NotEmpty(t, settings.values[videoCatalogSettingKey])
		})
	}
}

func TestUpstreamBillingProbeVideoCatalogUnchangedOnRejectedSnapshotOrProbe(t *testing.T) {
	for _, scenario := range []string{"CAS rejected", "unsupported source", "failed fetch"} {
		t.Run(scenario, func(t *testing.T) {
			account := videoProbeFixtureAccount()
			upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
			svc, repo := videoProbeFixtureService(account, upstream)
			const original = `{"version":"preserved","models":{},"changes":[{"group_id":12,"model":"audit-model","new":0.5}]}`
			settings := &videoProbeCatalogSettingsRepo{upstreamBillingProbeSettingRepo: &upstreamBillingProbeSettingRepo{values: map[string]string{videoCatalogSettingKey: original}}}
			svc.settingService.settingRepo = settings
			switch scenario {
			case "CAS rejected":
				svc.accountRepo = &videoProbeRejectedCASRepo{repo}
			case "unsupported source":
				account.Credentials["base_url"] = "https://other.example"
			case "failed fetch":
				upstream.resp = videoProbeHTTPResponse(http.StatusBadGateway, "fixture failure")
			}
			_, err := svc.ProbeAccount(context.Background(), account.ID)
			if scenario == "CAS rejected" {
				require.ErrorIs(t, err, ErrUpstreamBillingProbeIdentityChanged)
			} else {
				require.NoError(t, err)
			}
			require.Zero(t, settings.writes)
			require.Equal(t, original, settings.values[videoCatalogSettingKey])
		})
	}
}

func TestUpstreamBillingProbeVideoCatalogDoesNotReplaceNewerSuccess(t *testing.T) {
	account := videoProbeFixtureAccount()
	upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, videoProbeFixtureBody)}
	svc, _ := videoProbeFixtureService(account, upstream)
	newer := VideoSupplierCatalog{Version: "newer", LastSuccess: svc.currentTime().Add(time.Hour).Format(time.RFC3339)}
	raw, err := json.Marshal(newer)
	require.NoError(t, err)
	settings := &videoProbeCatalogSettingsRepo{upstreamBillingProbeSettingRepo: &upstreamBillingProbeSettingRepo{values: map[string]string{videoCatalogSettingKey: string(raw)}}}
	svc.settingService.settingRepo = settings
	_, err = svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Zero(t, settings.writes)
	require.Equal(t, string(raw), settings.values[videoCatalogSettingKey])
}
