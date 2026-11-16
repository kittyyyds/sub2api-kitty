package qqbot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCommunityGuideProjectsOnlyAdvertisedRules(t *testing.T) {
	bot := previewTestConfig()
	bot.AppSecret, bot.SystemPrompt = "private-app-secret", "private-system-prompt"
	bot.GroupPolicies = []GroupPolicy{{GroupID: "group", JoinReview: &JoinReviewConfig{Enabled: true, MinSpent: 10, Question: "已消费10元用户邮箱？"}, Welcome: &WelcomeConfig{Enabled: true, AutoEmailBinding: true}}}
	checkin := CheckinConfig{Enabled: true, MinAmount: .1, MaxAmount: 1, StreakDays: 7, StreakChances: 7,
		Growth: GrowthConfig{Tasks: []GrowthTask{{ID: "first_usage", Name: "首次成功 API 调用", Kind: "usage_count", Target: 10, Chances: 1, Enabled: true}, {ID: "hidden", Kind: "binding", Target: 1, Chances: 2}}}}
	lottery := defaultLotteryConfig()
	lottery.Enabled, lottery.RechargeStep = true, 30
	g := communityGuide(bot, checkin, lottery, RankingConfig{Enabled: true, RewardsEnabled: false})
	require.True(t, g.Enabled)
	require.True(t, g.AutoEmailBinding)
	require.Equal(t, float64(10), g.JoinReview.MinSpent)
	raw, err := json.Marshal(g)
	require.NoError(t, err)
	text := string(raw)
	for _, secret := range []string{bot.AppSecret, bot.APIKey, bot.SystemPrompt, "allowed_groups", "group_id", "app_id", "hidden", "guarantee_every"} {
		require.NotContains(t, text, secret)
	}
	require.Contains(t, text, "成功 API 调用累计 10 次")
	require.NotContains(t, text, "首次成功 API 调用")
	require.Contains(t, text, "当前未开启保底")
	for _, b := range g.Benefits {
		require.NotEqual(t, "invite", b.ID)
		require.NotEqual(t, "rankings", b.ID)
	}
}

func TestCommunityGuideDisabledGroupsAndDistinctJoinPolicies(t *testing.T) {
	bot := previewTestConfig()
	bot.Enabled = false
	require.Empty(t, communityGuide(bot, CheckinConfig{Enabled: true}, defaultLotteryConfig(), RankingConfig{}).Benefits)
	bot.Enabled = true
	bot.GroupPolicies = []GroupPolicy{{GroupID: "group", DisableActivities: true, DisableChat: true}}
	g := communityGuide(bot, CheckinConfig{Enabled: true}, defaultLotteryConfig(), RankingConfig{})
	require.Empty(t, g.Benefits)
	require.False(t, g.ChatEnabled)
	bot.AllowedGroups = append(bot.AllowedGroups, "second-group")
	bot.GroupPolicies[0].JoinReview = &JoinReviewConfig{Enabled: true, MinSpent: 10}
	require.Nil(t, communityGuide(bot, CheckinConfig{}, defaultLotteryConfig(), RankingConfig{}).JoinReview)
}

func TestCommunityGuideReflectsUpdatedRulesAndGuarantee(t *testing.T) {
	bot := previewTestConfig()
	lottery := defaultLotteryConfig()
	lottery.Enabled, lottery.GuaranteeEnabled = true, true
	ranking := RankingConfig{Enabled: true, RewardsEnabled: true, EffectiveDay: "2026-11-07", TokenRewards: []RankingPrize{{Rank: 1, Reward: GrowthReward{Type: "chances", Quantity: 3}}}}
	g := communityGuide(bot, CheckinConfig{Enabled: true, MinAmount: .2, MaxAmount: .8}, lottery, ranking)
	require.Equal(t, "0.2–0.8 站内余额", g.Benefits[0].Highlight)
	raw, err := json.Marshal(g)
	require.NoError(t, err)
	require.Contains(t, string(raw), "保底奖励")
	require.Contains(t, string(raw), "Token 消耗榜第 1 名：3 次抽奖机会")
}

func TestCommunityGuideReadsSettingsWithoutSideEffects(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	bot := previewTestConfig()
	raw, err := json.Marshal(bot)
	require.NoError(t, err)
	mock.ExpectQuery("(?s)SELECT jsonb_build_object.*FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	for _, key := range []string{checkinConfigKey, lotteryConfigKey, rankingConfigKey + bot.AppID} {
		mock.ExpectQuery("SELECT value FROM settings").WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{}`))
	}
	mock.ExpectCommit()
	guide, err := New(db, nil, false, nil).CommunityGuide(context.Background())
	require.NoError(t, err)
	require.True(t, guide.Enabled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityGuideReadFailureDoesNotInventRewards(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)SELECT jsonb_build_object.*FROM settings").WithArgs(configKey).WillReturnError(errors.New("private database detail"))
	mock.ExpectRollback()
	_, err = New(db, previewEncryptor{}, true, nil).CommunityGuide(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
