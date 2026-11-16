//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageTaskMemoryRepo struct {
	UsageLogRepository
	tasks map[string]*SuanliImageTask
}

func (r *imageTaskMemoryRepo) CreateSuanliImageTask(_ context.Context, t *SuanliImageTask) (*SuanliImageTask, bool, error) {
	for _, old := range r.tasks {
		if old.APIKeyID == t.APIKeyID && old.IdempotencyHash == t.IdempotencyHash {
			return old, false, nil
		}
	}
	r.tasks[t.ID] = t
	return t, true, nil
}
func (r *imageTaskMemoryRepo) GetSuanliImageTask(_ context.Context, id string, u, k int64) (*SuanliImageTask, error) {
	t := r.tasks[id]
	if t != nil && t.UserID == u && t.APIKeyID == k {
		return t, nil
	}
	return nil, nil
}
func (r *imageTaskMemoryRepo) UpdateSuanliImageTask(_ context.Context, t *SuanliImageTask, _ time.Duration) error {
	r.tasks[t.ID] = t
	return nil
}
func (r *imageTaskMemoryRepo) LeaseSuanliImageTasks(context.Context, int) ([]*SuanliImageTask, error) {
	return nil, nil
}
func (r *imageTaskMemoryRepo) ExpireSuanliImageResults(context.Context) error { return nil }

func TestSuanliImageForwardAliasAndLockedFinalPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"中文图片别名","prompt":"test","size":"2048x1152","n":1}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	group := &Group{Platform: PlatformOpenAI, AllowImageGeneration: true, ImageModelPrices: map[string]map[string]float64{"中文图片别名": {"2K": .25}}, ModelPricing: []ChannelModelPricing{{Models: []string{"中文图片别名"}, Image: &ImageModelConfig{Provider: "suanli", UpstreamModel: "nano-banana-pro-稳定出图"}}}}
	key := &APIKey{ID: 9, UserID: 7, User: &User{ID: 7}, Group: group}
	c.Set("api_key", key)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"![result](https://example.com/result.png)"}}]}`))}}
	repo := &imageTaskMemoryRepo{tasks: map[string]*SuanliImageTask{}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, usageLogRepo: repo}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.True(t, parsed.Suanli)
	require.Equal(t, OpenAIImagesCapabilityAPIKey, parsed.RequiredCapabilityForModel(""))
	account := &Account{ID: 51, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://suanliai.top/v1", "api_key": "synthetic-only", "model_mapping": map[string]any{"中文图片别名": "nano-banana-pro-稳定出图"}}}
	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, .25, *result.SuanliImageUnitPrice)
	require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
	require.Equal(t, "Bearer synthetic-only", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("Cookie"))
	group.ImageModelPrices["中文图片别名"]["2K"] = 9
	cost := svc.calculateOpenAIImageCost(context.Background(), "other", "中文图片别名", key, result, 100)
	require.Equal(t, .25, cost.ActualCost)
	require.True(t, cost.FinalPriceApplied)
	capability, ok := ImageGroupCapability(group, "中文图片别名")
	require.True(t, ok)
	require.Equal(t, "image", capability.MediaType)
	require.Equal(t, "ready", repo.tasks[result.SuanliImageTaskID].Status)
	// Retry with the same request ID can only query the saved task; even a
	// changed selling price cannot create a second paid supplier request.
	c.Request.Header.Set("Idempotency-Key", "retry-fixture")
	first, created, err := svc.beginSuanliImageTask(context.Background(), key, account, parsed, "remote", "retry-fixture", .25)
	require.NoError(t, err)
	require.True(t, created)
	second, created, err := svc.beginSuanliImageTask(context.Background(), key, account, parsed, "remote", "retry-fixture", 9)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, .25, second.Snapshot.UnitPrice)
	_, err = svc.GetSuanliImageTask(context.Background(), first.ID, ImageTaskOwner{UserID: 99, APIKeyID: 9}, nil)
	require.ErrorIs(t, err, ErrImageTaskNotFound)
}
func TestImageSyncDoesNotTouchOtherProviders(t *testing.T) {
	cfg := DefaultImageRuntimeSettings()
	cfg.AutoImportAccountModels = true
	old := ImagePricingGroup{ID: 1}
	got, warn := reconcileImageAccountPricing(old, []VideoAccountModelBinding{{BaseURL: "https://other.example", Models: map[string]string{"gpt-image-2": "gpt-image-2"}}}, nil, cfg, true)
	require.Equal(t, old, got)
	require.False(t, warn)
}
