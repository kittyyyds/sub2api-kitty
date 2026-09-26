package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// VideoTaskSummary excludes supplier credentials, prompts and result URLs.
type VideoTaskSummary struct {
	ID            string              `json:"id"`
	TaskID        string              `json:"task_id"`
	UserID        int64               `json:"user_id"`
	APIKeyID      int64               `json:"api_key_id"`
	GroupID       int64               `json:"group_id"`
	AccountID     int64               `json:"account_id,omitempty"`
	KeyName       string              `json:"key_name"`
	GroupName     string              `json:"group_name"`
	Model         string              `json:"model"`
	Status        string              `json:"status"`
	Resolution    string              `json:"resolution"`
	Seconds       int                 `json:"seconds"`
	Price         *VideoPriceSnapshot `json:"price_snapshot,omitempty"`
	UsageID       *int64              `json:"usage_id"`
	ActualCost    *float64            `json:"actual_cost"`
	Billed        bool                `json:"billed"`
	UsageRecorded bool                `json:"usage_recorded"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

type VideoTaskListFilter struct {
	UserID, APIKeyID, GroupID int64
	Admin                     bool
	Status, Model             string
	StartAt, EndAt            time.Time
	Offset, Limit             int
}

const VideoTaskMaxDateRange = 93 * 24 * time.Hour

// ValidateScope is shared by the service and SQL repository so an internal
// caller cannot accidentally issue an unbounded or ownerless aggregate.
func (f VideoTaskListFilter) ValidateScope() error {
	if (!f.Admin && f.UserID <= 0) || f.UserID < 0 || f.APIKeyID < 0 || f.GroupID < 0 {
		return fmt.Errorf("invalid video task owner scope")
	}
	if f.StartAt.IsZero() || f.EndAt.IsZero() || !f.EndAt.After(f.StartAt) || f.EndAt.Sub(f.StartAt) > VideoTaskMaxDateRange {
		return fmt.Errorf("video task date range must be within 93 days")
	}
	if len(f.Model) > 200 || strings.ContainsRune(f.Model, '\x00') {
		return fmt.Errorf("invalid video model")
	}
	switch f.Status {
	case "", "submitting", "unknown", "pending", "done", "failed", "needs_review":
	default:
		return fmt.Errorf("invalid video task status")
	}
	return nil
}

type VideoTaskMetrics struct {
	Total                    int64    `json:"total"`
	Succeeded                int64    `json:"succeeded"`
	Failed                   int64    `json:"failed"`
	InProgress               int64    `json:"in_progress"`
	NeedsReview              int64    `json:"needs_review"`
	SuccessRate              *float64 `json:"success_rate"`
	SuccessfulSeconds        int64    `json:"successful_seconds"`
	ActualCost               float64  `json:"actual_cost"`
	Unreconciled             int64    `json:"unreconciled"`
	AverageCompletionSeconds *float64 `json:"average_completion_seconds"`
}

type VideoTaskModelStats struct {
	Model string `json:"model"`
	VideoTaskMetrics
}

type VideoTaskStats struct {
	VideoTaskMetrics
	Models  []VideoTaskModelStats `json:"models"`
	StartAt time.Time             `json:"start_at"`
	EndAt   time.Time             `json:"end_at"`
}

type VideoTaskStatsReader interface {
	GetVideoTaskStats(context.Context, VideoTaskListFilter) (*VideoTaskStats, error)
}

type VideoTaskLister interface {
	ListVideoTaskSummaries(context.Context, VideoTaskListFilter) ([]VideoTaskSummary, error)
}

func (s *OpenAIGatewayService) ListVideoTaskSummaries(ctx context.Context, filter VideoTaskListFilter) ([]VideoTaskSummary, error) {
	if err := filter.ValidateScope(); err != nil {
		return nil, err
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	if filter.Offset < 0 || filter.Offset > 10000 {
		return nil, fmt.Errorf("invalid pagination")
	}
	repo, ok := s.usageLogRepo.(VideoTaskLister)
	if !ok {
		return nil, fmt.Errorf("video task listing unavailable")
	}
	return repo.ListVideoTaskSummaries(ctx, filter)
}

func (s *OpenAIGatewayService) GetVideoTaskStats(ctx context.Context, filter VideoTaskListFilter) (*VideoTaskStats, error) {
	if err := filter.ValidateScope(); err != nil {
		return nil, err
	}
	repo, ok := s.usageLogRepo.(VideoTaskStatsReader)
	if !ok {
		return nil, fmt.Errorf("video task statistics unavailable")
	}
	return repo.GetVideoTaskStats(ctx, filter)
}
