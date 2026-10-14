package service

import (
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ValidateConfiguredAccountProxy retains the proxy fail-closed boundary without
// any custom fingerprint or transport profile. A missing hydrated association
// must not silently turn an explicitly proxied account into a direct request.
func ValidateConfiguredAccountProxy(account *Account, proxyURL string) error {
	if account != nil && account.ProxyID != nil && *account.ProxyID > 0 && strings.TrimSpace(proxyURL) == "" {
		return infraerrors.New(http.StatusServiceUnavailable, "ACCOUNT_PROXY_UNAVAILABLE", "configured proxy is unavailable; request was not sent directly")
	}
	return nil
}
