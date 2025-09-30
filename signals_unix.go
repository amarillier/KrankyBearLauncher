//go:build linux || darwin
// +build linux darwin

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// ====== Signal handlers - non Windows ======
func setupSignalHandlers() {
	log.Println("Setting up signal handlers for graceful shutdown and config reload")
	log.Println("Press Ctrl+C to exit; send SIGUSR1 to reload config")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	go func() {
		for {
			sig := <-sigs
			log.Printf("Received signal: %v", sig)
			switch sig {
			case syscall.SIGUSR1:
				log.Println("Reloading config due to SIGUSR1...")
				reloadAndReschedule()
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("Shutting down...")
				runningMu.Lock()
				/*
					for _, cmd := range runningProcs {
						// Try graceful SIGTERM; fallback to Kill
						if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
							log.Printf("Failed to send SIGTERM to PID %d: %v", cmd.Process.Pid, err)
							_ = cmd.Process.Kill()
						}
					}
				*/
				for _, cmd := range runningProcs {
					if err := killProcessTree(cmd.Process.Pid); err != nil {
						_ = cmd.Process.Kill()
					}
				}

				runningMu.Unlock()
				os.Exit(0)
			}
		}
	}()
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
