//go:build linux

package sysutil

import (
	"os"
	"syscall"
)

func signalGracefulRestart() error {
	return syscall.Kill(os.Getpid(), syscall.SIGTERM)
}
