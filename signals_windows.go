//go:build windows
// +build windows

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// ====== Signal handlers - Windows ======
func setupSignalHandlers() {
	log.Println("Setting up Windows signal handlers")

	sigs := make(chan os.Signal, 1)
	// On Windows, os.Interrupt (Ctrl+C) is the primary console signal.
	// syscall.SIGTERM is defined but may not be delivered by the OS; ok to include.
	// SIGUSR1, SIGUSR2 and other Unix-specific signals are not supported on Windows.
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		log.Printf("Received signal: %v. Shutting down...", sig)
		runningMu.Lock()
		for _, cmd := range runningProcs {
			// Best-effort terminate; fall back to Kill if unsupported
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				log.Printf("Failed to terminate PID %d: %v", cmd.Process.Pid, err)
				_ = cmd.Process.Kill()
			}
		}
		runningMu.Unlock()
		os.Exit(0)
	}()
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
