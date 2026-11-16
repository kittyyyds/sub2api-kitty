//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// Migration 360 adds the Tibo question to the administrator test catalog. It
// must stay in step with the background probe prompt, parse as a capability
// question bank, and ship disabled and idempotent.
func TestMigration360ScheduledTestTiboDefinition(t *testing.T) {
	body, err := migrations.FS.ReadFile("360_scheduled_test_tibo_definition.sql")
	require.NoError(t, err)
	sql := string(body)
	require.NotContains(t, sql, "\r", "migration files use LF line endings")
	require.Contains(t, sql, "INSERT INTO scheduled_test_definitions")
	require.Contains(t, sql, "'tibo'")
	require.Contains(t, sql, "'capability'")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
	require.Contains(t, sql, openAITiboProbePrompt)

	start := strings.Index(sql, "$tibo$")
	end := strings.LastIndex(sql, "$tibo$")
	require.Greater(t, end, start)
	config, err := ParseQualityCapabilityConfig(sql[start+len("$tibo$") : end])
	require.NoError(t, err)
	require.Len(t, config.Questions, 1)
	question := config.Questions[0]
	require.Equal(t, openAITiboProbePrompt, question.Prompt)
	require.Equal(t, qualityCapabilityMatchTibo, question.Match)
	require.Equal(t, tiboAnswerTrue, question.Expected)
	require.True(t, qualityCapabilityAnswerMatches(question, "True."))
	require.False(t, qualityCapabilityAnswerMatches(question, "False"))

	// Disabled on install: the migration alone never sends a request.
	normalized := strings.Join(strings.Fields(sql[end:]), " ")
	require.Contains(t, normalized, "'capability', false,")
}
