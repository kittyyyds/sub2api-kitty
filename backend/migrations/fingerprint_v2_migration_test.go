package migrations

import (
	"strings"
	"testing"
)

func TestFingerprintV2PreparationKeepsOldApplicationRollbackSafe(t *testing.T) {
	data, err := FS.ReadFile("226_cutover_openai_identity_v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, required := range []string{"fingerprint_v2_seed", "gen_random_uuid()::text", "schema_version", "AND type = 'oauth'", "deleted_at IS NULL", "NOT COALESCE"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("missing migration guard %q", required)
		}
	}
	for _, forbidden := range []string{"- 'codex_fingerprint", "- 'openai_client_profile'", "DROP TABLE", "credentials =", "balance ="} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("preparation must not delete runtime settings or billing: %q", forbidden)
		}
	}
}
