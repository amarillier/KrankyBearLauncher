//go:build windows
// +build windows

package main

import (
	"errors"
	"os/exec"
	"strconv"

	"golang.org/x/sys/windows"
)

// Win32 STILL_ACTIVE exit code (0x00000103 / 259) returned by GetExitCodeProcess
const statusStillActive = 259

// isProcessRunning uses Win32 APIs to check if a PID is still active.
func isProcessRunning(pid int) (bool, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// If we can't open the process:
		// - ACCESS_DENIED (5) ⇒ process may be protected; assume it's running
		// - INVALID_PARAMETER (87) ⇒ likely no such PID; assume not running
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return true, nil
		}
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		return false, err
	}
	defer windows.CloseHandle(h)

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false, err
	}
	return code == statusStillActive, nil
}

// killProcessTree terminates the PID and all children (Chrome, etc.).
func killProcessTree(pid int) error {
	// /F = force, /T = include child processes
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F", "/T")
	return cmd.Run()
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942