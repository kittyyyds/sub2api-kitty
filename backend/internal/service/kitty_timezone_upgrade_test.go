package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTimezoneUpgradeLegacyAccountPolicyIsIgnored(t *testing.T) {
	env := "<environment_context><timezone>Asia/Shanghai</timezone><current_date>2026-10-15</current_date></environment_context>"
	body := localeTestBody([]string{env, env}, []string{"environments.environment_context", "user.text"})
	for _, search := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			proxy := timezoneProxyFixture()
			account := &Account{Platform: PlatformOpenAI, ProxyID: &proxy.ID, Proxy: proxy, Extra: map[string]any{
				openAIRequestTimezoneEnabledExtraKey: enabled, openAIRequestTimezoneModeExtraKey: "proxy",
				openAIRequestTimezoneSyncSearchExtraKey: search, openAIRequestTimezoneExtraKey: "Asia/Singapore",
			}}
			before, err := json.Marshal(account.Extra)
			require.NoError(t, err)
			for _, global := range []*SettingService{nil, timezoneAutoService(true, false, nil).settingService} {
				svc := &OpenAIGatewayService{settingService: global, proxyTimezone: newProxyTimezoneResolver(timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) {
					return timezoneResult("Asia/Singapore"), 0, nil
				}))}
				for _, transport := range []string{"http", "ws"} {
					got := svc.normalizeRequestTimezone(context.Background(), account, body, transport)
					if global == nil {
						require.Equal(t, body, got)
						continue
					}
					require.Contains(t, gjson.GetBytes(got, "input.0.content.0.text").String(), "Asia/Singapore")
					require.Equal(t, env, gjson.GetBytes(got, "input.0.content.1.text").String())
					wantSearch := "Asia/Singapore"
					require.Equal(t, wantSearch, gjson.GetBytes(got, "tools.0.user_location.timezone").String())
				}
			}
			after, err := json.Marshal(account.Extra)
			require.NoError(t, err)
			require.Equal(t, before, after)
		}
	}
}

func TestTimezoneUpgradeFailedProxyDoesNotFallBackToGlobalTimezone(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneModeExtraKey: "proxy"}}
	body := localeTestBody([]string{"<environment_context><timezone>Asia/Tokyo</timezone><cwd>/keep</cwd></environment_context>"}, nil)
	svc := &OpenAIGatewayService{settingService: timezoneAutoService(true, true, nil).settingService}
	require.Equal(t, body, svc.normalizeRequestTimezone(context.Background(), account, body, "http"))
}
