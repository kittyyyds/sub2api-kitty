//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestChannelQualityPublicSettings(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name, value string
		enabled     bool
	}{
		{"missing keeps existing visibility", "", true},
		{"enabled", "true", true},
		{"disabled", "false", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &settingPublicRepoStub{values: map[string]string{SettingKeyChannelQualityPublicEnabled: tt.value, SettingKeyChannelMonitorEnabled: "true"}}
			svc := NewSettingService(repo, &config.Config{})
			require.Equal(t, tt.enabled, svc.IsChannelQualityPublicEnabled(ctx))
			settings, err := svc.GetPublicSettings(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.enabled, settings.ChannelQualityPublicEnabled)
			injected, err := svc.GetPublicSettingsForInjection(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.enabled, injected.(*PublicSettingsInjectionPayload).ChannelQualityPublicEnabled)
			require.True(t, svc.GetChannelMonitorRuntime(ctx).Enabled, "hiding quality reports must not disable monitoring")
		})
	}
	repo := &settingPublicRepoStub{err: errors.New("settings unavailable")}
	require.False(t, NewSettingService(repo, &config.Config{}).IsChannelQualityPublicEnabled(ctx))
	var absent *SettingService
	require.False(t, absent.IsChannelQualityPublicEnabled(ctx))
}
