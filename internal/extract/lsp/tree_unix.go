//go:build !windows

package lsp

import (
	"os/exec"
	"syscall"
)

// ownTree puts the server in its own process group, so killTree reaches
// what a launcher script started.
func ownTree(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// killTree stops the server and everything it started.
func killTree(cmd *exec.Cmd) {
	if syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) != nil {
		cmd.Process.Kill()
	}
}
