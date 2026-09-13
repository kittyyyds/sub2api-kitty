package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func candyConfigFixture() CandyTestConfig {
	return CandyTestConfig{GroupID: 7, APIKeyID: 8, Label: "测试分组", Model: "test-model", ReasoningEffort: "xhigh", Question: "仅用于测试的题目", ExpectedAnswers: []string{".5"}, IntervalMinutes: 25, MaxOutputTokens: 4096}
}

func TestCandyAnswerDoesNotMatchAnIncidentalNumber(t *testing.T) {
	tests := []struct{ name, output, status string }{
		{"bare answer", ".5", "passed"},
		{"final line", "计算过程\n最终答案：.5", "passed"},
		{"bold final line", "**最终答案：.5**", "passed"},
		{"bold value", "最终答案：**.5**", "passed"},
		{"wrong bold value", "最终答案：**5**", "failed"},
		{"decimal is significant", "最终答案：5", "failed"},
		{"wrong final despite expected in reasoning", "先试 .5，推算后改变了。\n最终答案：6", "failed"},
		{"missing marker", "答案可能是 .5，也可能是 5", "inconclusive"},
		{"conflicting answers", "最终答案：.5\n最终答案：6", "inconclusive"},
		{"empty", "", "inconclusive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := judgeCandyAnswer(tt.output, []string{".5"})
			require.Equal(t, tt.status, status)
		})
	}
}

func TestCandyQuestionPayloadNeverContainsAnswerKey(t *testing.T) {
	c := candyConfigFixture()
	c.ExpectedAnswers = []string{"secret-answer-fixture"}
	body := candyTestPayload(c)
	require.Equal(t, c.Question, gjson.GetBytes(body, "input").String())
	require.Equal(t, c.Model, gjson.GetBytes(body, "model").String())
	require.Equal(t, "xhigh", gjson.GetBytes(body, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(body, "stream").Bool())
	require.NotContains(t, string(body), c.ExpectedAnswers[0])
	require.NotContains(t, string(body), "api_key")
}

func candyResponse(status, text string) string {
	data, _ := json.Marshal(map[string]any{"id": "resp_fixture", "status": status, "model": "test-model", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": text}}}}, "usage": map[string]any{"input_tokens": 25, "output_tokens": 30, "input_tokens_details": map[string]any{"cached_tokens": 0}}})
	return string(data)
}

func TestCandyRequiresACompleteResponseBeforeJudging(t *testing.T) {
	tests := []struct{ name, body, contentType, want string }{
		{"complete", candyResponse("completed", "最终答案：.5"), "application/json", "passed"},
		{"incomplete not wrong", candyResponse("incomplete", "最终答案：6"), "application/json", "inconclusive"},
		{"failed despite correct text", candyResponse("failed", "最终答案：.5"), "application/json", "error"},
		{"missing status", candyResponse("", "最终答案：.5"), "application/json", "error"},
		{"empty text", candyResponse("completed", ""), "application/json", "inconclusive"},
		{"done is not completion", "data: [DONE]\n\n", "text/event-stream", "error"},
		{"sse complete", "data: {\"type\":\"response.completed\",\"response\":" + candyResponse("completed", "最终答案：.5") + "}\n\ndata: [DONE]\n\n", "text/event-stream", "passed"},
		{"malformed", "not-json", "application/json", "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCandyTestResponse([]byte(tt.body), tt.contentType, []string{".5"})
			require.Equal(t, tt.want, result.Status)
		})
	}
	run := parseCandyTestResponse([]byte(candyResponse("completed", "最终答案：.5")), "application/json", []string{".5"})
	require.NotNil(t, run.InputTokens)
	require.Equal(t, 25, *run.InputTokens)
	require.NotNil(t, run.CacheReadTokens)
	require.Zero(t, *run.CacheReadTokens)
	run = parseCandyTestResponse([]byte(`{"status":"completed","output":[]}`), "application/json", []string{".5"})
	require.Nil(t, run.InputTokens)
}

func TestCandyConfigRejectsInvalidOrExpensiveAccidentalInputs(t *testing.T) {
	c := candyConfigFixture()
	normalized, err := normalizeCandyTestConfig(c)
	require.NoError(t, err)
	require.False(t, normalized.Enabled)
	require.False(t, normalized.Published)
	for name, change := range map[string]func(*CandyTestConfig){
		"missing question": func(c *CandyTestConfig) { c.Question = "" },
		"missing answers":  func(c *CandyTestConfig) { c.ExpectedAnswers = nil },
		"missing key":      func(c *CandyTestConfig) { c.APIKeyID = 0 },
		"too frequent":     func(c *CandyTestConfig) { c.IntervalMinutes = 1 },
		"unbounded output": func(c *CandyTestConfig) { c.MaxOutputTokens = 999999 },
	} {
		t.Run(name, func(t *testing.T) {
			value := candyConfigFixture()
			change(&value)
			_, err := normalizeCandyTestConfig(value)
			require.Error(t, err)
		})
	}
}

type candyKeyRepo struct {
	APIKeyRepository
	key *APIKey
}

func (r candyKeyRepo) GetByID(context.Context, int64) (*APIKey, error) { return r.key, nil }

type candyGroupRepo struct {
	GroupRepository
	group *Group
}

func (r candyGroupRepo) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }

func TestCandyUsesNormalGroupAPIAndRedactsCredentialEcho(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c := candyConfigFixture()
	key := &APIKey{ID: 8, UserID: 3, GroupID: &c.GroupID, Key: "fixture-not-a-real-credential", Status: StatusActive}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/responses", r.URL.Path)
		require.Equal(t, "Bearer "+key.Key, r.Header.Get("Authorization"))
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		require.Equal(t, c.Question, gjson.GetBytes(body, "input").String())
		require.NotContains(t, string(body), "expected_answers")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, candyResponse("completed", key.Key+"\n最终答案：.5"))
	}))
	defer server.Close()
	s := NewCandyTestService(db, candyGroupRepo{group: &Group{ID: 7, Platform: PlatformOpenAI, Status: StatusActive}}, candyKeyRepo{key: key}, &config.Config{})
	defer s.Stop()
	s.endpoint = server.URL + "/v1/responses"
	run := s.execute(context.Background(), c)
	require.Equal(t, 1, requests)
	require.Equal(t, "passed", run.Status)
	require.NotContains(t, run.Output, key.Key)
	require.Contains(t, run.Output, "[已脱敏]")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCandyCustomerHistoryCannotReadUnpublishedTest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(9), false).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	s := &CandyTestService{db: db}
	_, err = s.History(context.Background(), 9, false)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCandyCustomerRefreshIsReadOnlyAndContainsNoPrivateConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c := candyConfigFixture()
	c.Published = true
	c.ExpectedAnswers = []string{"21", "private-alternate"}
	snapshot, _ := json.Marshal(c)
	mock.ExpectQuery("SELECT t.id,t.config").WithArgs(false).WillReturnRows(sqlmock.NewRows([]string{"id", "config", "next", "count"}).AddRow(1, string(snapshot), nil, 1))
	mock.ExpectQuery("SELECT id,status,snapshot,result").WithArgs(int64(1), 1, false).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "snapshot", "result", "created", "finished"}).AddRow(2, "passed", string(snapshot), `{"output":"hello"}`, time.Now(), time.Now()))
	s := &CandyTestService{db: db}
	cards, err := s.List(context.Background(), false)
	require.NoError(t, err)
	data, err := json.Marshal(cards)
	require.NoError(t, err)
	require.Equal(t, "21", cards[0].History[0].StandardAnswer)
	for _, sensitive := range []string{"private-alternate", "expected_answers", "api_key_id", "account_id", "\"config\"", "\"group_id\""} {
		require.NotContains(t, string(data), sensitive)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCandyRejectsCustomerOwnedOrReboundKey(t *testing.T) {
	for _, rebound := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		c := candyConfigFixture()
		groupID := c.GroupID
		if rebound {
			groupID++
		}
		key := &APIKey{ID: 8, UserID: 3, GroupID: &groupID, Status: StatusActive}
		if !rebound {
			mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		}
		s := &CandyTestService{db: db, keys: candyKeyRepo{key: key}}
		_, err = s.testKey(context.Background(), c)
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

func TestCandyRunClickDeduplicatesAnExistingPendingJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT config").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"config"}).AddRow(`{}`))
	mock.ExpectQuery("INSERT INTO kitty_candy_test_runs").WithArgs(int64(1), "{}").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT id FROM kitty_candy_test_runs").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	mock.ExpectExec("UPDATE kitty_candy_tests").WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	s := &CandyTestService{db: db}
	id, err := s.RunNow(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, int64(41), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCandyDisablesProxyAndRedirectsForInternalCredentials(t *testing.T) {
	s := NewCandyTestService(nil, nil, nil, &config.Config{})
	defer s.Stop()
	require.Nil(t, s.client.Transport.(*http.Transport).Proxy)
	require.ErrorIs(t, s.client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	require.True(t, strings.HasPrefix(s.endpoint, "http://127.0.0.1:"))
}

func TestCandyCanDisableScheduleEvenWhenKeyIsNoLongerUsable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c := candyConfigFixture()
	c.ID = 1
	c.Enabled = false
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT group_id").WithArgs(c.ID).WillReturnRows(sqlmock.NewRows([]string{"group_id"}).AddRow(c.GroupID))
	mock.ExpectExec("UPDATE kitty_candy_tests SET config").WithArgs(c.ID, sqlmock.AnyArg(), false, false, 25).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kitty_candy_test_runs SET status='cancelled'").WithArgs(c.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM kitty_candy_test_runs").WithArgs(c.ID, 10).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	// No credential repository is available: disabling must not require one.
	s := &CandyTestService{db: db}
	saved, err := s.Save(context.Background(), c)
	require.NoError(t, err)
	require.False(t, saved.Enabled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCandyOversizedOutputPreservesUsageAndNeverJudgesTruncation(t *testing.T) {
	text := strings.Repeat("糖", 30000) + "\n最终答案：.5"
	run := parseCandyTestResponse([]byte(candyResponse("completed", text)), "application/json", []string{".5"})
	require.Equal(t, "inconclusive", run.Status)
	require.LessOrEqual(t, len(run.Output), 64<<10)
	require.NotNil(t, run.OutputTokens)
	require.Equal(t, 30, *run.OutputTokens)
	encoded, err := json.Marshal(run)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "\\ufffd")
}
