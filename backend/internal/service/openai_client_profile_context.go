package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type openAIClientProfileContextKey struct{}
type openAIClientProfileAccountContextKey struct{}

func withOpenAIAccountClientProfile(ctx context.Context, account *Account) context.Context {
	ctx = WithOpenAIClientProfile(ctx, account.GetOpenAIClientProfile())
	if account == nil {
		return ctx
	}
	return context.WithValue(ctx, openAIClientProfileAccountContextKey{}, account.ID)
}

// WithOpenAIClientProfile attaches an account-scoped transport profile to an
// upstream request. A nil profile preserves the legacy path exactly.
func WithOpenAIClientProfile(ctx context.Context, profile *OpenAIClientProfile) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if profile == nil {
		if OpenAIClientProfileFromContext(ctx) == nil {
			return ctx
		}
		// A retry may switch to an account without a profile. Clear a prior
		// account's value instead of inheriting its transport configuration.
		return context.WithValue(ctx, openAIClientProfileContextKey{}, (*OpenAIClientProfile)(nil))
	}
	return context.WithValue(ctx, openAIClientProfileContextKey{}, profile)
}

func OpenAIClientProfileFromContext(ctx context.Context) *OpenAIClientProfile {
	if ctx == nil {
		return nil
	}
	profile, _ := ctx.Value(openAIClientProfileContextKey{}).(*OpenAIClientProfile)
	return profile
}

// OpenAIClientProfileKey is a non-secret cache namespace. Changing any profile
// field uses a new client/connection pool; in-flight work remains untouched.
func OpenAIClientProfileKey(profile *OpenAIClientProfile) string {
	if profile == nil {
		return ""
	}
	encoded, _ := json.Marshal(profile)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:12])
}
