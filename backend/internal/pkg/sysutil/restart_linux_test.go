//go:build linux

package sysutil

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRestartServiceDeliversSignalAndRunsDeferredCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	marker := filepath.Join(t.TempDir(), "cleanup")
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRestartServiceSignalChild$")
	cmd.Env = append(os.Environ(), "KITTY_RESTART_TEST_CHILD=1", "KITTY_RESTART_TEST_MARKER="+marker)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restart child failed: %v\n%s", err, output)
	}
	contents, err := os.ReadFile(marker)
	if err != nil || string(contents) != "signal-received\ncleanup-ran\n" {
		t.Fatalf("restart bypassed signal handling or deferred cleanup: %q, %v", contents, err)
	}
}

func TestRestartServiceSignalChild(t *testing.T) {
	if os.Getenv("KITTY_RESTART_TEST_CHILD") != "1" {
		t.Skip("subprocess helper")
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM)
	defer signal.Stop(quit)
	marker := os.Getenv("KITTY_RESTART_TEST_MARKER")
	defer func() {
		f, err := os.OpenFile(marker, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString("cleanup-ran\n"); err != nil {
			t.Fatal(err)
		}
	}()
	if err := RestartService(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-quit:
		if err := os.WriteFile(marker, []byte("signal-received\n"), 0600); err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restart did not deliver SIGTERM")
	}
}
