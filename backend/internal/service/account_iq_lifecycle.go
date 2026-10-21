package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// IQBootstrap applies only to future, server-verified OpenAI 2FA imports.
type IQBootstrap struct {
	Enabled           bool   `json:"enabled"`
	Model             string `json:"model"`
	Reasoning         string `json:"reasoning"`
	IntervalSeconds   int    `json:"interval_seconds"`
	CorrectRequired   int    `json:"correct_required"`
	PassedConcurrency int    `json:"passed_concurrency"`
	QuarantineEnabled bool   `json:"quarantine_enabled"`
	QuarantineGroupID int64  `json:"quarantine_group_id"`
}

func DefaultIQBootstrap() IQBootstrap {
	return IQBootstrap{Model: "gpt-6.1-sol", Reasoning: "low", IntervalSeconds: 60, CorrectRequired: 6, PassedConcurrency: 2}
}
func (b IQBootstrap) Validate() error {
	if !b.Enabled {
		return nil
	}
	if strings.TrimSpace(b.Model) == "" || len(b.Model) > 200 || b.IntervalSeconds < 1 || b.IntervalSeconds > 86400 || b.CorrectRequired < 1 || b.CorrectRequired > 1000 || b.PassedConcurrency < 2 || b.PassedConcurrency > 10000 {
		return fmt.Errorf("糖果验证模型、间隔、连续正确次数或通过后并发无效")
	}
	switch b.Reasoning {
	case "low", "minimal", "medium", "high", "xhigh", "none":
	default:
		return fmt.Errorf("糖果验证思考强度无效")
	}
	if b.QuarantineEnabled && b.QuarantineGroupID <= 0 {
		return fmt.Errorf("请为连续回答 29 隔离规则选择目标分组")
	}
	return nil
}
func (b IQBootstrap) Plan() *ScheduledTestPlan {
	next := time.Now().UTC().Add(time.Duration(b.IntervalSeconds) * time.Second)
	return &ScheduledTestPlan{ModelID: b.Model, CronExpression: "* * * * *", Enabled: true, MaxResults: 20, NextRunAt: &next,
		PelicanConfig: &PelicanTestConfig{ScheduleMode: IQCompletionInterval, IntervalSeconds: b.IntervalSeconds, QuestionKind: "candy", Prompt: CandyPrompt, ReasoningEffort: b.Reasoning, ParallelCount: 1, TestChannel: "account"}}
}

type IQEvidenceSample struct {
	ResultID  int64     `json:"result_id"`
	StartedAt time.Time `json:"started_at"`
	Answer    string    `json:"answer"`
}
type IQLifecycleState struct {
	QuarantinePending bool               `json:"quarantine_pending"`
	Samples           []IQEvidenceSample `json:"-"`
	Config            IQBootstrap        `json:"config"`
	CorrectStreak     int                `json:"correct_streak"`
	Answer29Streak    int                `json:"answer29_streak"`
	PassedAt          *time.Time         `json:"passed_at,omitempty"`
	QuarantinedAt     *time.Time         `json:"quarantined_at,omitempty"`
	ActionError       string             `json:"action_error,omitempty"`
	Fingerprint       string             `json:"-"`
	LastStartedAt     *time.Time         `json:"-"`
}

func IQConfigFingerprint(model string, cfg *PelicanTestConfig) string {
	if cfg == nil {
		return ""
	}
	b, _ := json.Marshal([]string{model, cfg.ReasoningEffort, cfg.QuestionKind, cfg.Prompt, cfg.TestChannel})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Only complete answers count. Local admission waiting is not a test attempt.
func IQFinalAnswer(r *ScheduledTestResult) string {
	if r == nil || r.Status == "skipped" {
		return "waiting"
	}
	if r.Status != "success" && strings.TrimSpace(r.ErrorMessage) != "answer_mismatch: expected 21" {
		return "failed"
	}
	if CandyAnswerCorrect(r.ResponseText) {
		return "21"
	}
	if CandyAnswerMatches(r.ResponseText, 29) {
		return "29"
	}
	return "other"
}

// The repository owns locking, deduplication and the resulting account mutations.
func AdvanceIQLifecycle(s IQLifecycleState, result *ScheduledTestResult) IQLifecycleState {
	if IQFinalAnswer(result) == "waiting" {
		return s
	}
	if s.LastStartedAt != nil && !result.StartedAt.After(*s.LastStartedAt) {
		return s
	}
	if result.PelicanConfig == nil || result.PelicanConfig.QuestionKind != "candy" || result.PelicanConfig.TestChannel != "account" {
		return s
	}
	fp := IQConfigFingerprint(result.PelicanConfig.ModelID, result.PelicanConfig)
	if s.Fingerprint != fp {
		s.CorrectStreak = 0
		s.Answer29Streak = 0
		s.Fingerprint = fp
	}
	t := result.StartedAt
	s.LastStartedAt = &t
	switch IQFinalAnswer(result) {
	case "21":
		s.Answer29Streak = 0
		if s.CorrectStreak < s.Config.CorrectRequired {
			s.CorrectStreak++
		}
	case "29":
		s.CorrectStreak = 0
		if s.Answer29Streak < 6 {
			s.Answer29Streak++
		}
	default:
		s.CorrectStreak = 0
		s.Answer29Streak = 0
	}
	return s
}
