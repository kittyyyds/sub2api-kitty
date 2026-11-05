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
	"323_kitty_permanent_usage.sql":         "4fc15e33d22dd709f8aa93f8175778088fb6f13959c33980cdd07700e4c3b59f",
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
