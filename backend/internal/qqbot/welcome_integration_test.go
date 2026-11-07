//go:build integration

package qqbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWelcomeIntegration(t *testing.T) {
	for _, mode := range []string{"reviewed", "invited", "no review", "already bound", "account conflict", "member conflict", "SMTP failed", "QQ denied", "QQ empty result", "email quota", "changed email", "changed policy", "disabled while queued", "disabled during SMTP", "stale", "newer invitation", "mail disabled", "forged receipt"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s, m, c, _, uid := emailFixture(t)
			c.InviteEnabled, c.ChatDisabled = false, true
			rule := &JoinReviewConfig{Enabled: true, MinSpent: 10, Question: "请输入邮箱", RejectReason: "本群只审核已消费用户"}
			w := &WelcomeConfig{Enabled: true, AutoEmailBinding: true, ReviewedText: "审核欢迎", OtherText: "普通欢迎"}
			if mode == "mail disabled" {
				w.AutoEmailBinding = false
			}
			c.GroupPolicies = []GroupPolicy{{GroupID: "group", JoinReview: rule, Welcome: w}}
			var err error
			c, err = s.SaveConfig(ctx, c)
			require.NoError(t, err)
			_, err = s.db.Exec(`INSERT INTO kitty_permanent_usage(user_id,actual_cost,observed_at) VALUES($1,10,NOW())`, uid)
			require.NoError(t, err)
			var mu sync.Mutex
			var approvals, messages int
			var sentText string
			qq := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					_, _ = rw.Write([]byte(`{"access_token":"test","expires_in":7200}`))
					return
				}
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				mu.Lock()
				defer mu.Unlock()
				if strings.Contains(r.URL.Path, "approval_join_request") {
					approvals++
					require.Equal(t, "approve", body["op"])
				} else {
					messages++
					require.NotContains(t, body, "msg_id")
					require.NotContains(t, body, "event_id")
					sentText = body["markdown"].(map[string]any)["content"].(string)
					if mode == "QQ denied" {
						rw.WriteHeader(403)
						_, _ = rw.Write([]byte(`{"code":11253}`))
						return
					}
					if mode == "QQ empty result" {
						_, _ = rw.Write([]byte(`{}`))
						return
					}
				}
				_, _ = rw.Write([]byte(`{"id":"sent"}`))
			}))
			defer qq.Close()
			s.qqURL, s.tokenURL = qq.URL, qq.URL+"/token"
			now := time.Now().Truncate(time.Second)
			add := func(id, kind string, e Event) {
				raw, _ := json.Marshal(e)
				if mode == "forged receipt" {
					var value map[string]any
					require.NoError(t, json.Unmarshal(raw, &value))
					value["_kitty_approved"] = approvedJoin{UserID: uid, EmailHash: codeHash("Alice@example.com"), Policy: welcomePolicy(c, "group"), Applied: now.Add(-20 * time.Second)}
					raw, _ = json.Marshal(value)
				}
				require.NoError(t, s.Enqueue(ctx, c, Payload{ID: id, Type: kind, Data: raw}))
			}
			join := Event{GroupID: "group", MemberID: "member", ApplySource: "self_apply", ApplyAt: now.Add(-20 * time.Second).Format(time.RFC3339), JoinRequestID: "join", VerifyInfo: JoinVerifyInfo{Method: "verify_message", VerifyMessage: "alice@example.com"}}
			if mode == "invited" {
				join.ApplySource = "invited"
			}
			if mode != "no review" && mode != "forged receipt" {
				add("request", "GROUP_JOIN_REQUEST", join)
				s.runOne(ctx)
			}
			if mode == "newer invitation" {
				join.ApplySource, join.JoinRequestID = "invited", "invited-later"
				add("invitation", "GROUP_JOIN_REQUEST", join)
				s.runOne(ctx)
			}
			if mode == "already bound" || mode == "account conflict" || mode == "member conflict" {
				member, account := "member", uid
				if mode == "account conflict" {
					member = "another-member"
				}
				if mode == "member conflict" {
					account = user(t, s.db)
				}
				_, err = s.db.Exec(`INSERT INTO qq_bot_bindings(app_id,group_id,member_id,user_id) VALUES('app','group',$1,$2)`, member, account)
				require.NoError(t, err)
			}
			if mode == "SMTP failed" {
				m.fail = true
			}
			if mode == "disabled during SMTP" {
				m.beforeSend = func() {
					latest, configErr := s.Config(ctx)
					require.NoError(t, configErr)
					latest.GroupPolicies[0].Welcome.Enabled = false
					_, configErr = s.SaveConfig(ctx, latest)
					require.NoError(t, configErr)
				}
			}
			if mode == "changed email" {
				_, err = s.db.Exec(`UPDATE users SET email='new@example.com' WHERE id=$1`, uid)
				require.NoError(t, err)
			}
			if mode == "email quota" {
				require.NoError(t, s.reserveBindingEmails(ctx, "app", []string{"member:member"}, []int{5}, []int{60}))
			}
			if mode == "changed policy" {
				w.ReviewedText = "changed"
				c, err = s.SaveConfig(ctx, c)
				require.NoError(t, err)
			}
			e := Event{GroupID: "group", MemberID: "member", Timestamp: json.RawMessage(strconv.FormatInt(now.Unix(), 10))}
			if mode == "stale" {
				e.Timestamp = json.RawMessage(strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10))
			}
			add("member-add", "GROUP_MEMBER_ADD", e)
			if mode == "disabled while queued" {
				w.Enabled = false
				c, err = s.SaveConfig(ctx, c)
				require.NoError(t, err)
			}
			s.runOne(ctx)
			// A different delivery ID must not send another email or message.
			add("member-add-duplicate", "GROUP_MEMBER_ADD", e)
			s.runOne(ctx)
			var bindings, rewards int
			require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM qq_bot_bindings`).Scan(&bindings))
			require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM qq_bot_rewards`).Scan(&rewards))
			require.Zero(t, rewards)
			if strings.Contains(mode, "bound") || strings.Contains(mode, "conflict") {
				require.Equal(t, 1, bindings)
			} else {
				require.Zero(t, bindings, "email must never directly bind the account")
			}
			mu.Lock()
			count, text := messages, sentText
			mu.Unlock()
			if mode == "disabled while queued" || mode == "disabled during SMTP" || mode == "stale" {
				require.Zero(t, count)
			} else {
				require.Equal(t, 1, count)
			}
			require.NotContains(t, text, "@example.com")
			require.NotContains(t, text, "#token=")
			m.mu.Lock()
			mailCount := len(m.bodies)
			m.mu.Unlock()
			mailExpected := mode == "reviewed" || mode == "SMTP failed" || mode == "QQ denied" || mode == "QQ empty result" || mode == "disabled during SMTP"
			if mailExpected {
				require.Equal(t, 1, mailCount)
			} else {
				require.Zero(t, mailCount)
			}
			if mode == "reviewed" {
				require.Contains(t, text, "审核欢迎")
				require.Contains(t, text, "已向您入群申请填写的邮箱发送")
				token := m.token(t)
				_, err = s.ConfirmEmailBinding(ctx, token, "wrong-member")
				require.Error(t, err)
				_, err = s.ConfirmEmailBinding(ctx, token, "member")
				require.NoError(t, err)
				var got int64
				require.NoError(t, s.db.QueryRow(`SELECT user_id FROM qq_bot_bindings WHERE member_id='member'`).Scan(&got))
				require.Equal(t, uid, got)
				_, err = s.ConfirmEmailBinding(ctx, token, "member")
				require.Error(t, err)
			}
			if mode == "SMTP failed" {
				require.Contains(t, text, "邮件发送失败")
				_, err = s.PreviewEmailBinding(ctx, m.token(t))
				require.Error(t, err)
			}
			if mode == "QQ denied" {
				var reason, status string
				require.NoError(t, s.db.QueryRow(`SELECT decision_reason,status FROM qq_bot_jobs WHERE kind='welcome'`).Scan(&reason, &status))
				require.Equal(t, "delivery_failed_mail_sent", reason)
				require.Equal(t, "failed", status)
			}
			if mode == "QQ empty result" {
				var reason string
				require.NoError(t, s.db.QueryRow(`SELECT decision_reason FROM qq_bot_jobs WHERE kind='welcome'`).Scan(&reason))
				require.Equal(t, "delivery_failed_mail_sent", reason)
			}
			if mode == "forged receipt" {
				require.Zero(t, mailCount)
				require.Contains(t, text, "普通欢迎")
			}
			if mode == "invited" || mode == "newer invitation" || mode == "no review" || mode == "changed email" || mode == "changed policy" {
				require.Contains(t, text, "普通欢迎")
				require.Contains(t, text, "绑定 你的注册邮箱")
			}
			if mode == "already bound" {
				require.Contains(t, text, "继续使用现有绑定")
			}
			if strings.Contains(mode, "conflict") {
				require.Contains(t, text, "保留原绑定")
			}
			var balance string
			require.NoError(t, s.db.QueryRow(`SELECT balance::text FROM users WHERE id=$1`, uid).Scan(&balance))
			require.Equal(t, "0.00000000", balance, fmt.Sprintf("welcome must not change balance: %s", mode))
		})
	}
}
