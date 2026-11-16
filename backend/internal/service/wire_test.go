package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/collection"
)

// The account directory belonged to the retired OpenAI OAuth outbound capability;
// host services keep KV but refuse directory calls for every plugin.
func TestProvidePluginManagerGrantsNoHostAccountDirectory(t *testing.T) {
	manager := ProvidePluginManager(nil, nil, &config.Config{}, PluginHostInfo{}, newFakePluginKVStore())
	host := manager.buildHostServices(&PluginInstallation{
		PluginKey: "test.provider",
		Manifest: PluginManifest{Capabilities: []PluginCapability{{
			ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth,
		}}},
	})
	require.NotNil(t, host)
	_, err := host.ListAccounts(context.Background(), &pluginv1.ListAccountsRequest{
		Platform: PlatformOpenAI, AccountType: AccountTypeOAuth,
	})
	require.Error(t, err)
}

func TestProvideTimingWheelService_ReturnsError(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, _ collection.Execute) (*collection.TimingWheel, error) {
		return nil, errors.New("boom")
	}

	svc, err := ProvideTimingWheelService(nil)
	if err == nil {
		t.Fatalf("期望返回 error，但得到 nil")
	}
	if svc != nil {
		t.Fatalf("期望返回 nil svc，但得到非空")
	}
}

func TestProvideTimingWheelService_Success(t *testing.T) {
	svc, err := ProvideTimingWheelService(nil)
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	svc.Stop()
}
