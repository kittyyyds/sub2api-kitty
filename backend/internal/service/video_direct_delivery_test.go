//go:build unit

package service

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"testing"
	"time"
)

func TestVideoUnifiedSubmissionDoesNotExposeEarlyResultOrChangeLockedDuration(t *testing.T) {
	svc, repo, _, account, _ := newVideoTaskFixture()
	task := &VideoTask{ID: "local", UserID: 1, APIKeyID: 6, AccountID: account.ID, PreparedBody: []byte(`{"model":"unified-fixture","prompt":"motion","seconds":5,"resolution":"720p","aspect_ratio":"16:9"}`), Pending: GrokVideoPendingBilling{Unified: true, Model: "unified-fixture", VideoDurationSeconds: 5, VideoResolution: "720p", PrepaidAmount: 1.5, PriceSnapshot: &VideoPriceSnapshot{PrepaidTaskID: "local", DurationSeconds: 5, UnitPrice: .3}}}
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"supplier-task","status":"completed","seconds":99,"url":"https://cdn.example/video.mp4?signature=fixture"}`)}
	svc.httpUpstream = upstream
	c, rec := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos", nil)
	result, err := svc.ForwardGrokMedia(WithVideoTask(context.Background(), task), c, account, GrokMediaEndpointVideosGenerations, "", nil, "application/json")
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Zero(t, result.VideoCount)
	require.Equal(t, 5, repo.task.Pending.PriceSnapshot.DurationSeconds)
	require.NotContains(t, rec.Body.String(), "cdn.example")
	require.Equal(t, "local", rec.Header().Get("X-Video-Task-ID"))
	require.Equal(t, "local", gjson.Get(rec.Body.String(), "local_task_id").String())
	require.Equal(t, "/v1/videos", upstream.request.URL.Path)
}
func TestVideoUnifiedDeliveryRequiresCommittedSettlementAndDoesNotFetchFile(t *testing.T) {
	svc, repo, _, account, _ := newVideoTaskFixture()
	repo.billed = true
	repo.usageRecorded = true
	task := &VideoTask{ID: "local", UpstreamTaskID: "supplier-task", UserID: 1, APIKeyID: 6, AccountID: account.ID, Status: "pending", BillingState: "captured", CreatedAt: time.Now(), Pending: GrokVideoPendingBilling{Unified: true, PrepaidAmount: 1.5}}
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"supplier-task","status":"completed","url":"https://cdn.example/video.mp4?signature=fixture"}`)}
	svc.httpUpstream = upstream
	body, err := svc.UnifiedVideoStatus(context.Background(), task, nil)
	require.NoError(t, err)
	require.Equal(t, "completed", gjson.GetBytes(body, "status").String())
	require.Equal(t, "upstream_direct", gjson.GetBytes(body, "delivery_mode").String())
	require.Equal(t, "https://cdn.example/video.mp4?signature=fixture", gjson.GetBytes(body, "url").String())
	require.Len(t, upstream.requests, 1)
	require.Equal(t, http.MethodGet, upstream.requests[0].Method)
	task.Status = "failed"
	upstream.response = grokMediaContentStatusResponse(`{"status":"completed","url":"https://cdn.example/video.mp4"}`)
	body, err = svc.UnifiedVideoStatus(context.Background(), task, nil)
	require.Error(t, err)
	require.Empty(t, body)
}
func TestVideoUnifiedUnknownAndRefundedNeverQuerySupplier(t *testing.T) {
	svc, _, _, _, _ := newVideoTaskFixture()
	for _, state := range []string{"held", "refunded"} {
		body, err := svc.UnifiedVideoStatus(context.Background(), &VideoTask{ID: "local", BillingState: state, Pending: GrokVideoPendingBilling{Unified: true}}, nil)
		require.NoError(t, err)
		var v map[string]any
		require.NoError(t, json.Unmarshal(body, &v))
		require.Equal(t, "local", v["id"])
		require.Equal(t, "upstream_direct", v["delivery_mode"])
		require.Nil(t, v["url"])
	}
}

func TestVideoUnifiedAcceptsSupplierSignedContentButNotUnsignedRelay(t *testing.T) {
	base := "https://suanliai.top/v1/videos/task"
	require.True(t, unifiedDirectVideoURL(base+"/content?owner=fixture&expires=9999999999&video_signature=fixture", base, "task"))
	require.False(t, unifiedDirectVideoURL(base+"/content", base, "task"))
	require.False(t, unifiedDirectVideoURL("https://kittyyyds.xyz/v1/videos/task/content?owner=fixture&expires=9999999999&video_signature=fixture", base, "task"))
	require.True(t, unifiedDirectVideoURL("https://cdn.example/video.mp4", base, "task"))
}
