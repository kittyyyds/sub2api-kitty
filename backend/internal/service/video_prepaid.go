package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type PrepaidVideoRepository interface {
	CreatePrepaidVideoTask(context.Context, *VideoTask, int) (*VideoTask, bool, error)
	RefundVideoTask(context.Context, string, string) error
	MarkVideoRefundPending(context.Context, string) error
	PrepaidVideoReferenceHolds(context.Context) (map[string]time.Time, error)
}
type videoIdempotencyContextKey struct{}

func WithVideoIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, videoIdempotencyContextKey{}, key)
}

type VideoSubmissionReplay struct{ Task *VideoTask }

func (e *VideoSubmissionReplay) Error() string {
	return "该请求已提交，不会重复生成或扣费"
}

type VideoSubmissionRejected struct{}

func (e *VideoSubmissionRejected) Error() string {
	return "上游明确拒绝任务，预扣费用将自动退回"
}

func (s *OpenAIGatewayService) beginUnifiedVideoTask(ctx context.Context, key *APIKey, account *Account, sub *UserSubscription, model string, body []byte, ct string, price *VideoGroupPrice) (*VideoTask, error) {
	if !account.IsSuanliSpecialVideo() {
		return nil, errors.New("统一视频型号需要算力视频上游")
	}
	if sub != nil {
		return nil, errors.New("统一视频预扣目前需要余额计费分组")
	}
	input, err := PrepareUnifiedVideoInput(body, ct, price.Video.Spec)
	if err != nil {
		return nil, err
	}
	input.Model = account.GetMappedModel(model)
	if input.Model == "" {
		input.Model = model
	}
	// A configured alias may only use the exact supplier contract it was quoted for.
	if price.Video.Cost != nil && price.Video.Cost.Model != input.Model {
		return nil, errors.New("模型映射与已核实上游价格不一致")
	}
	canonical, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	referenceSeconds, referenceIDs, err := MeasureVideoReferences(ctx, key.UserID, &input, price.Video.Spec)
	if err != nil {
		return nil, err
	}
	unit := price.UnitPrice
	if len(price.Video.ResolutionPrices) > 0 {
		var ok bool
		unit, ok = price.Video.ResolutionPrices[input.Resolution]
		if !ok {
			return nil, errors.New("当前分辨率尚未配置价格")
		}
	}
	snapshot := &VideoPriceSnapshot{Model: model, BillingMode: price.BillingMode, UnitPrice: unit, RateMultiplier: 1, FinalUnitPrice: true, DurationSeconds: input.Seconds, ReferenceSeconds: referenceSeconds, ReferencePerSecond: price.Video.ReferencePerSecond}
	cost, err := calculateVideoPriceSnapshotCost(snapshot, 1)
	if err != nil {
		return nil, err
	}
	prepared, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	idem, _ := ctx.Value(videoIdempotencyContextKey{}).(string)
	if len(idem) > 128 || strings.ContainsAny(idem, "\r\n\x00") {
		return nil, errors.New("Idempotency-Key长度或格式无效")
	}
	cfg, err := s.settingService.GetVideoRuntime(ctx)
	if err != nil {
		return nil, err
	}
	task := &VideoTask{ID: uuid.NewString(), UserID: key.UserID, APIKeyID: key.ID, GroupID: key.Group.ID, AccountID: account.ID, Status: "submitting", CreatedAt: time.Now().UTC(), PreparedBody: prepared, IdempotencyKey: idem, RequestHash: HashUsageRequestPayload(canonical),
		Pending: GrokVideoPendingBilling{Unified: true, PrepaidAmount: QuantizeUsageBillingAmount(cost.ActualCost), ReferenceIDs: referenceIDs, Model: model, BillingModel: model, OriginalModel: model, UpstreamModel: input.Model, VideoProvider: SuanliSpecialVideoProvider, VideoResolution: input.Resolution, VideoDurationSeconds: input.Seconds, CreatedAt: GrokVideoPendingCreatedAtNow(), PriceSnapshot: snapshot}}
	repo, ok := s.usageLogRepo.(PrepaidVideoRepository)
	if !ok {
		return nil, errors.New("视频预扣账务存储不可用")
	}
	task.Pending.PriceSnapshot.PrepaidTaskID = task.ID
	existing, created, err := repo.CreatePrepaidVideoTask(ctx, task, cfg.MaxConcurrentTasks)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, &VideoSubmissionReplay{Task: existing}
	}
	_ = s.billingCacheService.InvalidateUserBalance(ctx, key.UserID)
	return task, nil
}

func (s *OpenAIGatewayService) RejectVideoTask(ctx context.Context, task *VideoTask) error {
	if task == nil || !task.Pending.Unified {
		return errors.New("没有可退回的视频预扣")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	repo := s.usageLogRepo.(PrepaidVideoRepository)
	if err := repo.MarkVideoRefundPending(ctx, task.ID); err != nil {
		return err
	}
	if err := repo.RefundVideoTask(ctx, task.ID, "上游明确拒绝或生成失败，预扣已退回"); err != nil {
		return err
	}
	return s.billingCacheService.InvalidateUserBalance(ctx, task.UserID)
}

// Unified lookups settle/refund before publishing any signed delivery URL. No
// video file is read here: the client downloads directly from the supplier.
func (s *OpenAIGatewayService) UnifiedVideoStatus(ctx context.Context, task *VideoTask, keys *APIKeyService) ([]byte, error) {
	if task.BillingState == "refunded" || task.BillingState == "refund_pending" {
		if task.BillingState == "refund_pending" {
			if err := s.RejectVideoTask(ctx, task); err != nil {
				return nil, err
			}
		}
		return json.Marshal(map[string]any{"id": task.ID, "local_task_id": task.ID, "status": "failed", "billing_status": "refunded", "delivery_mode": "upstream_direct"})
	}
	if task.UpstreamTaskID == "" {
		return json.Marshal(map[string]any{"id": task.ID, "local_task_id": task.ID, "status": "submission_unknown", "billing_status": task.BillingState, "delivery_mode": "upstream_direct", "message": "提交结果待核实，请查询原任务，不要重新生成"})
	}
	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil {
		return nil, err
	}
	target, err := buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoStatus, task.UpstreamTaskID)
	if err != nil {
		return nil, err
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)
	proxy := ""
	if account.Proxy != nil && account.ProxyID != nil {
		proxy = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	if err != nil {
		return nil, errors.New("视频状态暂时无法获取，请继续查询原任务")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("视频状态查询失败（%d），请继续查询原任务", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return nil, errors.New("视频状态响应无效")
	}
	body, err = normalizeSuanliVideoResponse(GrokMediaEndpointVideoStatus, body, task.UpstreamTaskID)
	if err != nil {
		return nil, err
	}
	status := gjson.GetBytes(body, "status").String()
	body, _ = sjson.SetBytes(body, "delivery_mode", "upstream_direct")
	// Never expose a supplier URL before a paid completion has been verified.
	if status != "done" {
		body, _ = sjson.DeleteBytes(body, "video")
		body, _ = sjson.DeleteBytes(body, "url")
	}
	if status == "done" {
		if err = s.SettleVideoTask(ctx, task, &OpenAIForwardResult{VideoCount: 1}, keys); err != nil {
			return nil, errors.New("视频已生成，正在完成账务结算，请稍后查询")
		}
		body, _ = sjson.SetBytes(body, "status", "completed")
	} else if status == "failed" {
		if err = s.RejectVideoTask(ctx, task); err != nil {
			return nil, errors.New("任务失败，预扣退款处理中，请稍后查询")
		}
	} else if status == "expired" {
		// A missing file is not evidence of failed generation. Never refund it.
		body, _ = sjson.SetBytes(body, "delivery_status", "expired")
		if task.BillingState == "captured" {
			body, _ = sjson.SetBytes(body, "status", "completed")
		} else {
			_ = s.videoTaskRepository().SetVideoTaskState(ctx, task.ID, "needs_review", "supplier file expired without generation evidence; hold retained for review")
		}
	} else {
		body, _ = sjson.SetBytes(body, "status", "in_progress")
	}
	if raw := gjson.GetBytes(body, "video.url").String(); raw != "" {
		if !unifiedDirectVideoURL(raw, target, task.UpstreamTaskID) {
			return nil, errors.New("上游尚未提供可直接下载的视频链接")
		}
		body, _ = sjson.SetBytes(body, "url", raw)
		body, _ = sjson.SetBytes(body, "delivery_mode", "upstream_direct")
	}
	body, _ = sjson.SetBytes(body, "prepaid_amount", task.Pending.PrepaidAmount)
	return body, nil
}

func unifiedDirectVideoURL(raw, supplier, taskID string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !suanliPublicMediaURL(raw, false) {
		return false
	}
	if isGrokMediaVideoContentURL(raw, taskID) {
		// The supplier documents a public signed /content URL. It is not the
		// site's legacy authenticated content relay, despite the matching path.
		base, err := url.Parse(supplier)
		q := u.Query()
		return err == nil && strings.EqualFold(u.Host, base.Host) && q.Get("owner") != "" && q.Get("expires") != "" && q.Get("video_signature") != ""
	}
	return true
}

func (s *OpenAIGatewayService) VideoReferenceHolds(ctx context.Context) (map[string]time.Time, error) {
	if repo, ok := s.usageLogRepo.(PrepaidVideoRepository); ok {
		return repo.PrepaidVideoReferenceHolds(ctx)
	}
	return nil, errors.New("视频素材引用查询不可用")
}
