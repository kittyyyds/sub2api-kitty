//go:build unit

package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoTasksMigrationSerializesIdentityCreationAndDeletion(t *testing.T) {
	body, err := FS.ReadFile("252_video_tasks.sql")
	require.NoError(t, err)
	sql := string(body)
	last := -1
	for _, fragment := range []string{
		"FROM users WHERE id=NEW.user_id FOR UPDATE",
		"FROM groups WHERE id=NEW.group_id FOR UPDATE",
		"FROM accounts WHERE id=NEW.account_id FOR UPDATE",
		"FROM api_keys WHERE id=NEW.api_key_id FOR UPDATE",
		"FROM user_subscriptions WHERE id=NEW.subscription_id FOR UPDATE",
	} {
		position := strings.Index(sql, fragment)
		require.Greater(t, position, last, "parent locks must keep one global order: %s", fragment)
		last = position
	}
	require.Contains(t, sql, "task_key.user_id<>NEW.user_id")
	require.Contains(t, sql, "task_key.group_id IS DISTINCT FROM NEW.group_id")
	require.Contains(t, sql, "task_account.platform<>'video' OR task_account.type<>'apikey'")
	require.Contains(t, sql, "task_subscription.deleted_at IS NOT NULL")
	require.Contains(t, sql, "video task identity is immutable")
	require.Contains(t, sql, "BEFORE INSERT ON video_tasks")
}

func TestVideoTasksMigrationUsesUnsettledIndexesAndNarrowParentTriggers(t *testing.T) {
	body, err := FS.ReadFile("252_video_tasks.sql")
	require.NoError(t, err)
	sql := string(body)
	for _, parent := range []string{"user", "group", "account", "key", "subscription"} {
		require.Contains(t, sql, "video_tasks_unsettled_"+parent+"_idx")
	}
	require.GreaterOrEqual(t, strings.Count(sql, "status IN ('submitting','pending','unknown','needs_review')"), 10)
	for _, fragment := range []string{
		"BEFORE UPDATE OF id,deleted_at OR DELETE ON users",
		"BEFORE UPDATE OF id,deleted_at,platform,subscription_type OR DELETE ON groups",
		"BEFORE UPDATE OF id,deleted_at,platform,type,credentials OR DELETE ON accounts",
		"BEFORE UPDATE OF id,deleted_at,user_id,group_id OR DELETE ON api_keys",
		"BEFORE UPDATE OF id,deleted_at,user_id,group_id OR DELETE ON user_subscriptions",
	} {
		require.Contains(t, sql, fragment)
	}
	require.Contains(t, sql, "NEW.credentials->>'base_url'")
	require.Contains(t, sql, "NEW.credentials->>'grok_media_provider'")
	require.NotContains(t, sql, "NEW.credentials->>'api_key'")
	require.NotContains(t, sql, "NEW.key IS DISTINCT")
	require.NotContains(t, sql, "BEFORE UPDATE OR DELETE ON users")
	require.NotContains(t, sql, "EXECUTE format(")
}
