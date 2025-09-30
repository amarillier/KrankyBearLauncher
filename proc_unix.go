//go:build !windows
// +build !windows

package main

import (
	"os"
	"syscall"
	"time"
)

// isProcessRunning: on Unix, signal 0 is a safe liveness probe.
func isProcessRunning(pid int) (bool, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil, nil
}

// killProcessTree: best-effort graceful terminate on Unix.
func killProcessTree(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// Try SIGTERM first
	if err := p.Signal(syscall.SIGTERM); err != nil {
		// Fall back to hard kill
		return p.Kill()
	}
	// Optional: brief grace period before returning
	time.Sleep(250 * time.Millisecond)
	// If still alive, escalate
	if err := p.Signal(syscall.Signal(0)); err == nil {
		return p.Kill()
	}
	return nil
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
