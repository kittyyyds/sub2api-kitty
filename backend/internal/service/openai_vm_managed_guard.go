package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// errOpenAIVMManagedNoToken answers every attempt to obtain, refresh or
// re-issue a token for an account whose credential lives in its VM2 slot.
var errOpenAIVMManagedNoToken = infraerrors.BadRequest("OPENAI_VM_MANAGED_NO_TOKEN", "This account is VM2-managed: its login and tokens are handled by its slot. Use the managed account panel.")

// A VM2-managed account has no token on this side; these keys must stay empty.
var openAIVMManagedTokenKeys = []string{"access_token", "refresh_token", "id_token"}

type openAIVMManagedCreateKey struct{}

// withOpenAIVMManagedCreate lets the managed service, and nothing else, create
// an account that carries the managed mark.
func withOpenAIVMManagedCreate(ctx context.Context) context.Context {
	return context.WithValue(ctx, openAIVMManagedCreateKey{}, true)
}

func openAIVMManagedHasToken(credentials map[string]any) bool {
	for _, key := range openAIVMManagedTokenKeys {
		if value, ok := credentials[key]; ok && strings.TrimSpace(asTrimmedString(value)) != "" {
			return true
		}
	}
	return false
}

func asTrimmedString(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	if value == nil {
		return ""
	}
	// Anything that is not a string still counts as "something was supplied".
	return "set"
}

// guardOpenAIVMManagedCreate keeps the managed mark out of every ordinary
// creation path (form, import, CRS sync): a marked account without a slot
// would be skipped by token refresh yet could never serve a request.
func guardOpenAIVMManagedCreate(ctx context.Context, input *CreateAccountInput) error {
	if input == nil {
		return nil
	}
	if _, marked := input.Extra[OpenAIVMManagedExtraKey]; !marked {
		return nil
	}
	if allowed, _ := ctx.Value(openAIVMManagedCreateKey{}).(bool); !allowed {
		return infraerrors.BadRequest("OPENAI_VM_MANAGED_MARK", "VM2-managed accounts are created from the managed account panel")
	}
	if input.Platform != PlatformOpenAI || input.Type != AccountTypeOAuth {
		return infraerrors.BadRequest("OPENAI_VM_MANAGED_TYPE", "Only OpenAI OAuth accounts can be VM2-managed")
	}
	if openAIVMManagedHasToken(input.Credentials) {
		return infraerrors.BadRequest("OPENAI_VM_MANAGED_TOKEN", "A VM2-managed account holds no token in Sub2API")
	}
	if input.ProxyID == nil || len(input.ProxyIDs) > 1 {
		return infraerrors.BadRequest("OPENAI_VM_PROXY_REQUIRED", "A VM2-managed account requires exactly one SOCKS5 proxy")
	}
	return nil
}

// guardOpenAIVMManagedUpdate refuses the edits that would hand a managed
// account a credential or a route of its own. Its proxy is changed through the
// managed panel, which moves the slot first.
func guardOpenAIVMManagedUpdate(account *Account, input *UpdateAccountInput) error {
	if account == nil || input == nil || !account.IsOpenAIVMManaged() {
		return nil
	}
	if input.Type != "" && input.Type != account.Type {
		return infraerrors.BadRequest("OPENAI_VM_MANAGED_TYPE", "A VM2-managed account must remain an OpenAI OAuth account")
	}
	if openAIVMManagedHasToken(input.Credentials) {
		return infraerrors.BadRequest("OPENAI_VM_MANAGED_TOKEN", "A VM2-managed account holds no token in Sub2API; log in from the managed account panel")
	}
	current := int64(0)
	if account.ProxyID != nil {
		current = *account.ProxyID
	}
	if input.ProxyID != nil && *input.ProxyID != current {
		return infraerrors.BadRequest("OPENAI_VM_MANAGED_PROXY", "Change the proxy of a VM2-managed account from the managed account panel")
	}
	if input.ProxyIDs != nil {
		ids := *input.ProxyIDs
		if len(ids) > 1 || (len(ids) == 1 && ids[0] != current) {
			return infraerrors.BadRequest("OPENAI_VM_MANAGED_PROXY", "Change the proxy of a VM2-managed account from the managed account panel")
		}
	}
	return nil
}

// openAIVMManagedUsageRefresher asks a managed account's slot to re-read its
// usage and returns the refreshed window fields. Set by the managed service;
// nil when the bridge is not configured.
var openAIVMManagedUsageRefresher func(ctx context.Context, accountID int64) (map[string]any, error)

// openAIVMManagedDeleter removes a managed account together with its slot.
// Set by the managed service; the generic account deletion defers to it.
var openAIVMManagedDeleter func(ctx context.Context, accountID int64) error

type openAIVMManagedDeleteKey struct{}

// withOpenAIVMManagedDelete marks the final account-row deletion the managed
// service performs after the slot is gone.
func withOpenAIVMManagedDelete(ctx context.Context) context.Context {
	return context.WithValue(ctx, openAIVMManagedDeleteKey{}, true)
}
