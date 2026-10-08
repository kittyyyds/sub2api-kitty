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

const videoRuntimeSettingKey = "kitty_video_runtime_v1"
const videoCatalogSettingKey = "kitty_suanli_catalog_v1"

type VideoMarkup struct {
	OutputPerSecond    float64 `json:"output_per_second"`
	ReferencePerSecond float64 `json:"reference_per_second"`
	PerRequest         float64 `json:"per_request"`
}

type VideoRuntimeSettings struct {
	AutoSync                bool        `json:"auto_sync"`
	FollowIncrease          bool        `json:"follow_increase"`
	FollowDecrease          bool        `json:"follow_decrease"`
	IntervalSeconds         int         `json:"interval_seconds"`
	Markup                  VideoMarkup `json:"markup"`
	ReferenceRetentionHours int         `json:"reference_retention_hours"`
	MaxConcurrentTasks      int         `json:"max_concurrent_tasks"`
	ReferenceUserMB         int         `json:"reference_user_mb"`
	ReferenceTotalMB        int         `json:"reference_total_mb"`
}

func DefaultVideoRuntimeSettings() VideoRuntimeSettings {
	return VideoRuntimeSettings{FollowIncrease: true, IntervalSeconds: 60, ReferenceRetentionHours: 24,
		MaxConcurrentTasks: 2, ReferenceUserMB: 256, ReferenceTotalMB: 2048}
}

func validVideoMoney(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v < 1e9 }
func (m VideoMarkup) validate() error {
	if !validVideoMoney(m.OutputPerSecond) || !validVideoMoney(m.ReferencePerSecond) || !validVideoMoney(m.PerRequest) {
		return errors.New("加价必须是有限的非负金额")
	}
	return nil
}

func (s *SettingService) GetVideoRuntime(ctx context.Context) (VideoRuntimeSettings, error) {
	cfg := DefaultVideoRuntimeSettings()
	if s == nil || s.settingRepo == nil {
		return cfg, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{videoRuntimeSettingKey})
	if err != nil {
		return cfg, err
	}
	if raw := values[videoRuntimeSettingKey]; raw != "" {
		err = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg, err
}

func (s *SettingService) SaveVideoRuntime(ctx context.Context, cfg VideoRuntimeSettings) error {
	if err := cfg.Markup.validate(); err != nil {
		return err
	}
	if cfg.IntervalSeconds < 30 || cfg.IntervalSeconds > 86400 || cfg.ReferenceRetentionHours < 1 || cfg.ReferenceRetentionHours > 168 ||
		cfg.MaxConcurrentTasks < 1 || cfg.MaxConcurrentTasks > 100 || cfg.ReferenceUserMB < 50 || cfg.ReferenceUserMB > 10240 ||
		cfg.ReferenceTotalMB < cfg.ReferenceUserMB || cfg.ReferenceTotalMB > 102400 {
		return errors.New("同步间隔30–86400秒、素材保留1–168小时、并发1–100；请检查素材容量设置")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, videoRuntimeSettingKey, string(raw))
}

// VideoModelConfig is stored in the existing group's model_pricing JSON. Old
// entries without this field keep their legacy provider and billing contract.
type VideoModelConfig struct {
	AutoPrice          bool                `json:"auto_price"`
	Markup             *VideoMarkup        `json:"markup,omitempty"`
	FollowIncrease     *bool               `json:"follow_increase,omitempty"`
	FollowDecrease     *bool               `json:"follow_decrease,omitempty"`
	ReferencePerSecond float64             `json:"reference_per_second"`
	ResolutionPrices   map[string]float64  `json:"resolution_prices,omitempty"`
	Spec               UnifiedVideoSpec    `json:"spec"`
	Cost               *VideoSupplierPrice `json:"cost,omitempty"`
	SyncedAt           string              `json:"synced_at,omitempty"`
	AppliedMarkup      VideoMarkup         `json:"applied_markup"`
}

type VideoSupplierPrice struct {
	Model              string             `json:"model"`
	BillingMode        BillingMode        `json:"billing_mode"`
	UnitPrice          float64            `json:"unit_price"`
	ReferencePerSecond float64            `json:"reference_per_second"`
	ResolutionPrices   map[string]float64 `json:"resolution_prices,omitempty"`
	Spec               *UnifiedVideoSpec  `json:"spec,omitempty"`
	UnavailableReason  string             `json:"unavailable_reason,omitempty"`
}
type VideoPriceChange struct {
	At           string  `json:"at"`
	GroupID      int64   `json:"group_id"`
	Model        string  `json:"model"`
	Old          float64 `json:"old"`
	New          float64 `json:"new"`
	OldReference float64 `json:"old_reference"`
	NewReference float64 `json:"new_reference"`
}
type VideoSupplierCatalog struct {
	Version     string                        `json:"version"`
	LastAttempt string                        `json:"last_attempt"`
	LastSuccess string                        `json:"last_success"`
	LastError   string                        `json:"last_error"`
	Models      map[string]VideoSupplierPrice `json:"models"`
	Changes     []VideoPriceChange            `json:"changes"`
}

func (s *SettingService) GetVideoCatalog(ctx context.Context) (VideoSupplierCatalog, error) {
	c := VideoSupplierCatalog{Models: map[string]VideoSupplierPrice{}, Changes: []VideoPriceChange{}}
	if s == nil || s.settingRepo == nil {
		return c, nil
	}
	v, err := s.settingRepo.GetMultiple(ctx, []string{videoCatalogSettingKey})
	if err != nil {
		return c, err
	}
	if raw := v[videoCatalogSettingKey]; raw != "" {
		err = json.Unmarshal([]byte(raw), &c)
	}
	return c, err
}

func parseVideoSupplierCatalog(body []byte) (map[string]VideoSupplierPrice, string, error) {
	var payload struct {
		Success        bool               `json:"success"`
		PricingVersion json.RawMessage    `json:"pricing_version"`
		GroupRatio     map[string]float64 `json:"group_ratio"`
		Data           []struct {
			Model string   `json:"model_name"`
			Mode  string   `json:"billing_mode"`
			Price *float64 `json:"model_price"`
			Video *struct {
				Output     *float64           `json:"output_per_second"`
				Reference  *float64           `json:"ref_video_per_second"`
				Resolution map[string]float64 `json:"resolution_per_second"`
			} `json:"video_pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", err
	}
	if !payload.Success || len(payload.Data) == 0 {
		return nil, "", errors.New("上游价格列表为空或未成功")
	}
	ratio, ok := payload.GroupRatio["default"]
	if !ok || ratio != 1 {
		return nil, "", errors.New("上游默认分组倍率已变化，需要核实成本")
	}
	out := map[string]VideoSupplierPrice{}
	for _, row := range payload.Data {
		if strings.TrimSpace(row.Model) == "" {
			continue
		}
		spec, known := UnifiedVideoSpecification(row.Model)
		if !known {
			continue
		} // The mixed catalog also contains text and image models.
		p := VideoSupplierPrice{Model: row.Model, Spec: &spec}
		if row.Price == nil || !validVideoMoney(*row.Price) {
			p.UnavailableReason = "缺少有效价格"
			out[row.Model] = p
			continue
		}
		p.UnitPrice = *row.Price
		switch row.Mode {
		case "per_request":
			p.BillingMode = BillingModePerRequest
		case "per_second":
			p.BillingMode = BillingModeVideo
		case "video_multi":
			p.BillingMode = BillingModeVideo
			if row.Video == nil || row.Video.Output == nil || row.Video.Reference == nil {
				p.UnavailableReason = "多维视频价格不完整"
				break
			}
			p.UnitPrice = *row.Video.Output
			p.ReferencePerSecond = *row.Video.Reference
			p.ResolutionPrices = row.Video.Resolution
		default:
			p.UnavailableReason = "视频Token或未知计费方式尚未核实，不能开放"
		}
		// These documented Wan models charge reference and output at the same rate
		// even though the public API only labels them per_second.
		if row.Mode == "per_second" && spec.ReferenceBilled {
			p.ReferencePerSecond = p.UnitPrice
		}
		if spec.MaxVideos > 0 && row.Mode == "per_second" && !spec.ReferenceBilled {
			p.UnavailableReason = "参考视频计费依据未明确，需要核实后开放"
		}
		if !validVideoMoney(p.UnitPrice) || !validVideoMoney(p.ReferencePerSecond) {
			return nil, "", errors.New("上游视频金额无效")
		}
		for _, v := range p.ResolutionPrices {
			if !validVideoMoney(v) {
				return nil, "", errors.New("上游分辨率价格无效")
			}
		}
		if _, duplicate := out[row.Model]; duplicate {
			return nil, "", errors.New("上游包含重复模型价格")
		}
		out[row.Model] = p
	}
	if len(out) == 0 {
		return nil, "", errors.New("未获得任何可识别的视频模型价格")
	}
	return out, strings.Trim(string(payload.PricingVersion), "\""), nil
}

type VideoPricingGroup struct {
	ID      int64
	Pricing []ChannelModelPricing
}
type VideoPricingRepository interface {
	ListVideoPricingGroups(context.Context, int64, int) ([]VideoPricingGroup, error)
	CompareAndSwapVideoPricing(context.Context, int64, []ChannelModelPricing, []ChannelModelPricing) (bool, error)
	ListVideoPriceChanges(context.Context) ([]VideoPriceChange, error)
}

var videoPriceSyncMu sync.Mutex

func repriceVideoEntry(entry ChannelModelPricing, cost VideoSupplierPrice, cfg VideoRuntimeSettings, at string, initial bool) (ChannelModelPricing, error) {
	if entry.Video == nil || len(entry.Models) != 1 || cost.UnavailableReason != "" {
		return entry, errors.New("型号能力或计费规则尚未核实")
	}
	if !initial && entry.BillingMode != cost.BillingMode {
		return entry, errors.New("计费方式已改变，保留原价格等待核实")
	}
	markup := cfg.Markup
	if entry.Video.Markup != nil {
		markup = *entry.Video.Markup
	}
	if err := markup.validate(); err != nil {
		return entry, err
	}
	up, down := cfg.FollowIncrease, cfg.FollowDecrease
	if entry.Video.FollowIncrease != nil {
		up = *entry.Video.FollowIncrease
	}
	if entry.Video.FollowDecrease != nil {
		down = *entry.Video.FollowDecrease
	}
	choose := func(old, oldMarkup, cost, markup float64) float64 {
		oldCost := math.Max(0, old-oldMarkup)
		if initial || (cost > oldCost && up) || (cost < oldCost && down) {
			return QuantizeUsageBillingAmount(cost + markup)
		}
		return QuantizeUsageBillingAmount(oldCost + markup)
	}
	old := 0.0
	if entry.PerRequestPrice != nil {
		old = *entry.PerRequestPrice
	}
	v := *entry.Video
	add, oldAdd := markup.OutputPerSecond, v.AppliedMarkup.OutputPerSecond
	if cost.BillingMode == BillingModePerRequest {
		add, oldAdd = markup.PerRequest, v.AppliedMarkup.PerRequest
	}
	unit := choose(old, oldAdd, cost.UnitPrice, add)
	entry.PerRequestPrice = &unit
	entry.BillingMode = cost.BillingMode
	v.ReferencePerSecond = 0
	if cost.BillingMode == BillingModeVideo && (entry.Video.Spec.ReferenceBilled || entry.Video.ReferencePerSecond > 0 || cost.Spec != nil && cost.Spec.ReferenceBilled || cost.ReferencePerSecond > 0) {
		v.ReferencePerSecond = choose(entry.Video.ReferencePerSecond, v.AppliedMarkup.ReferencePerSecond, cost.ReferencePerSecond, markup.ReferencePerSecond)
	}
	v.ResolutionPrices = map[string]float64{}
	for resolution, price := range cost.ResolutionPrices {
		oldPrice, exists := entry.Video.ResolutionPrices[resolution]
		if !exists && !initial {
			return entry, errors.New("新增分辨率档位，需要核实后重新导入")
		}
		v.ResolutionPrices[resolution] = choose(oldPrice, v.AppliedMarkup.OutputPerSecond, price, markup.OutputPerSecond)
	}
	if !initial && len(entry.Video.ResolutionPrices) != len(v.ResolutionPrices) {
		return entry, errors.New("分辨率档位变化，保留原价格")
	}
	v.Cost = &cost
	v.AppliedMarkup = markup
	v.SyncedAt = at
	entry.Video = &v
	return entry, nil
}

// The only network target is the provider's public read-only price API. No
// customer/supplier credential or configurable URL is sent to this endpoint.
func (s *OpenAIGatewayService) SyncVideoPrices(ctx context.Context, force bool) (VideoSupplierCatalog, error) {
	if !videoPriceSyncMu.TryLock() {
		return VideoSupplierCatalog{}, errors.New("价格同步正在进行，请稍后查看结果")
	}
	defer videoPriceSyncMu.Unlock()
	cfg, err := s.settingService.GetVideoRuntime(ctx)
	if err != nil {
		return VideoSupplierCatalog{}, err
	}
	state, err := s.settingService.GetVideoCatalog(ctx)
	if err != nil {
		return state, err
	}
	if !force && !cfg.AutoSync {
		return state, nil
	}
	if at, e := time.Parse(time.RFC3339, state.LastAttempt); !force && e == nil && time.Since(at) < time.Duration(cfg.IntervalSeconds)*time.Second {
		return state, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	state.LastAttempt = now
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://suanliai.top/api/pricing", nil)
	if err != nil {
		return state, err
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, fetchErr := client.Do(request)
	var models map[string]VideoSupplierPrice
	var version string
	if fetchErr == nil {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			fetchErr = fmt.Errorf("价格接口 HTTP %d", resp.StatusCode)
		} else {
			var body []byte
			body, fetchErr = io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
			if fetchErr == nil && len(body) > 8<<20 {
				fetchErr = errors.New("价格响应超出大小限制")
			}
			if fetchErr == nil {
				models, version, fetchErr = parseVideoSupplierCatalog(body)
			}
		}
	}
	if fetchErr != nil {
		state.LastError = "价格同步失败，继续使用最后有效价格"
	} else {
		state.Models, state.Version, state.LastSuccess, state.LastError = models, version, now, ""
		if repo, ok := s.usageLogRepo.(VideoPricingRepository); ok {
			for after := int64(0); ; {
				groups, e := repo.ListVideoPricingGroups(ctx, after, 50)
				if e != nil {
					return state, e
				}
				if len(groups) == 0 {
					break
				}
				for _, g := range groups {
					after = g.ID
					next := append([]ChannelModelPricing(nil), g.Pricing...)
					changes := []VideoPriceChange{}
					for i, entry := range next {
						if entry.Video == nil || entry.PerRequestPrice == nil || !entry.Video.AutoPrice || len(entry.Models) != 1 {
							continue
						}
						cost, exists := models[entry.Models[0]]
						if !exists {
							state.LastError = "部分型号缺价，保留其最后有效价格"
							continue
						}
						updated, e := repriceVideoEntry(entry, cost, cfg, now, false)
						if e != nil {
							state.LastError = "部分型号计费规则变化，保留其最后有效价格"
							continue
						}
						next[i] = updated
						if *entry.PerRequestPrice != *updated.PerRequestPrice || entry.Video.ReferencePerSecond != updated.Video.ReferencePerSecond || !reflect.DeepEqual(entry.Video.ResolutionPrices, updated.Video.ResolutionPrices) {
							changes = append(changes, VideoPriceChange{At: now, GroupID: g.ID, Model: entry.Models[0], Old: *entry.PerRequestPrice, New: *updated.PerRequestPrice, OldReference: entry.Video.ReferencePerSecond, NewReference: updated.Video.ReferencePerSecond})
						}
					}
					if !reflect.DeepEqual(g.Pricing, next) {
						applied, e := repo.CompareAndSwapVideoPricing(ctx, g.ID, g.Pricing, next)
						if e != nil {
							return state, e
						}
						if applied {
							state.Changes = append(state.Changes, changes...)
							s.invalidateVideoPricingCache(ctx, g.ID)
						}
					}
				}
			}
		}
	}
	if len(state.Changes) > 500 {
		state.Changes = state.Changes[len(state.Changes)-500:]
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return state, err
	}
	if err = s.settingService.settingRepo.Set(ctx, videoCatalogSettingKey, string(raw)); err != nil {
		return state, err
	}
	return state, fetchErr
}

func (s *OpenAIGatewayService) VideoAdminSettings(ctx context.Context) (VideoRuntimeSettings, VideoSupplierCatalog, error) {
	cfg, err := s.settingService.GetVideoRuntime(ctx)
	if err != nil {
		return cfg, VideoSupplierCatalog{}, err
	}
	cat, err := s.settingService.GetVideoCatalog(ctx)
	if err == nil {
		if repo, ok := s.usageLogRepo.(VideoPricingRepository); ok {
			cat.Changes, err = repo.ListVideoPriceChanges(ctx)
		}
	}
	return cfg, cat, err
}

func (s *OpenAIGatewayService) invalidateVideoPricingCache(ctx context.Context, groupID int64) {
	if s.channelService != nil && s.channelService.authCacheInvalidator != nil {
		s.channelService.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
	}
}
func (s *OpenAIGatewayService) SaveVideoAdminSettings(ctx context.Context, cfg VideoRuntimeSettings) error {
	return s.settingService.SaveVideoRuntime(ctx, cfg)
}

func (s *OpenAIGatewayService) VideoRuntimeSettings(ctx context.Context) (VideoRuntimeSettings, error) {
	return s.settingService.GetVideoRuntime(ctx)
}

func (s *OpenAIGatewayService) ImportVideoPricing(ctx context.Context, groupID int64, models []string) error {
	if len(models) < 1 || len(models) > 200 {
		return errors.New("请选择1–200个视频型号")
	}
	cfg, cat, err := s.VideoAdminSettings(ctx)
	if err != nil {
		return err
	}
	group, err := s.channelService.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group.Platform != PlatformVideo {
		return errors.New("请选择视频分组")
	}
	next := append([]ChannelModelPricing(nil), group.ModelPricing...)
	existing := map[string]bool{}
	for _, p := range next {
		for _, m := range p.Models {
			existing[m] = true
		}
	}
	sort.Strings(models)
	for _, model := range models {
		if existing[model] {
			return fmt.Errorf("型号 %s 已有价格，请先在分组中检查，导入不会覆盖", model)
		}
		cost, ok := cat.Models[model]
		if !ok || cost.Spec == nil || cost.UnavailableReason != "" || !cost.Spec.Supported {
			return fmt.Errorf("型号 %s 的参数或计费尚未核实", model)
		}
		p := ChannelModelPricing{Platform: PlatformVideo, Models: []string{model}, BillingMode: cost.BillingMode, Video: &VideoModelConfig{AutoPrice: true, Spec: *cost.Spec}}
		p, err = repriceVideoEntry(p, cost, cfg, time.Now().UTC().Format(time.RFC3339), true)
		if err != nil {
			return err
		}
		next = append(next, p)
		existing[model] = true
	}
	if err := validateVideoGroupPricing(next); err != nil {
		return err
	}
	repo, ok := s.usageLogRepo.(VideoPricingRepository)
	if !ok {
		return errors.New("视频价格存储不可用")
	}
	applied, err := repo.CompareAndSwapVideoPricing(ctx, groupID, group.ModelPricing, next)
	if err != nil {
		return err
	}
	if !applied {
		return errors.New("分组已被修改，请刷新后再试")
	}
	s.invalidateVideoPricingCache(ctx, groupID)
	return nil
}
