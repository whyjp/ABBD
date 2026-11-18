//go:build !windows

package lsp

import (
	"os/exec"
	"syscall"
)

// ownTree puts the server in its own process group, so killTree reaches
// what a launcher script started. Ctrl-C then no longer reaches the server,
// but it exits on the stdin EOF an interrupted abbd leaves.
func ownTree(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// killTree stops the server and everything it started.
func killTree(cmd *exec.Cmd) {
	if syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) != nil {
		cmd.Process.Kill()
	}
}
