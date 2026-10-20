package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type GrokMediaEndpoint string

const (
	GrokMediaEndpointImagesGenerations GrokMediaEndpoint = "images_generations"
	GrokMediaEndpointImagesEdits       GrokMediaEndpoint = "images_edits"
	GrokMediaEndpointVideosGenerations GrokMediaEndpoint = "videos_generations"
	GrokMediaEndpointVideosEdits       GrokMediaEndpoint = "videos_edits"
	GrokMediaEndpointVideosExtensions  GrokMediaEndpoint = "videos_extensions"
	GrokMediaEndpointVideoStatus       GrokMediaEndpoint = "video_status"
	GrokMediaEndpointVideoContent      GrokMediaEndpoint = "video_content"

	// Official xAI Imagine image-edit limit.
	grokMediaMaxEditSourceImages = 3
)

func (e GrokMediaEndpoint) RequiresRequestBody() bool {
	return !e.IsVideoLookupRequest()
}

func (e GrokMediaEndpoint) IsVideoLookupRequest() bool {
	return e == GrokMediaEndpointVideoStatus || e == GrokMediaEndpointVideoContent || e == SeedanceEndpointStatus || e == SeedanceEndpointDelete || e == MiniMaxH3EndpointStatus || e == MiniMaxH3EndpointContent
}

func (e GrokMediaEndpoint) IsGenerationRequest() bool {
	if e == SeedanceEndpointCreate || e == MiniMaxH3EndpointCreate {
		return true
	}
	switch e {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits, GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}

type GrokMediaRequestInfo struct {
	Model               string
	Prompt              string
	N                   int
	Size                string
	ResponseFormat      string
	SizeTier            string
	RequestedResolution string
	AspectRatio         string
	ImageResolution     string
	Resolution          string
	DurationSeconds     int
	InputImageURLs      []string
	ReferenceImageURLs  []string
	MaskImageURL        string
	Uploads             []OpenAIImagesUpload
	MaskUpload          *OpenAIImagesUpload
}

func (r GrokMediaRequestInfo) ModerationBody() []byte {
	payload := map[string]any{}
	if prompt := strings.TrimSpace(r.Prompt); prompt != "" {
		payload["prompt"] = prompt
	}

	images := make([]map[string]string, 0, len(r.InputImageURLs)+len(r.ReferenceImageURLs)+len(r.Uploads)+1)
	for _, imageURL := range r.InputImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, map[string]string{"image_url": imageURL})
		}
	}
	for _, imageURL := range r.ReferenceImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, map[string]string{"image_url": imageURL})
		}
	}
	for _, upload := range r.Uploads {
		if dataURL := upload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if maskURL := strings.TrimSpace(r.MaskImageURL); maskURL != "" {
		images = append(images, map[string]string{"image_url": maskURL})
	}
	if r.MaskUpload != nil {
		if dataURL := r.MaskUpload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if len(images) > 0 {
		payload["images"] = images
	}
	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return body
}

func (e GrokMediaEndpoint) httpMethod() string {
	if e.IsVideoLookupRequest() {
		return http.MethodGet
	}
	return http.MethodPost
}

func ExtractGrokMediaModel(contentType string, body []byte) string {
	return ParseGrokMediaRequest(contentType, body).Model
}

func ParseGrokMediaRequest(contentType string, body []byte) GrokMediaRequestInfo {
	info := GrokMediaRequestInfo{N: 1}
	if gjson.ValidBytes(body) {
		parseGrokMediaJSONRequest(body, &info)
	} else {
		parseGrokMediaMultipartRequest(contentType, body, &info)
	}
	info.Model = strings.TrimSpace(info.Model)
	info.Prompt = strings.TrimSpace(info.Prompt)
	info.Size = strings.TrimSpace(info.Size)
	info.ResponseFormat = strings.TrimSpace(info.ResponseFormat)
	info.RequestedResolution = strings.TrimSpace(info.RequestedResolution)
	info.AspectRatio = strings.TrimSpace(info.AspectRatio)
	info.SizeTier = NormalizeImageBillingTierOrDefault(info.Size)
	info.ImageResolution = grokImagineImageResolution(info.ImageResolution)
	info.Resolution = NormalizeVideoBillingResolutionOrDefault(info.Resolution)
	info.DurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(info.DurationSeconds)
	if info.N <= 0 {
		info.N = 1
	}
	return info
}

func parseGrokMediaJSONRequest(body []byte, info *GrokMediaRequestInfo) {
	if info == nil {
		return
	}
	info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	info.Size = strings.TrimSpace(gjson.GetBytes(body, "size").String())
	info.ResponseFormat = strings.TrimSpace(gjson.GetBytes(body, "response_format").String())
	info.RequestedResolution = strings.TrimSpace(gjson.GetBytes(body, "resolution").String())
	info.AspectRatio = strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String())
	if info.AspectRatio == "" {
		info.AspectRatio = strings.TrimSpace(gjson.GetBytes(body, "ratio").String())
	}
	assignGrokMediaResolution(info.RequestedResolution, info)
	if duration := gjson.GetBytes(body, "duration"); duration.Exists() && duration.Type == gjson.Number {
		info.DurationSeconds = int(duration.Int())
	}
	if n := gjson.GetBytes(body, "n"); n.Exists() && n.Type == gjson.Number {
		info.N = int(n.Int())
	}
	appendJSONImageURLs := func(value gjson.Result, target *[]string) {
		if !value.Exists() {
			return
		}
		switch {
		case value.IsArray():
			for _, item := range value.Array() {
				if imageURL := extractGrokMediaImageURL(item); imageURL != "" {
					*target = append(*target, imageURL)
				}
			}
		default:
			if imageURL := extractGrokMediaImageURL(value); imageURL != "" {
				*target = append(*target, imageURL)
			}
		}
	}
	appendJSONImageURLs(gjson.GetBytes(body, "image"), &info.InputImageURLs)
	appendJSONImageURLs(gjson.GetBytes(body, "images"), &info.InputImageURLs)
	appendJSONImageURLs(gjson.GetBytes(body, "reference_images"), &info.ReferenceImageURLs)
	info.MaskImageURL = extractGrokMediaImageURL(gjson.GetBytes(body, "mask"))
}

func extractGrokMediaImageURL(value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String())
	}
	if imageURL := strings.TrimSpace(value.Get("url").String()); imageURL != "" {
		return imageURL
	}
	if nested := value.Get("image_url"); nested.Exists() {
		if nested.Type == gjson.String {
			return strings.TrimSpace(nested.String())
		}
		if imageURL := strings.TrimSpace(nested.Get("url").String()); imageURL != "" {
			return imageURL
		}
	}
	return strings.TrimSpace(value.Get("image_url").String())
}

func grokMediaImageObject(imageURL string) map[string]string {
	return map[string]string{"url": imageURL, "type": "image_url"}
}

func parseGrokMediaMultipartRequest(contentType string, body []byte, info *GrokMediaRequestInfo) {
	if info == nil {
		return
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize))
		_ = part.Close()
		if err != nil {
			return
		}
		fileName := strings.TrimSpace(part.FileName())
		partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))
		if fileName != "" {
			upload := OpenAIImagesUpload{
				FieldName:   name,
				FileName:    fileName,
				ContentType: partContentType,
				Data:        data,
			}
			if name == "mask" {
				info.MaskUpload = &upload
				continue
			}
			if name == "image" || strings.HasPrefix(name, "image[") {
				info.Uploads = append(info.Uploads, upload)
			}
			continue
		}

		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			info.Model = value
		case "prompt":
			info.Prompt = value
		case "size":
			info.Size = value
		case "response_format":
			info.ResponseFormat = value
		case "resolution":
			info.RequestedResolution = value
			assignGrokMediaResolution(value, info)
		case "aspect_ratio", "ratio":
			info.AspectRatio = value
		case "duration":
			if duration, err := strconv.Atoi(value); err == nil {
				info.DurationSeconds = duration
			}
		case "n":
			if n, err := strconv.Atoi(value); err == nil {
				info.N = n
			}
		case "image", "image_url":
			if value != "" {
				info.InputImageURLs = append(info.InputImageURLs, value)
			}
		case "mask", "mask_image_url":
			info.MaskImageURL = value
		}
	}
}

func GrokMediaVideoRequestSessionHash(requestID string, userID, apiKeyID int64) string {
	return grokMediaVideoRequestHash("grok-video:", requestID, userID, apiKeyID)
}

func grokMediaVideoRequestOwnerBindingHash(requestID string, userID, apiKeyID int64) string {
	return grokMediaVideoRequestHash("grok-video-owner:", requestID, userID, apiKeyID)
}

func grokMediaVideoRequestHash(prefix, requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	ownerSeed := fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID)
	return prefix + DeriveSessionHashFromSeed(ownerSeed)
}

func (s *OpenAIGatewayService) BindGrokMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID, accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok video request binding cache is unavailable")
	}
	ownerCacheKey := s.openAISessionCacheKey(grokMediaVideoRequestOwnerBindingHash(requestID, userID, apiKeyID))
	routingCacheKey := s.openAISessionCacheKey(GrokMediaVideoRequestSessionHash(requestID, userID, apiKeyID))
	if ownerCacheKey == "" || routingCacheKey == "" || accountID <= 0 {
		return fmt.Errorf("grok video request binding is invalid")
	}
	group := derefGroupID(groupID)
	// The owner binding is authorization state for an already-created video,
	// not a transient scheduler hint. A zero go-redis expiration keeps it
	// readable for as long as the upstream retains the video.
	if err := s.cache.SetSessionAccountID(ctx, group, ownerCacheKey, accountID, 0); err != nil {
		return fmt.Errorf("persist grok video request owner binding: %w", err)
	}
	// Keep the scheduler route alive for at least the deferred billing window.
	if err := s.cache.SetSessionAccountID(ctx, group, routingCacheKey, accountID, grokVideoRequestBindingTTL(s.cfg)); err != nil {
		return fmt.Errorf("bind grok video request routing session: %w", err)
	}
	return nil
}

func (s *OpenAIGatewayService) ResolveGrokMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID int64,
) (int64, error) {
	if task, err := s.FindVideoTask(ctx, requestID, userID, apiKeyID); err != nil {
		return 0, err
	} else if task != nil {
		if groupID == nil || *groupID != task.GroupID {
			return 0, ErrStickySessionNotFound
		}
		return task.AccountID, nil
	}
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("grok video request binding cache is unavailable")
	}
	ownerCacheKey := s.openAISessionCacheKey(grokMediaVideoRequestOwnerBindingHash(requestID, userID, apiKeyID))
	routingCacheKey := s.openAISessionCacheKey(GrokMediaVideoRequestSessionHash(requestID, userID, apiKeyID))
	if ownerCacheKey == "" || routingCacheKey == "" {
		return 0, fmt.Errorf("grok video request binding is invalid")
	}
	group := derefGroupID(groupID)
	accountID, err := s.cache.GetSessionAccountID(ctx, group, ownerCacheKey)
	if errors.Is(err, ErrStickySessionNotFound) {
		// Before owner bindings were separated from sticky routing, the routing
		// key was the only record. Promote a live legacy key on first read.
		accountID, err = s.cache.GetSessionAccountID(ctx, group, routingCacheKey)
		if err == nil && accountID > 0 {
			if setErr := s.cache.SetSessionAccountID(ctx, group, ownerCacheKey, accountID, 0); setErr != nil {
				return 0, fmt.Errorf("promote grok video request owner binding: %w", setErr)
			}
		}
	}
	if errors.Is(err, ErrStickySessionNotFound) {
		ownerRepo, ok := s.usageLogRepo.(GrokMediaVideoOwnerAccountRepository)
		if !ok || ownerRepo == nil {
			return 0, err
		}
		accountID, err = ownerRepo.FindGrokMediaVideoOwnerAccountID(
			ctx, requestID, userID, apiKeyID, groupID,
		)
		if err != nil {
			return 0, fmt.Errorf("recover grok video request owner binding: %w", err)
		}
		if accountID <= 0 {
			return 0, fmt.Errorf("grok video request binding is invalid")
		}
		if err := s.BindGrokMediaVideoRequestAccount(ctx, groupID, requestID, userID, apiKeyID, accountID); err != nil {
			return 0, fmt.Errorf("rehydrate grok video request binding: %w", err)
		}
		return accountID, nil
	}
	if err != nil {
		return 0, err
	}
	if accountID <= 0 {
		return 0, fmt.Errorf("grok video request binding is invalid")
	}
	// Rehydrate the short-lived scheduler key without shortening the durable
	// owner binding. The scheduler may continue refreshing this key normally.
	if err := s.cache.SetSessionAccountID(ctx, group, routingCacheKey, accountID, grokVideoRequestBindingTTL(s.cfg)); err != nil {
		return 0, fmt.Errorf("refresh grok video request routing session: %w", err)
	}
	return accountID, nil
}

// SelectGrokMediaVideoRequestAccount only admits the already authenticated
// task owner. Generic sticky fallback can query another account and overwrite
// the ownership key; video lookups must neither escape nor refresh that key.
func (s *OpenAIGatewayService) SelectGrokMediaVideoRequestAccount(
	ctx context.Context, groupID *int64, sessionHash string, accountID int64, requestedModel string,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	platform := PlatformGrok
	if s.accountRepo != nil {
		account, err := s.accountRepo.GetByID(ctx, accountID)
		if err != nil {
			return nil, OpenAIAccountScheduleDecision{}, errors.Join(ErrNoAvailableAccounts, err)
		}
		if account != nil && account.Platform == PlatformVideo {
			platform = PlatformVideo
		}
	}
	return s.SelectMediaVideoRequestAccount(ctx, groupID, sessionHash, accountID, requestedModel, platform)
}

func (s *OpenAIGatewayService) SelectMediaVideoRequestAccount(
	ctx context.Context, groupID *int64, sessionHash string, accountID int64, requestedModel, platform string,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerSessionSticky}
	if accountID <= 0 || strings.TrimSpace(sessionHash) == "" {
		return nil, decision, ErrNoAvailableAccounts
	}
	ctx = s.withOpenAIGroupPrivacyRequirement(WithOpenAIProfitControlSuppressed(ctx), groupID)
	scheduler := &defaultOpenAIAccountScheduler{service: s}
	selection, _, err := scheduler.selectBySessionHash(ctx, OpenAIAccountScheduleRequest{
		GroupID: groupID, Platform: platform, SessionHash: sessionHash,
		StickyAccountID: accountID, PreserveStickyBinding: true, DisableStickyEscape: true,
		RequestedModel: requestedModel, RequiredTransport: OpenAIUpstreamTransportHTTPSSE,
		RequirePrivacySet: s.openAIGroupRequiresPrivacySet(ctx, groupID),
	})
	if err != nil {
		return nil, decision, err
	}
	if selection == nil || selection.Account == nil {
		return nil, decision, ErrNoAvailableAccounts
	}
	decision.StickySessionHit = true
	decision.SelectedAccountID = selection.Account.ID
	decision.SelectedAccountType = selection.Account.Type
	return selection, decision, nil
}

// GrokVideoPendingBilling is the create-time snapshot used when status polling
// first observes a completed video URL. Status may omit model/duration; we fall
// back to this snapshot, then defaults.
type GrokVideoPendingBilling struct {
	Unified              bool                      `json:"unified,omitempty"`
	PrepaidAmount        float64                   `json:"prepaid_amount,omitempty"`
	ReferenceIDs         []string                  `json:"reference_ids,omitempty"`
	InflightReservation  *VideoInflightReservation `json:"inflight_reservation,omitempty"`
	PriceSnapshot        *VideoPriceSnapshot       `json:"price_snapshot,omitempty"`
	VideoProvider        string                    `json:"video_provider,omitempty"`
	Model                string                    `json:"model"`
	BillingModel         string                    `json:"billing_model,omitempty"`
	UpstreamModel        string                    `json:"upstream_model,omitempty"`
	VideoResolution      string                    `json:"video_resolution,omitempty"`
	VideoDurationSeconds int                       `json:"video_duration_seconds,omitempty"`
	OriginalModel        string                    `json:"original_model,omitempty"`
	MiniMaxH3BaseCost    *float64                  `json:"minimax_h3_base_cost,omitempty"`
	// CreatedAt is when the gateway accepted the async create (RFC3339Nano UTC).
	// duration_ms for deferred billing is measured from this instant until the
	// first official done+video.url observation (status poll or content download),
	// not the latency of that single discovery request alone.
	CreatedAt string `json:"created_at,omitempty"`
}

// GrokVideoPendingCreatedAtNow formats a create-accept timestamp for pending billing.
func GrokVideoPendingCreatedAtNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// GrokVideoE2EDuration returns wall time from create accept to discovery of completion.
// Returns 0 when CreatedAt is missing or unparseable (caller keeps poll-only Duration).
func GrokVideoE2EDuration(createdAt string, discoveredAt time.Time) time.Duration {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return 0
	}
	if discoveredAt.IsZero() {
		discoveredAt = time.Now()
	}
	var created time.Time
	var err error
	if created, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		if created, err = time.Parse(time.RFC3339, createdAt); err != nil {
			return 0
		}
	}
	if created.IsZero() {
		return 0
	}
	d := discoveredAt.Sub(created)
	if d < 0 {
		return 0
	}
	return d
}

func grokVideoPendingBillingKey(requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID)
}

func grokVideoPendingBillingTTL(cfg *config.Config) time.Duration {
	// Video generation can take several minutes; keep create-time pricing for a day.
	_ = cfg
	return 24 * time.Hour
}

func grokVideoRequestBindingTTL(cfg *config.Config) time.Duration {
	ttl := grokVideoPendingBillingTTL(cfg)
	if cfg != nil && cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		if sticky := time.Duration(cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second; sticky > ttl {
			return sticky
		}
	}
	return ttl
}

func grokVideoBilledClaimTTL(cfg *config.Config) time.Duration {
	_ = cfg
	return 48 * time.Hour
}

// StoreGrokVideoPendingBilling persists create-time billing params for deferred status billing.
func (s *OpenAIGatewayService) StoreGrokVideoPendingBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
	pending GrokVideoPendingBilling,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok video pending billing cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return fmt.Errorf("grok video pending billing key is invalid")
	}
	if pending.InflightReservation == nil {
		if reservation := InflightReservationFromContext(ctx); reservation != nil {
			ref, err := reservation.transferToVideo(grokVideoPendingBillingTTL(s.cfg))
			if err != nil {
				return err
			}
			pending.InflightReservation = ref
		}
	}
	pending.Model = strings.TrimSpace(pending.Model)
	pending.BillingModel = strings.TrimSpace(pending.BillingModel)
	pending.UpstreamModel = strings.TrimSpace(pending.UpstreamModel)
	pending.OriginalModel = strings.TrimSpace(pending.OriginalModel)
	if pending.VideoResolution != "" && pending.MiniMaxH3BaseCost == nil {
		pending.VideoResolution = NormalizeVideoBillingResolutionOrDefault(pending.VideoResolution)
	}
	if pending.VideoDurationSeconds > 0 {
		pending.VideoDurationSeconds = NormalizeVideoBillingDurationForProvider(pending.VideoProvider, pending.VideoDurationSeconds)
	}
	// Always stamp create-accept time when missing so deferred duration_ms is E2E.
	if strings.TrimSpace(pending.CreatedAt) == "" {
		pending.CreatedAt = GrokVideoPendingCreatedAtNow()
	} else {
		pending.CreatedAt = strings.TrimSpace(pending.CreatedAt)
	}
	payload, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return s.cache.SetGrokVideoPendingBilling(ctx, key, payload, grokVideoPendingBillingTTL(s.cfg))
}

// LoadGrokVideoPendingBilling returns the create-time snapshot (may be nil on miss).
func (s *OpenAIGatewayService) LoadGrokVideoPendingBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
) (*GrokVideoPendingBilling, error) {
	if task, err := s.FindVideoTask(ctx, requestID, userID, apiKeyID); err != nil {
		return nil, err
	} else if task != nil {
		return &task.Pending, nil
	}
	if s == nil || s.cache == nil {
		return nil, fmt.Errorf("grok video pending billing cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return nil, fmt.Errorf("grok video pending billing key is invalid")
	}
	payload, err := s.cache.GetGrokVideoPendingBilling(ctx, key)
	if err != nil || len(payload) == 0 {
		return nil, err
	}
	var pending GrokVideoPendingBilling
	if err := json.Unmarshal(payload, &pending); err != nil {
		return nil, err
	}
	return &pending, nil
}

// ClaimGrokVideoBilling returns true once for a completed video request so status
// polls do not double-bill. Fail-closed: claim errors are treated as already billed.
func (s *OpenAIGatewayService) ClaimGrokVideoBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
) (bool, error) {
	if s == nil || s.cache == nil {
		return false, fmt.Errorf("grok video billing claim cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return false, fmt.Errorf("grok video billing claim key is invalid")
	}
	return s.cache.ClaimGrokVideoBilled(ctx, key, grokVideoBilledClaimTTL(s.cfg))
}

// ReleaseGrokVideoBilling clears a claim after a failed durable RecordUsage so a
// later status/content poll can retry billing.
func (s *OpenAIGatewayService) ReleaseGrokVideoBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok video billing claim cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return fmt.Errorf("grok video billing claim key is invalid")
	}
	return s.cache.ReleaseGrokVideoBilled(ctx, key)
}

// StableGrokVideoBillingRequestID is the durable usage_logs / dedup key for one
// async video task (not the per-poll gateway request id).
func StableGrokVideoBillingRequestID(taskRequestID string) string {
	taskRequestID = strings.TrimSpace(taskRequestID)
	if taskRequestID == "" {
		return ""
	}
	if strings.HasPrefix(taskRequestID, "grok-video:") {
		return taskRequestID
	}
	return "grok-video:" + taskRequestID
}

// Official xAI async video status success shape (docs.x.ai Video Generation):
//
//	{"status":"done","model":"grok-imagine-video-1.5","video":{"url":"...","duration":8,"respect_moderation":true}}
//
// Request may include resolution ("480p"|"720p"|"1080p"); completed status does not
// document a resolution field — bill resolution from the create-time request snapshot.

// IsGrokVideoStatusBillable matches official success: status == "done" AND non-empty video.url.
// pending / expired / failed, or done without a video URL, are not billable.
func IsGrokVideoStatusBillable(statusBody []byte) bool {
	if len(statusBody) == 0 || !gjson.ValidBytes(statusBody) {
		return false
	}
	if !isOfficialGrokVideoStatusDone(statusBody) {
		return false
	}
	return strings.TrimSpace(gjson.GetBytes(statusBody, "video.url").String()) != ""
}

func isOfficialGrokVideoStatusDone(statusBody []byte) bool {
	// Official enum: pending | done | expired | failed.
	return strings.EqualFold(strings.TrimSpace(gjson.GetBytes(statusBody, "status").String()), "done")
}

// ExtractGrokVideoBillingFromStatusBody builds usage units from an official done status.
// Field priority (official docs):
//   - duration: video.duration (seconds)
//   - model: top-level model
//   - resolution: not in status response → create-time pending snapshot → default 480p
func ExtractGrokVideoBillingFromStatusBody(statusBody []byte, pending *GrokVideoPendingBilling, requestID string, providers ...string) *OpenAIForwardResult {
	if !IsGrokVideoStatusBillable(statusBody) {
		return nil
	}
	model := ""
	billingModel := ""
	upstreamModel := ""
	resolution := ""
	durationSeconds := 0
	provider := ""
	if len(providers) > 0 {
		provider = providers[0]
	}
	if pending != nil && pending.VideoProvider != "" {
		provider = pending.VideoProvider
	}

	if gjson.ValidBytes(statusBody) {
		// Official: top-level model.
		model = strings.TrimSpace(gjson.GetBytes(statusBody, "model").String())
		// Official: video.duration (number of seconds).
		if v := gjson.GetBytes(statusBody, "video.duration"); v.Exists() && v.Type == gjson.Number {
			durationSeconds = int(v.Int())
			if durationSeconds == 0 && v.Float() > 0 {
				// Sub-second values are unexpected for this API; still accept truncated int path above.
				durationSeconds = int(v.Float())
			}
		}
	}
	if pending != nil {
		if model == "" {
			model = firstNonEmpty(pending.BillingModel, pending.Model, pending.OriginalModel)
		}
		if billingModel == "" {
			billingModel = firstNonEmpty(pending.BillingModel, pending.Model)
		}
		if upstreamModel == "" {
			upstreamModel = pending.UpstreamModel
		}
		// Official status has no resolution — always take create request when available.
		resolution = pending.VideoResolution
		if durationSeconds <= 0 {
			durationSeconds = pending.VideoDurationSeconds
		}
	}
	if model == "" && provider != SuanliSpecialVideoProvider {
		// Official default video model family when status omits model.
		model = "grok-imagine-video"
	}
	if billingModel == "" {
		billingModel = model
	}
	// Resolution is request-only per docs; empty → handler applies official default 480p.
	if resolution != "" {
		resolution = NormalizeVideoBillingResolutionOrDefault(resolution)
	}
	if durationSeconds > 0 {
		durationSeconds = NormalizeVideoBillingDurationForProvider(provider, durationSeconds)
	}
	responseID := extractGrokMediaVideoRequestID(statusBody)
	if responseID == "" {
		responseID = strings.TrimSpace(requestID)
	}
	return &OpenAIForwardResult{
		ResponseID:           responseID,
		VideoProvider:        provider,
		Model:                model,
		BillingModel:         billingModel,
		UpstreamModel:        upstreamModel,
		VideoCount:           1,
		VideoResolution:      resolution,
		VideoDurationSeconds: durationSeconds,
	}
}

func (s *OpenAIGatewayService) ForwardGrokMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("grok account is required")
	}
	if account.Platform != PlatformGrok && !(account.Platform == PlatformVideo && account.Type == AccountTypeAPIKey) {
		return nil, fmt.Errorf("account platform %s is not supported for grok media", account.Platform)
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	if endpoint == GrokMediaEndpointVideoContent {
		return s.forwardGrokMediaVideoContent(ctx, c, account, token, requestID, startTime)
	}
	targetURL, err := buildGrokMediaURL(account, s.cfg, endpoint, requestID)
	if err != nil {
		writeGrokMediaRequestValidationError(c, err)
		return nil, err
	}

	preparedTask, _ := ctx.Value(videoTaskContextKey{}).(*VideoTask)
	unified := preparedTask != nil && preparedTask.Pending.Unified
	if unified {
		c.Header("X-Video-Task-ID", preparedTask.ID)
		body = preparedTask.PreparedBody
		contentType = "application/json"
	}
	body, contentType, err = prepareGrokMediaForwardBodyForAccount(account, endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	requestInfo := ParseGrokMediaRequest(contentType, body)
	originalModel := requestInfo.Model
	billingModel, err := NormalizeGrokMediaRequestForEndpoint(endpoint, requestInfo)
	if err != nil {
		return nil, err
	}
	if billingModel == "" {
		billingModel = originalModel
	}
	requestInfo.Model = billingModel

	if endpoint.RequiresRequestBody() && !unified {
		mappedModel := strings.TrimSpace(account.GetMappedModel(originalModel))
		if mappedModel != "" && mappedModel != originalModel {
			body, contentType, err = rewriteGrokMediaModel(body, contentType, mappedModel)
			if err != nil {
				return nil, fmt.Errorf("rewrite grok media account mapped model: %w", err)
			}
		}
	}
	if !unified {
		body, contentType, err = normalizeGrokMediaForwardBodyForAccount(account, endpoint, body, contentType)
	}
	if err != nil {
		writeGrokMediaRequestValidationError(c, err)
		return nil, err
	}
	upstreamModel := ParseGrokMediaRequest(contentType, body).Model
	body, contentType, err = sanitizeGrokMediaForwardBodyForAccount(account, endpoint, body, contentType)
	if err != nil {
		writeGrokMediaRequestValidationError(c, err)
		return nil, err
	}
	if account.IsSuanliSpecialVideo() && endpoint.RequiresRequestBody() {
		suanliRequestBillingInfo(body, &requestInfo)
	}
	if unified {
		requestInfo.DurationSeconds = preparedTask.Pending.VideoDurationSeconds
		requestInfo.Resolution = preparedTask.Pending.VideoResolution
	}

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	if account.IsSuanliSpecialVideo() {
		upstreamCtx = WithHTTPUpstreamRedirectsDisabled(upstreamCtx)
	}
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(targetURL) {
		applyGrokCLIHeaders(upstreamReq.Header)
	}
	if endpoint.RequiresRequestBody() {
		contentType = strings.TrimSpace(contentType)
		if contentType == "" {
			contentType = "application/json"
		}
		upstreamReq.Header.Set("Content-Type", contentType)
	}
	// 账号级请求头覆写最后应用，配置值优先于内置默认头。
	account.ApplyHeaderOverrides(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	s.rateLimitService.observeQualityResponse(upstreamReq.Context(), account, resp, err)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		if account.IsSuanliSpecialVideo() && endpoint.IsGenerationRequest() {
			// A timeout may occur after acceptance. Never transparently resubmit
			// a potentially paid job to another account.
			writeGrokMediaErrorResponse(c, http.StatusBadGateway, "submission_unknown", "Video submission state is unknown; check supplier tasks before retrying")
			return nil, fmt.Errorf("suanli video submission transport failed: %w", err)
		}
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	requestModel := requestInfo.Model
	if account.IsSuanliSpecialVideo() && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		writeGrokMediaErrorResponse(c, http.StatusBadGateway, "video_redirect_rejected", "Video supplier redirects are not allowed")
		return nil, fmt.Errorf("suanli video redirect is not allowed")
	}
	if resp.StatusCode >= 400 {
		if account.IsSuanliSpecialVideo() && endpoint.IsGenerationRequest() {
			if unified && (resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 || resp.StatusCode == 405 || resp.StatusCode == 413 || resp.StatusCode == 422 || resp.StatusCode == 429) {
				writeGrokMediaErrorResponse(c, resp.StatusCode, "video_submission_rejected", "上游未接受任务，预扣费用将退回")
				return nil, &VideoSubmissionRejected{}
			}
			writeGrokMediaErrorResponse(c, resp.StatusCode, "video_submission_error", "Video supplier rejected or could not confirm this submission; inspect supplier tasks before retrying")
			return nil, fmt.Errorf("suanli video submission returned HTTP %d", resp.StatusCode)
		}
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, requestIDHeader, requestModel)
	}

	s.updateGrokUsageFromResponse(withGrokTeamRateLimitModel(ctx, requestModel), account, resp.Header, resp.StatusCode)
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if account.IsSuanliSpecialVideo() {
		respBody, err = normalizeSuanliVideoResponse(endpoint, respBody, requestID)
		if err != nil {
			writeGrokMediaErrorResponse(c, http.StatusBadGateway, "invalid_video_response", err.Error())
			return nil, err
		}
	}
	if endpoint == GrokMediaEndpointImagesGenerations || endpoint == GrokMediaEndpointImagesEdits {
		if countOpenAIResponseImageOutputsFromJSONBytes(respBody) <= 0 {
			setOpsUpstreamError(c, http.StatusBadGateway, "xAI upstream returned no image output", truncateString(string(respBody), 512))
			return nil, &UpstreamFailoverError{
				StatusCode:      http.StatusBadGateway,
				ResponseBody:    respBody,
				ResponseHeaders: resp.Header.Clone(),
			}
		}
	}
	if endpoint == GrokMediaEndpointVideoStatus {
		sourceURL := grokMediaVideoSourceURL(respBody, requestID)
		if sourceURL != "" && !s.settingService.IsGrokVideoSourceURLEnabled(ctx) {
			sourceURL = ""
		}
		respBody = rewriteGrokMediaVideoContentURLs(
			respBody,
			requestID,
			grokMediaContentProxyURL(c, requestID),
		)
		respBody = setGrokMediaVideoSourceURL(respBody, sourceURL)
	}
	usage := grokMediaUsageFromResponse(endpoint, requestInfo, respBody, account.VideoMediaProvider())
	resultModel := requestModel
	resultBillingModel := requestModel
	if endpoint == GrokMediaEndpointVideoStatus {
		// Status has no request body model; use upstream status fields when billable.
		if m := strings.TrimSpace(usage.Model); m != "" {
			resultModel = m
		}
		if m := strings.TrimSpace(usage.BillingModel); m != "" {
			resultBillingModel = m
		}
	}
	result := &OpenAIForwardResult{
		RequestID:            requestIDHeader,
		UpstreamHeaders:      resp.Header,
		ResponseID:           usage.ResponseID,
		Usage:                usage.Usage,
		Model:                resultModel,
		BillingModel:         resultBillingModel,
		UpstreamModel:        upstreamModel,
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		ImageCount:           usage.ImageCount,
		ImageSize:            usage.ImageSize,
		ImageInputSize:       usage.ImageInputSize,
		ImageOutputSizes:     usage.ImageOutputSizes,
		VideoCount:           usage.VideoCount,
		VideoProvider:        account.VideoMediaProvider(),
		VideoResolution:      usage.VideoResolution,
		VideoDurationSeconds: usage.VideoDurationSeconds,
	}
	if unified {
		result.VideoCount = 0
		result.VideoDurationSeconds = preparedTask.Pending.VideoDurationSeconds
		result.VideoResolution = preparedTask.Pending.VideoResolution
	}
	if endpoint.IsGenerationRequest() && account.Platform == PlatformVideo {
		if err := s.acceptVideoTask(ctx, result); err != nil {
			writeGrokMediaErrorResponse(c, http.StatusBadGateway, "video_task_persistence_failed", "Video submission could not be recorded; check supplier tasks before retrying")
			return nil, err
		}
	}
	if unified {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusAccepted, gin.H{"id": result.ResponseID, "request_id": result.ResponseID, "local_task_id": preparedTask.ID, "status": "queued", "model": preparedTask.Pending.Model, "prepaid_amount": preparedTask.Pending.PrepaidAmount})
		return result, nil
	}
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	return result, nil
}

func (s *OpenAIGatewayService) forwardGrokMediaVideoContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token, requestID string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	statusURL, err := buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoStatus, requestID)
	if err != nil {
		return nil, err
	}

	parent := ctx
	if c != nil && c.Request != nil {
		parent = c.Request.Context()
	}
	upstreamCtx, downloadProgress, releaseUpstreamCtx := mediaDownloadContext(parent, 15*time.Minute, 45*time.Second)
	defer releaseUpstreamCtx()
	statusReq, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(upstreamCtx),
		http.MethodGet,
		statusURL,
		nil,
	)
	if err != nil {
		return nil, err
	}
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusReq.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(statusURL) {
		applyGrokCLIHeaders(statusReq.Header)
	}
	account.ApplyHeaderOverrides(statusReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	statusResp, err := s.httpUpstream.Do(statusReq, proxyURL, account.ID, account.Concurrency)
	s.rateLimitService.observeQualityResponse(statusReq.Context(), account, statusResp, err)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	statusRequestID := firstNonEmpty(statusResp.Header.Get("x-request-id"), statusResp.Header.Get("xai-request-id"))
	statusResp.Body = &mediaDownloadBody{ReadCloser: statusResp.Body, progress: downloadProgress}
	if statusResp.StatusCode >= 300 {
		defer func() { _ = statusResp.Body.Close() }()
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if statusResp.StatusCode < 400 {
			return nil, fmt.Errorf("grok media status redirect is not allowed")
		}
		return s.handleGrokMediaErrorResponse(ctx, statusResp, c, account, statusRequestID, "")
	}
	statusBody, err := ReadUpstreamResponseBody(statusResp.Body, s.cfg, c, openAITooLargeError)
	_ = statusResp.Body.Close()
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}

	contentURL := ""
	if account.IsSuanliSpecialVideo() {
		statusBody, err = normalizeSuanliVideoResponse(GrokMediaEndpointVideoStatus, statusBody, requestID)
	} else {
		contentURL, err = grokMediaSignedVideoContentURL(statusBody, requestID)
	}
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}
	signedContent := contentURL != ""
	if !signedContent {
		contentURL, err = buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoContent, requestID)
		if err != nil {
			SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
			return nil, err
		}
	}

	contentReq, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(upstreamCtx),
		http.MethodGet,
		contentURL,
		nil,
	)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}
	contentReq.Header.Set("Accept", "*/*")
	if c != nil {
		if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
			contentReq.Header.Set("Range", rangeHeader)
			if ifRange := strings.TrimSpace(c.GetHeader("If-Range")); ifRange != "" {
				contentReq.Header.Set("If-Range", ifRange)
			}
		}
	}
	if !signedContent {
		contentReq.Header.Set("Authorization", "Bearer "+token)
		if account.IsGrokOAuth() && isGrokCLIProxyTarget(contentURL) {
			applyGrokCLIHeaders(contentReq.Header)
		}
		account.ApplyHeaderOverrides(contentReq.Header)
	}

	contentResp, err := s.httpUpstream.Do(contentReq, proxyURL, account.ID, account.Concurrency)
	s.rateLimitService.observeQualityResponse(contentReq.Context(), account, contentResp, err)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = contentResp.Body.Close() }()
	contentResp.Body = &mediaDownloadBody{ReadCloser: contentResp.Body, progress: downloadProgress}
	contentRequestID := firstNonEmpty(contentResp.Header.Get("x-request-id"), contentResp.Header.Get("xai-request-id"), statusRequestID)
	if contentResp.StatusCode >= 300 && contentResp.StatusCode < 400 {
		return nil, fmt.Errorf("grok media signed content redirect is not allowed")
	}
	if contentResp.StatusCode >= 400 && contentResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return s.handleGrokMediaErrorResponse(ctx, contentResp, c, account, contentRequestID, "")
	}

	s.updateGrokUsageFromResponse(withGrokTeamRateLimitModel(ctx, ""), account, contentResp.Header, contentResp.StatusCode)
	if err := writeGrokMediaContentResponse(c, contentResp); err != nil {
		return nil, err
	}
	// Content download is an alternate completion observation: when status body is
	// official done+video.url, attach billable units so the handler can claim once
	// (same path as status polling). Pending snapshot is merged in the handler.
	result := &OpenAIForwardResult{
		RequestID:       contentRequestID,
		VideoProvider:   account.VideoMediaProvider(),
		UpstreamHeaders: contentResp.Header,
		ResponseHeaders: contentResp.Header.Clone(),
		Duration:        time.Since(startTime),
	}
	if billed := ExtractGrokVideoBillingFromStatusBody(statusBody, nil, requestID, account.VideoMediaProvider()); billed != nil {
		result.ResponseID = firstNonEmpty(billed.ResponseID, strings.TrimSpace(requestID))
		result.Model = billed.Model
		result.BillingModel = billed.BillingModel
		result.UpstreamModel = billed.UpstreamModel
		result.VideoCount = billed.VideoCount
		result.VideoResolution = billed.VideoResolution
		result.VideoDurationSeconds = billed.VideoDurationSeconds
	}
	return result, nil
}

func grokMediaSignedVideoContentURL(body []byte, requestID string) (string, error) {
	rawURL := strings.TrimSpace(gjson.GetBytes(body, "video.url").String())
	if rawURL == "" {
		return "", nil
	}
	// An upstream Sub2API rewrites protected content URLs to its own proxy
	// endpoint. Treat that as an authenticated relay path, not as a signed URL;
	// the caller will rebuild it against the configured account base URL and
	// attach the upstream API key.
	if isGrokMediaVideoContentURL(rawURL, requestID) {
		return "", nil
	}
	return validateGrokSignedVideoURL(rawURL)
}

func validateGrokSignedVideoURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
		!strings.EqualFold(parsed.Hostname(), "vidgen.x.ai") ||
		(parsed.Port() != "" && parsed.Port() != "443") || parsed.User != nil {
		return "", fmt.Errorf("grok media status returned an unsupported video content URL")
	}
	return parsed.String(), nil
}

// grokMediaVideoSourceURL picks the anonymous xAI media URL from a status body:
// video.url from xAI itself, or video.source_url from an upstream Sub2API relay.
func grokMediaVideoSourceURL(body []byte, requestID string) string {
	if sourceURL, err := grokMediaSignedVideoContentURL(body, requestID); err == nil && sourceURL != "" {
		return sourceURL
	}
	relayed := strings.TrimSpace(gjson.GetBytes(body, "video.source_url").String())
	if relayed == "" {
		return ""
	}
	sourceURL, err := validateGrokSignedVideoURL(relayed)
	if err != nil {
		return ""
	}
	return sourceURL
}

// setGrokMediaVideoSourceURL writes video.source_url, or removes any upstream
// value when sourceURL is empty so a disabled switch never leaks the media URL.
func setGrokMediaVideoSourceURL(body []byte, sourceURL string) []byte {
	if len(body) == 0 || !gjson.GetBytes(body, "video").IsObject() {
		return body
	}
	var (
		out []byte
		err error
	)
	if sourceURL == "" {
		if !gjson.GetBytes(body, "video.source_url").Exists() {
			return body
		}
		out, err = sjson.DeleteBytes(body, "video.source_url")
	} else {
		out, err = sjson.SetBytes(body, "video.source_url", sourceURL)
	}
	if err != nil {
		return body
	}
	return out
}

func isGrokCLIProxyTarget(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && strings.EqualFold(parsed.Hostname(), "cli-chat-proxy.grok.com")
}

func prepareGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if endpoint != GrokMediaEndpointImagesEdits {
		return body, contentType, nil
	}
	if gjson.ValidBytes(body) {
		out, err := normalizeGrokMediaJSONImageRefs(body)
		return out, contentType, err
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, nil
	}

	info := ParseGrokMediaRequest(contentType, body)
	payload := make(map[string]any)
	if info.Model != "" {
		payload["model"] = info.Model
	}
	if info.Prompt != "" {
		payload["prompt"] = info.Prompt
	}
	if info.N > 1 {
		payload["n"] = info.N
	}
	if info.Size != "" {
		payload["size"] = info.Size
	}
	if info.ImageResolution != "" {
		payload["resolution"] = info.ImageResolution
	}
	if info.AspectRatio != "" {
		payload["aspect_ratio"] = info.AspectRatio
	}

	images := make([]map[string]string, 0, len(info.InputImageURLs)+len(info.Uploads))
	for _, imageURL := range info.InputImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, grokMediaImageObject(imageURL))
		}
	}
	for _, upload := range info.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, "", err
		}
		images = append(images, grokMediaImageObject(dataURL))
	}
	if len(images) > grokMediaMaxEditSourceImages {
		return nil, "", fmt.Errorf("a maximum of %d source images is supported for image edits", grokMediaMaxEditSourceImages)
	}
	if len(images) > 0 {
		payload["image"] = images[0]
		if len(images) > 1 {
			payload["images"] = images
		}
	}

	maskImageURL := strings.TrimSpace(info.MaskImageURL)
	if info.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*info.MaskUpload)
		if err != nil {
			return nil, "", err
		}
		maskImageURL = dataURL
	}
	if maskImageURL != "" {
		payload["mask"] = grokMediaImageObject(maskImageURL)
	}

	out, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, "", err
	}
	return out, "application/json", nil
}

// prepareGrokMediaForwardBodyForAccount keeps the wire contract of a
// third-party OpenAI-compatible Grok endpoint. Those endpoints commonly
// require /v1/images/edits as multipart/form-data with image[0], image[1], ...
// file parts. The official xAI contract is JSON, so only that path uses the
// legacy multipart-to-data-URL conversion above.
func prepareGrokMediaForwardBodyForAccount(
	account *Account,
	endpoint GrokMediaEndpoint,
	body []byte,
	contentType string,
) ([]byte, string, error) {
	if account != nil && (account.IsGrokOpenAICompatibleMedia() || account.IsSuanliSpecialVideo()) {
		return body, contentType, nil
	}
	return prepareGrokMediaForwardBody(endpoint, body, contentType)
}

func rewriteGrokMediaModel(body []byte, contentType, model string) ([]byte, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return rewriteOpenAIImagesMultipartModel(body, contentType, model)
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite media request model: %w", err)
	}
	return rewritten, contentType, nil
}

func normalizeGrokMediaJSONImageRefs(body []byte) ([]byte, error) {
	info := ParseGrokMediaRequest("application/json", body)
	if len(info.InputImageURLs) > grokMediaMaxEditSourceImages {
		return nil, fmt.Errorf("a maximum of %d source images is supported for image edits", grokMediaMaxEditSourceImages)
	}
	out := body
	var err error
	for _, field := range []string{"image", "images", "mask"} {
		out, err = rewriteGrokMediaJSONImageField(out, field)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rewriteGrokMediaJSONImageField(body []byte, path string) ([]byte, error) {
	value := gjson.GetBytes(body, path)
	if !value.Exists() {
		return body, nil
	}
	if value.IsArray() {
		rewritten := make([]map[string]string, 0, len(value.Array()))
		for _, item := range value.Array() {
			imageURL := extractGrokMediaImageURL(item)
			if imageURL == "" {
				return body, nil
			}
			rewritten = append(rewritten, grokMediaImageObject(imageURL))
		}
		out, err := sjson.SetBytes(body, path, rewritten)
		if err != nil {
			return nil, fmt.Errorf("rewrite grok media %s: %w", path, err)
		}
		return out, nil
	}
	imageURL := extractGrokMediaImageURL(value)
	if imageURL == "" {
		return body, nil
	}
	out, err := sjson.SetBytes(body, path, grokMediaImageObject(imageURL))
	if err != nil {
		return nil, fmt.Errorf("rewrite grok media %s: %w", path, err)
	}
	return out, nil
}

func normalizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	var imageFields []string
	switch endpoint {
	case GrokMediaEndpointImagesEdits:
		imageFields = []string{"image", "images", "mask"}
	case GrokMediaEndpointVideosGenerations:
		imageFields = []string{"image", "images", "reference_images"}
	}
	var err error
	body, err = canonicalizeGrokMediaImageURLFields(body, imageFields...)
	if err != nil {
		return nil, "", err
	}
	info := ParseGrokMediaRequest(contentType, body)
	upstreamModel, err := NormalizeGrokMediaRequestForEndpoint(endpoint, info)
	if err != nil {
		return nil, "", err
	}
	if upstreamModel == "" || upstreamModel == info.Model {
		return body, contentType, nil
	}
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite grok media model: %w", err)
	}
	return out, contentType, nil
}

func normalizeGrokMediaForwardBodyForAccount(
	account *Account,
	endpoint GrokMediaEndpoint,
	body []byte,
	contentType string,
) ([]byte, string, error) {
	if account.IsSuanliSpecialVideo() {
		return normalizeSuanliVideoRequest(endpoint, body, contentType)
	}
	out, normalizedContentType, err := normalizeGrokMediaForwardBody(endpoint, body, contentType)
	if err != nil || account == nil || !account.IsGrokOpenAICompatibleMedia() {
		return out, normalizedContentType, err
	}
	return normalizeOpenAICompatibleGrokMediaBody(endpoint, out, normalizedContentType)
}

func canonicalizeGrokMediaImageURLFields(body []byte, fields ...string) ([]byte, error) {
	out := body
	for _, field := range fields {
		value := gjson.GetBytes(out, field)
		if !value.Exists() {
			continue
		}
		if value.IsArray() {
			for index := range value.Array() {
				var err error
				out, err = canonicalizeGrokMediaImageURLObject(out, fmt.Sprintf("%s.%d", field, index))
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		var err error
		out, err = canonicalizeGrokMediaImageURLObject(out, field)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func canonicalizeGrokMediaImageURLObject(body []byte, path string) ([]byte, error) {
	legacyPath := path + ".image_url"
	legacy := gjson.GetBytes(body, legacyPath)
	if !legacy.Exists() {
		return body, nil
	}

	out := body
	if strings.TrimSpace(gjson.GetBytes(out, path+".url").String()) == "" {
		var err error
		out, err = sjson.SetBytes(out, path+".url", legacy.Value())
		if err != nil {
			return nil, fmt.Errorf("normalize grok media image url: %w", err)
		}
	}
	out, err := sjson.DeleteBytes(out, legacyPath)
	if err != nil {
		return nil, fmt.Errorf("remove legacy grok media image url: %w", err)
	}
	return out, nil
}

func sanitizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		out, err := applyGrokImagineImageGeometry(body)
		if err != nil {
			return nil, "", fmt.Errorf("sanitize grok media size: %w", err)
		}
		return out, contentType, nil
	default:
		return body, contentType, nil
	}
}

func sanitizeGrokMediaForwardBodyForAccount(
	account *Account,
	endpoint GrokMediaEndpoint,
	body []byte,
	contentType string,
) ([]byte, string, error) {
	if account != nil && (account.IsGrokOpenAICompatibleMedia() || account.IsSuanliSpecialVideo()) {
		return body, contentType, nil
	}
	return sanitizeGrokMediaForwardBody(endpoint, body, contentType)
}

type grokMediaRequestValidationError struct {
	message string
}

func (e *grokMediaRequestValidationError) Error() string {
	if e == nil {
		return "invalid Grok media request"
	}
	return e.message
}

func newGrokMediaRequestValidationError(format string, args ...any) error {
	return &grokMediaRequestValidationError{message: fmt.Sprintf(format, args...)}
}

func writeGrokMediaRequestValidationError(c *gin.Context, err error) {
	validationErr, ok := err.(*grokMediaRequestValidationError)
	if !ok || validationErr == nil || c == nil || IsResponseCommitted(c) {
		return
	}
	MarkResponseCommitted(c)
	writeGrokMediaErrorResponse(c, http.StatusBadRequest, "invalid_request_error", validationErr.Error())
}

var openAICompatibleGrokVideoRatios = map[string]struct{}{
	"16:9": {},
	"1:1":  {},
	"9:16": {},
	"3:2":  {},
	"2:3":  {},
}

func normalizeOpenAICompatibleGrokMediaBody(
	endpoint GrokMediaEndpoint,
	body []byte,
	contentType string,
) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() {
		return body, contentType, nil
	}
	mediaType, _, mediaTypeErr := mime.ParseMediaType(strings.TrimSpace(contentType))
	if mediaTypeErr == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return normalizeOpenAICompatibleGrokImageMultipartBody(endpoint, body, contentType)
	}
	if !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	payload, err := decodeGrokMediaJSONObject(body)
	if err != nil {
		return nil, "", err
	}

	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		payload, err = normalizeOpenAICompatibleGrokImagePayload(endpoint, payload)
	case GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		payload, err = normalizeOpenAICompatibleGrokVideoPayload(endpoint, payload)
	default:
		return body, contentType, nil
	}
	if err != nil {
		return nil, "", err
	}
	out, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, "", fmt.Errorf("serialize OpenAI-compatible Grok media request: %w", err)
	}
	return out, "application/json", nil
}

// normalizeOpenAICompatibleGrokImageMultipartBody preserves uploaded image
// bytes while adapting OpenAI client field names to the upstream contract.
// In particular, the upstream expects image[0], image[1], ... rather than a
// single image field or an images[] JSON value.
func normalizeOpenAICompatibleGrokImageMultipartBody(
	endpoint GrokMediaEndpoint,
	body []byte,
	contentType string,
) ([]byte, string, error) {
	if endpoint != GrokMediaEndpointImagesEdits {
		return body, contentType, nil
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, nil
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}

	info := ParseGrokMediaRequest(contentType, body)
	model, err := NormalizeGrokMediaRequestForEndpoint(endpoint, info)
	if err != nil {
		return nil, "", err
	}
	size, resolution, err := openAICompatibleGrokImageDimensions(info)
	if err != nil {
		return nil, "", err
	}
	ratio, err := normalizeOpenAICompatibleGrokImageAspectRatio(info.AspectRatio)
	if err != nil {
		return nil, "", err
	}

	var output bytes.Buffer
	writer := multipart.NewWriter(&output)
	imageIndex := 0
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, readErr := reader.NextPart()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, "", fmt.Errorf("read multipart body: %w", readErr)
		}
		name := strings.TrimSpace(part.FormName())
		fileName := strings.TrimSpace(part.FileName())
		data, dataErr := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize))
		_ = part.Close()
		if dataErr != nil {
			return nil, "", fmt.Errorf("read multipart field %s: %w", name, dataErr)
		}

		isImage := fileName != "" && (name == "image" || name == "images" ||
			strings.HasPrefix(name, "image[") || strings.HasPrefix(name, "images["))
		if isImage {
			header := cloneMultipartHeader(part.Header)
			disposition, dispositionParams, dispositionErr := mime.ParseMediaType(header.Get("Content-Disposition"))
			if dispositionErr != nil || disposition == "" {
				disposition = "form-data"
				dispositionParams = map[string]string{}
			}
			dispositionParams["name"] = fmt.Sprintf("image[%d]", imageIndex)
			header.Set("Content-Disposition", mime.FormatMediaType(disposition, dispositionParams))
			target, createErr := writer.CreatePart(header)
			if createErr != nil {
				return nil, "", fmt.Errorf("create normalized image part: %w", createErr)
			}
			if _, writeErr := target.Write(data); writeErr != nil {
				return nil, "", fmt.Errorf("write normalized image part: %w", writeErr)
			}
			imageIndex++
			continue
		}

		// Keep a mask upload if a client supplied one. All other file fields are
		// outside the edit contract and are deliberately omitted.
		if fileName != "" {
			if name != "mask" {
				continue
			}
			header := cloneMultipartHeader(part.Header)
			target, createErr := writer.CreatePart(header)
			if createErr != nil {
				return nil, "", fmt.Errorf("create normalized mask part: %w", createErr)
			}
			if _, writeErr := target.Write(data); writeErr != nil {
				return nil, "", fmt.Errorf("write normalized mask part: %w", writeErr)
			}
			continue
		}

		// Preserve URL-style image fields as indexed form fields for clients
		// that send a URL instead of a file. The upstream can then apply its own
		// URL validation without losing the user's input.
		if name == "image" || name == "images" || strings.HasPrefix(name, "image[") || strings.HasPrefix(name, "images[") {
			if err := writer.WriteField(fmt.Sprintf("image[%d]", imageIndex), strings.TrimSpace(string(data))); err != nil {
				return nil, "", fmt.Errorf("write normalized image URL field: %w", err)
			}
			imageIndex++
		}
	}

	if model != "" {
		if err := writer.WriteField("model", model); err != nil {
			return nil, "", fmt.Errorf("write normalized model field: %w", err)
		}
	}
	if info.Prompt != "" {
		if err := writer.WriteField("prompt", info.Prompt); err != nil {
			return nil, "", fmt.Errorf("write normalized prompt field: %w", err)
		}
	}
	if err := writer.WriteField("n", strconv.Itoa(info.N)); err != nil {
		return nil, "", fmt.Errorf("write normalized n field: %w", err)
	}
	if err := writer.WriteField("size", size); err != nil {
		return nil, "", fmt.Errorf("write normalized size field: %w", err)
	}
	if err := writer.WriteField("resolution", resolution); err != nil {
		return nil, "", fmt.Errorf("write normalized resolution field: %w", err)
	}
	if ratio != "" && imageIndex > 1 {
		if err := writer.WriteField("aspect_ratio", ratio); err != nil {
			return nil, "", fmt.Errorf("write normalized aspect ratio field: %w", err)
		}
	}
	if err := writer.WriteField("response_format", "b64_json"); err != nil {
		return nil, "", fmt.Errorf("write normalized response format field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize normalized multipart body: %w", err)
	}
	return output.Bytes(), writer.FormDataContentType(), nil
}

func decodeGrokMediaJSONObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse OpenAI-compatible Grok media request: %w", err)
	}
	if payload == nil {
		return nil, newGrokMediaRequestValidationError("Grok media request body must be a JSON object")
	}
	return payload, nil
}

func normalizeOpenAICompatibleGrokImagePayload(
	endpoint GrokMediaEndpoint,
	payload map[string]any,
) (map[string]any, error) {
	infoBody, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, err
	}
	info := ParseGrokMediaRequest("application/json", infoBody)
	size, resolution, err := openAICompatibleGrokImageDimensions(info)
	if err != nil {
		return nil, err
	}
	ratio, err := normalizeOpenAICompatibleGrokImageAspectRatio(info.AspectRatio)
	if err != nil {
		return nil, err
	}

	allowed := []string{"model", "prompt"}
	if endpoint == GrokMediaEndpointImagesEdits {
		allowed = append(allowed, "image", "images", "mask")
	}
	out := copyGrokMediaPayloadFields(payload, allowed...)
	out["n"] = info.N
	out["size"] = size
	out["resolution"] = resolution
	if ratio != "" && (endpoint == GrokMediaEndpointImagesGenerations || len(info.InputImageURLs) > 1) {
		out["aspect_ratio"] = ratio
	}
	out["response_format"] = "b64_json"
	return out, nil
}

func openAICompatibleGrokImageDimensions(info GrokMediaRequestInfo) (string, string, error) {
	for _, candidate := range []string{info.RequestedResolution, info.Size} {
		switch strings.ToLower(strings.TrimSpace(candidate)) {
		case "1k", "1024x1024":
			return "1024x1024", "1k", nil
		case "2k", "2048x2048":
			return "2048x2048", "2k", nil
		case "4k", "4096x4096", "3840x2160", "2160x3840":
			return "", "", newGrokMediaRequestValidationError(
				"OpenAI-compatible Grok image upstream supports only 1K or 2K, got %q",
				candidate,
			)
		}
	}
	if strings.TrimSpace(info.Size) != "" || strings.TrimSpace(info.RequestedResolution) != "" {
		return "", "", newGrokMediaRequestValidationError(
			"OpenAI-compatible Grok image upstream requires size 1024x1024 or 2048x2048",
		)
	}
	return "1024x1024", "1k", nil
}

func normalizeOpenAICompatibleGrokImageAspectRatio(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	ratio, ok := normalizeGrokImageAspectRatio(value)
	if !ok {
		return "", newGrokMediaRequestValidationError(
			"unsupported Grok image aspect ratio %q; use %s",
			value,
			strings.Join(grokImageAspectRatios, ", "),
		)
	}
	return ratio, nil
}

func normalizeOpenAICompatibleGrokVideoPayload(
	endpoint GrokMediaEndpoint,
	payload map[string]any,
) (map[string]any, error) {
	ratio := strings.TrimSpace(grokMediaStringValue(payload["aspect_ratio"]))
	if ratio == "" {
		ratio = strings.TrimSpace(grokMediaStringValue(payload["ratio"]))
	}
	if ratio == "" {
		ratio = "16:9"
	}
	if _, ok := openAICompatibleGrokVideoRatios[ratio]; !ok {
		return nil, newGrokMediaRequestValidationError(
			"unsupported Grok video aspect ratio %q; use 16:9, 1:1, 9:16, 3:2, or 2:3",
			ratio,
		)
	}

	if rawDuration, exists := payload["duration"]; exists {
		duration, ok := jsonNumberAsInt(rawDuration)
		if !ok || duration < VideoBillingMinDurationSeconds || duration > VideoBillingMaxDurationSeconds {
			return nil, newGrokMediaRequestValidationError(
				"Grok video duration must be an integer from %d to %d seconds",
				VideoBillingMinDurationSeconds,
				VideoBillingMaxDurationSeconds,
			)
		}
	}

	if resolution := strings.ToLower(strings.TrimSpace(grokMediaStringValue(payload["resolution"]))); resolution != "" {
		switch resolution {
		case VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P:
		default:
			return nil, newGrokMediaRequestValidationError(
				"unsupported Grok video resolution %q; use 480p, 720p, or 1080p",
				resolution,
			)
		}
	}

	allowed := []string{"model", "prompt", "duration", "resolution", "image", "images", "reference_images"}
	if endpoint == GrokMediaEndpointVideosEdits || endpoint == GrokMediaEndpointVideosExtensions {
		allowed = append(allowed, "video")
	}
	out := copyGrokMediaPayloadFields(payload, allowed...)
	out["aspect_ratio"] = ratio
	return out, nil
}

func copyGrokMediaPayloadFields(payload map[string]any, fields ...string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := payload[field]; ok {
			out[field] = value
		}
	}
	return out
}

func grokMediaStringValue(value any) string {
	result, _ := value.(string)
	return result
}

func jsonNumberAsInt(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	case float64:
		parsed := int(typed)
		return parsed, float64(parsed) == typed
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func (r GrokMediaRequestInfo) HasInputImage() bool {
	return r.HasSourceImage() || len(r.ReferenceImageURLs) > 0
}

func (r GrokMediaRequestInfo) HasSourceImage() bool {
	return len(r.InputImageURLs) > 0 || len(r.Uploads) > 0
}

// NormalizeGrokMediaModelForEndpoint resolves upstream's built-in media alias.
// The hasInputImage argument remains part of the upstream contract so model
// selection can evolve without changing account-test callers.
func NormalizeGrokMediaModelForEndpoint(endpoint GrokMediaEndpoint, model string, hasInputImage bool) string {
	model = strings.TrimSpace(model)
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		if strings.EqualFold(model, "grok-imagine") {
			return xai.ModelGrokImagineImageQuality
		}
	case GrokMediaEndpointVideosGenerations:
		_ = hasInputImage
	}
	return model
}

// NormalizeGrokMediaRequestForEndpoint resolves documented aliases to a
// canonical xAI media model and validates endpoint-specific capabilities before
// account-level model mapping and scheduling.
func NormalizeGrokMediaRequestForEndpoint(endpoint GrokMediaEndpoint, info GrokMediaRequestInfo) (string, error) {
	rawModel := NormalizeGrokMediaModelForEndpoint(endpoint, info.Model, info.HasInputImage())
	model := strings.ToLower(strings.TrimSpace(rawModel))
	if model == "" {
		return "", nil
	}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		if model == "grok-imagine-edit" {
			return xai.ModelGrokImagineImageQuality, nil
		}
		if canonical, ok := xai.CanonicalImageGenerationModel(model); ok {
			return canonical, nil
		}
		return rawModel, nil
	case GrokMediaEndpointVideosGenerations:
		canonical, ok := xai.CanonicalVideoGenerationModel(model)
		if !ok {
			return rawModel, nil
		}
		if canonical == xai.ModelGrokImagineVideo15 && len(info.ReferenceImageURLs) > 0 {
			return "", fmt.Errorf("reference-to-video requires %s", xai.ModelGrokImagineVideo)
		}
		if err := validateGrokVideoGenerationResolution(canonical, info); err != nil {
			return "", err
		}
		return canonical, nil
	case GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		canonical, ok := xai.CanonicalVideoGenerationModel(model)
		if !ok {
			return rawModel, nil
		}
		if canonical != xai.ModelGrokImagineVideo {
			return "", fmt.Errorf("video edit and extension endpoints require %s", xai.ModelGrokImagineVideo)
		}
		return canonical, nil
	default:
		return rawModel, nil
	}
}

func validateGrokVideoGenerationResolution(model string, info GrokMediaRequestInfo) error {
	resolution := strings.ToLower(strings.TrimSpace(info.RequestedResolution))
	if resolution == "" {
		return nil
	}
	switch resolution {
	case VideoBillingResolution480P, VideoBillingResolution720P:
		return nil
	case VideoBillingResolution1080P:
		if model != xai.ModelGrokImagineVideo15 {
			return fmt.Errorf("1080p video generation requires %s", xai.ModelGrokImagineVideo15)
		}
		return nil
	default:
		return fmt.Errorf("unsupported video resolution %q", info.RequestedResolution)
	}
}

type grokMediaUsageMetadata struct {
	ResponseID           string
	Usage                OpenAIUsage
	Model                string
	BillingModel         string
	ImageCount           int
	ImageSize            string
	ImageInputSize       string
	ImageOutputSizes     []string
	VideoCount           int
	VideoResolution      string
	VideoDurationSeconds int
}

func grokMediaUsageFromResponse(endpoint GrokMediaEndpoint, requestInfo GrokMediaRequestInfo, responseBody []byte, providers ...string) grokMediaUsageMetadata {
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	meta := grokMediaUsageMetadata{Usage: usage}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		meta.ImageCount = countOpenAIResponseImageOutputsFromJSONBytes(responseBody)
		meta.ImageSize = requestInfo.SizeTier
		meta.ImageInputSize = requestInfo.Size
		meta.ImageOutputSizes = collectOpenAIResponseImageOutputSizesFromJSONBytes(responseBody)
	case GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		// Async video: capture request_id + create-time pricing params only.
		// Billable VideoCount is set later when status polling observes video.url.
		meta.ResponseID = extractGrokMediaVideoRequestID(responseBody)
		meta.VideoResolution = requestInfo.Resolution
		meta.VideoDurationSeconds = requestInfo.DurationSeconds
	case GrokMediaEndpointVideoStatus:
		// Prefer status-body URL success + upstream duration/resolution when present.
		if IsGrokVideoStatusBillable(responseBody) {
			// provisional units; handler merges with pending snapshot before RecordUsage.
			if billed := ExtractGrokVideoBillingFromStatusBody(responseBody, nil, "", providers...); billed != nil {
				meta.ResponseID = billed.ResponseID
				meta.Model = billed.Model
				meta.BillingModel = billed.BillingModel
				meta.VideoCount = billed.VideoCount
				meta.VideoResolution = billed.VideoResolution
				meta.VideoDurationSeconds = billed.VideoDurationSeconds
			}
		}
	}
	return meta
}

func extractGrokMediaVideoRequestID(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{"request_id", "id", "data.request_id", "data.id", "video.request_id", "video.id", "task_id", "data.task_id", "video.task_id"} {
		if id := strings.TrimSpace(gjson.GetBytes(body, path).String()); id != "" {
			return id
		}
	}
	return ""
}

func (s *OpenAIGatewayService) handleGrokMediaErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	requestedModel string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	// Reconcile readiness before configurable passthrough branches can return;
	// otherwise a Grok 429 can remain schedulable.
	s.handleGrokAccountUpstreamError(withGrokTeamRateLimitModel(ctx, requestedModel), account, resp.StatusCode, resp.Header, body)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	if isGrokContentPolicyRejection(resp.StatusCode, body) {
		clientMsg := grokContentPolicyClientMessage(body)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            clientMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, http.StatusForbidden, "invalid_request_error", clientMsg)
		return nil, &grokContentPolicyError{message: clientMsg}
	}

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, status, errType, errMsg)
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	if account.IsGrokOpenAICompatibleMedia() && shouldPassthroughOpenAICompatibleGrokMediaError(resp.StatusCode, body) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeGrokMediaResponse(c, resp, body, s.responseHeaderFilter)
		return nil, fmt.Errorf("OpenAI-compatible Grok media upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, http.StatusInternalServerError, "upstream_error", "Upstream gateway error")
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	kind := "http_error"
	if s.shouldFailoverGrokUpstreamError(resp.StatusCode, body) {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID:            opsUpstreamProxyID(account),
		ProxyName:          opsUpstreamProxyName(account),
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if kind == "failover" {
		retryable, retryDelay, retryDeadline, retryMax := grokSameAccountRetryMetadata(account, resp.StatusCode, body)
		return nil, (&UpstreamFailoverError{
			StatusCode:               resp.StatusCode,
			ResponseBody:             body,
			ResponseHeaders:          resp.Header.Clone(),
			RetryableOnSameAccount:   retryable,
			RequestScopedTransient:   retryable && resp.StatusCode == http.StatusTooManyRequests,
			SameAccountRetryDelay:    retryDelay,
			SameAccountRetryDeadline: retryDeadline,
			SameAccountRetryMax:      retryMax,
		}).WithGrokForbiddenPolicy(account)
	}

	MarkResponseCommitted(c)
	writeGrokMediaErrorResponse(c, resp.StatusCode, grokMediaErrorType(resp.StatusCode), upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func shouldPassthroughOpenAICompatibleGrokMediaError(statusCode int, body []byte) bool {
	if statusCode < http.StatusBadRequest || statusCode >= http.StatusInternalServerError || !gjson.ValidBytes(body) {
		return false
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests:
		return false
	default:
		return true
	}
}

func grokMediaErrorType(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}

func writeGrokMediaErrorResponse(c *gin.Context, statusCode int, errType, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
		},
	})
}

func writeGrokMediaResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}

func writeGrokMediaContentResponse(c *gin.Context, resp *http.Response) error {
	if c == nil || resp == nil || resp.Body == nil {
		return fmt.Errorf("grok media content response is incomplete")
	}

	for _, name := range []string{
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"ETag",
		"Last-Modified",
	} {
		if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
			c.Header(name, value)
		}
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	mediaType, _, contentTypeErr := mime.ParseMediaType(contentType)
	if contentType == "" || (contentTypeErr == nil && strings.EqualFold(mediaType, "application/octet-stream")) {
		contentType = "video/mp4"
	}
	c.Header("Content-Type", contentType)

	disposition := strings.TrimSpace(resp.Header.Get("Content-Disposition"))
	if disposition == "" {
		c.Header("Content-Disposition", "inline")
	} else if mediaDisposition, params, err := mime.ParseMediaType(disposition); err == nil {
		if strings.EqualFold(mediaDisposition, "attachment") {
			mediaDisposition = "inline"
		}
		c.Header("Content-Disposition", mime.FormatMediaType(mediaDisposition, params))
	} else {
		c.Header("Content-Disposition", "inline")
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Length")) == "" && resp.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	c.Status(resp.StatusCode)
	MarkResponseCommitted(c)
	n, err := io.Copy(c.Writer, resp.Body)
	if err == nil && resp.ContentLength >= 0 && n != resp.ContentLength {
		return io.ErrUnexpectedEOF
	}
	return err
}

func rewriteGrokMediaVideoContentURLs(body []byte, requestID, proxyURL string) []byte {
	if len(body) == 0 || strings.TrimSpace(requestID) == "" || strings.TrimSpace(proxyURL) == "" || !gjson.ValidBytes(body) {
		return body
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return body
	}
	changed := rewriteGrokMediaKnownVideoURL(&value, proxyURL)
	if rewriteGrokMediaVideoContentURLValue(&value, requestID, proxyURL) {
		changed = true
	}
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return rewritten
}

func rewriteGrokMediaKnownVideoURL(value *any, proxyURL string) bool {
	if value == nil {
		return false
	}
	root, ok := (*value).(map[string]any)
	if !ok {
		return false
	}
	video, ok := root["video"].(map[string]any)
	if !ok {
		return false
	}
	rawURL, ok := video["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return false
	}
	video["url"] = proxyURL
	return true
}

func rewriteGrokMediaVideoContentURLValue(value *any, requestID, proxyURL string) bool {
	if value == nil {
		return false
	}
	switch typed := (*value).(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			childValue := child
			if rewriteGrokMediaVideoContentURLValue(&childValue, requestID, proxyURL) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for index, child := range typed {
			childValue := child
			if rewriteGrokMediaVideoContentURLValue(&childValue, requestID, proxyURL) {
				typed[index] = childValue
				changed = true
			}
		}
		return changed
	case string:
		if isGrokMediaVideoContentURL(typed, requestID) {
			*value = proxyURL
			return true
		}
	}
	return false
}

func isGrokMediaVideoContentURL(rawURL, requestID string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Path == "" {
		return false
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) < 3 {
		return false
	}
	requestID = strings.Trim(requestID, "/")
	decodedID, err := url.PathUnescape(segments[len(segments)-2])
	if err != nil {
		return false
	}
	return segments[len(segments)-3] == "videos" &&
		decodedID == requestID &&
		segments[len(segments)-1] == "content"
}

func grokMediaContentProxyURL(c *gin.Context, requestID string) string {
	if c == nil || c.Request == nil || c.Request.URL == nil || strings.TrimSpace(requestID) == "" {
		return ""
	}
	pathPrefix := ""
	if strings.HasPrefix(c.Request.URL.Path, "/v1/") {
		pathPrefix = "/v1"
	}
	return pathPrefix + "/videos/" + url.PathEscape(strings.Trim(requestID, "/")) + "/content"
}
