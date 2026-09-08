//go:build !windows

package kittyrisk

import (
	"os"
	"syscall"
)

func lockModerationFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
func unlockModerationFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
func moderationDiskFree(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
