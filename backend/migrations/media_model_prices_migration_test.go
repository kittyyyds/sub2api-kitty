package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaModelPricesAuthCacheMigrationTracksBothModelMaps(t *testing.T) {
	migration, err := FS.ReadFile("229_group_media_model_prices_auth_cache.sql")
	require.NoError(t, err)
	contents := string(migration)
	require.Contains(t, contents, "OLD.image_model_prices IS NOT DISTINCT FROM NEW.image_model_prices")
	require.Contains(t, contents, "OLD.video_model_prices IS NOT DISTINCT FROM NEW.video_model_prices")
	require.Contains(t, contents, "groups.image_model_prices")
	require.Contains(t, contents, "groups.video_model_prices")
	require.Contains(t, contents, "CNY")
}
