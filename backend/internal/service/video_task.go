package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// A task keeps the customer price agreed at submission, independent of later
// edits to group prices, account mappings or the requested output duration.
type VideoPriceSnapshot struct {
	PrepaidTaskID      string      `json:"prepaid_task_id,omitempty"`
	ReferencePerSecond float64     `json:"reference_per_second,omitempty"`
	ReferenceSeconds   float64     `json:"reference_seconds,omitempty"`
	Model              string      `json:"model"`
	BillingMode        BillingMode `json:"billing_mode"`
	UnitPrice          float64     `json:"unit_price"`
	RateMultiplier     float64     `json:"rate_multiplier"`
	// Missing on older tasks, which must retain their agreed multiplier.
	FinalUnitPrice  bool `json:"final_unit_price,omitempty"`
	DurationSeconds int  `json:"duration_seconds"`
}

type VideoTask struct {
	BillingState                         string
	IdempotencyKey, RequestHash          string
	PreparedBody                         []byte
	ID                                   string
	UpstreamTaskID                       string
	UserID, APIKeyID, GroupID, AccountID int64
	SubscriptionID                       *int64
	Pending                              GrokVideoPendingBilling
	Status                               string
	Attempts                             int
	CreatedAt                            time.Time
}

// Implemented by the SQL usage repository so lifecycle and billing use the
// same database. Redis remains a cache, never the only record of a new task.
type VideoTaskRepository interface {
	CreateVideoTask(context.Context, *VideoTask) error
	AcceptVideoTask(context.Context, *VideoTask) error
	FindVideoTask(context.Context, string, int64, int64) (*VideoTask, error)
	LeaseVideoTasks(context.Context, int) ([]*VideoTask, error)
	SetVideoTaskState(context.Context, string, string, string) error
	VideoTaskWasBilled(context.Context, string, int64) (bool, error)
	VideoTaskUsageRecorded(context.Context, string, int64) (bool, error)
}

type videoTaskContextKey struct{}

func WithVideoTask(ctx context.Context, task *VideoTask) context.Context {
	return context.WithValue(ctx, videoTaskContextKey{}, task)
}

func (s *OpenAIGatewayService) videoTaskRepository() VideoTaskRepository {
	if s == nil {
		return nil
	}
	repo, _ := s.usageLogRepo.(VideoTaskRepository)
	return repo
}

// BeginVideoTask must succeed before sending a potentially paid POST upstream.
func (s *OpenAIGatewayService) BeginVideoTask(ctx context.Context, apiKey *APIKey, account *Account,
	subscription *UserSubscription, model string, endpoint GrokMediaEndpoint, body []byte, contentType string,
) (*VideoTask, error) {
	repo := s.videoTaskRepository()
	if repo == nil || apiKey == nil || apiKey.Group == nil || apiKey.User == nil || account == nil {
		return nil, errors.New("video task storage is unavailable")
	}
	price, err := s.ResolveVideoGroupPricing(ctx, apiKey, model)
	if err != nil {
		return nil, err
	}
	if price.Video != nil {
		return s.beginUnifiedVideoTask(ctx, apiKey, account, subscription, model, body, contentType, price)
	}
	prepared, ct, err := prepareGrokMediaForwardBodyForAccount(account, endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	mapped := strings.TrimSpace(account.GetMappedModel(model))
	if mapped != "" && mapped != model {
		prepared, ct, err = rewriteGrokMediaModel(prepared, ct, mapped)
		if err != nil {
			return nil, err
		}
	}
	prepared, ct, err = normalizeGrokMediaForwardBodyForAccount(account, endpoint, prepared, ct)
	if err != nil {
		return nil, err
	}
	prepared, ct, err = sanitizeGrokMediaForwardBodyForAccount(account, endpoint, prepared, ct)
	if err != nil {
		return nil, err
	}
	info := ParseGrokMediaRequest(ct, prepared)
	if info.N != 1 {
		return nil, errors.New("video requests must create one output per task")
	}
	if account.IsSuanliSpecialVideo() {
		suanliRequestBillingInfo(prepared, &info)
	}
	task := &VideoTask{
		ID: uuid.NewString(), UserID: apiKey.User.ID, APIKeyID: apiKey.ID,
		GroupID: apiKey.Group.ID, AccountID: account.ID, Status: "submitting", CreatedAt: time.Now().UTC(),
		Pending: GrokVideoPendingBilling{
			Model: model, BillingModel: model, OriginalModel: model, UpstreamModel: info.Model,
			VideoProvider: account.VideoMediaProvider(), VideoResolution: info.Resolution,
			VideoDurationSeconds: info.DurationSeconds, CreatedAt: GrokVideoPendingCreatedAtNow(),
			PriceSnapshot: &VideoPriceSnapshot{Model: model, BillingMode: price.BillingMode,
				// Dedicated video prices are final customer prices, just like the
				// per-model image/Grok price cells. Keep a 1x multiplier as well so
				// an older binary can settle this task safely after a rollback.
				UnitPrice: price.UnitPrice, RateMultiplier: 1, FinalUnitPrice: true, DurationSeconds: info.DurationSeconds},
		},
	}
	if subscription != nil {
		sid := subscription.ID
		task.SubscriptionID = &sid
	}
	cost, err := calculateVideoPriceSnapshotCost(task.Pending.PriceSnapshot, 1)
	if err != nil {
		return nil, err
	}
	reservation, err := s.billingCacheService.ReserveInflight(ctx, apiKey.User, apiKey.Group, subscription, cost.ActualCost)
	if err != nil {
		return nil, err
	}
	if reservation != nil {
		task.Pending.InflightReservation, err = reservation.transferToVideo(videoInflightReservationTTL)
		if err != nil {
			reservation.Release()
			return nil, err
		}
	}
	if err := repo.CreateVideoTask(ctx, task); err != nil {
		reservation.Release()
		return nil, err
	}
	return task, nil
}

// Called before the accepted response is written to the customer. Persist the
// upstream ID and normalized billing units even if the customer disconnected.
func (s *OpenAIGatewayService) acceptVideoTask(ctx context.Context, result *OpenAIForwardResult) error {
	task, _ := ctx.Value(videoTaskContextKey{}).(*VideoTask)
	if task == nil {
		return nil
	}
	if result == nil || strings.TrimSpace(result.ResponseID) == "" {
		return errors.New("video supplier returned no task ID; do not automatically resubmit")
	}
	task.UpstreamTaskID = result.ResponseID
	task.Status = "pending"
	task.Pending.UpstreamModel = result.UpstreamModel
	task.Pending.VideoProvider = result.VideoProvider
	task.Pending.VideoResolution = result.VideoResolution
	task.Pending.VideoDurationSeconds = result.VideoDurationSeconds
	if task.Pending.PriceSnapshot != nil && !task.Pending.Unified {
		task.Pending.PriceSnapshot.DurationSeconds = result.VideoDurationSeconds
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer cancel()
	return s.videoTaskRepository().AcceptVideoTask(persistCtx, task)
}

func (s *OpenAIGatewayService) MarkVideoSubmissionUnknown(ctx context.Context, task *VideoTask) {
	if task == nil {
		return
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	// A transport timeout cannot prove that a paid submission was rejected.
	if err := s.videoTaskRepository().SetVideoTaskState(persistCtx, task.ID, "unknown", "submission could not be confirmed; check supplier before retrying"); err != nil {
		slog.Error("video task unknown state could not be persisted", "task_id", task.ID)
	}
}

func (s *OpenAIGatewayService) FindVideoTask(ctx context.Context, remoteID string, userID, keyID int64) (*VideoTask, error) {
	if repo := s.videoTaskRepository(); repo != nil {
		return repo.FindVideoTask(ctx, remoteID, userID, keyID)
	}
	return nil, nil
}

// SettleVideoTask is shared by foreground lookups and the recovery worker.
// The SQL usage billing dedup is authoritative, including after process/Redis loss.
func (s *OpenAIGatewayService) SettleVideoTask(ctx context.Context, task *VideoTask, result *OpenAIForwardResult, keys *APIKeyService) error {
	if task == nil || result == nil || result.VideoCount <= 0 {
		return nil
	}
	repo := s.videoTaskRepository()
	if task.Status == "failed" {
		if task.Pending.Unified {
			return errors.New("该任务已失败或退款，不能交付成功结果")
		}
		return nil
	}
	billed, err := repo.VideoTaskWasBilled(ctx, StableGrokVideoBillingRequestID(task.UpstreamTaskID), task.APIKeyID)
	if err != nil {
		return err
	}
	if billed {
		recorded, err := repo.VideoTaskUsageRecorded(ctx, StableGrokVideoBillingRequestID(task.UpstreamTaskID), task.APIKeyID)
		if err != nil {
			return err
		}
		if recorded {
			return s.finishVideoTask(ctx, task, "done", "")
		}
		if task.Pending.PriceSnapshot == nil {
			return s.markVideoUsageNeedsReview(ctx, task, ErrVideoUsageEvidenceMissing)
		}
	}
	key, err := keys.GetByID(ctx, task.APIKeyID)
	if err != nil {
		return err
	}
	if key.UserID != task.UserID {
		return errors.New("video task ownership changed")
	}
	user, err := s.userRepo.GetByID(ctx, task.UserID)
	if err != nil {
		return err
	}
	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil {
		return err
	}
	if s.channelService == nil || s.channelService.groupRepo == nil {
		return errors.New("video group repository unavailable")
	}
	group, err := s.channelService.groupRepo.GetByID(ctx, task.GroupID)
	if err != nil {
		return err
	}
	keyCopy := *key
	keyCopy.GroupID, keyCopy.Group, keyCopy.User = &task.GroupID, group, user
	var sub *UserSubscription
	if task.SubscriptionID != nil {
		sub, err = s.userSubRepo.GetByID(ctx, *task.SubscriptionID)
		if err != nil {
			return err
		}
	}
	merged := *result
	merged.RequestID = StableGrokVideoBillingRequestID(task.UpstreamTaskID)
	merged.ResponseID = task.UpstreamTaskID
	merged.Model, merged.BillingModel, merged.UpstreamModel = task.Pending.Model, task.Pending.BillingModel, task.Pending.UpstreamModel
	merged.VideoCount, merged.ImageCount = 1, 0
	merged.VideoProvider, merged.VideoResolution = task.Pending.VideoProvider, task.Pending.VideoResolution
	merged.VideoDurationSeconds = task.Pending.VideoDurationSeconds
	merged.VideoPriceSnapshot = task.Pending.PriceSnapshot
	merged.Duration = GrokVideoE2EDuration(task.Pending.CreatedAt, time.Now())
	if merged.VideoPriceSnapshot == nil {
		return errors.New("video task has no immutable price snapshot")
	}
	if err := s.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &merged, APIKey: &keyCopy, User: user, Account: account, Subscription: sub,
		InboundEndpoint: "/v1/videos/generations", UpstreamEndpoint: "/v1/videos",
		RequestPayloadHash: HashUsageRequestPayload([]byte(task.UpstreamTaskID)), APIKeyService: keys,
		QuotaPlatform:      PlatformVideo,
		ChannelUsageFields: ChannelUsageFields{OriginalModel: task.Pending.OriginalModel, ChannelMappedModel: task.Pending.Model},
	}); err != nil {
		if errors.Is(err, ErrUsageBillingRequestConflict) {
			return s.markVideoUsageNeedsReview(ctx, task, err)
		}
		return err
	}
	return s.finishVideoTask(ctx, task, "done", "")
}

func (s *OpenAIGatewayService) markVideoUsageNeedsReview(ctx context.Context, task *VideoTask, cause error) error {
	if err := s.videoTaskRepository().SetVideoTaskState(ctx, task.ID, "needs_review", "video charge and usage evidence require reconciliation; no additional charge applied"); err != nil {
		return err
	}
	return cause
}

func (s *OpenAIGatewayService) StartVideoTaskWorker(keys *APIKeyService) {
	if s == nil || s.videoTaskRepository() == nil || keys == nil {
		return
	}
	s.videoTaskWorkerMu.Lock()
	defer s.videoTaskWorkerMu.Unlock()
	if s.videoTaskWorkerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.videoTaskWorkerCancel = cancel
	s.videoTaskWorkerDone = make(chan struct{})
	go func() {
		defer close(s.videoTaskWorkerDone)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runVideoTaskCycle(ctx, keys)
			}
		}
	}()
}

func (s *OpenAIGatewayService) runVideoTaskCycle(ctx context.Context, keys *APIKeyService) {
	// Keep an admitted settlement batch alive while managed handoff drains.
	// Only periodic scans need leadership; customer status lookups remain usable.
	ctx, release, ok := managedinstance.Begin(ctx)
	if !ok {
		return
	}
	defer release()
	// Settle admitted customer tasks before spending time fetching public prices.
	defer func() {
		s.runSuanliImageTaskCycle(ctx, keys)
		if s.settingService != nil && ctx.Err() == nil {
			syncCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			_, _ = s.SyncVideoPrices(syncCtx, false)
			imageCtx, imageCancel := context.WithTimeout(ctx, 20*time.Second)
			defer imageCancel()
			_, _ = s.SyncImagePrices(imageCtx, false, true)
		}
	}()
	tasks, err := s.videoTaskRepository().LeaseVideoTasks(ctx, 5)
	if err != nil {
		slog.Warn("video task recovery scan failed")
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		pollCtx, stop := context.WithTimeout(ctx, 20*time.Second)
		err := s.pollVideoTask(pollCtx, task, keys)
		stop()
		if err != nil {
			// Status polling is safe to repeat; paid submissions are never retried here.
			slog.Warn("video task recovery will retry", "task_id", task.ID)
		}
	}
}

func (s *OpenAIGatewayService) StopVideoTaskWorker() {
	if s == nil {
		return
	}
	s.videoTaskWorkerMu.Lock()
	cancel, done := s.videoTaskWorkerCancel, s.videoTaskWorkerDone
	s.videoTaskWorkerMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *OpenAIGatewayService) pollVideoTask(ctx context.Context, task *VideoTask, keys *APIKeyService) error {
	if task.Pending.Unified {
		if task.BillingState == "refund_pending" {
			return s.RejectVideoTask(ctx, task)
		}
		_, err := s.UnifiedVideoStatus(ctx, task, keys)
		return err
	}
	repo := s.videoTaskRepository()
	billed, err := repo.VideoTaskWasBilled(ctx, StableGrokVideoBillingRequestID(task.UpstreamTaskID), task.APIKeyID)
	if err != nil {
		return err
	}
	if billed {
		// A charge proves completion. Repair its receipt without another supplier
		// request; the billing repository validates the original fingerprint.
		return s.SettleVideoTask(ctx, task, &OpenAIForwardResult{VideoCount: 1}, keys)
	}
	if time.Since(task.CreatedAt) > 7*24*time.Hour {
		return repo.SetVideoTaskState(ctx, task.ID, "needs_review", "supplier task unresolved for seven days; review billing")
	}
	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil {
		return err
	}
	if account.Platform != PlatformVideo || account.Type != AccountTypeAPIKey {
		return errors.New("video task account no longer has compatible protocol")
	}
	targetURL, err := buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoStatus, task.UpstreamTaskID)
	if err != nil {
		return err
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return err
	}
	ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.Proxy != nil && account.ProxyID != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("video query HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return errors.New("video status exceeds limit")
	}
	if account.IsSuanliSpecialVideo() {
		body, err = normalizeSuanliVideoResponse(GrokMediaEndpointVideoStatus, body, task.UpstreamTaskID)
		if err != nil {
			return err
		}
	}
	result := ExtractGrokVideoBillingFromStatusBody(body, &task.Pending, task.UpstreamTaskID, task.Pending.VideoProvider)
	if result != nil {
		return s.SettleVideoTask(ctx, task, result, keys)
	}
	switch strings.ToLower(gjson.GetBytes(body, "status").String()) {
	case "failed", "expired", "cancelled", "canceled":
		return s.finishVideoTask(ctx, task, "failed", "supplier returned terminal failure; no charge")
	}
	return nil
}
