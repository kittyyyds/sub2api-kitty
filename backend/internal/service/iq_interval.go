package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"strings"
	"time"
)

const IQCompletionInterval = "completion_interval"

func IsIQIntervalPlan(p *ScheduledTestPlan) bool {
	return p != nil && p.PelicanConfig != nil && p.PelicanConfig.ScheduleMode == IQCompletionInterval
}

type IQIntervalRepository interface {
	ListDueIQ(context.Context, time.Time) ([]*ScheduledTestPlan, error)
	FinishIQ(context.Context, *ScheduledTestPlan, time.Time, time.Time) error
	IQSummaries(context.Context, []int64) ([]IQSummary, error)
}
type IQResult struct {
	ID         int64     `json:"id"`
	Verdict    string    `json:"verdict"`
	Answer     string    `json:"answer"`
	Error      string    `json:"error,omitempty"`
	Model      string    `json:"model"`
	Reasoning  string    `json:"reasoning"`
	FinishedAt time.Time `json:"finished_at"`
}
type IQSummary struct {
	AccountID       int64      `json:"account_id"`
	PlanID          int64      `json:"plan_id"`
	Enabled         bool       `json:"enabled"`
	Model           string     `json:"model"`
	Reasoning       string     `json:"reasoning"`
	IntervalSeconds int        `json:"interval_seconds"`
	RunningUntil    *time.Time `json:"running_until"`
	NextRunAt       *time.Time `json:"next_run_at"`
	Results         []IQResult `json:"results"`
}

func IQAnswerVerdict(status, answer string, errorMessage ...string) string {
	// A completed answer mismatch is a yellow result, not a transport failure.
	answerMismatch := len(errorMessage) > 0 && strings.TrimSpace(errorMessage[0]) == "answer_mismatch: expected 21" && strings.TrimSpace(answer) != ""
	if status != "success" && !answerMismatch {
		return "error"
	}
	if CandyAnswerCorrect(answer) {
		return "correct"
	}
	return "other"
}
func (s *ScheduledTestService) IQSummaries(ctx context.Context, ids []int64) ([]IQSummary, error) {
	if len(ids) < 1 || len(ids) > 200 {
		return nil, fmt.Errorf("每次最多查询 200 个账号")
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("无效账号 ID")
		}
	}
	store, ok := s.planRepo.(IQIntervalRepository)
	if !ok {
		return nil, fmt.Errorf("定时测试仓储未就绪")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return store.IQSummaries(ctx, ids)
}
func (s *ScheduledTestRunnerService) finishIQOrPelican(ctx context.Context, p *ScheduledTestPlan, until, finished time.Time) error {
	if IsIQIntervalPlan(p) {
		if store, ok := s.planRepo.(IQIntervalRepository); ok {
			return store.FinishIQ(ctx, p, until, finished)
		}
	}
	return s.planRepo.FinishPelican(ctx, p.ID, until, finished)
}
func (s *ScheduledTestRunnerService) startIQIntervals() {
	store, ok := s.planRepo.(IQIntervalRepository)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.iqCancel = cancel
	slots := make(chan struct{}, 8)
	s.iqWG.Add(1)
	go func() {
		defer s.iqWG.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if len(slots) == cap(slots) {
					continue
				}
				scanCtx, stop := context.WithTimeout(ctx, 2*time.Second)
				plans, err := store.ListDueIQ(scanCtx, time.Now())
				stop()
				if err != nil {
					logger.LegacyPrintf("service.iq_interval", "scan failed")
					continue
				}
				for _, plan := range plans {
					acquired := false
					select {
					case slots <- struct{}{}:
						acquired = true
					default:
					}
					if !acquired {
						break
					}
					// Each dispatched job owns one local slot. Persistent leases enforce the
					// same eight-request ceiling across overlapping deployment processes.
					s.iqWG.Add(1)
					go func(p *ScheduledTestPlan) {
						defer s.iqWG.Done()
						defer func() { <-slots }()
						parent, done, allowed := managedinstance.Begin(ctx)
						if !allowed {
							return
						}
						defer done()
						runCtx, end := context.WithTimeout(parent, 11*time.Minute)
						defer end()
						s.runOnePlan(runCtx, p)
					}(plan)
					if len(slots) == cap(slots) {
						break
					}
				}
			}
		}
	}()
}
