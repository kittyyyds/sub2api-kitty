package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

type accountIntelligenceObservationKey struct{}

func accountIntelligenceObservation(ctx context.Context) bool {
	v, _ := ctx.Value(accountIntelligenceObservationKey{}).(bool)
	return v
}

var ErrAccountIntelligenceBusy = infraerrors.New(409, "ACCOUNT_INTELLIGENCE_BUSY", "此账号正在检测，或同时检测任务已满，请稍后重试。")
var accountIntelligenceSlots = make(chan struct{}, 2)

const accountIntelligenceTimeout = 5 * time.Minute

type AccountIntelligenceInput struct {
	RequestID       string `json:"request_id"`
	Kind            string `json:"kind"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Prompt          string `json:"prompt"`
}
type AccountIntelligenceTest struct {
	ID              string     `json:"id"`
	RequestID       string     `json:"request_id"`
	AccountID       int64      `json:"account_id"`
	Source          string     `json:"source,omitempty"`
	Kind            string     `json:"kind"`
	Model           string     `json:"model"`
	ReasoningEffort string     `json:"reasoning_effort"`
	Prompt          string     `json:"prompt"`
	ExpectedAnswer  string     `json:"expected_answer,omitempty"`
	ScoringVersion  string     `json:"scoring_version"`
	Status          string     `json:"status"`
	Response        string     `json:"response"`
	Error           string     `json:"error,omitempty"`
	UpstreamModel   string     `json:"upstream_model,omitempty"`
	LatencyMs       int64      `json:"latency_ms"`
	CreatedAt       time.Time  `json:"created_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}
type AccountIntelligenceRepository interface {
	IntelligenceFind(context.Context, int64, string) (*AccountIntelligenceTest, error)
	IntelligenceClaim(context.Context, *AccountIntelligenceTest) (*AccountIntelligenceTest, bool, error)
	IntelligenceFinish(context.Context, *AccountIntelligenceTest) error
	IntelligenceHistory(context.Context, []int64, int, int) ([]AccountIntelligenceTest, error)
}

func validateAccountIntelligence(in *AccountIntelligenceInput) error {
	in.Model = strings.TrimSpace(in.Model)
	in.ReasoningEffort = strings.ToLower(strings.TrimSpace(in.ReasoningEffort))
	if _, err := uuid.Parse(in.RequestID); err != nil {
		return infraerrors.BadRequest("INVALID_INTELLIGENCE_INPUT", "检测请求编号无效。")
	}
	if in.Kind != "candy" && in.Kind != "custom" {
		return infraerrors.BadRequest("INVALID_INTELLIGENCE_INPUT", "请选择糖果题或自定义题目。")
	}
	if in.Model == "" || len(in.Model) > 200 || strings.ContainsAny(in.Model, "\r\n") {
		return infraerrors.BadRequest("INVALID_INTELLIGENCE_INPUT", "请填写有效的文本模型。")
	}
	low := strings.ToLower(in.Model)
	for _, word := range []string{"image", "video", "tts", "whisper", "audio"} {
		if strings.Contains(low, word) {
			return infraerrors.BadRequest("INVALID_INTELLIGENCE_INPUT", "智商检测仅支持文本模型。")
		}
	}
	switch in.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return infraerrors.BadRequest("INVALID_INTELLIGENCE_INPUT", "思考强度无效。")
	}
	if in.Kind == "candy" {
		in.Prompt = CandyPrompt + "\n只输出最终整数，不要解释。"
	}
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 16000 {
		return infraerrors.BadRequest("INVALID_INTELLIGENCE_INPUT", "题目须为 1–16000 字节。")
	}
	return nil
}
func (s *AccountTestService) intelligenceRepo() (AccountIntelligenceRepository, error) {
	if s == nil {
		return nil, infraerrors.New(503, "INTELLIGENCE_UNAVAILABLE", "检测暂时不可用。")
	}
	repo, ok := s.accountRepo.(AccountIntelligenceRepository)
	if !ok {
		return nil, infraerrors.New(503, "INTELLIGENCE_UNAVAILABLE", "检测记录服务暂时不可用。")
	}
	return repo, nil
}
func (s *AccountTestService) StartAccountIntelligence(ctx context.Context, id int64, in AccountIntelligenceInput) (*AccountIntelligenceTest, error) {
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
	if previous, e := repo.IntelligenceFind(ctx, id, in.RequestID); e != nil {
		return nil, e
	} else if previous != nil {
		return previous, nil
	}
	select {
	case accountIntelligenceSlots <- struct{}{}:
	default:
		return nil, ErrAccountIntelligenceBusy
	}
	item := newAccountIntelligenceTest(id, in)
	saved, inserted, err := repo.IntelligenceClaim(ctx, item)
	if err != nil || !inserted {
		<-accountIntelligenceSlots
		return saved, err
	}
	go func() {
		defer func() { <-accountIntelligenceSlots }()
		runCtx, cancel := context.WithTimeout(context.Background(), accountIntelligenceTimeout)
		defer cancel()
		runCtx = context.WithValue(runCtx, accountIntelligenceObservationKey{}, true)
		runCtx = withAccountTestMaxOutputTokens(runCtx, 8192)
		defer func() {
			if recovered := recover(); recovered != nil {
				item.Status = "error"
				item.Error = "检测执行中断，请重试。"
				s.finishAccountIntelligence(repo, item)
			}
		}()
		result, e := s.RunTestBackgroundWithPromptAndReasoning(runCtx, id, in.Model, in.Prompt, in.ReasoningEffort)
		finishAccountIntelligenceResult(item, result, e)
		s.finishAccountIntelligence(repo, item)
	}()
	return saved, nil
}
func validateIntelligenceModel(a *Account, model string) error {
	if a.Platform != PlatformOpenAI && a.Platform != PlatformAnthropic {
		return infraerrors.BadRequest("INTELLIGENCE_UNSUPPORTED", "目前仅支持 OpenAI 和 Claude 文本账号。")
	}
	mapped := strings.ToLower(a.GetMappedModel(model))
	if a.Platform == PlatformOpenAI && isOpenAIImageModel(mapped) {
		return infraerrors.BadRequest("INTELLIGENCE_UNSUPPORTED", "此模型别名实际映射到图片模型，请选择文本模型。")
	}
	for _, word := range []string{"video", "audio", "tts", "whisper"} {
		if strings.Contains(mapped, word) {
			return infraerrors.BadRequest("INTELLIGENCE_UNSUPPORTED", "此模型别名实际映射到媒体模型，请选择文本模型。")
		}
	}
	return nil
}
func newAccountIntelligenceTest(id int64, in AccountIntelligenceInput) *AccountIntelligenceTest {
	item := &AccountIntelligenceTest{ID: uuid.NewString(), RequestID: in.RequestID, AccountID: id, Source: "manual", Kind: in.Kind, Model: in.Model, ReasoningEffort: in.ReasoningEffort, Prompt: in.Prompt, ScoringVersion: "candy-21-v1", Status: "running", CreatedAt: time.Now()}
	if in.Kind == "candy" {
		item.ExpectedAnswer = "21"
	} else {
		item.ScoringVersion = "unscored-v1"
	}
	return item
}
func finishAccountIntelligenceResult(item *AccountIntelligenceTest, result *ScheduledTestResult, err error) {
	item.Status = "error"
	if err != nil {
		item.Error = truncateIntelligenceText("检测未完成："+err.Error(), 4096)
		return
	}
	if result == nil {
		item.Error = "检测没有返回结果。"
		return
	}
	item.Response = truncateIntelligenceText(result.ResponseText, 65536)
	item.LatencyMs = result.LatencyMs
	item.UpstreamModel = result.UpstreamModel
	if result.Status != "success" || result.ErrorMessage != "" {
		item.Error = truncateIntelligenceText(result.ErrorMessage, 4096)
		if item.Error == "" {
			item.Error = "检测未成功完成。"
		}
		return
	}
	if strings.TrimSpace(result.ResponseText) == "" {
		item.Error = "模型返回空内容。"
		return
	}
	item.Status = "completed"
	if item.Kind == "candy" {
		item.Status = "incorrect"
		if CandyAnswerCorrect(result.ResponseText) {
			item.Status = "correct"
		}
	}
}
func truncateIntelligenceText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
func (s *AccountTestService) finishAccountIntelligence(repo AccountIntelligenceRepository, item *AccountIntelligenceTest) {
	now := time.Now()
	item.FinishedAt = &now
	if item.LatencyMs == 0 {
		item.LatencyMs = now.Sub(item.CreatedAt).Milliseconds()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := repo.IntelligenceFinish(ctx, item); err != nil { /* History marks abandoned running jobs after the execution deadline. */
	}
}
func (s *AccountTestService) AccountIntelligenceHistory(ctx context.Context, ids []int64, limit, offset int) ([]AccountIntelligenceTest, error) {
	repo, err := s.intelligenceRepo()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 || len(ids) > 100 {
		return nil, fmt.Errorf("invalid account count")
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 || offset > 100000 {
		return nil, errors.New("invalid history offset")
	}
	return repo.IntelligenceHistory(ctx, ids, limit, offset)
}
