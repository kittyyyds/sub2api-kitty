package qqbot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKittyFreshQQBotHasNoEnabledFeature(t *testing.T) {
	c := DefaultConfig()
	require.Empty(t, c.AppID)
	require.Empty(t, c.AppSecret)
	require.Empty(t, c.APIKey)
	require.Empty(t, c.AllowedGroups)
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(raw, &data))
	var check func(map[string]any)
	check = func(m map[string]any) {
		for key, value := range m {
			switch v := value.(type) {
			case bool:
				if key == "chat_disabled" {
					require.True(t, v)
				} else {
					require.False(t, v, "fresh boolean switch %s", key)
				}
			case map[string]any:
				check(v)
			}
		}
	}
	check(data)
	require.False(t, defaultLotteryConfig().Enabled)
	require.False(t, defaultRankingConfig().Enabled)
	require.False(t, defaultRankingConfig().RewardsEnabled)
}
