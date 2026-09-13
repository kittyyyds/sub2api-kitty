package service

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// CandyTestConfig contains no credentials. Group selection reuses the gateway.
// The administrator supplies the question; we deliberately do not invent one.
type CandyTestConfig struct {
	ID              int64    `json:"id"`
	GroupID         int64    `json:"group_id"`
	APIKeyID        int64    `json:"api_key_id"`
	Label           string   `json:"label"`
	Model           string   `json:"model"`
	ReasoningEffort string   `json:"reasoning_effort"`
	Question        string   `json:"question"`
	ExpectedAnswers []string `json:"expected_answers"`
	IntervalMinutes int      `json:"interval_minutes"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	Enabled         bool     `json:"enabled"`
	Published       bool     `json:"published"`
}

type CandyTestRun struct {
	ID              int64      `json:"id"`
	TestID          int64      `json:"test_id"`
	Status          string     `json:"status"`
	Question        string     `json:"question"`
	Model           string     `json:"model"`
	ResponseModel   string     `json:"response_model,omitempty"`
	ResponseID      string     `json:"response_id,omitempty"`
	ReasoningEffort string     `json:"reasoning_effort"`
	Output          string     `json:"output"`
	Answer          string     `json:"answer"`
	StandardAnswer  string     `json:"standard_answer,omitempty"`
	Message         string     `json:"message,omitempty"`
	InputTokens     *int       `json:"input_tokens"`
	OutputTokens    *int       `json:"output_tokens"`
	CacheReadTokens *int       `json:"cache_read_tokens"`
	LatencyMS       int64      `json:"latency_ms"`
	CreatedAt       time.Time  `json:"created_at"`
	FinishedAt      *time.Time `json:"finished_at"`
}

type CandyTestCard struct {
	GroupID         int64            `json:"-"`
	HistoryCount    int              `json:"history_count"`
	ID              int64            `json:"id"`
	Label           string           `json:"label"`
	Model           string           `json:"model"`
	ReasoningEffort string           `json:"reasoning_effort"`
	Enabled         bool             `json:"enabled"`
	IntervalMinutes int              `json:"interval_minutes"`
	NextRunAt       *time.Time       `json:"next_run_at"`
	History         []CandyTestRun   `json:"history"`
	Config          *CandyTestConfig `json:"config,omitempty"`
}

func normalizeCandyTestConfig(c CandyTestConfig) (CandyTestConfig, error) {
	c.Label = strings.TrimSpace(c.Label)
	c.Model = strings.TrimSpace(c.Model)
	c.Question = strings.TrimSpace(c.Question)
	c.ReasoningEffort = strings.ToLower(strings.TrimSpace(c.ReasoningEffort))
	bad := func(message string) (CandyTestConfig, error) {
		return c, infraerrors.BadRequest("CANDY_TEST_INVALID", message)
	}
	if c.GroupID <= 0 || c.ID < 0 || c.APIKeyID <= 0 {
		return bad("请选择有效分组和测试 API Key")
	}
	if c.Label == "" || utf8.RuneCountInString(c.Label) > 80 {
		return bad("展示名称须为 1–80 个字")
	}
	if c.Model == "" || len(c.Model) > 160 || strings.ContainsAny(c.Model, "\r\n\t") {
		return bad("请填写有效的模型名称")
	}
	if c.Question == "" || utf8.RuneCountInString(c.Question) > 8000 {
		return bad("题目须为 1–8000 个字")
	}
	switch c.ReasoningEffort {
	case "low", "medium", "high", "xhigh":
	default:
		return bad("思考强度须为 low、medium、high 或 xhigh")
	}
	if c.IntervalMinutes < 5 || c.IntervalMinutes > 1440 {
		return bad("测试周期须为 5–1440 分钟")
	}
	if c.MaxOutputTokens < 256 || c.MaxOutputTokens > 16384 {
		return bad("输出 Token 上限须为 256–16384")
	}
	if len(c.ExpectedAnswers) == 0 || len(c.ExpectedAnswers) > 10 {
		return bad("请填写 1–10 个可接受的最终答案")
	}
	answers := make([]string, 0, len(c.ExpectedAnswers))
	for _, answer := range c.ExpectedAnswers {
		answer = strings.TrimSpace(answer)
		if answer == "" || utf8.RuneCountInString(answer) > 500 {
			return bad("每个标准答案须为 1–500 个字")
		}
		answers = append(answers, answer)
	}
	c.ExpectedAnswers = answers
	return c, nil
}

// Only the question and an answer-format instruction go upstream. The answer
// key stays in the job snapshot and cannot accidentally become part of input.
func candyTestPayload(c CandyTestConfig) []byte {
	body, _ := json.Marshal(map[string]any{
		"model": c.Model, "stream": false, "store": false,
		"input":             c.Question,
		"instructions":      "请独立解答用户的题目。可以给出简短说明，并在最后单独一行写：最终答案：你的答案。",
		"reasoning":         map[string]string{"effort": c.ReasoningEffort},
		"max_output_tokens": c.MaxOutputTokens,
	})
	return body
}

// Compare the entire final answer, never a substring in the explanation.
// Ambiguous/missing markers need review, not an automatic wrong-answer verdict.
func judgeCandyAnswer(output string, expected []string) (status, answer string) {
	output = strings.TrimSpace(strings.ReplaceAll(output, "\r\n", "\n"))
	stripMarkup := func(v string) string {
		v = strings.TrimSpace(v)
		for _, marker := range []string{"**", "`"} {
			if len(v) > 2*len(marker) && strings.HasPrefix(v, marker) && strings.HasSuffix(v, marker) {
				v = strings.TrimSpace(v[len(marker) : len(v)-len(marker)])
			}
		}
		return v
	}
	normalize := func(v string) string {
		return stripMarkup(strings.TrimSuffix(strings.TrimSpace(v), "。"))
	}
	for _, line := range strings.Split(output, "\n") {
		line = stripMarkup(line)
		for _, prefix := range []string{"最终答案：", "最终答案:"} {
			if strings.HasPrefix(line, prefix) {
				candidate := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				if answer != "" && normalize(answer) != normalize(candidate) {
					return "inconclusive", ""
				}
				answer = candidate
			}
		}
	}
	if answer == "" {
		// A bare exact answer is also accepted; longer free-form output is not
		// guessed to be an answer merely because it contains the right number.
		for _, candidate := range expected {
			if normalize(output) == normalize(candidate) && output != "" {
				return "passed", output
			}
		}
		return "inconclusive", ""
	}
	for _, candidate := range expected {
		if normalize(answer) == normalize(candidate) {
			return "passed", answer
		}
	}
	return "failed", answer
}
