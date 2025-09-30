//go:build !windows
// +build !windows

// Modified to uses setsid to start a new session because Chrome does
// weird stuff with it's parent process
// Starts the process in a new session, detaching it from the terminal.
// Helps prevent Chrome’s parent process from exiting immediately.
// Makes it easier to track the actual running process.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func buildCmd(app AppConfig, args []string) *exec.Cmd {
	p := expandPath(app.Path)
	cmd := exec.Command(p, args...)
	cmd.Dir = filepath.Dir(p)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Start a new session to isolate the process
	}
	return cmd
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
