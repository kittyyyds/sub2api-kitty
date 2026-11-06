//go:build integration

package qqbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJoinReviewIntegration(t *testing.T) {
	db := testDB(t)
	s := savedBot(t, db)
	c, err := s.Config(context.Background())
	require.NoError(t, err)
	rule := JoinReviewConfig{Enabled: true, MinSpent: 10, Question: "请输入邮箱", RejectReason: "本群只审核已消费用户"}
	c.GroupPolicies = []GroupPolicy{{GroupID: "group", JoinReview: &rule}}
	c, err = s.SaveConfig(context.Background(), c)
	require.NoError(t, err)
	var mu sync.Mutex
	calls := []map[string]any{}
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":7200}`))
			return
		}
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v2/groups/group/approval_join_request/member", r.URL.Path)
		require.Equal(t, "QQBot test-token", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, body)
		if fail {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"code":11253}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	s.tokenURL = server.URL + "/token"
	s.qqURL = server.URL
	for i, tc := range []struct {
		name, email, spending, op string
		inactive, deleted         bool
	}{
		{"exact threshold", "exact@example.com", "10", "approve", false, false},
		{"below threshold", "below@example.com", "9.99", "decline", false, false},
		{"no consumption", "unused@example.com", "0", "decline", false, false},
		{"missing account", "missing@example.com", "", "decline", false, false},
		{"inactive", "inactive@example.com", "100", "decline", true, false},
		{"deleted", "deleted@example.com", "100", "decline", false, true},
		{"invalid answer", "not-an-email", "", "decline", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.spending != "" {
				var uid int64
				require.NoError(t, db.QueryRow(`INSERT INTO users(email,balance) VALUES($1,999) RETURNING id`, tc.email).Scan(&uid))
				if tc.inactive {
					_, err = db.Exec(`UPDATE users SET status='disabled' WHERE id=$1`, uid)
					require.NoError(t, err)
				}
				if tc.deleted {
					_, err = db.Exec(`UPDATE users SET deleted_at=NOW() WHERE id=$1`, uid)
					require.NoError(t, err)
				}
				_, err = db.Exec(`INSERT INTO kitty_permanent_usage(user_id,actual_cost,observed_at) VALUES($1,$2::numeric,NOW())`, uid, tc.spending)
				require.NoError(t, err)
			}
			e := Event{GroupID: "group", MemberID: "member", ApplySource: "self_apply", ApplyAt: time.Now().Format(time.RFC3339), JoinRequestID: fmt.Sprintf("request-%d", i), VerifyInfo: JoinVerifyInfo{Method: "verify_message", VerifyMessage: tc.email}}
			raw, err := json.Marshal(e)
			require.NoError(t, err)
			p := Payload{ID: fmt.Sprintf("event-%d", i), Type: "GROUP_JOIN_REQUEST", Data: raw}
			require.NoError(t, s.Enqueue(context.Background(), c, p))
			s.runOne(context.Background())
			mu.Lock()
			require.Len(t, calls, i+1)
			body := calls[len(calls)-1]
			mu.Unlock()
			require.Equal(t, tc.op, body["op"])
			require.Equal(t, e.JoinRequestID, body["join_request_id"])
			if tc.op == "decline" {
				require.Equal(t, rule.RejectReason, body["reject_reason"])
				require.Equal(t, false, body["add_to_member_blacklist"])
			}
			// A repeated signed delivery with a different event ID still cannot
			// submit the same join request twice, or create an account binding.
			p.ID += "-redelivered"
			require.NoError(t, s.Enqueue(context.Background(), c, p))
			s.runOne(context.Background())
			mu.Lock()
			require.Len(t, calls, i+1)
			mu.Unlock()
			var bindings, rewards int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qq_bot_bindings`).Scan(&bindings))
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qq_bot_rewards`).Scan(&rewards))
			require.Zero(t, bindings)
			require.Zero(t, rewards)
		})
	}
	// Missing verification metadata must not turn into a rejection. A rule
	// changed while queued must not authorize either approval or rejection.
	for i, mode := range []string{"missing metadata", "changed config", "permission denied"} {
		e := Event{GroupID: "group", MemberID: "member", ApplySource: "self_apply", ApplyAt: time.Now().Format(time.RFC3339), JoinRequestID: fmt.Sprintf("extra-%d", i), VerifyInfo: JoinVerifyInfo{Method: "verify_message", VerifyMessage: "exact@example.com"}}
		if i == 0 {
			e.VerifyInfo = JoinVerifyInfo{}
		}
		raw, _ := json.Marshal(e)
		require.NoError(t, s.Enqueue(context.Background(), c, Payload{ID: mode, Type: "GROUP_JOIN_REQUEST", Data: raw}))
		if i == 1 {
			draft := c
			v := rule
			v.MinSpent = 11
			draft.GroupPolicies = []GroupPolicy{{GroupID: "group", JoinReview: &v}}
			_, err = s.SaveConfig(context.Background(), draft)
			require.NoError(t, err)
		}
		if i == 2 {
			mu.Lock()
			fail = true
			mu.Unlock()
		}
		s.runOne(context.Background())
		if i == 1 {
			c, err = s.SaveConfig(context.Background(), c)
			require.NoError(t, err)
		}
	}
	mu.Lock()
	require.Len(t, calls, 8)
	mu.Unlock()
	var status, reason string
	require.NoError(t, db.QueryRow(`SELECT status,decision_reason FROM qq_bot_jobs WHERE decision_reason LIKE 'manual_qq_http%'`).Scan(&status, &reason))
	require.Equal(t, "failed", status)
	require.Equal(t, "manual_qq_http_403_code_11253", reason)
}
