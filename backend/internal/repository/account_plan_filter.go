package repository

import (
	"context"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func applyAccountPlanFilter(ctx context.Context, q *dbent.AccountQuery) *dbent.AccountQuery {
	key := service.AccountPlanFilter(ctx)
	if key == "" {
		return q
	}
	aliases := []string{key}
	switch key {
	case "prolite":
		aliases = []string{"prolite", "pro100"}
	case "pro":
		aliases = []string{"pro", "chatgptpro", "pro200"}
	case "promax":
		aliases = []string{"promax", "pro500"}
	}
	return q.Where(dbaccount.PlatformEQ(service.PlatformOpenAI), dbaccount.TypeEQ(service.AccountTypeOAuth), func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("regexp_replace(lower(COALESCE(NULLIF(").Ident(s.C(dbaccount.FieldCredentials)).WriteString("->>'plan_type',''),(SELECT p.credentials->>'plan_type' FROM accounts p WHERE p.id=").Ident(s.C("parent_account_id")).WriteString(" AND p.deleted_at IS NULL))), '[[:space:]_-]+', '', 'g') IN (")
			for i, a := range aliases {
				if i > 0 {
					b.Comma()
				}
				b.Arg(a)
			}
			b.WriteByte(')')
		}))
	})
}
