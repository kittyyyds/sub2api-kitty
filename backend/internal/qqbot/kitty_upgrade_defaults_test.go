package qqbot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKittyUpgradeLeavesSavedAutonomousPolicyAndRewardBudgetUnchanged(t *testing.T) {
	const saved = `{"enabled":true,"chat_disabled":false,"allowed_groups":["old-group"],"checkin_reward":0.37,"invite_reward":0.82,"autonomous":{"enabled":true,"groups":["old-group"],"names":["原名称"],"sample_percent":7,"cooldown_seconds":231,"min_messages":9,"daily_calls":12,"start_hour":8,"end_hour":19,"followup_seconds":30}}`
	c := DefaultConfig()
	require.NoError(t, json.Unmarshal([]byte(saved), &c))
	require.Equal(t, "legacy", replyMode(c.Autonomous))
	require.Nil(t, c.RewardBudgetDaily, "missing hard budget is unlimited, never zero")
	require.Empty(t, c.AIConnection, "existing upstream connection stays external/legacy")
	require.Empty(t, c.ImageAIConnection)
	require.Equal(t, .37, c.CheckinReward)
	require.Equal(t, .82, c.InviteReward)
	require.Equal(t, 7, c.Autonomous.SamplePercent)
	require.Equal(t, 231, c.Autonomous.CooldownSeconds)
	require.Equal(t, 12, c.Autonomous.DailyCalls)
	require.False(t, c.Autonomous.InheritGroups)
	require.Equal(t, c.Autonomous, normalizeAutonomous(AutonomousConfig{}, c.Autonomous))
	c.HistoryMessages = 0
	msgs := (&Service{}).history(context.Background(), c, Job{GroupID: "old-group", MemberID: "member"})
	for _, msg := range msgs {
		require.NotContains(t, msg.Content, naturalConversationInstructions)
	}
}

func TestKittyLegacyModeKeepsMediaAndFollowupAdmission(t *testing.T) {
	e := Event{Attachments: []Attachment{{URL: "https://example.invalid/picture.png", ContentType: "image/png"}}}
	_, reason := legacyAutonomousText(e)
	require.Equal(t, "non_text", reason, "old autonomous profiles do not gain paid vision calls")
	_, reason = autonomousPolicyText(AutonomousConfig{ReplyMode: "natural"}, e)
	require.Empty(t, reason, "explicit new mode supports vision")
	require.False(t, legacyFollowupText("没看懂"))
	require.True(t, followupText("没看懂"))
}

func TestKittyZeroInputBudgetDoesNotDiscardExistingContext(t *testing.T) {
	messages := []Message{{Role: "system", Content: "operator persona"}, {Role: "user", Content: "question"}}
	got, estimate, err := budgetAutonomousInput(messages, 0, nil)
	require.NoError(t, err)
	require.Equal(t, messages, got)
	require.Zero(t, estimate.Removed)
}
