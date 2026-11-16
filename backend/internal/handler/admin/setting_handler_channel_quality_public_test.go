//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsChannelQualityPublicRoundTrip(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyChannelMonitorEnabled:       "true",
		service.SettingKeyChannelQualityPublicEnabled: "true",
	})
	for _, enabled := range []bool{false, true, false} {
		rec := doUpdateSettings(t, h, map[string]any{"channel_quality_public_enabled": enabled}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var result struct {
			Data struct {
				Enabled bool `json:"channel_quality_public_enabled"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
		require.Equal(t, enabled, result.Data.Enabled)
		stored := repo.values[service.SettingKeyChannelQualityPublicEnabled]
		require.Equal(t, "true", repo.values[service.SettingKeyChannelMonitorEnabled])
		rec = doUpdateSettings(t, h, map[string]any{"site_name": "Preserved configuration"}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, stored, repo.values[service.SettingKeyChannelQualityPublicEnabled], "unrelated settings save must preserve the display switch")
	}
}
