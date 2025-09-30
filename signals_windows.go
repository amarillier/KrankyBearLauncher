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

		// Best-effort terminate; fall back to Kill if unsupported
		/*
			for _, cmd := range runningProcs {
				if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
					log.Printf("Failed to terminate PID %d: %v", cmd.Process.Pid, err)
					_ = cmd.Process.Kill()
				}
			}
		*/
		// On Windows, use taskkill to ensure child processes are also terminated.
		// This is a best-effort attempt; taskkill may fail for protected processes.
		// Note: this may produce console output if taskkill fails; ignoring errors here.
		// Also note: taskkill /T may not always capture all child processes in complex trees.
		// For more robust solutions, consider using job objects or third-party libraries.
		/*
			for _, cmd := range runningProcs {
				_ = killProcessTree(cmd.Process.Pid)
			}
		*/
		for _, cmd := range runningProcs {
			if err := killProcessTree(cmd.Process.Pid); err != nil {
				log.Printf("Tree kill failed for PID %d: %v; falling back to Kill()", cmd.Process.Pid, err)
				_ = cmd.Process.Kill()
			}
		}

		runningMu.Unlock()
		os.Exit(0)
	}()
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
