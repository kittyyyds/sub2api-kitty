package service

import (
	"context"
	"errors"
	"time"
)

// Return handled even for zero: a final free cell must not fall back to a
// positive default/channel price. Missing image sizes retain normal estimation.
func (d inflightEstimateDeps) finalMediaEstimate(key *APIKey, req InflightEstimateRequest) (cost float64, handled, priced bool) {
	if key == nil || key.Group == nil {
		return 0, false, false
	}
	units := req.Units
	if units <= 0 {
		units = 1
	}
	if req.Kind == InflightEstimateVideo && key.Group.Platform == PlatformVideo {
		price, err := ResolveVideoGroupPrice(key.Group, req.Model)
		if err != nil {
			return 0, true, false
		}
		snapshot := &VideoPriceSnapshot{Model: req.Model, BillingMode: price.BillingMode, UnitPrice: price.UnitPrice, RateMultiplier: 1, FinalUnitPrice: true, DurationSeconds: req.VideoDurationSeconds}
		bill, err := calculateVideoPriceSnapshotCost(snapshot, units)
		if err != nil {
			return 0, true, false
		}
		return bill.ActualCost, true, true
	}
	if d.billing == nil {
		return 0, false, false
	}
	switch req.Kind {
	case InflightEstimateImage:
		cfg := imagePriceConfigFromAPIKey(key)
		if cfg == nil {
			return 0, false, false
		}
		// The request has not selected an output size yet: keep upstream's
		// conservative maximum across sizes, but honor final prices at 1x.
		allFinal := true
		for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
			if LookupImageModelPrice(cfg.ModelPrices, req.Model, tier) == nil {
				allFinal = false
				break
			}
			bill := d.billing.CalculateImageCost(req.Model, tier, units, cfg, 1)
			if bill.ActualCost > cost {
				cost = bill.ActualCost
			}
		}
		if allFinal {
			return cost, true, true
		}
	case InflightEstimateVideo:
		cfg := videoPriceConfigFromAPIKey(key)
		if cfg != nil && LookupVideoModelPrice(cfg.ModelPrices, req.Model, req.VideoResolution) != nil {
			bill := d.billing.CalculateVideoCost(req.Model, req.VideoResolution, units, req.VideoDurationSeconds, cfg, 1)
			return bill.ActualCost, true, true
		}
	}
	return 0, false, false
}

// A transferred reservation outlives its creation HTTP handler and survives
// process restarts through the existing pending JSON. This is still an estimate
// cache, with bounded expiry, not a replacement for the SQL billing ledger.
type VideoInflightReservation struct {
	RequestID string `json:"request_id"`
}

const videoInflightReservationTTL = 7 * 24 * time.Hour

func (r *InflightReservation) transferToVideo(ttl time.Duration) (*VideoInflightReservation, error) {
	if r == nil {
		return nil, nil
	}
	r.stopRenewal()
	renewer, ok := r.cache.(InflightBalanceReservationRenewer)
	if !ok {
		return nil, errors.New("video reservation requires renewable cache")
	}
	ctx, cancel := context.WithTimeout(context.Background(), inflightReservationRenewTimeout)
	defer cancel()
	alive, err := renewer.RenewInflightBalance(ctx, r.userID, r.requestID, ttl)
	if err != nil {
		return nil, err
	}
	if !alive {
		return nil, errors.New("video reservation expired before transfer")
	}
	r.transferred.Store(true)
	return &VideoInflightReservation{RequestID: r.requestID}, nil
}

func (s *BillingCacheService) releaseVideoInflight(ctx context.Context, userID int64, ref *VideoInflightReservation) error {
	if s == nil || ref == nil || ref.RequestID == "" {
		return nil
	}
	cache, ok := s.cache.(InflightBalanceReservationCache)
	if !ok {
		return nil
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inflightReservationReleaseTimeout)
	defer cancel()
	return cache.ReleaseInflightBalance(releaseCtx, userID, ref.RequestID)
}

func (s *OpenAIGatewayService) finishVideoTask(ctx context.Context, task *VideoTask, state, message string) error {
	if err := s.videoTaskRepository().SetVideoTaskState(ctx, task.ID, state, message); err != nil {
		return err
	}
	return s.billingCacheService.releaseVideoInflight(ctx, task.UserID, task.Pending.InflightReservation)
}

func (s *OpenAIGatewayService) ReleaseVideoInflightAfterBilling(ctx context.Context, remoteID string, userID, keyID int64) error {
	pending, err := s.LoadGrokVideoPendingBilling(ctx, remoteID, userID, keyID)
	if err != nil || pending == nil {
		return err
	}
	return s.billingCacheService.releaseVideoInflight(ctx, userID, pending.InflightReservation)
}
