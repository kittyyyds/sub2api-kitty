//go:build !linux

package sysutil

import "errors"

func signalGracefulRestart() error {
	return errors.New("service restart is supported only on Linux")
}
