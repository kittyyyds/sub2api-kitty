//go:build integration

package qqbot

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKittyExistingSettingsReadAndRoundTripPreserveEffectivePolicy(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	s := savedBot(t, db)
	c, err := s.Config(ctx)
	require.NoError(t, err)
	c.Autonomous.Enabled = true
	c.Autonomous.Groups = []string{"group"}
	c.Autonomous.SamplePercent = 7
	c.Autonomous.CooldownSeconds = 231
	c.Autonomous.MinMessages = 9
	c.Autonomous.DailyCalls = 12
	c.Autonomous.StartHour, c.Autonomous.EndHour = 8, 19
	c.Autonomous.Names = []string{"原昵称"}
	c.GroupPolicies = []GroupPolicy{{GroupID: "group", JoinReview: &JoinReviewConfig{Enabled: true, MinSpent: 10, Question: "请输入已消费邮箱", RejectReason: "本群只审核已消费用户"}, Welcome: &WelcomeConfig{Enabled: true, AutoEmailBinding: true, ReviewedText: "原审核欢迎词", OtherText: "原邀请欢迎词"}}}
	c.TransferEnabled, c.TransferFeePercent, c.TransferMinAmount, c.TransferMaxAmount = true, 1.5, 2, 123
	_, err = s.SaveConfig(ctx, c)
	require.NoError(t, err)
	// Simulate a pre-upgrade encrypted config which cannot contain new fields.
	var raw string
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE key=$1`, configKey).Scan(&raw))
	var stored map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	for _, key := range []string{"reward_budget_daily", "ai_connection", "site_api_key_id", "site_group_id", "site_owner_id", "image_ai_connection", "image_site_api_key_id", "image_site_group_id", "image_site_owner_id", "invite_coupon_template_id", "invite_coupon_quantity"} {
		delete(stored, key)
	}
	var auto map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stored["autonomous"], &auto))
	for _, key := range []string{"reply_mode", "inherit_groups", "merge_seconds", "input_token_budget"} {
		delete(auto, key)
	}
	stored["autonomous"], err = json.Marshal(auto)
	require.NoError(t, err)
	encoded, err := json.Marshal(stored)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE settings SET value=$2 WHERE key=$1`, configKey, string(encoded))
	require.NoError(t, err)
	const oldCheckin = `{"enabled":false,"min_amount":0.31,"max_amount":0.79,"streak_days":5,"streak_chances":3,"growth":{"binding_chances":2,"first_usage_chances":4,"checkin_30_chances":6},"min_recharge_total":12,"min_recharge_30d":3,"recharge_window_days":7,"reward_distribution":{"lower_half_bps":8000,"next_quarter_bps":1500,"upper_quarter_bps":500}}`
	const oldLottery = `{"enabled":true,"title":"原抽奖","recharge_step":27,"chances_per_step":2,"prizes":[{"name":"原余额奖","amount":0.23,"probability_bps":6000},{"name":"原机会奖","chances":3,"probability_bps":4000}],"guarantee_enabled":true,"guarantee_every":11,"guarantee_prizes":[{"name":"原保底","amount":1.7,"probability_bps":10000}]}`
	for key, value := range map[string]string{checkinConfigKey: oldCheckin, lotteryConfigKey: oldLottery} {
		_, err = db.Exec(`INSERT INTO settings(key,value) VALUES($1,$2)`, key, value)
		require.NoError(t, err)
	}
	original, err := s.Config(ctx)
	require.NoError(t, err)
	checkin, err := s.CheckinConfig(ctx)
	require.NoError(t, err)
	lottery, err := s.LotteryConfig(ctx)
	require.NoError(t, err)
	invite, err := s.InviteConfig(ctx)
	require.NoError(t, err)
	require.Nil(t, original.RewardBudgetDaily)
	require.Equal(t, "legacy", original.Autonomous.ReplyMode)
	require.Empty(t, original.AIConnection)
	require.Empty(t, original.ImageAIConnection)
	require.Equal(t, 6, checkin.Growth.Checkin30Chances)
	require.Empty(t, checkin.Milestones)
	for key, value := range map[string]string{configKey: string(encoded), checkinConfigKey: oldCheckin, lotteryConfigKey: oldLottery} {
		var actual string
		require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE key=$1`, key).Scan(&actual))
		require.Equal(t, value, actual, "reads must leave persisted bytes untouched")
	}
	_, err = s.SaveBasicConfig(ctx, original.Public())
	require.NoError(t, err)
	require.NoError(t, s.SaveCheckinConfig(ctx, checkin))
	require.NoError(t, s.SaveLotteryConfig(ctx, lottery))
	require.NoError(t, s.SaveInviteConfig(ctx, invite))
	after, err := s.Config(ctx)
	require.NoError(t, err)
	require.Equal(t, original.Public(), after.Public(), "no-op save must preserve effective policy including group review, welcome, quotas and transfer")
	require.Equal(t, original.AppSecret, after.AppSecret)
	require.Equal(t, original.APIKey, after.APIKey)
	require.Equal(t, original.ImageAPIKey, after.ImageAPIKey)
	checkinAfter, err := s.CheckinConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, checkin, checkinAfter)
	lotteryAfter, err := s.LotteryConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, lottery, lotteryAfter)
}
