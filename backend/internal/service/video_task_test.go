//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type videoTaskMemoryRepo struct {
	UsageLogRepository
	task                 *VideoTask
	createErr, acceptErr error
	billed               bool
	usageRecorded        bool
	state                string
	beforeAccept         func()
}

func (r *videoTaskMemoryRepo) CreateVideoTask(_ context.Context, t *VideoTask) error {
	r.task = t
	return r.createErr
}
func (r *videoTaskMemoryRepo) AcceptVideoTask(_ context.Context, t *VideoTask) error {
	if r.beforeAccept != nil {
		r.beforeAccept()
	}
	r.task = t
	return r.acceptErr
}
func (r *videoTaskMemoryRepo) FindVideoTask(_ context.Context, id string, uid, kid int64) (*VideoTask, error) {
	if r.task != nil && r.task.UpstreamTaskID == id && r.task.UserID == uid && r.task.APIKeyID == kid {
		return r.task, nil
	}
	return nil, nil
}
func (r *videoTaskMemoryRepo) LeaseVideoTasks(context.Context, int) ([]*VideoTask, error) {
	return nil, nil
}
func (r *videoTaskMemoryRepo) SetVideoTaskState(_ context.Context, _ string, state, _ string) error {
	r.state = state
	return nil
}
func (r *videoTaskMemoryRepo) VideoTaskWasBilled(context.Context, string, int64) (bool, error) {
	return r.billed, nil
}

func (r *videoTaskMemoryRepo) VideoTaskUsageRecorded(context.Context, string, int64) (bool, error) {
	return r.usageRecorded, nil
}

type videoTaskAccountRepo struct {
	AccountRepository
	account *Account
}

func (r videoTaskAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func newVideoTaskFixture() (*OpenAIGatewayService, *videoTaskMemoryRepo, *APIKey, *Account, []byte) {
	price := 1.30
	group := &Group{ID: 25, Platform: PlatformVideo, RateMultiplier: 1, ModelPricing: []ChannelModelPricing{{Platform: PlatformVideo, Models: []string{"特价/sd-2.5"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price}}}
	key := &APIKey{ID: 6, UserID: 1, User: &User{ID: 1}, Group: group, GroupID: &group.ID}
	account := suanliTestAccount()
	account.Platform = PlatformVideo
	repo := &videoTaskMemoryRepo{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, usageLogRepo: repo, accountRepo: videoTaskAccountRepo{account: account}}
	body := []byte(`{"model":"特价/sd-2.5","prompt":"motion","images":["https://example.com/ref.png"]}`)
	return svc, repo, key, account, body
}

func TestVideoTaskPersistsBeforeSubmissionAndLocksPrice(t *testing.T) {
	svc, repo, key, account, body := newVideoTaskFixture()
	task, err := svc.BeginVideoTask(context.Background(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
	require.NoError(t, err)
	require.Same(t, task, repo.task)
	require.Equal(t, 30, task.Pending.PriceSnapshot.DurationSeconds)
	require.Equal(t, 1.30, task.Pending.PriceSnapshot.UnitPrice)
	*key.Group.ModelPricing[0].PerRequestPrice = 99
	key.Group.RateMultiplier = 9
	require.Equal(t, 1.30, task.Pending.PriceSnapshot.UnitPrice)
	require.Equal(t, 1.0, task.Pending.PriceSnapshot.RateMultiplier)
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"durable-1","status":"queued"}`)}
	svc.httpUpstream = upstream
	c, rec := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
	repo.beforeAccept = func() { require.Empty(t, rec.Body.String(), "client must not receive accepted ID before durable save") }
	result, err := svc.ForwardGrokMedia(WithVideoTask(context.Background(), task), c, account, GrokMediaEndpointVideosGenerations, "", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, "durable-1", task.UpstreamTaskID)
	require.Zero(t, result.VideoCount)
	require.Equal(t, 30, task.Pending.VideoDurationSeconds)
	// Simulate complete Redis loss; SQL is sufficient for ownership and pending.
	owner, err := svc.ResolveGrokMediaVideoRequestAccount(context.Background(), key.GroupID, "durable-1", 1, 6)
	require.NoError(t, err)
	require.Equal(t, account.ID, owner)
	pending, err := svc.LoadGrokVideoPendingBilling(context.Background(), "durable-1", 1, 6)
	require.NoError(t, err)
	require.Equal(t, 1.30, pending.PriceSnapshot.UnitPrice)
	found, err := svc.FindVideoTask(context.Background(), "durable-1", 2, 6)
	require.NoError(t, err)
	require.Nil(t, found, "task must not be exposed to another owner")
}

func TestVideoTaskDatabaseFailurePreventsAcceptedSuccess(t *testing.T) {
	svc, repo, key, account, body := newVideoTaskFixture()
	repo.createErr = errors.New("db down")
	_, err := svc.BeginVideoTask(context.Background(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
	require.Error(t, err)
	repo.createErr = nil
	task, err := svc.BeginVideoTask(context.Background(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
	require.NoError(t, err)
	repo.acceptErr = errors.New("db disconnected")
	svc.httpUpstream = &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"accepted-but-unrecorded","status":"queued"}`)}
	c, rec := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
	_, err = svc.ForwardGrokMedia(WithVideoTask(context.Background(), task), c, account, GrokMediaEndpointVideosGenerations, "", body, "application/json")
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "video_task_persistence_failed")
}

func TestVideoTaskWorkerOnlyQueriesAndDoesNotChargeFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"pending", `{"id":"task","status":"processing"}`, "", 200},
		{"failed", `{"id":"task","status":"failed"}`, "failed", 200},
		{"http-error", `{"error":"temporary"}`, "", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, key, account, body := newVideoTaskFixture()
			task, err := svc.BeginVideoTask(context.Background(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
			require.NoError(t, err)
			task.UpstreamTaskID = "task"
			task.Status = "pending"
			response := grokMediaContentStatusResponse(tc.body)
			response.StatusCode = tc.status
			upstream := &grokMediaContentUpstreamStub{response: response}
			svc.httpUpstream = upstream
			err = svc.pollVideoTask(context.Background(), task, nil)
			if tc.status >= 400 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, repo.state)
			require.Equal(t, http.MethodGet, upstream.request.Method)
			require.Equal(t, "/v1/videos/task", upstream.request.URL.Path)
		})
	}
}

func TestVideoTaskRecoveryRecognizesSQLDedupWithoutNewCharge(t *testing.T) {
	svc, repo, _, _, _ := newVideoTaskFixture()
	repo.billed = true
	repo.usageRecorded = true
	task := &VideoTask{ID: "local", UpstreamTaskID: "done", APIKeyID: 6, Status: "pending", CreatedAt: time.Now()}
	require.NoError(t, svc.pollVideoTask(context.Background(), task, nil))
	require.Equal(t, "done", repo.state)
	require.NoError(t, svc.SettleVideoTask(context.Background(), task, &OpenAIForwardResult{VideoCount: 1}, nil))
	require.Equal(t, "done", repo.state)
}

func TestVideoTaskBilledWithoutReceiptRequiresOriginalEvidence(t *testing.T) {
	svc, repo, _, _, _ := newVideoTaskFixture()
	repo.billed = true
	task := &VideoTask{ID: "local", UpstreamTaskID: "old-charge", APIKeyID: 6,
		Status: "pending", CreatedAt: time.Now().Add(-8 * 24 * time.Hour)}
	// No key service or supplier client: no additional charge or remote request
	// is possible, even when an old receipt is missing after the normal timeout.
	err := svc.pollVideoTask(context.Background(), task, nil)
	require.ErrorIs(t, err, ErrVideoUsageEvidenceMissing)
	require.Equal(t, "needs_review", repo.state)
}
