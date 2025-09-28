//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

func buildCmd(app AppConfig, args []string) *exec.Cmd {
	p := expandPath(app.Path)
	cmd := exec.Command(p, args...)
	cmd.Dir = filepath.Dir(p)
	cmd.Env = os.Environ()
	return cmd
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
