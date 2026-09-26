package service

import (
	_ "embed"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const DefaultOpenAIRequestTimezone = "Asia/Singapore"
const openAIRequestTimezoneExtraKey = "openai_request_timezone"
const openAIRequestTimezoneEnabledExtraKey = "openai_request_timezone_enabled"
const openAIRequestTimezoneSyncSearchExtraKey = "openai_request_timezone_sync_search"

// IANA area/location names bundled with Go's timezone database, plus UTC.
// No migration changes existing account settings or enables this feature.
//
//go:embed openai_request_timezones.txt
var openAIRequestTimezoneNames string

var openAIRequestTimezoneOptions = strings.Fields(openAIRequestTimezoneNames)
var openAIRequestTimezoneAllowed = func() map[string]bool {
	names := make(map[string]bool, len(openAIRequestTimezoneOptions))
	for _, name := range openAIRequestTimezoneOptions {
		names[name] = true
	}
	return names
}()

func OpenAIRequestTimezoneOptions() []string {
	return append([]string(nil), openAIRequestTimezoneOptions...)
}

func isOpenAIRequestTimezoneKey(key string) bool {
	return key == openAIRequestTimezoneExtraKey || key == openAIRequestTimezoneEnabledExtraKey || key == openAIRequestTimezoneSyncSearchExtraKey
}

func ValidateOpenAIRequestTimezoneExtra(platform string, extra map[string]any) error {
	invalid := func() error {
		return infraerrors.New(http.StatusBadRequest, "INVALID_OPENAI_REQUEST_TIMEZONE", "request timezone settings require an OpenAI account, boolean switches and a supported IANA timezone")
	}
	for key, value := range extra {
		if !isOpenAIRequestTimezoneKey(key) {
			continue
		}
		if platform != PlatformOpenAI {
			return invalid()
		}
		if key == openAIRequestTimezoneExtraKey {
			name, ok := value.(string)
			if !ok || !openAIRequestTimezoneAllowed[name] {
				return invalid()
			}
			if _, err := time.LoadLocation(name); err != nil {
				return invalid()
			}
		} else if _, ok := value.(bool); !ok {
			return invalid()
		}
	}
	return nil
}

// A timezone-only edit must not replace tier routing, quotas or other custom
// fields. Other editors that omit the new keys retain the stored policy.
func preserveOpenAIRequestTimezoneExtra(account *Account, incoming map[string]any) map[string]any {
	if incoming == nil || account == nil {
		return incoming
	}
	onlyTimezone := len(incoming) > 0
	for key := range incoming {
		if !isOpenAIRequestTimezoneKey(key) {
			onlyTimezone = false
			break
		}
	}
	merged := make(map[string]any, len(incoming)+len(account.Extra))
	for key, value := range account.Extra {
		if onlyTimezone || isOpenAIRequestTimezoneKey(key) {
			merged[key] = value
		}
	}
	for key, value := range incoming {
		merged[key] = value
	}
	return merged
}

func (account *Account) OpenAIRequestTimezoneEnabled() bool {
	return account != nil && account.IsOpenAI() && account.Extra[openAIRequestTimezoneEnabledExtraKey] == true
}

func (account *Account) OpenAIRequestTimezoneSyncSearch() bool {
	return account.OpenAIRequestTimezoneEnabled() && account.Extra[openAIRequestTimezoneSyncSearchExtraKey] == true
}

func (account *Account) OpenAIRequestTimezone() string {
	if account == nil {
		return ""
	}
	raw, present := account.Extra[openAIRequestTimezoneExtraKey]
	if !present {
		return DefaultOpenAIRequestTimezone
	}
	name, ok := raw.(string)
	if !ok || !openAIRequestTimezoneAllowed[name] {
		return ""
	}
	return name
}
