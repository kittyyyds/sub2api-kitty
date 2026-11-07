package qqbot

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWelcomeValidationAndPolicy(t *testing.T) {
	require.NoError(t, validateWelcome(nil))
	require.NoError(t, validateWelcome(&WelcomeConfig{ReviewedText: "欢迎\n请查收邮件"}))
	for _, text := range []string{"<qqbot-at-everyone />", "bad\x00text", strings.Repeat("好", 501)} {
		require.Error(t, validateWelcome(&WelcomeConfig{OtherText: text}))
	}
	c := DefaultConfig()
	c.AppID = "app"
	c.GroupPolicies = []GroupPolicy{{GroupID: "group", Welcome: &WelcomeConfig{Enabled: true}}}
	first := welcomePolicy(c, "group")
	c.Sandbox = true
	require.NotEqual(t, first, welcomePolicy(c, "group"))
	c.Sandbox = false
	c.GroupPolicies[0].Welcome.AutoEmailBinding = true
	require.NotEqual(t, first, welcomePolicy(c, "group"))
}
