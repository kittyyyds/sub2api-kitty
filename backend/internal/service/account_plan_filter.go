package service

import (
	"context"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"strings"
	"unicode"
)

type accountPlanFilterKey struct{}

func CanonicalAccountPlanType(value string) string {
	key := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '_' || r == '-' {
			return -1
		}
		return unicode.ToLower(r)
	}, value)
	switch key {
	case "pro100":
		return "prolite"
	case "chatgptpro", "pro200":
		return "pro"
	case "pro500":
		return "promax"
	}
	return key
}
func WithAccountPlanFilter(ctx context.Context, value string) (context.Context, error) {
	value = CanonicalAccountPlanType(value)
	switch value {
	case "", "plus", "prolite", "pro", "promax", "free":
	default:
		return ctx, infraerrors.BadRequest("INVALID_PLAN_FILTER", "GPT 档位筛选无效。")
	}
	return context.WithValue(ctx, accountPlanFilterKey{}, value), nil
}
func AccountPlanFilter(ctx context.Context) string {
	value, _ := ctx.Value(accountPlanFilterKey{}).(string)
	return value
}
