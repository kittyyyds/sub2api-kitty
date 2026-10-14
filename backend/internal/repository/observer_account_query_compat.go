package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// observerAccountQuery is retained as a no-op because this Kitty branch removed
// observer-group scoping. Keeping the compatibility hook lets upstream v2.9.9
// admission code compile without reintroducing observer behavior.
func observerAccountQuery(_ context.Context, query *dbent.AccountQuery) *dbent.AccountQuery {
	return query
}
