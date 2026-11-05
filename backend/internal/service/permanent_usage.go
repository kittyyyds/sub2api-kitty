package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type PermanentUsageSnapshot struct {
	QueriedAt time.Time       `json:"queried_at"`
	State     json.RawMessage `json:"state"`
	Totals    json.RawMessage `json:"totals"`
}
type PermanentUsageRepository interface {
	ReadPermanentUsage(context.Context) (*PermanentUsageSnapshot, error)
	BackfillPermanentUsage(context.Context) (int64, error)
}

func (s *ChannelService) PermanentUsage(ctx context.Context) (*PermanentUsageSnapshot, error) {
	r, ok := s.repo.(PermanentUsageRepository)
	if !ok {
		return nil, errors.New("permanent usage unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return r.ReadPermanentUsage(ctx)
}
func (s *ChannelService) BackfillPermanentUsage(ctx context.Context) (int64, error) {
	r, ok := s.repo.(PermanentUsageRepository)
	if !ok {
		return 0, errors.New("permanent usage unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return r.BackfillPermanentUsage(ctx)
}
