package qqbot

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJoinReviewAnswer(t *testing.T) {
	rule := JoinReviewConfig{Enabled: true, MinSpent: 10, Question: "请输入邮箱", RejectReason: "本群只审核已消费用户"}
	for _, tc := range []struct {
		name, raw, email string
		valid            bool
	}{
		{"qa", `{"verify_info":{"method":"admin_review_qa","review_qa_list":[{"question":"请输入邮箱","answer":" User@Example.COM "},{"question":"其他问题","answer":"ignore@example.com"}]}}`, "user@example.com", true},
		{"message", `{"verify_info":{"method":"verify_message","verify_message":"user@example.com"}}`, "user@example.com", true},
		{"missing question", `{"verify_info":{"method":"admin_review_qa","review_qa_list":[{"question":"其他问题","answer":"user@example.com"}]}}`, "", false},
		{"ambiguous questions", `{"verify_info":{"method":"admin_review_qa","review_qa_list":[{"question":"请输入邮箱","answer":"user@example.com"},{"question":"请输入邮箱","answer":"other@example.com"}]}}`, "", false},
		{"multiple emails", `{"verify_info":{"method":"verify_message","verify_message":"user@example.com, other@example.com"}}`, "", false},
		{"display name", `{"verify_info":{"method":"verify_message","verify_message":"Alice <user@example.com>"}}`, "", false},
		{"header injection", `{"verify_info":{"method":"verify_message","verify_message":"user@example.com\r\nBcc:x@example.com"}}`, "", false},
		{"empty", `{}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var e Event
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &e))
			email, valid := joinReviewEmail(e, rule)
			require.Equal(t, tc.valid, valid)
			require.Equal(t, tc.email, email)
		})
	}
}

func TestJoinReviewValidationAndPolicy(t *testing.T) {
	rule := JoinReviewConfig{Enabled: true, MinSpent: 10, Question: "请输入邮箱", RejectReason: "本群只审核已消费用户"}
	require.NoError(t, validateJoinReview(nil))
	require.NoError(t, validateJoinReview(&rule))
	for _, n := range []float64{0, -1, math.NaN(), math.Inf(1), 10.001} {
		draft := rule
		draft.MinSpent = n
		require.Error(t, validateJoinReview(&draft))
	}
	c := DefaultConfig()
	c.AppID = "app"
	c.AllowedGroups = []string{"group"}
	c.GroupPolicies = []GroupPolicy{{GroupID: "group", JoinReview: &rule}}
	require.NoError(t, validateGroupPolicies(c))
	original := joinReviewPolicy(c, "group")
	c.Sandbox = true
	require.NotEqual(t, original, joinReviewPolicy(c, "group"))
	c.Sandbox = false
	rule.MinSpent = 11
	require.NotEqual(t, original, joinReviewPolicy(c, "group"))
	require.NotEqual(t, original, joinReviewPolicy(c, "another-group"))
}
