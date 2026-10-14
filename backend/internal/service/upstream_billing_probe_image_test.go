package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const imageProbeBody = `{"success":true,"pricing_version":"image-test","group_ratio":{"default":1},"data":[{"model_name":"Image-A","billing_mode":"per_request","model_price":0.08,"description":"supplier description"},{"model_name":"video-a","billing_mode":"video_multi","model_price":0.9}]}`

func TestUpstreamBillingProbeImagePublicPriceAndRepricingSeparation(t *testing.T) {
	a := videoProbeFixtureAccount()
	a.Platform = PlatformOpenAI
	a.Credentials["model_mapping"] = map[string]any{"Customer-A": "Image-A"}
	upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, imageProbeBody)}
	svc, _ := videoProbeFixtureService(a, upstream)
	old := ImageSupplierCatalog{LastSyncAttempt: "2026-10-08T00:00:00Z"}
	raw, err := json.Marshal(old)
	require.NoError(t, err)
	require.NoError(t, svc.settingService.settingRepo.Set(context.Background(), imageCatalogSettingKey, string(raw)))
	snapshot, err := svc.ProbeAccount(context.Background(), a.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeKindImageModelPrices, snapshot.Kind)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.Len(t, snapshot.ImagePricing.Models, 1)
	require.Equal(t, 0.08, snapshot.ImagePricing.Models["Image-A"].UnitPrice)
	require.Equal(t, http.MethodGet, upstream.lastReq.Method)
	require.Equal(t, videoModelPriceProbeURL, upstream.lastReq.URL.String())
	require.Equal(t, http.Header{"Accept": []string{"application/json"}}, upstream.lastReq.Header)
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
	require.Nil(t, snapshot.SyncedRateMultiplier)
	require.Nil(t, snapshot.Data)
	require.False(t, upstreamBillingRateSyncEnabled(a))
	require.Equal(t, 0.7, *a.RateMultiplier)
	require.Equal(t, 0.4, a.Extra[AccountCostMultiplierExtraKey])
	catalog, err := svc.settingService.GetImageCatalog(context.Background())
	require.NoError(t, err)
	require.Equal(t, old.LastSyncAttempt, catalog.LastSyncAttempt, "observations must not defer auto repricing")
	require.Equal(t, snapshot.ImagePricing.Models, catalog.Models)
	// Even a legacy token payload attached to an image observation cannot sync costs.
	snapshot.Data = map[string]any{"resolved_rate_multiplier": 99.0}
	_, valid := snapshot.CostMultiplierToSync()
	require.False(t, valid)
}

func TestUpstreamBillingProbeImageFailureRetainsPricesAndCAS(t *testing.T) {
	a := videoProbeFixtureAccount()
	a.Platform = PlatformOpenAI
	upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, imageProbeBody)}
	svc, repo := videoProbeFixtureService(a, upstream)
	first, err := svc.ProbeAccount(context.Background(), a.ID)
	require.NoError(t, err)
	upstream.resp = videoProbeHTTPResponse(503, "private upstream response")
	next, err := svc.ProbeAccount(context.Background(), a.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", next.Status)
	require.Equal(t, "image_model_prices", next.Kind)
	require.Equal(t, first.ImagePricing.Models, next.ImagePricing.Models)
	require.Equal(t, first.ReceivedAt, next.ReceivedAt)
	require.Empty(t, first.ImagePricing.LastError)
	raw, err := json.Marshal(next)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private upstream response")
	upstream.resp = videoProbeHTTPResponse(http.StatusOK, imageProbeBody)
	svc.accountRepo = &videoProbeRejectedCASRepo{repo}
	require.NoError(t, svc.settingService.settingRepo.Set(context.Background(), imageCatalogSettingKey, `{"version":"unchanged","models":{}}`))
	_, err = svc.ProbeAccount(context.Background(), a.ID)
	require.ErrorIs(t, err, ErrUpstreamBillingProbeIdentityChanged)
	cat, err := svc.settingService.GetImageCatalog(context.Background())
	require.NoError(t, err)
	require.Equal(t, "unchanged", cat.Version)
}

func TestUpstreamBillingProbeImageBadPriceDoesNotReplaceLastValidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"success":true,"group_ratio":{"default":2},"data":[{"model_name":"Image-A","billing_mode":"per_request","model_price":0.08}]}`,
		`{"success":true,"group_ratio":{"default":1},"data":[{"model_name":"Image-A","billing_mode":"per_request","model_price":-1}]}`,
	} {
		a := videoProbeFixtureAccount()
		a.Platform = PlatformOpenAI
		upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusOK, body)}
		svc, _ := videoProbeFixtureService(a, upstream)
		snapshot, err := svc.ProbeAccount(context.Background(), a.ID)
		require.NoError(t, err)
		require.Equal(t, "failed", snapshot.Status)
		require.Equal(t, "invalid_response", snapshot.LastError)
		require.Nil(t, snapshot.ImagePricing)
	}
}

func TestImageProbeRecognitionAndOlderObservation(t *testing.T) {
	for _, base := range []string{"https://suanliai.top.evil.test/v1", "http://suanliai.top", "https://suanliai.top:8443", "https://user@suanliai.top", "https://suanliai.top/v1?key=x"} {
		a := videoProbeFixtureAccount()
		a.Platform = PlatformOpenAI
		a.Credentials["base_url"] = base
		require.False(t, isModelPriceProbeAccount(a), base)
	}
	a := videoProbeFixtureAccount()
	a.Platform = PlatformOpenAI
	svc, _ := videoProbeFixtureService(a, nil)
	ctx := context.Background()
	newer := &ImageSupplierCatalog{Version: "newer", LastSuccess: time.Now().UTC().Format(time.RFC3339)}
	require.NoError(t, svc.refreshSharedImageCatalog(ctx, newer))
	require.NoError(t, svc.refreshSharedImageCatalog(ctx, &ImageSupplierCatalog{Version: "older", LastSuccess: "2020-01-01T00:00:00Z"}))
	cat, err := svc.settingService.GetImageCatalog(ctx)
	require.NoError(t, err)
	require.Equal(t, "newer", cat.Version)
}
