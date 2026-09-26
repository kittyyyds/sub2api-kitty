//go:build unit

package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type suanliBillingTestCache struct {
	testutil.StubGatewayCache
	pending []byte
	claimed map[string]bool
}

func (s *suanliBillingTestCache) SetGrokVideoPendingBilling(_ context.Context, _ string, value []byte, _ time.Duration) error {
	s.pending = append([]byte(nil), value...)
	return nil
}
func (s *suanliBillingTestCache) GetGrokVideoPendingBilling(context.Context, string) ([]byte, error) {
	return s.pending, nil
}
func (s *suanliBillingTestCache) ClaimGrokVideoBilled(_ context.Context, key string, _ time.Duration) (bool, error) {
	if s.claimed[key] {
		return false, nil
	}
	s.claimed[key] = true
	return true, nil
}

func TestSuanliVideoCompletionRetainsThirtySecondsAndClaimsOnce(t *testing.T) {
	cache := &suanliBillingTestCache{claimed: map[string]bool{}}
	gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cache, &config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &OpenAIGatewayHandler{gatewayService: gateway}
	ctx := context.Background()
	groupID := int64(9)
	key := &service.APIKey{ID: 20, UserID: 10, GroupID: &groupID, Group: &service.Group{ID: groupID}}
	subject := middleware2.AuthSubject{UserID: 10}
	result := &service.OpenAIForwardResult{ResponseID: "task", VideoProvider: service.SuanliSpecialVideoProvider, VideoCount: 1, VideoDurationSeconds: 15, Model: "supplier-echo"}
	// Without the snapshot we cannot guess the model/resolution/charge.
	require.Nil(t, prepareGrokVideoCompletionBilling(ctx, h, zap.NewNop(), key, subject, "task", result))
	require.Empty(t, cache.claimed)
	require.NoError(t, gateway.StoreGrokVideoPendingBilling(ctx, "task", 10, 20, service.GrokVideoPendingBilling{
		VideoProvider: service.SuanliSpecialVideoProvider, Model: "特价/sd-2.5", BillingModel: "特价/sd-2.5", UpstreamModel: "特价/sd-2.5", VideoResolution: "720p", VideoDurationSeconds: 30,
	}))
	for i := 0; i < 20; i++ {
		bill := prepareGrokVideoCompletionBilling(ctx, h, zap.NewNop(), key, subject, "task", result)
		if i == 0 {
			require.NotNil(t, bill)
			require.Equal(t, 30, bill.VideoDurationSeconds)
			require.Equal(t, "特价/sd-2.5", bill.BillingModel)
			require.Equal(t, service.SuanliSpecialVideoProvider, bill.VideoProvider)
			require.Equal(t, service.StableGrokVideoBillingRequestID("task"), bill.RequestID)
		} else {
			require.Nil(t, bill)
		}
	}
	require.Len(t, cache.claimed, 1)
}
