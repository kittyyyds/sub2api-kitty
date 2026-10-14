package repository

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

// Run inside the binding transaction after locking groups, before removing any
// old memberships. Video accounts and authoritative video prices stay together.
func validateVideoGroupBindingPlatforms(ctx context.Context, exec sqlExecutor, accountIDs, groupIDs []int64) error {
	if len(accountIDs) == 0 || len(groupIDs) == 0 {
		return nil
	}
	rows, err := exec.QueryContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM accounts a CROSS JOIN groups g
		WHERE a.id = ANY($1::bigint[]) AND g.id = ANY($2::bigint[])
		AND ((a.platform = 'video') <> (g.platform = 'video'))
	)`, pq.Array(accountIDs), pq.Array(groupIDs))
	if err != nil {
		return err
	}
	defer rows.Close()
	var mismatch bool
	if rows.Next() {
		if err := rows.Scan(&mismatch); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if mismatch {
		return infraerrors.BadRequest("VIDEO_GROUP_PLATFORM_MISMATCH", "video accounts can only bind video groups, and video groups only accept video accounts")
	}
	return nil
}
