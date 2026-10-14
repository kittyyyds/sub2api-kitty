//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestEndpointLabelsPersistAndReachPublicHTML(t *testing.T) {
	ctx := context.Background()
	custom := `[{"name":"International","endpoint":"https://global.example","description":"International route","recommended":true}]`
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	err := svc.UpdateSettings(ctx, &SystemSettings{
		APIBaseURL:            "https://regional.example",
		APIBaseURLDescription: "Regional route",
		CustomEndpoints:       custom,
	})
	require.NoError(t, err)
	require.Equal(t, "Regional route", repo.updates[SettingKeyAPIBaseURLDescription])
	require.JSONEq(t, custom, repo.updates[SettingKeyCustomEndpoints])

	publicSvc := NewSettingService(&settingPublicRepoStub{values: repo.updates}, &config.Config{})
	public, err := publicSvc.GetPublicSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "Regional route", public.APIBaseURLDescription)
	require.JSONEq(t, custom, public.CustomEndpoints)
	injection, err := publicSvc.GetPublicSettingsForInjection(ctx)
	require.NoError(t, err)
	raw, err := json.Marshal(injection)
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.JSONEq(t, `"Regional route"`, string(payload["api_base_url_description"]))
	require.JSONEq(t, custom, string(payload["custom_endpoints"]))
}
