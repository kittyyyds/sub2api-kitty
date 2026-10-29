package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
)

const kittyBPSPluginKey = "local.kitty.oai-basispoints"
const preparedBPSPluginHeader = "X-Sub2api-BPS-Wire-Source-Model"

type excelBPSPluginContextKey struct{}

// Keep the native Excel OAuth admission, token lifecycle, protocol bridge,
// proxy leases and accounting. Only the actual Responses HTTP send is delegated.
type excelBPSPluginRoute struct {
	manager     *PluginManager
	route       *pluginRoute
	accountID   int64
	sourceModel string
	targetModel string
}

func excelBPSPluginFromContext(ctx context.Context) *excelBPSPluginRoute {
	route, _ := ctx.Value(excelBPSPluginContextKey{}).(*excelBPSPluginRoute)
	return route
}

func bpsPluginSupportsPreparedWire(version string) bool {
	parts := strings.Split(strings.SplitN(version, "-", 2)[0], ".")
	if len(parts) != 3 {
		return false
	}
	numbers := [3]int{}
	for i, p := range parts {
		n, e := strconv.Atoi(p)
		if e != nil || n < 0 {
			return false
		}
		numbers[i] = n
	}
	return numbers[0] > 0 || numbers[1] > 1 || (numbers[1] == 1 && numbers[2] >= 2)
}

func matchBPSPluginScope(raw []byte, accountID int64, model string) (string, error) {
	var c struct {
		AccountIDs []int64 `json:"account_ids"`
		Models     []struct {
			Model         string `json:"model"`
			UpstreamModel string `json:"upstream_model"`
		} `json:"models"`
	}
	if len(raw) == 0 || len(raw) > pluginConfigMaxBytes || json.Unmarshal(raw, &c) != nil {
		return "", errors.New("BPS plugin scope is invalid")
	}
	allowed := false
	for _, id := range c.AccountIDs {
		if id == accountID {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil
	}
	for _, mapping := range c.Models {
		if mapping.Model == model && strings.TrimSpace(mapping.UpstreamModel) != "" {
			return mapping.UpstreamModel, nil
		}
	}
	return "", nil
}

// Read the committed plugin scope, not a guessed account flag or an arbitrary
// plugin capability. The plugin revalidates this scope at the send boundary.
func (m *PluginManager) resolveExcelBPSPlugin(ctx context.Context, account *Account, model string) (*excelBPSPluginRoute, error) {
	if m == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return nil, nil
	}
	route := m.route.Load()
	if route == nil || route.rolloutPercent <= 0 || int(stablePluginBucket(account.ID)) >= route.rolloutPercent {
		return nil, nil
	}
	if m.repo == nil {
		return nil, errors.New("BPS plugin repository is unavailable")
	}
	installation, err := m.repo.GetByID(ctx, route.pluginID)
	if err != nil || installation == nil {
		return nil, errors.New("BPS plugin binding could not be verified")
	}
	if installation.PluginKey != kittyBPSPluginKey {
		return nil, nil
	}
	raw, err := m.decryptConfig(installation)
	if err != nil {
		return nil, errors.New("BPS plugin scope could not be read")
	}
	target, err := matchBPSPluginScope(raw, account.ID, model)
	if err != nil || target == "" {
		return nil, err
	}
	if !hasEnabledCapability(installation.Bindings, PluginCapabilityOpenAIOAuthOutbound) || m.route.Load() != route {
		return nil, errors.New("BPS plugin binding changed; request not sent")
	}
	if !bpsPluginSupportsPreparedWire(installation.Version) {
		return nil, errors.New("BPS plugin 0.1.2 or newer is required for Excel OAuth routing")
	}
	return &excelBPSPluginRoute{manager: m, route: route, accountID: account.ID, sourceModel: model, targetModel: target}, nil
}

func (r *excelBPSPluginRoute) roundTrip(req *http.Request, proxy string, account *Account) (*http.Response, error) {
	if r == nil || r.manager == nil || r.route == nil || account == nil || account.ID != r.accountID || req == nil || req.URL == nil || req.URL.String() != basispoints.ResponsesURL || req.Method != http.MethodPost {
		return nil, errors.New("BPS plugin request scope mismatch")
	}
	if r.manager.route.Load() != r.route {
		return nil, errors.New("BPS plugin binding changed; native fallback is disabled")
	}
	cloned := req.Clone(req.Context())
	cloned.Header.Set(preparedBPSPluginHeader, r.sourceModel)
	response, handled, err := r.manager.roundTripOpenAIOAuthRoute(req.Context(), cloned, proxy, account, r.route)
	if !handled {
		return nil, errors.New("BPS plugin did not handle the request; native fallback is disabled")
	}
	return response, err
}
