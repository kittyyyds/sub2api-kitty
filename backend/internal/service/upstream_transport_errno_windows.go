//go:build windows

package service

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Windows network errors use Winsock codes rather than the synthetic POSIX
// syscall constants. Keep this typed so localized error messages work too.
func isPlatformPersistentTransportError(err error) bool {
	return errors.Is(err, windows.WSAECONNREFUSED) ||
		errors.Is(err, windows.WSAEHOSTUNREACH) ||
		errors.Is(err, windows.WSAENETUNREACH)
}
