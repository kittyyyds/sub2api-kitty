package service

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIQLifecycleSequentialAdmission(t *testing.T) {
	s := IQLifecycleState{Config: DefaultIQBootstrap()}
	next := time.Now().UTC()
	cfg := s.Config.Plan().PelicanConfig
	cfg.ModelID = s.Config.Model
	result := func(status, answer, message string) *ScheduledTestResult {
		next = next.Add(time.Second)
		return &ScheduledTestResult{StartedAt: next, Status: status, ResponseText: answer, ErrorMessage: message, PelicanConfig: cfg}
	}
	for i := 1; i <= 5; i++ {
		s = AdvanceIQLifecycle(s, result("success", "21", ""))
		require.Equal(t, i, s.CorrectStreak)
		require.Nil(t, s.PassedAt)
	}
	s = AdvanceIQLifecycle(s, result("skipped", "", "capacity"))
	require.Equal(t, 5, s.CorrectStreak)
	sixth := result("success", "21", "")
	s = AdvanceIQLifecycle(s, sixth)
	require.Equal(t, 6, s.CorrectStreak)
	s = AdvanceIQLifecycle(s, sixth)
	require.Equal(t, 6, s.CorrectStreak)
	s = AdvanceIQLifecycle(s, result("failed", "21", "stream disconnected"))
	require.Zero(t, s.CorrectStreak)
	for i := 1; i <= 6; i++ {
		s = AdvanceIQLifecycle(s, result("failed", "29", "answer_mismatch: expected 21"))
		require.Equal(t, i, s.Answer29Streak)
	}
	s = AdvanceIQLifecycle(s, result("success", "21或29", ""))
	require.Zero(t, s.Answer29Streak)
	require.Zero(t, s.CorrectStreak)
	for i := 0; i < 5; i++ {
		s = AdvanceIQLifecycle(s, result("success", "21", ""))
	}
	cfg.ModelID = "another-model"
	s = AdvanceIQLifecycle(s, result("success", "21", ""))
	require.Equal(t, 1, s.CorrectStreak)
}

func TestIQFinalAnswerRequiresCompleteSingleAnswer(t *testing.T) {
	for _, tt := range []struct{ status, answer, err, want string }{
		{"success", "21", "", "21"}, {"failed", "29", "answer_mismatch: expected 21", "29"},
		{"failed", "29", "network error", "failed"}, {"success", "129", "", "other"},
		{"success", "答案为29。", "", "29"}, {"success", "21 or 29", "", "other"},
		{"failed", "", "401", "failed"}, {"skipped", "", "wait", "waiting"},
	} {
		t.Run(tt.status+tt.answer+tt.err, func(t *testing.T) {
			require.Equal(t, tt.want, IQFinalAnswer(&ScheduledTestResult{Status: tt.status, ResponseText: tt.answer, ErrorMessage: tt.err}))
		})
	}
}
