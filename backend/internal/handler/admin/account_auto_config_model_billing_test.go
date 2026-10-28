//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type modelBillingHandlerGroups struct{ service.GroupRepository }

func (modelBillingHandlerGroups) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return &service.Group{ID: id, Status: service.StatusActive}, nil
}

// Kitty keeps the explicit group + threshold contract; upstream global defaults
// must never replace it or silently broaden an existing customer's charge.
func TestAutoConfigModelBillingScopedRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := service.DefaultOAuthAutoConfig()
	original.Concurrency = 9
	original.ModelBilling = service.ModelBillingConfig{SchemaVersion: 2, Enabled: true, Rules: []service.ModelBillingRule{{Model: "gpt-6.1-sol", Multiplier: 2, GroupID: 5, Enabled: true, Mode: service.ModelBillingContext, ThresholdTokens: 500000}}}
	raw, err := json.Marshal(original)
	require.NoError(t, err)
	repo := &opsHandlerWritableSettings{raw: string(raw)}
	cfg := &config.Config{}
	cfg.Runtime.Role = config.RuntimeRoleGateway
	svc := service.ProvideAccountOpsService(repo, opsHandlerQueue{}, nil, nil, modelBillingHandlerGroups{}, cfg, nil, nil, nil, nil, nil)
	svc.Stop()
	handler := NewAccountOpsHandler(svc, nil)
	router := gin.New()
	router.PUT("/config", handler.SaveAutoConfig)
	router.GET("/config", handler.GetAutoConfig)
	for _, groupID := range []int64{5, -1} {
		requested := original
		requested.ModelBilling.Rules = append([]service.ModelBillingRule(nil), original.ModelBilling.Rules...)
		requested.ModelBilling.Rules[0].GroupID = groupID
		payload, err := json.Marshal(requested)
		require.NoError(t, err)
		before := repo.raw
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if groupID < 0 {
			require.Equal(t, 400, w.Code)
			require.Equal(t, before, repo.raw)
			continue
		}
		require.Equal(t, 200, w.Code, w.Body.String())
		var saved service.OAuthAutoConfig
		require.NoError(t, json.Unmarshal([]byte(repo.raw), &saved))
		require.Equal(t, original.ModelBilling, saved.ModelBilling)
		require.Equal(t, original.IQBootstrap, saved.IQBootstrap)
		require.Equal(t, original.OpenAITwoFAAdmission, saved.OpenAITwoFAAdmission)
		require.Equal(t, original.ExcelBPS, saved.ExcelBPS)
		require.Equal(t, 9, saved.Concurrency)
		get := httptest.NewRecorder()
		router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/config", nil))
		require.Equal(t, 200, get.Code)
		var response struct{ Data service.OAuthAutoConfig }
		require.NoError(t, json.Unmarshal(get.Body.Bytes(), &response))
		require.Equal(t, saved.ModelBilling, response.Data.ModelBilling)
	}
}
