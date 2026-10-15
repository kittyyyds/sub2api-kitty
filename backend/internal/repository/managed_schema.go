package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
)

// Historical ledger identities are retained after feature retirement.
// These allowances do not execute SQL or enable any retired feature.
var managedHistoricalMigrations = map[string]string{
	// Exact v2.11.0 migrations reviewed for code rollback; no SQL is executed.
	"239_payment_order_subscription_renewal_mode.sql": "1bb3177ec7bce2f98f9c7c6e5062df203627b138d28596729d698568141bcb4f",
	"242_group_i18n.sql":                              "e81c6818cc806bc54365c993f4cec1634f16a311451faf5cb1a875a1358774cd",
	"243_gemini_cached_contents.sql":                  "2d345cff5c64b01e705f04ec972e5d5ec3d618fe71dc7d45bc80fdd38c6a89d5",
	"244_gemini_cached_contents_channel_usage.sql":    "9072cb37752a23c3a29cc0330a69837f09c5102375ee053d7bccf14634d3da29",
	"245_gemini_cached_contents_mutation_lease.sql":   "75434a7beeb8aad09b3339708ed520604d153e99c7fe680a376e8c958aadb1e8",
	"265_new_api_site_authorizations.sql":             "e1702c9f19362e23734ca2368cfdcb30221daa0e8bf8fd66d8e84c53aa9d7395",
	"266_account_ops_notifications.sql":               "e5fe16d7ca21268508c22a479ea0cb97888b42195f213269ada406dc8055de51",
	"267_account_ops_threshold_episodes.sql":          "64bbc5098037ff4829e25ca6eadc2c25f948545531b2e91a237b651e58df53f8",
	"268_controlled_experiments.sql":                  "860dd3cb52a0ab8c767cfb4a61ef47f893f4b1d0b1d37c644b2c5ed294c6b254",
	"268_support_tickets.sql":                         "f50ea3ce86c8f6c37e34bb0c0700f7cb3178ab605bbd9da94847b7d60bc84d84",
	"269_openai_totp_rotation.sql":                    "0f6892bd2bb82f547e75da2ff223baab0def3c45e33ec20b807cc1361ba24251",
	"270_drop_platform_check_constraints.sql":         "a7725f31f637208d200e0f37d5270e9f263743d99fa9e4f1bb9a7387e8be5a61",
	"271_openai_excel_dual_oauth.sql":                 "7dea786101dde7d3bf75b82d3058207d9e0e002d3701005206ab43ea5c5911dc",
	"272_account_ops_groups.sql":                      "446ac1f43b78e111934729cb396ee57ec3e4ddf75f288f7c53e61a6850ad6896",
	"273_channel_monitor_v3_idle_probe.sql":           "1d2eabf1007fe0c07277a8beb417aaa372223fae5de71d6e471727bdc0233ca9",
	"274_mihomo_static_proxy_bindings.sql":            "0edb83ba3595b73b6ba95f2547986de3a3f301ea6375f962dc0d676502890620",
	"275_channel_monitor_v2_candy_multi_model.sql":    "0f3a7ca8be24fd356fd99dc85d9f204e944a239282ade0a18e0291d93a807d20",
	"276_channel_monitor_v2_candy_history_min.sql":    "315e80dd37951ba8c59f51ebbb489bef7601af70a6e6c4c7acbda10e78de263f",
	"277_regional_scheduler_streams.sql":              "31cef6b2aa434d42b26b9b87323034bc22a9206ebefc5de505ebba108c326172",
	"278_regional_auth_streams.sql":                   "9b43ada04f7f8f24894ba6e842015e5e291ed5650218a3da1e893eb2536ee51c",
	"279_regional_ledgers.sql":                        "064a57ff28e7760cdbc7ca18d9a07b799a79181251e1baef7dcc9e223b8a2042",
	"280_regional_slot_leases.sql":                    "52cf547d6aefccc90992a4afb5841c90cd0d6707466afeec000ba4b15a73bde2",
	"281_regional_admission.sql":                      "d2cce4832270e6f131bd3b785948834492768f21ada6da128d00479244e5375e",
	"282_regional_pricing.sql":                        "acb019c9611d2b64eb650ba245cba9cfda4bb291d10e1184d4c1b4ed5b490c00",
	"284_regional_allocation_billing_metadata.sql":    "55763019386824220eee76d017d9025e5a3e384899090fa46b3157608c09e524",
	"285_kitty_support_center_import.sql":             "511642ea28856647ead35b2c319d8587cee60d06864c7f65a46cf9a0def15546",

	"188_enable_grok_enhancement_group.sql": "d53c9ab1f36d012ae13dfdb22b7303c94d6d9ebe01704430b681f9c6fc24fbe1",
	"239_codex_harvest_node_learning.sql":   "40a2d7a2d40a9d29cfd77781c7f70ff96ef95e8824b03dc6e06cf4bc891636d9",
	"240_codex_harvest_flow_events.sql":     "5a438fe8536068659b50d4d03d24fd57da21f1f9e226ac920332506e985d173e",

	"231_disable_long_context_pricing.sql": "1ce0bd2e37030134fc709dbbcb497b4fca053f7489fc46ebab62f8cede6cecc0",
}

// validateManagedMigrations performs a bounded read-only ledger check. Standby
// must not create schema/Atlas tables, take migration locks, or apply new SQL.
func validateManagedMigrations(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	expected := map[string]string{}
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	for _, name := range files {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		content := strings.TrimSpace(string(b))
		if content == "" {
			continue
		}
		sum := sha256.Sum256([]byte(content))
		expected[name] = hex.EncodeToString(sum[:])
	}
	if len(expected) == 0 {
		return fmt.Errorf("managed migration source is empty")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL statement_timeout = '5s'"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout = '1s'"); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT filename, checksum FROM schema_migrations ORDER BY filename")
	if err != nil {
		return fmt.Errorf("managed migration ledger unavailable: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return err
		}
		want, known := expected[name]
		if !known {
			if historical, ok := managedHistoricalMigrations[name]; !ok || historical != checksum {
				return fmt.Errorf("managed migration ledger has an unreviewed entry: %s", name)
			}
			continue
		}
		if checksum != want {
			return fmt.Errorf("managed migration checksum differs: %s", name)
		}
		delete(expected, name)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(expected) != 0 {
		return fmt.Errorf("managed database is missing %d migrations; prepare the schema separately", len(expected))
	}
	return tx.Commit()
}
