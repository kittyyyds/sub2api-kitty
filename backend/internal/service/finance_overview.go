package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type FinanceOverview struct {
	From       time.Time       `json:"from"`
	Until      time.Time       `json:"until"`
	QueriedAt  time.Time       `json:"queried_at"`
	Usage      json.RawMessage `json:"usage"`
	Cash       json.RawMessage `json:"cash"`
	Balances   json.RawMessage `json:"balances"`
	Credits    json.RawMessage `json:"credits"`
	Monitoring json.RawMessage `json:"monitoring"`
}
type FinanceOverviewRepository interface {
	ReadFinanceOverview(context.Context, time.Time, time.Time) (*FinanceOverview, error)
}

func ValidateFinanceRange(from, until time.Time) bool {
	return !from.IsZero() && until.After(from) && until.Sub(from) <= 31*24*time.Hour
}
func (s *ChannelService) FinanceOverview(ctx context.Context, from, until time.Time) (*FinanceOverview, error) {
	if !ValidateFinanceRange(from, until) {
		return nil, ErrUpstreamOperationsValidation
	}
	repo, ok := s.repo.(FinanceOverviewRepository)
	if !ok {
		return nil, errors.New("finance overview unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return repo.ReadFinanceOverview(ctx, from, until)
}
