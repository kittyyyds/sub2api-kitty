// Package sysutil provides system-level utilities for process management.
package sysutil

import (
	"log"
	"runtime"
	"time"
)

// RestartService asks the normal SIGTERM shutdown path to exit the process.
// That path runs HTTP shutdown and deferred accounting/service cleanup. It
// retains the application's existing shutdown deadlines; administrators must
// drain active requests before restarting, particularly long-lived WebSockets.
//
// This relies on systemd's Restart=always configuration to automatically
// restart the service after it exits. This is the industry-standard approach:
//   - Simple and reliable
//   - No sudo permissions needed
//   - No complex process management
//   - Leverages systemd's native restart capability
//
// Prerequisites:
//   - Linux OS with systemd
//   - Service configured with Restart=always in systemd unit file
func RestartService() error {
	if runtime.GOOS != "linux" {
		log.Println("Service restart is supported only on Linux with a restart supervisor")
		return nil
	}

	log.Println("Initiating service restart through the normal SIGTERM shutdown path...")
	log.Println("The supervisor must restart normal exits (systemd Restart=always or Docker unless-stopped)")

	// Give a moment for logs to flush and response to be sent
	go func() {
		time.Sleep(100 * time.Millisecond)
		if err := signalGracefulRestart(); err != nil {
			log.Printf("Could not signal service restart: %v", err)
		}
	}()

	return nil
}

// RestartServiceAsync is a fire-and-forget version of RestartService.
// It logs errors instead of returning them, suitable for goroutine usage.
func RestartServiceAsync() {
	if err := RestartService(); err != nil {
		log.Printf("Service restart failed: %v", err)
		log.Println("Please restart the service manually: sudo systemctl restart sub2api")
	}
}
