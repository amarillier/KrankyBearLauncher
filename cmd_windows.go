//go:build windows
// +build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// buildCmd builds the right *exec.Cmd for Windows.
//   - .exe/.com: started directly, CWD set to exe folder.
//   - .bat/.cmd: started via cmd.exe /C call
//   - .ps1: started via powershell.exe -NoProfile -ExecutionPolicy Bypass -File
//   - everything else: "shell-open" via cmd.exe /C start "" <path-or-url> [args...]
//     (NOTE: "start" detaches; you won't get a reliable PID for duration-based kill.)
func buildCmd(app AppConfig, args []string) *exec.Cmd {
	p := os.ExpandEnv(expandPath(app.Path))
	ext := strings.ToLower(filepath.Ext(p))

	switch ext {
	case ".exe", ".com":
		cmd := exec.Command(p, args...)
		cmd.Dir = filepath.Dir(p)
		cmd.Env = os.Environ()
		// Optional: hide console windows for GUI apps
		// cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd

	case ".bat", ".cmd":
		// Use cmd.exe to run batch scripts.
		// Separate arguments are safe—Go will quote as needed.
		base := []string{"/D /C", "call", p}
		cmd := exec.Command("cmd.exe", append(base, args...)...)
		cmd.Dir = filepath.Dir(p)
		cmd.Env = os.Environ()
		// cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd

	case ".ps1":
		// PowerShell execution policy varies by environment; Bypass is pragmatic for tools.
		base := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", p}
		cmd := exec.Command("powershell.exe", append(base, args...)...)
		cmd.Dir = filepath.Dir(p)
		cmd.Env = os.Environ()
		// cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd

	default:
		// Shell-open via "start": documents, URLs, or .lnk (shortcuts)
		// Warning: this launches detached—PID tracking won't represent the real app.
		base := []string{"/D /C", "start", `""`, p}
		cmd := exec.Command("cmd.exe", append(base, args...)...)
		cmd.Env = os.Environ()
		// No reliable working dir or PID in this branch.
		// cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd
	}
}

// Optional: to check whether the command is "detached" (start/open),
// we can split this into buildCmd + a helper that returns a bool, or
// parse on extension here and attach metadata in RunningApp if we extend the type.
// For our current .exe/.com use-case (Notepad/Chrome), the above is enough.

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
