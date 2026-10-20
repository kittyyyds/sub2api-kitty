package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestRequestTimezonePassthroughFirstAndLaterTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baselineCacheKeys := make(map[string]string)
	for _, test := range []struct {
		enabled     bool
		accountType string
		proxyZone   string
	}{
		{false, AccountTypeAPIKey, ""}, {true, AccountTypeAPIKey, ""},
		{false, AccountTypeOAuth, ""}, {true, AccountTypeOAuth, ""},
		{true, AccountTypeAPIKey, "Asia/Tokyo"}, {true, AccountTypeAPIKey, "America/Los_Angeles"},
		{true, AccountTypeOAuth, "Asia/Tokyo"}, {true, AccountTypeOAuth, "America/Los_Angeles"},
	} {
		t.Run(fmt.Sprint(test.accountType, "/", test.enabled, "/", test.proxyZone), func(t *testing.T) {
			enabled := test.enabled
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			upstream := newStagedPassthroughConn()
			account := passthroughLifecycleAccount()
			account.Type = test.accountType
			account.Credentials["access_token"] = "oauth-fixture-token"
			account.Credentials["chatgpt_account_id"] = "fixture-account"
			account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModePassthrough
			account.Extra[openAIRequestTimezoneEnabledExtraKey] = enabled
			account.Extra[openAIRequestTimezoneExtraKey] = "Asia/Tokyo"
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			svc := newPassthroughLifecycleService(cfg, upstream)
			svc.settingService = timezoneAutoService(enabled, false, nil).settingService
			if test.proxyZone != "" {
				p := timezoneProxyFixture()
				account.ProxyID, account.Proxy = &p.ID, p
				account.Extra[openAIRequestTimezoneModeExtraKey] = "proxy"
				svc.proxyTimezone = newProxyTimezoneResolver(timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) {
					return timezoneResult(test.proxyZone), 0, nil
				}))
			}
			server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
			defer server.Close()
			text := "<environment_context><timezone>Asia/Shanghai</timezone><cwd>/keep</cwd></environment_context>"
			body := localeTestBody([]string{text, text}, []string{"environments.environment_context", "user.text"})
			body, _ = sjson.SetBytes(body, "type", "response.create")
			body, _ = sjson.SetBytes(body, "model", "gpt-6-sol")
			body, _ = sjson.SetBytes(body, "reasoning.effort", "max")
			body, _ = sjson.SetBytes(body, "prompt_cache_key", "stable-cache")
			client := dialPassthroughLifecycleClientWithPayload(t, server, string(body))
			defer client.CloseNow()
			for turn := 1; turn <= 2; turn++ {
				if turn == 2 {
					body, _ = sjson.SetBytes(body, "previous_response_id", "resp_timezone_1")
					require.NoError(t, client.Write(ctx, coderws.MessageText, body))
				}
				request := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
				wanted := "Asia/Shanghai"
				if enabled {
					wanted = DefaultOpenAIRequestTimezone
				}

				require.Contains(t, gjson.GetBytes(request, "input.0.content.0.text").String(), "<timezone>"+wanted+"</timezone>")
				require.Contains(t, gjson.GetBytes(request, "input.0.content.0.text").String(), "<cwd>/keep</cwd>")
				require.Equal(t, text, gjson.GetBytes(request, "input.0.content.1.text").String())
				require.Equal(t, "gpt-6-sol", gjson.GetBytes(request, "model").String())
				require.Equal(t, "max", gjson.GetBytes(request, "reasoning.effort").String())
				cacheKey := gjson.GetBytes(request, "prompt_cache_key").String()
				if !enabled && turn == 1 {
					// OAuth already applies its account-isolated cache identity. The
					// timezone policy must preserve that behavior across turns.
					baselineCacheKeys[test.accountType] = cacheKey
				}
				require.NotEmpty(t, cacheKey)
				require.Equal(t, baselineCacheKeys[test.accountType], cacheKey)
				searchZone := "America/New_York"
				if enabled {
					searchZone = DefaultOpenAIRequestTimezone
				}
				require.Equal(t, searchZone, gjson.GetBytes(request, "tools.0.user_location.timezone").String())
				require.Equal(t, !account.IsOpenAIOAuthLike(), gjson.GetBytes(request, "input.0.internal_chat_message_metadata_passthrough").Exists())
				if turn == 2 {
					require.Equal(t, "resp_timezone_1", gjson.GetBytes(request, "previous_response_id").String())
				}
				upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_timezone_%d","model":"gpt-6-sol","usage":{"input_tokens":1,"output_tokens":1}}}`, turn))
				completed, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
			}
			_ = client.CloseNow()
			select {
			case <-serverErr:
			case <-ctx.Done():
				t.Fatal("websocket did not finish after client disconnect")
			}
		})
	}
}
