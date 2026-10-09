package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

const imageRuntimeSettingKey = "kitty_image_runtime_v1"
const imageCatalogSettingKey = "kitty_suanli_image_catalog_v1"

// Image settings deliberately have their own storage and save operation.
type ImageRuntimeSettings struct {
	AutoSync                bool    `json:"auto_sync"`
	AutoImportAccountModels bool    `json:"auto_import_account_models"`
	FollowIncrease          bool    `json:"follow_increase"`
	FollowDecrease          bool    `json:"follow_decrease"`
	IntervalSeconds         int     `json:"interval_seconds"`
	Markup                  float64 `json:"markup"`
}

func DefaultImageRuntimeSettings() ImageRuntimeSettings {
	return ImageRuntimeSettings{FollowIncrease: true, IntervalSeconds: 60}
}

type ImageSupplierPrice struct {
	Model       string  `json:"model"`
	UnitPrice   float64 `json:"unit_price"`
	Description string  `json:"description,omitempty"`
	BillingMode string  `json:"billing_mode"`
}

// Stored beside the existing group pricing. The final tier cells remain the
// billing authority; a separately edited cell is always treated as manual.
type ImageModelConfig struct {
	Provider      string             `json:"provider"`
	UpstreamModel string             `json:"upstream_model"`
	Description   string             `json:"description,omitempty"`
	AutoPrice     bool               `json:"auto_price"`
	Cost          float64            `json:"cost"`
	AppliedMarkup float64            `json:"applied_markup"`
	AppliedPrices map[string]float64 `json:"applied_prices"`
}
type ImagePricingGroup struct {
	ID      int64                         `json:"id"`
	Name    string                        `json:"name"`
	Pricing []ChannelModelPricing         `json:"pricing"`
	Prices  map[string]map[string]float64 `json:"prices"`
}
type ImagePricingRepository interface {
	ListImagePricingGroups(context.Context, int64, int) ([]ImagePricingGroup, error)
	ListImageAccountModelBindings(context.Context, int64) ([]VideoAccountModelBinding, error)
	CompareAndSwapImagePricing(context.Context, ImagePricingGroup, ImagePricingGroup) (bool, error)
}
type ImageSupplierCatalog struct {
	Version         string                        `json:"version"`
	LastAttempt     string                        `json:"last_attempt"`
	LastSyncAttempt string                        `json:"last_sync_attempt"`
	LastSuccess     string                        `json:"last_success"`
	LastError       string                        `json:"last_error"`
	Models          map[string]ImageSupplierPrice `json:"models"`
}
type ImageAdminState struct {
	Settings ImageRuntimeSettings `json:"settings"`
	Catalog  ImageSupplierCatalog `json:"catalog"`
	Groups   []ImagePricingGroup  `json:"groups"`
}

func (s *SettingService) GetImageRuntime(ctx context.Context) (ImageRuntimeSettings, error) {
	cfg := DefaultImageRuntimeSettings()
	if s == nil || s.settingRepo == nil {
		return cfg, nil
	}
	v, err := s.settingRepo.GetMultiple(ctx, []string{imageRuntimeSettingKey})
	if err != nil {
		return cfg, err
	}
	if v[imageRuntimeSettingKey] != "" {
		err = json.Unmarshal([]byte(v[imageRuntimeSettingKey]), &cfg)
	}
	return cfg, err
}
func (s *OpenAIGatewayService) SaveImageAdminSettings(ctx context.Context, cfg ImageRuntimeSettings) error {
	if !validVideoMoney(cfg.Markup) || cfg.IntervalSeconds < 30 || cfg.IntervalSeconds > 86400 {
		return errors.New("加价必须为非负金额，同步间隔为30–86400秒")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.settingService.settingRepo.Set(ctx, imageRuntimeSettingKey, string(raw))
}
func (s *SettingService) GetImageCatalog(ctx context.Context) (ImageSupplierCatalog, error) {
	out := ImageSupplierCatalog{Models: map[string]ImageSupplierPrice{}}
	if s == nil || s.settingRepo == nil {
		return out, nil
	}
	v, err := s.settingRepo.GetMultiple(ctx, []string{imageCatalogSettingKey})
	if err != nil {
		return out, err
	}
	if v[imageCatalogSettingKey] != "" {
		err = json.Unmarshal([]byte(v[imageCatalogSettingKey]), &out)
	}
	return out, err
}
func parseImageSupplierCatalog(body []byte) (map[string]ImageSupplierPrice, string, error) {
	var p struct {
		Success    bool               `json:"success"`
		Version    string             `json:"pricing_version"`
		GroupRatio map[string]float64 `json:"group_ratio"`
		Data       []struct {
			Model       string   `json:"model_name"`
			Price       *float64 `json:"model_price"`
			Mode        string   `json:"billing_mode"`
			Description string   `json:"description"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, "", err
	}
	if !p.Success || len(p.Data) == 0 || p.GroupRatio["default"] != 1 {
		return nil, "", errors.New("上游目录或默认分组倍率变化，需要核实")
	}
	out := map[string]ImageSupplierPrice{}
	for _, r := range p.Data {
		// No static model-name allowlist. Only explicitly selected image accounts
		// may publish these candidates; video and text groups never participate.
		if r.Mode != "per_request" || r.Price == nil || !validVideoMoney(*r.Price) || strings.TrimSpace(r.Model) == "" {
			continue
		}
		if _, ok := out[r.Model]; ok {
			return nil, "", errors.New("上游存在重复型号")
		}
		out[r.Model] = ImageSupplierPrice{r.Model, *r.Price, strings.TrimSpace(r.Description), r.Mode}
	}
	if len(out) == 0 {
		return nil, "", errors.New("上游没有可解析的按次价格")
	}
	return out, p.Version, nil
}
func ImageGroupModelConfig(g *Group, model string) *ImageModelConfig {
	if g == nil || g.Platform != PlatformOpenAI || !g.AllowImageGeneration {
		return nil
	}
	for _, p := range g.ModelPricing {
		if p.Image != nil && p.Image.Provider == "suanli" {
			for _, m := range p.Models {
				if m == model || CanonicalImagePriceModel(m) == CanonicalImagePriceModel(model) {
					return p.Image
				}
			}
		}
	}
	return nil
}
func ImageGroupCapability(g *Group, model string) (CanvasModelCapability, bool) {
	if ImageGroupModelConfig(g, model) == nil {
		return CanvasModelCapability{}, false
	}
	return CanvasModelCapability{MediaType: "image", Supported: true, Provider: "suanli", Generic: true, Operations: []string{"text_to_image", "image_edit"}, InputTransport: []string{"data_uri", "multipart"}}, true
}

func reconcileImageAccountPricing(old ImagePricingGroup, bindings []VideoAccountModelBinding, costs map[string]ImageSupplierPrice, cfg ImageRuntimeSettings, reprice bool) (ImagePricingGroup, bool) {
	participates := false
	for _, b := range bindings {
		if isSuanliImageAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": b.BaseURL}}) {
			participates = true
			break
		}
	}
	for _, p := range old.Pricing {
		if p.Image != nil && p.Image.Provider == "suanli" {
			participates = true
			break
		}
	}
	if !participates {
		return old, false
	}
	next := ImagePricingGroup{ID: old.ID, Name: old.Name, Prices: map[string]map[string]float64{}}
	for m, tiers := range old.Prices {
		next.Prices[m] = map[string]float64{}
		for k, v := range tiers {
			next.Prices[m][k] = v
		}
	}
	selected, conflicts := videoAccountSelectedModels(bindings)
	for _, b := range bindings {
		if !isSuanliImageAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": b.BaseURL}}) {
			for model := range b.Models {
				conflicts[model] = true
			}
		}
	}
	canonical := map[string]string{}
	for model := range selected {
		key := CanonicalImagePriceModel(model)
		if other, ok := canonical[key]; ok && other != model {
			conflicts[other] = true
			conflicts[model] = true
		} else {
			canonical[key] = model
		}
	}
	warning := len(conflicts) > 0
	existing := map[string]bool{}
	for _, p := range old.Pricing {
		for _, m := range p.Models {
			existing[CanonicalImagePriceModel(m)] = true
		}
		if p.Image == nil || len(p.Models) != 1 || p.Image.Provider != "suanli" {
			next.Pricing = append(next.Pricing, p)
			continue
		}
		model := p.Models[0]
		key := CanonicalImagePriceModel(model)
		upstream, bound := selected[model]
		if !bound && !conflicts[model] && cfg.AutoImportAccountModels && p.Image.AutoPrice && reflect.DeepEqual(next.Prices[key], p.Image.AppliedPrices) {
			delete(next.Prices, key)
			continue
		}
		if !bound || conflicts[model] {
			next.Pricing = append(next.Pricing, p)
			continue
		}
		cost, ok := costs[upstream]
		if !ok {
			warning = true
			next.Pricing = append(next.Pricing, p)
			continue
		}
		cp := *p.Image
		cp.AppliedPrices = map[string]float64{}
		for k, v := range p.Image.AppliedPrices {
			cp.AppliedPrices[k] = v
		}
		p.Image = &cp
		// Any edit outside this synchronizer turns this model into a manual price.
		if !reflect.DeepEqual(next.Prices[key], cp.AppliedPrices) {
			cp.AutoPrice = false
		}
		if cp.AutoPrice && reprice {
			for tier, previous := range cp.AppliedPrices {
				target := math.Round((cost.UnitPrice+cfg.Markup)*1e8) / 1e8
				priorCost := previous - cp.AppliedMarkup
				if cp.UpstreamModel == upstream && ((cost.UnitPrice > priorCost && !cfg.FollowIncrease) || (cost.UnitPrice < priorCost && !cfg.FollowDecrease)) {
					target = math.Round((priorCost+cfg.Markup)*1e8) / 1e8
				}
				next.Prices[key][tier] = target
				cp.AppliedPrices[tier] = target
			}
			cp.AppliedMarkup = cfg.Markup
		}
		cp.Cost = cost.UnitPrice
		cp.Description = cost.Description
		cp.UpstreamModel = upstream
		next.Pricing = append(next.Pricing, p)
	}
	if cfg.AutoImportAccountModels {
		names := make([]string, 0, len(selected))
		for m := range selected {
			names = append(names, m)
		}
		sort.Strings(names)
		for _, model := range names {
			key := CanonicalImagePriceModel(model)
			if existing[key] || conflicts[model] {
				continue
			}
			cost, ok := costs[selected[model]]
			if !ok {
				warning = true
				continue
			}
			manual := len(next.Prices[key]) > 0
			tiers := next.Prices[key]
			if !manual {
				price := math.Round((cost.UnitPrice+cfg.Markup)*1e8) / 1e8
				tiers = map[string]float64{"1K": price, "2K": price, "4K": price}
				next.Prices[key] = tiers
			}
			applied := map[string]float64{}
			for t, v := range tiers {
				applied[t] = v
			}
			price := cost.UnitPrice + cfg.Markup
			next.Pricing = append(next.Pricing, ChannelModelPricing{Platform: PlatformOpenAI, Models: []string{model}, BillingMode: BillingModeImage, PerRequestPrice: &price,
				Image: &ImageModelConfig{Provider: "suanli", UpstreamModel: selected[model], Description: cost.Description, AutoPrice: !manual, Cost: cost.UnitPrice, AppliedMarkup: cfg.Markup, AppliedPrices: applied}})
			existing[key] = true
		}
	}
	if len(next.Pricing) == 0 && old.Pricing != nil {
		next.Pricing = []ChannelModelPricing{}
	}
	return next, warning
}

// Explicit editor action: nil restores automatic pricing; zero is a deliberate
// free manual price. Costs always come from the server-side catalog.
func (s *OpenAIGatewayService) SaveImageModelPrice(ctx context.Context, groupID int64, model string, price *float64) error {
	if price != nil && !validVideoMoney(*price) {
		return errors.New("售价必须是非负金额")
	}
	repo, ok := s.usageLogRepo.(ImagePricingRepository)
	if !ok {
		return errors.New("图片定价存储不可用")
	}
	cfg, err := s.settingService.GetImageRuntime(ctx)
	if err != nil {
		return err
	}
	cat, err := s.settingService.GetImageCatalog(ctx)
	if err != nil {
		return err
	}
	rows, err := repo.ListImagePricingGroups(ctx, groupID-1, 1)
	if err != nil {
		return err
	}
	if len(rows) != 1 || rows[0].ID != groupID {
		return errors.New("图片分组不存在")
	}
	old := rows[0]
	raw, _ := json.Marshal(old)
	var next ImagePricingGroup
	if err = json.Unmarshal(raw, &next); err != nil {
		return err
	}
	for i := range next.Pricing {
		p := &next.Pricing[i]
		if len(p.Models) != 1 || p.Models[0] != model || p.Image == nil {
			continue
		}
		cost, ok := cat.Models[p.Image.UpstreamModel]
		if !ok {
			return errors.New("未取得有效上游价格")
		}
		value := cost.UnitPrice + cfg.Markup
		if price != nil {
			value = *price
		}
		tiers := map[string]float64{"1K": value, "2K": value, "4K": value}
		p.Image.AutoPrice = price == nil
		p.Image.Cost = cost.UnitPrice
		p.Image.AppliedMarkup = cfg.Markup
		p.Image.AppliedPrices = tiers
		if next.Prices == nil {
			next.Prices = map[string]map[string]float64{}
		}
		next.Prices[CanonicalImagePriceModel(model)] = tiers
		applied, e := repo.CompareAndSwapImagePricing(ctx, old, next)
		if e != nil {
			return e
		}
		if !applied {
			return errors.New("分组刚被修改，请刷新后重试")
		}
		s.invalidateVideoPricingCache(ctx, groupID)
		return nil
	}
	return errors.New("请先同步账号中已配置的图片模型")
}

var imagePriceSyncMu sync.Mutex

func (s *OpenAIGatewayService) ImageAdminSettings(ctx context.Context) (ImageAdminState, error) {
	var out ImageAdminState
	var err error
	out.Settings, err = s.settingService.GetImageRuntime(ctx)
	if err != nil {
		return out, err
	}
	out.Catalog, err = s.settingService.GetImageCatalog(ctx)
	if err != nil {
		return out, err
	}
	out.Groups = []ImagePricingGroup{}
	if repo, ok := s.usageLogRepo.(ImagePricingRepository); ok {
		for after := int64(0); ; {
			rows, e := repo.ListImagePricingGroups(ctx, after, 50)
			if e != nil {
				return out, e
			}
			if len(rows) == 0 {
				break
			}
			out.Groups = append(out.Groups, rows...)
			after = rows[len(rows)-1].ID
		}
	}
	return out, nil
}
func (s *OpenAIGatewayService) SyncImagePrices(ctx context.Context, force, apply bool) (ImageSupplierCatalog, error) {
	if !imagePriceSyncMu.TryLock() {
		return ImageSupplierCatalog{}, errors.New("图片价格同步正在进行")
	}
	defer imagePriceSyncMu.Unlock()
	cfg, err := s.settingService.GetImageRuntime(ctx)
	if err != nil {
		return ImageSupplierCatalog{}, err
	}
	state, err := s.settingService.GetImageCatalog(ctx)
	if err != nil {
		return state, err
	}
	if !force && !cfg.AutoSync && !cfg.AutoImportAccountModels {
		return state, nil
	}
	if at, e := time.Parse(time.RFC3339, state.LastSyncAttempt); !force && e == nil && time.Since(at) < time.Duration(cfg.IntervalSeconds)*time.Second {
		return state, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	state.LastAttempt = now
	if apply {
		state.LastSyncAttempt = now
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://suanliai.top/api/pricing", nil)
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, fetchErr := client.Do(req)
	if fetchErr == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			fetchErr = fmt.Errorf("价格接口HTTP %d", resp.StatusCode)
		} else {
			var raw []byte
			raw, fetchErr = io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
			if fetchErr == nil && len(raw) > 8<<20 {
				fetchErr = errors.New("价格目录超限")
			}
			if fetchErr == nil {
				var models map[string]ImageSupplierPrice
				var version string
				models, version, fetchErr = parseImageSupplierCatalog(raw)
				if fetchErr == nil {
					state.Models = models
					state.Version = version
				}
			}
		}
	}
	if fetchErr != nil {
		state.LastError = "图片价格读取失败，保留最后有效售价"
	} else {
		state.LastSuccess = now
		state.LastError = ""
		if repo, ok := s.usageLogRepo.(ImagePricingRepository); ok && apply {
			for after := int64(0); ; {
				groups, e := repo.ListImagePricingGroups(ctx, after, 50)
				if e != nil {
					return state, e
				}
				if len(groups) == 0 {
					break
				}
				for _, g := range groups {
					after = g.ID
					bindings, e := repo.ListImageAccountModelBindings(ctx, g.ID)
					if e != nil {
						return state, e
					}
					next, warn := reconcileImageAccountPricing(g, bindings, state.Models, cfg, cfg.AutoSync || force)
					if warn {
						state.LastError = "部分账号型号缺价或映射冲突；已保留原价，请检查账号模型"
					}
					if !reflect.DeepEqual(g, next) {
						applied, e := repo.CompareAndSwapImagePricing(ctx, g, next)
						if e != nil {
							return state, e
						}
						if applied {
							s.invalidateVideoPricingCache(ctx, g.ID)
						}
					}
				}
			}
		}
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return state, err
	}
	if err = s.settingService.settingRepo.Set(ctx, imageCatalogSettingKey, string(raw)); err != nil {
		return state, err
	}
	return state, fetchErr
}
