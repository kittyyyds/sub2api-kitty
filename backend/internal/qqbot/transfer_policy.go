package qqbot

import (
	"context"
	"database/sql"
)

// SetTransferBalancePolicy adds the hosting site's rules while the sender row
// is locked. Both the request and its single-use confirmation apply the policy.
// It must not send messages or commit the caller's transaction.
func (s *Service) SetTransferBalancePolicy(policy func(context.Context, *sql.Tx, int64, string) (string, error)) {
	s.transferBalancePolicy = policy
}
