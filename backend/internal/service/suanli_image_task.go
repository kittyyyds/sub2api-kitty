package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ImageUsageBillingRepository interface {
	ApplyImageUsage(context.Context, *UsageBillingCommand, *UsageLog) (*UsageBillingApplyResult, error)
}
type imageAtomicBillingAdapter struct{ ImageUsageBillingRepository }

func (a imageAtomicBillingAdapter) ApplyVideoUsage(ctx context.Context, c *UsageBillingCommand, l *UsageLog) (*UsageBillingApplyResult, error) {
	return a.ApplyImageUsage(ctx, c, l)
}

// The task journal survives process loss. Only status GET/POST requests are
// retried; an indeterminate paid submission requires supplier reconciliation.
type SuanliImageSnapshot struct {
	GroupID                                                              int64  `json:"group_id"`
	SubscriptionID                                                       *int64 `json:"subscription_id,omitempty"`
	Model, UpstreamModel, SizeTier, Size, Endpoint, IPAddress, UserAgent string
	UnitPrice                                                            float64                   `json:"unit_price"`
	Reservation                                                          *VideoInflightReservation `json:"reservation,omitempty"`
}
type SuanliImageTask struct {
	ID, IdempotencyHash, RequestHash, UpstreamTaskID, Status string
	UserID, APIKeyID, AccountID                              int64
	Snapshot                                                 SuanliImageSnapshot
	Result                                                   []byte
	CreatedAt                                                time.Time
}
type SuanliImageTaskRepository interface {
	CreateSuanliImageTask(context.Context, *SuanliImageTask) (*SuanliImageTask, bool, error)
	GetSuanliImageTask(context.Context, string, int64, int64) (*SuanliImageTask, error)
	UpdateSuanliImageTask(context.Context, *SuanliImageTask, time.Duration) error
	LeaseSuanliImageTasks(context.Context, int) ([]*SuanliImageTask, error)
	ExpireSuanliImageResults(context.Context) error
}

func (s *OpenAIGatewayService) suanliImageTaskRepository() SuanliImageTaskRepository {
	repo, _ := s.usageLogRepo.(SuanliImageTaskRepository)
	return repo
}
func (s *OpenAIGatewayService) beginSuanliImageTask(ctx context.Context, key *APIKey, account *Account, parsed *OpenAIImagesRequest, model, idem string, unit float64) (*SuanliImageTask, bool, error) {
	repo := s.suanliImageTaskRepository()
	if repo == nil || key.User == nil || key.Group == nil {
		return nil, false, errors.New("图片任务存储不可用，尚未向上游提交")
	}
	if len(idem) > 256 || strings.ContainsAny(idem, "\r\n") {
		return nil, false, errors.New("Idempotency-Key 格式无效")
	}
	id := "simg_" + uuid.NewString()
	if idem == "" {
		idem = id
	}
	t := &SuanliImageTask{ID: id, UserID: key.UserID, APIKeyID: key.ID, AccountID: account.ID, Status: "submitting", CreatedAt: time.Now().UTC(), IdempotencyHash: HashUsageRequestPayload([]byte(idem)), RequestHash: HashUsageRequestPayload(append([]byte(parsed.Endpoint+"\n"+parsed.ContentType+"\n"), parsed.Body...)), Snapshot: SuanliImageSnapshot{GroupID: key.Group.ID, Model: parsed.Model, UpstreamModel: model, SizeTier: parsed.SizeTier, Size: parsed.Size, Endpoint: parsed.Endpoint, UnitPrice: unit, SubscriptionID: parsed.SuanliSubscriptionID, IPAddress: parsed.SuanliIPAddress, UserAgent: parsed.SuanliUserAgent}}
	var sub *UserSubscription
	if parsed.SuanliSubscriptionID != nil {
		sub = &UserSubscription{ID: *parsed.SuanliSubscriptionID}
	}
	reservation, err := s.billingCacheService.ReserveInflight(ctx, key.User, key.Group, sub, unit)
	if err != nil {
		return nil, false, err
	}
	t.Snapshot.Reservation, err = reservation.transferToVideo(24 * time.Hour)
	if err != nil {
		reservation.Release()
		return nil, false, err
	}
	stored, created, err := repo.CreateSuanliImageTask(ctx, t)
	if err != nil || !created {
		_ = s.billingCacheService.releaseVideoInflight(ctx, t.UserID, t.Snapshot.Reservation)
	}
	if err != nil {
		return nil, false, err
	}
	if stored.RequestHash != t.RequestHash {
		return nil, false, errors.New("同一 Idempotency-Key 不能用于不同图片请求")
	}
	return stored, created, nil
}
func suanliTaskResult(t *SuanliImageTask) *OpenAIForwardResult {
	p := t.Snapshot
	return &OpenAIForwardResult{RequestID: t.ID, Model: p.Model, UpstreamModel: p.UpstreamModel, ImageCount: extractOpenAIImageCountFromJSONBytes(t.Result), ImageSize: p.SizeTier, ImageInputSize: p.Size, ImageOutputSizes: collectOpenAIResponseImageOutputSizesFromJSONBytes(t.Result), SuanliImageUnitPrice: &p.UnitPrice, SuanliImageTaskID: t.ID, Duration: time.Since(t.CreatedAt)}
}
func (s *OpenAIGatewayService) finishSuanliImageTask(ctx context.Context, t *SuanliImageTask, status string) error {
	t.Status = status
	if err := s.suanliImageTaskRepository().UpdateSuanliImageTask(ctx, t, 30*time.Second); err != nil {
		return err
	}
	if status == "completed" || status == "failed" {
		return s.billingCacheService.releaseVideoInflight(ctx, t.UserID, t.Snapshot.Reservation)
	}
	return nil
}
func (s *OpenAIGatewayService) settleSuanliImageTask(ctx context.Context, t *SuanliImageTask, keys *APIKeyService) error {
	if t.Status == "completed" {
		return nil
	}
	if t.Status != "ready" || extractOpenAIImageCountFromJSONBytes(t.Result) != 1 {
		return errors.New("图片结果尚未完成")
	}
	key, err := keys.GetByID(ctx, t.APIKeyID)
	if err != nil {
		return err
	}
	if key.UserID != t.UserID {
		return errors.New("图片任务归属不匹配")
	}
	user, err := s.userRepo.GetByID(ctx, t.UserID)
	if err != nil {
		return err
	}
	account, err := s.accountRepo.GetByID(ctx, t.AccountID)
	if err != nil {
		return err
	}
	if s.channelService == nil || s.channelService.groupRepo == nil {
		return errors.New("图片分组不可用")
	}
	group, err := s.channelService.groupRepo.GetByID(ctx, t.Snapshot.GroupID)
	if err != nil {
		return err
	}
	copied := *key
	copied.User = user
	copied.Group = group
	copied.GroupID = &t.Snapshot.GroupID
	var sub *UserSubscription
	if t.Snapshot.SubscriptionID != nil {
		sub, err = s.userSubRepo.GetByID(ctx, *t.Snapshot.SubscriptionID)
		if err != nil {
			return err
		}
	}
	result := suanliTaskResult(t)
	if err = s.RecordUsage(ctx, &OpenAIRecordUsageInput{Result: result, APIKey: &copied, User: user, Account: account, Subscription: sub, APIKeyService: keys, InboundEndpoint: t.Snapshot.Endpoint, UpstreamEndpoint: "/v1/images/generations", IPAddress: t.Snapshot.IPAddress, UserAgent: t.Snapshot.UserAgent, RequestPayloadHash: t.RequestHash, QuotaPlatform: PlatformOpenAI, ChannelUsageFields: ChannelUsageFields{OriginalModel: t.Snapshot.Model}}); err != nil {
		return err
	}
	return s.finishSuanliImageTask(ctx, t, "completed")
}

// The same owner check applies to API calls, Canvas and duplicate submissions.
func (s *OpenAIGatewayService) GetSuanliImageTask(ctx context.Context, id string, owner ImageTaskOwner, keys *APIKeyService) (*ImageTask, error) {
	repo := s.suanliImageTaskRepository()
	if repo == nil {
		return nil, ErrImageTaskUnavailable
	}
	t, err := repo.GetSuanliImageTask(ctx, id, owner.UserID, owner.APIKeyID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrImageTaskNotFound
	}
	if t.Status == "ready" {
		if err = s.settleSuanliImageTask(ctx, t, keys); err != nil {
			return nil, err
		}
	}
	out := &ImageTask{ID: t.ID, TaskID: t.ID, Object: "image.task", Status: ImageTaskStatusProcessing, CreatedAt: t.CreatedAt.Unix(), ExpiresAt: t.CreatedAt.Add(24 * time.Hour).Unix()}
	switch t.Status {
	case "completed":
		out.Status = ImageTaskStatusCompleted
		out.Result = json.RawMessage(t.Result)
		if len(t.Result) == 0 {
			out.Error = json.RawMessage(`{"message":"图片结果已过保存期，请使用此前下载的文件"}`)
		}
	case "failed":
		out.Status = ImageTaskStatusFailed
		out.Error = json.RawMessage(`{"message":"上游图片生成失败，未扣费"}`)
	case "unknown":
		out.Status = "needs_review"
		out.Error = json.RawMessage(`{"message":"上游提交结果待核实，请勿重复提交，联系管理员查单"}`)
	}
	return out, nil
}
func (s *OpenAIGatewayService) runSuanliImageTaskCycle(ctx context.Context, keys *APIKeyService) {
	repo := s.suanliImageTaskRepository()
	if repo == nil {
		return
	}
	cycle, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	tasks, err := repo.LeaseSuanliImageTasks(cycle, 3)
	if err != nil {
		return
	}
	for _, t := range tasks {
		if cycle.Err() != nil {
			return
		}
		if t.Status == "ready" {
			_ = s.settleSuanliImageTask(cycle, t, keys)
			continue
		}
		if t.Status == "submitting" || time.Since(t.CreatedAt) > 24*time.Hour {
			_ = s.finishSuanliImageTask(cycle, t, "unknown")
			continue
		}
		a, e := s.accountRepo.GetByID(cycle, t.AccountID)
		if e != nil || !isSuanliImageAccount(a) {
			continue
		}
		raw, _ := json.Marshal(map[string]string{"id": t.UpstreamTaskID})
		poll, pollCancel := context.WithTimeout(cycle, 12*time.Second)
		body, ct, e := s.sendSuanliImageRequest(poll, a, "/v1/images/result", raw, "application/json")
		pollCancel()
		if e != nil {
			continue
		}
		normalized, pending, e := suanliImageResult(body, ct)
		if e != nil {
			if errors.Is(e, errSuanliImageFailed) {
				_ = s.finishSuanliImageTask(cycle, t, "failed")
			}
			continue
		}
		if pending != "" {
			continue
		}
		t.Result = normalized
		if e = s.finishSuanliImageTask(cycle, t, "ready"); e == nil {
			_ = s.settleSuanliImageTask(cycle, t, keys)
		}
	}
	_ = repo.ExpireSuanliImageResults(cycle)
}
