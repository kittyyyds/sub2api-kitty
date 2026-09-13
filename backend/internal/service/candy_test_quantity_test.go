package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCandyFinalQuantityIgnoresNotesButNotDifferentAnswers(t *testing.T) {
	for _, tt := range []struct{ output, status, answer string }{
		{"最终答案：21个（9个圆形、12个五角星形）。", "passed", "21"},
		{"最终答案：29个娱乐主管", "failed", "29"},
		{"最终答案：**21** 个（9个圆形、12个五角星形）。", "passed", "21"},
		{"最终答案：21（9+12）", "passed", "21"},
		{"21 个", "passed", "21"},
		{"最终答案：121个", "failed", "121"},
		{"试过21但修正为29。\n最终答案：29个", "failed", "29"},
		{"最终答案：21\n最终答案：21个", "passed", "21"},
		{"最终答案：21\n最终答案：29", "inconclusive", ""},
		{"最终答案：21个或29个", "inconclusive", ""},
		{"最终答案：21个（其实是29个）", "inconclusive", ""},
		{"最终答案：21个不是正确答案", "inconclusive", ""},
		{"最终答案：21.5", "inconclusive", ""},
		{"最终答案：21/2", "inconclusive", ""},
		{"最终答案：21e3", "inconclusive", ""},
		{"最终答案：21-29", "inconclusive", ""},
		{"最终答案：21至29个", "inconclusive", ""},
		{"最终答案：21%", "inconclusive", ""},
		{"最终答案：21，29", "inconclusive", ""},
		{"最终答案：21,000", "inconclusive", ""},
		{"最终答案：21个（9个圆形", "inconclusive", ""},
		{"答案可能是21，也可能是29", "inconclusive", ""},
		{"21个只是计算中的一个数字\n没有最终答案", "inconclusive", ""},
		{"", "inconclusive", ""},
	} {
		t.Run(tt.output, func(t *testing.T) {
			status, answer := judgeCandyAnswer(tt.output, []string{"21", "21个", "21 颗糖果"})
			require.Equal(t, tt.status, status)
			require.Equal(t, tt.answer, answer)
		})
	}
}

func TestCandyHistoryRejudgesSavedAnswerWithoutWritingOrNewRequests(t *testing.T) {
	for _, storedStatus := range []string{"failed", "passed", "inconclusive", "error", "running", "cancelled"} {
		t.Run(storedStatus, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			c := candyConfigFixture()
			c.ExpectedAnswers = []string{"21", "21个"}
			snapshot, _ := json.Marshal(c)
			original := "因为形状可选择。\n最终答案：21个（9个圆形、12个五角星形）。"
			oldAnswer := "21个（9个圆形、12个五角星形）。"
			stored, _ := json.Marshal(CandyTestRun{Status: storedStatus, Output: original, Answer: oldAnswer})
			created := time.Now().Add(-time.Hour)
			finished := created.Add(time.Minute)
			mock.ExpectQuery("SELECT id,status,snapshot,result").WithArgs(int64(2), 10, true).
				WillReturnRows(sqlmock.NewRows([]string{"id", "status", "snapshot", "result", "created", "finished"}).
					AddRow(42, storedStatus, string(snapshot), string(stored), created, finished))
			// No configured endpoint or Key repository: rejudging must need only
			// the immutable response and its original snapshot, and only SELECT.
			s := &CandyTestService{db: db}
			runs, err := s.history(context.Background(), 2, 10, true)
			require.NoError(t, err)
			require.Len(t, runs, 1)
			r := runs[0]
			require.Equal(t, original, r.Output)
			require.Equal(t, "21", r.StandardAnswer)
			require.Equal(t, created, r.CreatedAt)
			require.Equal(t, finished, *r.FinishedAt)
			if storedStatus == "failed" || storedStatus == "passed" {
				require.Equal(t, "passed", r.Status)
				require.Equal(t, "21", r.Answer)
				require.Contains(t, r.Message, "未重新调用模型")
			} else {
				require.Equal(t, storedStatus, r.Status)
				require.Equal(t, oldAnswer, r.Answer)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
