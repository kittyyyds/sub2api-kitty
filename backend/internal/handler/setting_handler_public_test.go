//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ForceEmailOnThirdPartySignup bool `json:"force_email_on_third_party_signup"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.ForceEmailOnThirdPartySignup)
}

func TestSettingHandler_GetPublicSettings_ExposesTencentCaptchaConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyTencentCaptchaEnabled: "true",
			service.SettingKeyTencentCaptchaAppID:   "123456789",
			service.SettingKeyTencentCaptchaRegion:  service.TencentCaptchaRegionINTL,
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			TencentCaptchaEnabled bool   `json:"tencent_captcha_enabled"`
			TencentCaptchaAppID   string `json:"tencent_captcha_app_id"`
			TencentCaptchaRegion  string `json:"tencent_captcha_region"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.TencentCaptchaEnabled)
	require.Equal(t, "123456789", resp.Data.TencentCaptchaAppID)
	require.Equal(t, service.TencentCaptchaRegionINTL, resp.Data.TencentCaptchaRegion)
}

func TestSettingHandler_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWeChatConnectEnabled:             "true",
			service.SettingKeyWeChatConnectAppID:               "wx-mp-app",
			service.SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			service.SettingKeyWeChatConnectMode:                "mp",
			service.SettingKeyWeChatConnectScopes:              "snsapi_base",
			service.SettingKeyWeChatConnectOpenEnabled:         "true",
			service.SettingKeyWeChatConnectMPEnabled:           "true",
			service.SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			service.SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			WeChatOAuthEnabled     bool `json:"wechat_oauth_enabled"`
			WeChatOAuthOpenEnabled bool `json:"wechat_oauth_open_enabled"`
			WeChatOAuthMPEnabled   bool `json:"wechat_oauth_mp_enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.WeChatOAuthEnabled)
	require.True(t, resp.Data.WeChatOAuthOpenEnabled)
	require.True(t, resp.Data.WeChatOAuthMPEnabled)
}

func TestSettingHandler_GetPublicSettings_OmitsRetiredCodexTicketSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, value := range []string{"", "false", "true"} {
		t.Run(value, func(t *testing.T) {
			repo := &settingHandlerPublicRepoStub{values: map[string]string{"openai_codex_ticket_enabled": value}}
			h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
			h.GetPublicSettings(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var resp struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
			require.NotContains(t, resp.Data, "openai_codex_ticket_enabled")
		})
	}
}

// The frontend hydrates cachedPublicSettings from the SSR-injected payload and
// replaces it with this response on forced refreshes (for example after an
// admin saves system settings). A field the handler forgets to copy reaches the
// browser as its zero value and silently flips the feature flag, so every
// injected field must come back here with the same value.
func TestSettingHandler_GetPublicSettings_MatchesInjectionPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// "true" for every key turns on every boolean flag and fills every string,
	// so a field missing from the handler cannot hide behind its zero value.
	// custom_endpoints needs a real JSON array: the injection forwards the raw
	// JSON while the handler decodes it into dto.CustomEndpoint.
	repo := &settingHandlerPublicRepoStub{
		fallback: "true",
		values: map[string]string{
			service.SettingKeyCustomEndpoints: `[{"name":"Primary","endpoint":"https://api.example.com","description":"Main entry"}]`,
		},
	}
	settingService := service.NewSettingService(repo, &config.Config{})
	settingService.SetVersion("test-version")
	h := NewSettingHandler(settingService, "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int                        `json:"code"`
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	injection, err := settingService.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	raw, err := json.Marshal(injection)
	require.NoError(t, err)
	var injected map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &injected))

	keys := make([]string, 0, len(injected))
	for key := range injected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		got, ok := resp.Data[key]
		if !assert.Truef(t, ok, "/api/v1/settings/public is missing injected field %q", key) {
			continue
		}
		assert.JSONEqf(t, string(injected[key]), string(got), "/api/v1/settings/public disagrees with the injected value of %q", key)
	}
}

type settingHandlerPublicRepoStub struct {
	values   map[string]string
	fallback string
}

func (s *settingHandlerPublicRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}
func (s *settingHandlerPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}
func (s *settingHandlerPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}
func (s *settingHandlerPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}
func (s *settingHandlerPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}
func (s *settingHandlerPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}
func (s *settingHandlerPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}
