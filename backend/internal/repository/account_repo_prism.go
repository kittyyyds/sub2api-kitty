package repository

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func lockAndMergePrismCredentials(ctx context.Context, client *dbent.Client, account *service.Account, edit service.PrismCredentialsUpdate) error {
	rows, err := client.QueryContext(ctx, `SELECT platform, type, credentials FROM accounts
		WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE`, account.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return service.ErrAccountNotFound
	}
	var platform, accountType string
	var raw []byte
	if err := rows.Scan(&platform, &accountType, &raw); err != nil {
		return err
	}
	expectedPlatform, expectedType := edit.OriginalPlatform, edit.OriginalType
	if expectedPlatform == "" {
		expectedPlatform = account.Platform
	}
	if expectedType == "" {
		expectedType = account.Type
	}
	if platform != expectedPlatform || accountType != expectedType {
		return errors.New("Prism account identity changed during edit; reload the account")
	}
	current := make(map[string]any)
	if err := json.Unmarshal(raw, &current); err != nil {
		return err
	}
	if current == nil {
		current = make(map[string]any)
	}
	explicit := make(map[string]bool, len(edit.ExplicitKeys))
	for _, key := range edit.ExplicitKeys {
		explicit[key] = true
	}
	desired := account.Credentials
	for key, value := range desired {
		old, existed := edit.Original[key]
		if explicit[key] || !existed || !reflect.DeepEqual(old, value) {
			current[key] = value
		}
	}
	for key, old := range edit.Original {
		if _, kept := desired[key]; !kept && (explicit[key] || reflect.DeepEqual(current[key], old)) {
			delete(current, key)
		}
	}
	for key := range explicit {
		if _, kept := desired[key]; !kept {
			delete(current, key)
		}
	}
	delete(current, service.PrismCookieKey)
	if account.IsPrismEnabled() {
		if cookie, _ := current[service.PrismCookieEncryptedKey].(string); cookie == "" {
			return errors.New("Prism session changed during edit; reload the account")
		}
	}
	account.Credentials = current
	return nil
}
