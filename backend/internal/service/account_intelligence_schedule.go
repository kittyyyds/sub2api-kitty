package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

// Kept outside account.Extra so a stale edit, account import or token refresh
// cannot overwrite a running schedule. Existing settings storage needs no DDL.
const AccountIntelligenceSchedulePrefix = "account_intelligence_schedule:"

type AccountIntelligenceSchedule struct {
	AccountID       int64      `json:"account_id"`
	Revision        int64      `json:"revision"`
	Enabled         bool       `json:"enabled"`
	Kind            string     `json:"kind"`
	Model           string     `json:"model"`
	ReasoningEffort string     `json:"reasoning_effort"`
	Prompt          string     `json:"prompt"`
	CronExpression  string     `json:"cron_expression"`
	Timezone        string     `json:"timezone"`
	MaxResults      int        `json:"max_results"`
	NextRunAt       *time.Time `json:"next_run_at"`
	LastAttemptAt   *time.Time `json:"last_attempt_at"`
	LastError       string     `json:"last_error"`
}

type AccountIntelligenceScheduleRepository interface {
	IntelligenceScheduleGet(context.Context, int64) (*AccountIntelligenceSchedule, error)
	IntelligenceScheduleSave(context.Context, *AccountIntelligenceSchedule, int64) (bool, error)
	IntelligenceSchedules(context.Context) ([]AccountIntelligenceSchedule, error)
	IntelligenceScheduleClaim(context.Context, *AccountIntelligenceSchedule, time.Time, time.Time) (bool, error)
	IntelligenceScheduleError(context.Context, *AccountIntelligenceSchedule, time.Time, string) error
	IntelligencePruneScheduled(context.Context, int64, int) error
}

func intelligenceScheduleError(message string) error {
	return infraerrors.BadRequest("INVALID_INTELLIGENCE_SCHEDULE", message)
}
func defaultIntelligenceSchedule(id int64) *AccountIntelligenceSchedule {
	return &AccountIntelligenceSchedule{AccountID: id, Kind: "candy", CronExpression: "*/30 * * * *", MaxResults: 100}
}
func (s *AccountTestService) intelligenceScheduleRepo() (AccountIntelligenceScheduleRepository, error) {
	if s != nil {
		if r, ok := s.accountRepo.(AccountIntelligenceScheduleRepository); ok {
			return r, nil
		}
	}
	return nil, infraerrors.New(503, "INTELLIGENCE_UNAVAILABLE", "自动检测配置服务暂时不可用。")
}
func (s *AccountTestService) intelligenceLocation() *time.Location {
	if s != nil && s.cfg != nil {
		if loc, e := time.LoadLocation(s.cfg.Timezone); e == nil {
			return loc
		}
	}
	return time.Local
}
func validateIntelligenceSchedule(plan *AccountIntelligenceSchedule, now time.Time, loc *time.Location) error {
	plan.CronExpression = strings.TrimSpace(plan.CronExpression)
	if len(plan.CronExpression) > 100 || len(strings.Fields(plan.CronExpression)) != 5 {
		return intelligenceScheduleError("Cron 须为 5 个字段，例如 */30 * * * *；不要填写中文说明。")
	}
	schedule, err := scheduledTestCronParser.Parse(plan.CronExpression)
	if err != nil {
		return intelligenceScheduleError("Cron 格式无效，请选择预设周期，或填写例如 */30 * * * *。")
	}
	// The five-field parser only schedules at minute boundaries, so it already
	// guarantees the one-minute minimum. Do not expand a year's worth of runs
	// when validating an every-minute schedule.
	at := now.In(loc)
	next := schedule.Next(at)
	if next.IsZero() {
		return intelligenceScheduleError("该 Cron 没有可执行的日期。")
	}
	if plan.MaxResults < 1 || plan.MaxResults > 1000 {
		return intelligenceScheduleError("自动检测记录保留数量须为 1–1000。")
	}
	input := AccountIntelligenceInput{RequestID: uuid.NewString(), Kind: plan.Kind, Model: plan.Model, ReasoningEffort: plan.ReasoningEffort, Prompt: plan.Prompt}
	if err := validateAccountIntelligence(&input); err != nil {
		return err
	}
	plan.Kind = input.Kind
	plan.Model = input.Model
	plan.ReasoningEffort = input.ReasoningEffort
	plan.Prompt = input.Prompt
	plan.Timezone = loc.String()
	plan.NextRunAt = nil
	if plan.Enabled {
		n := schedule.Next(at)
		plan.NextRunAt = &n
	}
	return nil
}
func (s *AccountTestService) GetIntelligenceSchedule(ctx context.Context, id int64) (*AccountIntelligenceSchedule, error) {
	if _, err := s.accountRepo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	r, err := s.intelligenceScheduleRepo()
	if err != nil {
		return nil, err
	}
	p, err := r.IntelligenceScheduleGet(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		p = defaultIntelligenceSchedule(id)
	}
	p.Timezone = s.intelligenceLocation().String()
	return p, nil
}
func (s *AccountTestService) SaveIntelligenceSchedule(ctx context.Context, id int64, plan AccountIntelligenceSchedule) (*AccountIntelligenceSchedule, error) {
	a, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Platform != PlatformOpenAI && a.Platform != PlatformAnthropic {
		return nil, intelligenceScheduleError("目前仅支持 OpenAI 和 Claude 文本账号。")
	}
	if plan.Revision < 0 {
		return nil, intelligenceScheduleError("配置版本无效，请重新打开窗口。")
	}
	if err = validateIntelligenceSchedule(&plan, time.Now(), s.intelligenceLocation()); err != nil {
		return nil, err
	}
	if err = validateIntelligenceModel(a, plan.Model); err != nil {
		return nil, err
	}
	r, err := s.intelligenceScheduleRepo()
	if err != nil {
		return nil, err
	}
	revision := plan.Revision
	plan.AccountID = id
	plan.Revision++
	plan.LastAttemptAt = nil
	plan.LastError = ""
	saved, err := r.IntelligenceScheduleSave(ctx, &plan, revision)
	if err != nil {
		return nil, err
	}
	if !saved {
		return nil, infraerrors.New(409, "INTELLIGENCE_SCHEDULE_CONFLICT", "配置已被其他窗口修改，请重新加载后保存。")
	}
	return &plan, nil
}

// Called only by the managed background owner's minute sweep. Its lifecycle
// context is retained until all admitted executions finish or are canceled.
func (s *ScheduledTestRunnerService) runAccountIntelligenceSchedules(ctx context.Context, now time.Time) {
	if s.accountTestSvc == nil {
		return
	}
	repo, err := s.accountTestSvc.intelligenceScheduleRepo()
	if err != nil {
		return
	}
	queryCtx, cancel := context.WithTimeout(ctx, scheduledTestPersistenceTimeout)
	plans, err := repo.IntelligenceSchedules(queryCtx)
	cancel()
	if err != nil {
		return
	}
	for _, plan := range plans {
		if ctx.Err() != nil {
			return
		}
		if !plan.Enabled || plan.NextRunAt == nil || plan.NextRunAt.After(now) {
			continue
		}
		select {
		case accountIntelligenceSlots <- struct{}{}:
		default:
			return
		}
		runCtx, finish, err := s.beginRunContext(context.Background())
		if err != nil {
			<-accountIntelligenceSlots
			return
		}
		loc, err := time.LoadLocation(plan.Timezone)
		if err != nil {
			loc = s.accountTestSvc.intelligenceLocation()
		}
		cron, err := scheduledTestCronParser.Parse(plan.CronExpression)
		if err != nil {
			finish()
			<-accountIntelligenceSlots
			continue
		}
		next := cron.Next(now.In(loc))
		if next.IsZero() {
			finish()
			<-accountIntelligenceSlots
			continue
		}
		claimCtx, claimCancel := context.WithTimeout(ctx, scheduledTestPersistenceTimeout)
		claimed, err := repo.IntelligenceScheduleClaim(claimCtx, &plan, now, next)
		claimCancel()
		if err != nil || !claimed {
			finish()
			<-accountIntelligenceSlots
			continue
		}
		go func(p AccountIntelligenceSchedule) {
			defer finish()
			defer func() { <-accountIntelligenceSlots }()
			testCtx, cancel := context.WithTimeout(runCtx, accountIntelligenceTimeout)
			defer cancel()
			testCtx = context.WithValue(testCtx, accountIntelligenceObservationKey{}, true)
			testCtx = withAccountTestMaxOutputTokens(testCtx, 8192)
			// Deterministic for this claimed occurrence, even across process restarts.
			request := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("intelligence:%d:%d:%s", p.AccountID, p.Revision, p.NextRunAt.UTC().Format(time.RFC3339Nano)))).String()
			in := AccountIntelligenceInput{RequestID: request, Kind: p.Kind, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Prompt: p.Prompt}
			_, err := s.accountTestSvc.runScheduledAccountIntelligence(testCtx, p.AccountID, in)
			message := ""
			if err != nil {
				message = truncateIntelligenceText(err.Error(), 1000)
			}
			persist, cancel := context.WithTimeout(context.Background(), scheduledTestPersistenceTimeout)
			defer cancel()
			_ = repo.IntelligenceScheduleError(persist, &p, now, message)
			if current, e := repo.IntelligenceScheduleGet(persist, p.AccountID); e == nil && current != nil && current.Revision == p.Revision {
				_ = repo.IntelligencePruneScheduled(persist, p.AccountID, current.MaxResults)
			}
		}(plan)
	}
}

func (s *AccountTestService) runScheduledAccountIntelligence(ctx context.Context, id int64, in AccountIntelligenceInput) (*AccountIntelligenceTest, error) {
	if err := validateAccountIntelligence(&in); err != nil {
		return nil, err
	}
	a, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err = validateIntelligenceModel(a, in.Model); err != nil {
		return nil, err
	}
	repo, err := s.intelligenceRepo()
	if err != nil {
		return nil, err
	}
	if _, err = repo.IntelligenceFind(ctx, id, in.RequestID); err != nil {
		return nil, err
	}
	item := newAccountIntelligenceTest(id, in)
	item.Source = "scheduled"
	saved, inserted, err := repo.IntelligenceClaim(ctx, item)
	if err != nil || !inserted {
		return saved, err
	}
	func() {
		defer func() {
			if recover() != nil {
				item.Status = "error"
				item.Error = "检测执行中断，请重试。"
			}
		}()
		result, e := s.RunTestBackgroundWithPromptAndReasoning(ctx, id, in.Model, in.Prompt, in.ReasoningEffort)
		finishAccountIntelligenceResult(item, result, e)
	}()
	s.finishAccountIntelligence(repo, item)
	return item, nil
}
