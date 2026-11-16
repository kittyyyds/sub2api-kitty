//go:build integration

package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func accountFeatureFixture(t *testing.T) (*sql.DB, *Service, Config, int64, Job) {
	t.Helper()
	db := testDB(t)
	s := savedBot(t, db)
	uid := user(t, db)
	bind(t, db, uid, "member")
	_, err := db.Exec(`ALTER TABLE users ADD COLUMN username TEXT NOT NULL DEFAULT '',ADD COLUMN concurrency INTEGER NOT NULL DEFAULT 2,ADD COLUMN balance_notify_enabled BOOLEAN NOT NULL DEFAULT TRUE;
 CREATE TABLE api_keys(id BIGSERIAL PRIMARY KEY,user_id BIGINT,name TEXT NOT NULL,key TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',deleted_at TIMESTAMPTZ);
 CREATE TABLE usage_logs(user_id BIGINT,api_key_id BIGINT,input_tokens BIGINT DEFAULT 0,output_tokens BIGINT DEFAULT 0,cache_creation_tokens BIGINT DEFAULT 0,cache_read_tokens BIGINT DEFAULT 0,actual_cost NUMERIC DEFAULT 0,created_at TIMESTAMPTZ NOT NULL);`)
	require.NoError(t, err)
	c, err := s.Config(context.Background())
	require.NoError(t, err)
	c.BalanceAlertsEnabled = true
	_, err = s.SaveConfig(context.Background(), c)
	require.NoError(t, err)
	s.SetBalanceAlertValidator(func(context.Context, int64, float64) (bool, error) { return true, nil })
	j := Job{AppID: c.AppID, MemberID: "member", GroupID: privateJobGroup, EventType: privateMessageEvent, MessageID: "private-message", CreatedAt: time.Now()}
	return db, s, c, uid, j
}

func TestAccountFeatureKeyIsolationMaskingAndDateAggregation(t *testing.T) {
	db, s, c, uid, j := accountFeatureFixture(t)
	ctx := context.Background()
	other := user(t, db)
	_, err := db.Exec(`INSERT INTO api_keys(id,user_id,name,key) VALUES(1,$1,'own','sk-sensitive-owned-full-key'),(2,$2,'foreign','sk-sensitive-foreign-key')`, uid, other)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO usage_logs(user_id,api_key_id,input_tokens,output_tokens,actual_cost,created_at) VALUES($1,1,20,30,0.25,NOW()),($2,2,2000,3000,99,NOW())`, uid, other)
	require.NoError(t, err)
	list, err := s.myAPIKeys(ctx, c, j, "")
	require.NoError(t, err)
	require.Contains(t, list, "own")
	require.NotContains(t, list, "foreign")
	require.NotContains(t, list, "sk-sensitive-owned-full-key")
	text, err := s.myAPIKeyUsage(ctx, c, j, "2", time.Now())
	require.NoError(t, err)
	require.Contains(t, text, "未找到属于你的")
	text, err = s.myAPIKeyUsage(ctx, c, j, "1", time.Now())
	require.NoError(t, err)
	require.Contains(t, text, "0.250000")
	require.Contains(t, text, "请求：1 次")
	require.NotContains(t, text, "99.000000")
	text, err = s.personalInfo(ctx, c, j, time.Now())
	require.NoError(t, err)
	require.Contains(t, text, "当前 RPM（近60秒）：1")
}

func TestBalanceAlertRequiresPrivateOptInAndCancelsOnUnsubscribe(t *testing.T) {
	db, s, c, uid, j := accountFeatureFixture(t)
	ctx := context.Background()
	require.NoError(t, s.EnqueueBalanceAlert(ctx, uid, 1, 5, "Kitty", "https://example.invalid/recharge"))
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qq_bot_balance_alerts`).Scan(&count))
	require.Zero(t, count)
	group := j
	group.EventType = groupAtMessageCreate
	group.GroupID = "group"
	text, err := s.balanceSubscription(ctx, c, group, "开启")
	require.NoError(t, err)
	require.Contains(t, text, "请私聊")
	text, err = s.balanceSubscription(ctx, c, j, "开启")
	require.NoError(t, err)
	require.Contains(t, text, "已订阅")
	for range 2 {
		require.NoError(t, s.EnqueueBalanceAlert(ctx, uid, 1, 5, "Kitty", "https://example.invalid/recharge"))
	}
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM qq_bot_balance_alerts`).Scan(&count))
	require.Equal(t, 1, count)
	_, err = s.balanceSubscription(ctx, c, j, "关闭")
	require.NoError(t, err)
	var status string
	require.NoError(t, db.QueryRow(`SELECT status FROM qq_bot_balance_alerts`).Scan(&status))
	require.Equal(t, "cancelled", status)
}

func TestBalanceAlertConcurrentLeaseRetryAndSuccessfulAcknowledgement(t *testing.T) {
	db, s, c, uid, j := accountFeatureFixture(t)
	ctx := context.Background()
	var sends atomic.Int64
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			fmt.Fprint(w, `{"access_token":"fake-test-token","expires_in":7200}`)
			return
		}
		require.Equal(t, "/v2/users/member/messages", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotContains(t, body, "msg_id")
		require.NotContains(t, body, "group_id")
		sends.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			fmt.Fprint(w, `{"code":123}`)
			return
		}
		fmt.Fprint(w, `{"id":"sent"}`)
	}))
	defer srv.Close()
	s.tokenURL = srv.URL + "/token"
	s.qqURL = srv.URL
	_, err := s.balanceSubscription(ctx, c, j, "开启")
	require.NoError(t, err)
	require.NoError(t, s.EnqueueBalanceAlert(ctx, uid, 1, 5, "Kitty", ""))
	require.NoError(t, s.deliverOneBalanceAlert(ctx))
	require.Equal(t, int64(1), sends.Load())
	var status string
	var next time.Time
	require.NoError(t, db.QueryRow(`SELECT status,next_attempt_at FROM qq_bot_balance_alerts`).Scan(&status, &next))
	require.Equal(t, "pending", status)
	require.True(t, next.After(time.Now()))
	require.NoError(t, s.deliverOneBalanceAlert(ctx))
	require.Equal(t, int64(1), sends.Load())
	_, err = db.Exec(`UPDATE qq_bot_balance_alerts SET next_attempt_at=NOW()-INTERVAL '1 second'`)
	require.NoError(t, err)
	fail.Store(false)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.deliverOneBalanceAlert(ctx) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	require.Equal(t, int64(2), sends.Load())
	require.NoError(t, db.QueryRow(`SELECT status FROM qq_bot_balance_alerts`).Scan(&status))
	require.Equal(t, "sent", status)
}

func TestBalanceAlertRebindAndPolicyChangeNeverDeliver(t *testing.T) {
	for _, scenario := range []string{"rebind", "policy"} {
		t.Run(scenario, func(t *testing.T) {
			db, s, c, uid, j := accountFeatureFixture(t)
			ctx := context.Background()
			_, err := s.balanceSubscription(ctx, c, j, "开启")
			require.NoError(t, err)
			require.NoError(t, s.EnqueueBalanceAlert(ctx, uid, 1, 5, "Kitty", ""))
			if scenario == "rebind" {
				_, err = db.Exec(`UPDATE qq_bot_bindings SET user_id=$1 WHERE app_id='app' AND member_id='member'`, user(t, db))
				require.NoError(t, err)
			} else {
				s.SetBalanceAlertValidator(func(context.Context, int64, float64) (bool, error) { return false, nil })
			}
			s.qqURL = "http://127.0.0.1:1"
			require.NoError(t, s.deliverOneBalanceAlert(ctx))
			var status string
			require.NoError(t, db.QueryRow(`SELECT status FROM qq_bot_balance_alerts`).Scan(&status))
			require.Equal(t, "cancelled", status)
		})
	}
}

func TestPrivateAccountEnqueueDeduplicatesSignedIdentity(t *testing.T) {
	db, s, c, _, _ := accountFeatureFixture(t)
	body, _ := json.Marshal(map[string]any{"id": "same", "content": "个人信息", "timestamp": time.Now().UTC().Format(time.RFC3339), "author": map[string]string{"user_openid": "member", "member_openid": "forged"}, "group_openid": "forged"})
	for range 2 {
		require.NoError(t, s.Enqueue(context.Background(), c, Payload{Type: privateMessageEvent, Data: body}))
	}
	var count int
	var member, group string
	require.NoError(t, db.QueryRow(`SELECT COUNT(*),MIN(member_id),MIN(group_id) FROM qq_bot_jobs WHERE event_type=$1`, privateMessageEvent).Scan(&count, &member, &group))
	require.Equal(t, 1, count)
	require.Equal(t, "member", member)
	require.Equal(t, privateJobGroup, group)
}

func TestBalanceAlertExpiredLeaseRecoveryAndConfigCancellation(t *testing.T) {
	db, s, c, uid, j := accountFeatureFixture(t)
	ctx := context.Background()
	_, err := s.balanceSubscription(ctx, c, j, "开启")
	require.NoError(t, err)
	require.NoError(t, s.EnqueueBalanceAlert(ctx, uid, 1, 5, "Kitty", ""))
	first, err := s.claimBalanceAlert(ctx, c)
	require.NoError(t, err)
	_, err = s.claimBalanceAlert(ctx, c)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.Exec(`UPDATE qq_bot_balance_alerts SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, first.ID)
	require.NoError(t, err)
	recovered, err := s.claimBalanceAlert(ctx, c)
	require.NoError(t, err)
	require.Equal(t, first.ID, recovered.ID)
	require.NotEqual(t, first.LeaseToken, recovered.LeaseToken)
	require.Equal(t, 2, recovered.Attempts)
	// An old worker cannot acknowledge a lease taken over after its expiry.
	require.NoError(t, s.finishBalanceAlert(ctx, first, "sent", "", time.Now()))
	var status string
	require.NoError(t, db.QueryRow(`SELECT status FROM qq_bot_balance_alerts WHERE id=$1`, first.ID).Scan(&status))
	require.Equal(t, "leased", status)
	c.BalanceAlertsEnabled = false
	_, err = s.SaveConfig(ctx, c)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT status FROM qq_bot_balance_alerts WHERE id=$1`, first.ID).Scan(&status))
	require.Equal(t, "cancelled", status)
	require.NoError(t, s.finishBalanceAlert(ctx, recovered, "sent", "", time.Now()))
	require.NoError(t, db.QueryRow(`SELECT status FROM qq_bot_balance_alerts WHERE id=$1`, first.ID).Scan(&status))
	require.Equal(t, "cancelled", status)
}
