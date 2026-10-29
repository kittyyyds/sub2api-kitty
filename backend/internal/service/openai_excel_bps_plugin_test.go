package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/stretchr/testify/require"
)

func TestBPSPluginScopeIsExplicit(t *testing.T) {
	raw := []byte(`{"account_ids":[12637],"models":[{"model":"gpt-6-astra","upstream_model":"gpt-6-astra"}]}`)
	for _, tc := range []struct {
		id          int64
		model, want string
	}{
		{12637, "gpt-6-astra", "gpt-6-astra"}, {12635, "gpt-6-astra", ""}, {12637, "gpt-6-sol", ""},
	} {
		got, err := matchBPSPluginScope(raw, tc.id, tc.model)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	for _, raw := range []string{`{"account_ids":[],"models":[]}`, `{}`} {
		got, err := matchBPSPluginScope([]byte(raw), 12637, "gpt-6-astra")
		require.NoError(t, err)
		require.Empty(t, got)
	}
	_, err := matchBPSPluginScope([]byte(`{`), 12637, "gpt-6-astra")
	require.Error(t, err)
}

func TestBPSPreparedWireRequiresCompatibleVersion(t *testing.T) {
	for _, v := range []string{"0.1.0", "0.1.1", "", "garbage"} {
		require.False(t, bpsPluginSupportsPreparedWire(v), v)
	}
	for _, v := range []string{"0.1.2", "0.1.3", "0.2.0", "1.0.0"} {
		require.True(t, bpsPluginSupportsPreparedWire(v), v)
	}
}

func TestBPSPinnedPluginUnavailableNeverUsesNativeTransport(t *testing.T) {
	manager := &PluginManager{}
	selected := &pluginRoute{pluginID: 2, rolloutPercent: 100, unavailable: "unavailable"}
	manager.route.Store(selected)
	route := &excelBPSPluginRoute{manager: manager, route: selected, accountID: 12637, sourceModel: "gpt-6-astra", targetModel: "gpt-6-astra"}
	ctx := context.WithValue(context.Background(), excelBPSPluginContextKey{}, route)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, basispoints.ResponsesURL, nil)
	require.NoError(t, err)
	upstream := &pluginRoutingHTTPUpstream{}
	service := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{ID: 12637, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 100}
	response, err := service.doPriorityObservedHTTP(req, "", account)
	require.Error(t, err)
	require.Nil(t, response)
	require.Zero(t, upstream.doCalls)
	manager.route.Store(nil)
	response, err = service.doPriorityObservedHTTP(req, "", account)
	require.Error(t, err)
	require.Nil(t, response)
	require.Zero(t, upstream.doCalls)
}

func TestBPSPluginCannotChangeAccountOrDestination(t *testing.T) {
	route := &excelBPSPluginRoute{accountID: 12637}
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	require.NoError(t, err)
	_, err = route.roundTrip(req, "", &Account{ID: 12637})
	require.Error(t, err)
	req, err = http.NewRequest(http.MethodPost, basispoints.ResponsesURL, nil)
	require.NoError(t, err)
	_, err = route.roundTrip(req, "", &Account{ID: 12635})
	require.Error(t, err)
}

type bpsScopeRepository struct {
	PluginRepository
	row       *PluginInstallation
	afterRead func()
}

func (r *bpsScopeRepository) GetByID(context.Context, int64) (*PluginInstallation, error) {
	if r.afterRead != nil {
		r.afterRead()
	}
	return r.row, nil
}

func TestBPSPluginResolverChecksCommittedScopeAndBinding(t *testing.T) {
	for _, mode := range []string{"matched", "other-account", "other-model", "old-version", "disabled-binding", "changed-route", "other-plugin", "corrupt-config", "zero-rollout"} {
		t.Run(mode, func(t *testing.T) {
			repo := &bpsScopeRepository{row: &PluginInstallation{ID: 2, PluginKey: kittyBPSPluginKey, Version: "0.1.2", ConfigEncrypted: "ENC:{\"account_ids\":[12637],\"models\":[{\"model\":\"gpt-6-astra\",\"upstream_model\":\"gpt-6-astra\"}]}", Bindings: []PluginBinding{{Enabled: true, Capability: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}}}}
			manager := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{}}
			selected := &pluginRoute{pluginID: 2, rolloutPercent: 100}
			manager.route.Store(selected)
			account := &Account{ID: 12637, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			model := "gpt-6-astra"
			wantErr := false
			switch mode {
			case "other-account":
				account.ID = 12635
			case "other-model":
				model = "gpt-6-sol"
			case "old-version":
				repo.row.Version = "0.1.1"
				wantErr = true
			case "disabled-binding":
				repo.row.Bindings[0].Enabled = false
				wantErr = true
			case "changed-route":
				repo.afterRead = func() { manager.route.Store(nil) }
				wantErr = true
			case "other-plugin":
				repo.row.PluginKey = "another.plugin"
			case "corrupt-config":
				repo.row.ConfigEncrypted = "invalid"
				wantErr = true
			case "zero-rollout":
				selected.rolloutPercent = 0
			}
			got, err := manager.resolveExcelBPSPlugin(context.Background(), account, model)
			if wantErr {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			if mode == "matched" {
				require.NotNil(t, got)
				require.Same(t, selected, got.route)
				require.Equal(t, "gpt-6-astra", got.targetModel)
			} else {
				require.Nil(t, got)
			}
		})
	}
}
