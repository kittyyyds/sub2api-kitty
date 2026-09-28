//go:build windows && unit

package service

import (
	"net"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestClassifyWindowsUpstreamTransportError(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       syscall.Errno
		persistent bool
	}{
		{"connection refused", windows.WSAECONNREFUSED, true},
		{"host unreachable", windows.WSAEHOSTUNREACH, true},
		{"network unreachable", windows.WSAENETUNREACH, true},
		{"timeout", windows.WSAETIMEDOUT, false},
		{"reset after connect", windows.WSAECONNRESET, false},
		{"aborted", windows.WSAECONNABORTED, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connectex", Err: tc.code}}
			if got := classifyUpstreamTransportError(err).Persistent; got != tc.persistent {
				t.Fatalf("persistent = %v, want %v", got, tc.persistent)
			}
		})
	}
}
