package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"time"
)

type cs1ProviderStore interface {
	CS1UserBalance(context.Context) (float64, error)
	LoadProviderRecords(context.Context, any) error
	SaveCS1Provider(context.Context, string, int64, any, any) error
	DeleteCS1Provider(context.Context, string, int64) error
}

func (h *AccountHandler) ProviderUserBalance(c *gin.Context) {
	value, e := h.providerSettings.CS1UserBalance(c.Request.Context())
	if e != nil {
		response.Error(c, 500, "Failed to load user balance")
		return
	}
	response.Success(c, gin.H{"total_user_balance": value})
}

func (h *ChannelHandler) ConfigureCS1Providers(accounts *AccountHandler, probe *service.UpstreamBillingProbeService) {
	accounts.providerSettings = h.channelService
	accounts.providerEncryptor = h.channelService
	if probe != nil {
		probe.SetCS1ProviderRefresh(accounts.refreshCS1Providers)
	}
}
func (h *AccountHandler) saveProviderRecord(ctx context.Context, record *providerRecord) error {
	return h.providerSettings.SaveCS1Provider(ctx, record.ID, record.Revision, *record, record)
}
func (h *AccountHandler) updateCachedProviderWarning(_ context.Context, record *providerRecord) {
	record.LowBalanceAlert = !record.SuppressLowBalanceAlert && record.Metrics.Balance != nil && *record.Metrics.Balance*record.ExchangeRate < 300
}
func (h *AccountHandler) refreshCS1Providers(ctx context.Context) {
	if !providerWriteMu.TryLock() {
		return
	}
	defer providerWriteMu.Unlock()
	var records []providerRecord
	if h.providerSettings.LoadProviderRecords(ctx, &records) != nil {
		return
	}
	n := 0
	for i := range records {
		record := &records[i]
		if record.AttemptedAt != nil && time.Since(*record.AttemptedAt) < 10*time.Minute {
			continue
		}
		if ctx.Err() != nil || n >= 3 {
			return
		}
		n++
		probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		err := h.probeRecordContext(probeCtx, record, "")
		cancel()
		if err != nil {
			markProviderDisconnected(record, err)
		} else {
			h.updateCachedProviderWarning(ctx, record)
		}
		_ = h.saveProviderRecord(ctx, record)
	}
}
