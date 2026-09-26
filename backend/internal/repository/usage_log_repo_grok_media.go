package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.GrokMediaVideoOwnerAccountRepository = (*usageLogRepository)(nil)

const grokMediaVideoOwnerAccountQuery = `
SELECT DISTINCT ul.account_id
FROM application_run_steps ars
JOIN application_runs ar
  ON ar.id = ars.run_id
JOIN usage_logs ul
  ON ul.session_id = ar.session_id
 AND ul.user_id = ar.user_id
 AND ul.api_key_id = ar.media_api_key_id
WHERE ars.gateway_request_id = $1
  AND ar.user_id = $2
  AND ar.media_api_key_id = $3
  AND ul.group_id IS NOT DISTINCT FROM $4
  AND ars.executor IN ('video_generation', 'video_edit', 'video_extension')
  AND ul.video_count > 0
  AND ul.account_id > 0
LIMIT 2`

// FindGrokMediaVideoOwnerAccountID recovers the only upstream account that can
// be proven to own a historical application-center video request. Ambiguous
// matches fail closed so a known request ID can never route to another account.
func (r *usageLogRepository) FindGrokMediaVideoOwnerAccountID(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
	groupID *int64,
) (accountID int64, err error) {
	requestID = strings.TrimSpace(requestID)
	if r == nil || r.sql == nil || requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return 0, sql.ErrNoRows
	}

	var groupArg any
	if groupID != nil {
		groupArg = *groupID
	}
	rows, err := r.sql.QueryContext(ctx, grokMediaVideoOwnerAccountQuery, requestID, userID, apiKeyID, groupArg)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	uniqueAccounts := make(map[int64]struct{}, 2)
	for rows.Next() {
		var candidate int64
		if scanErr := rows.Scan(&candidate); scanErr != nil {
			return 0, scanErr
		}
		if candidate > 0 {
			uniqueAccounts[candidate] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(uniqueAccounts) == 0 {
		return 0, sql.ErrNoRows
	}
	if len(uniqueAccounts) != 1 {
		return 0, fmt.Errorf("ambiguous grok video owner account")
	}
	for candidate := range uniqueAccounts {
		return candidate, nil
	}
	return 0, sql.ErrNoRows
}
