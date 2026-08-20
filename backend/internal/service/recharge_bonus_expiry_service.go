package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type RechargeBonusExpiryService struct {
	repo         RechargeBonusRepository
	balanceCache *BillingCacheService
	interval     time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
}

func NewRechargeBonusExpiryService(repo RechargeBonusRepository, balanceCache *BillingCacheService, interval time.Duration) *RechargeBonusExpiryService {
	return &RechargeBonusExpiryService{
		repo:         repo,
		balanceCache: balanceCache,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

func (s *RechargeBonusExpiryService) Start() {
	if s == nil || s.repo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *RechargeBonusExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *RechargeBonusExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	total := 0
	for {
		result, err := s.repo.ExpireDue(ctx, time.Now().UTC(), 500)
		if err != nil {
			slog.Error("expire recharge bonus grants failed", "error", err)
			return
		}
		if result == nil {
			break
		}
		total += result.ExpiredGrants
		if s.balanceCache != nil {
			for _, userID := range result.UserIDs {
				if err := s.balanceCache.InvalidateUserBalance(ctx, userID); err != nil {
					slog.Warn("invalidate balance cache after scheduled bonus expiry failed", "user_id", userID, "error", err)
				}
			}
		}
		if result.ExpiredGrants < 500 {
			break
		}
	}
	if total > 0 {
		slog.Info("expired recharge bonus grants", "count", total)
	}
}
