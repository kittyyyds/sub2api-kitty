//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIBaseURLDescriptionPreservesMissingAndAllowsClear(t *testing.T) {
	value, err := resolveAPIBaseURLDescription("Regional route", nil)
	require.NoError(t, err)
	require.Equal(t, "Regional route", value)
	empty := "  "
	value, err = resolveAPIBaseURLDescription("Regional route", &empty)
	require.NoError(t, err)
	require.Empty(t, value)
	tooLong := strings.Repeat("线", 201)
	_, err = resolveAPIBaseURLDescription("", &tooLong)
	require.Error(t, err)
}

func TestEndpointRecommendationsSurviveOldClientReorder(t *testing.T) {
	previous := dto.ParseCustomEndpoints(`[{"name":"one","endpoint":"https://one.example","recommended":true},{"name":"two","endpoint":"https://two.example","recommended":false}]`)
	requested := dto.ParseCustomEndpoints(`[{"name":"two renamed","endpoint":"https://two.example"},{"name":"one","endpoint":"https://one.example"},{"name":"new","endpoint":"https://new.example"}]`)
	result := preserveEndpointRecommendations(previous, requested)
	require.NotNil(t, result[0].Recommended)
	require.False(t, *result[0].Recommended)
	require.NotNil(t, result[1].Recommended)
	require.True(t, *result[1].Recommended)
	require.Nil(t, result[2].Recommended)
	require.Nil(t, requested[0].Recommended, "do not mutate the request")
	explicit := dto.ParseCustomEndpoints(`[{"name":"one","endpoint":"https://one.example","recommended":false}]`)
	result = preserveEndpointRecommendations(previous, explicit)
	require.False(t, *result[0].Recommended)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"recommended":false`)
}

func TestEndpointLabelsSettingsRoundTripPreservesUnrelatedSettings(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName:              "Existing site",
		service.SettingKeyAPIBaseURL:            "https://regional.example",
		service.SettingKeyAPIBaseURLDescription: "Old regional label",
		service.SettingKeyCustomEndpoints:       `[{"name":"Global","endpoint":"https://global.example","description":"Old label","recommended":true},{"name":"Backup","endpoint":"https://backup.example","description":"Keep this"}]`,
	})
	rec := doUpdateSettings(t, h, map[string]any{
		"api_base_url_description": "Regional route",
		"custom_endpoints": []map[string]any{
			{"name": "Global", "endpoint": "https://global.example", "description": "International route"},
			{"name": "Backup", "endpoint": "https://backup.example", "description": "Keep this"},
		},
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "Existing site", repo.values[service.SettingKeySiteName])
	require.Equal(t, "https://regional.example", repo.values[service.SettingKeyAPIBaseURL])
	require.Equal(t, "Regional route", repo.values[service.SettingKeyAPIBaseURLDescription])
	endpoints := dto.ParseCustomEndpoints(repo.values[service.SettingKeyCustomEndpoints])
	require.Len(t, endpoints, 2)
	require.True(t, *endpoints[0].Recommended)
	require.Equal(t, "Keep this", endpoints[1].Description)
	var response struct {
		Data dto.SystemSettings `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "Regional route", response.Data.APIBaseURLDescription)
	require.True(t, *response.Data.CustomEndpoints[0].Recommended)

	rec = doUpdateSettings(t, h, map[string]any{"site_name": "Changed site"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "Regional route", repo.values[service.SettingKeyAPIBaseURLDescription])
}
