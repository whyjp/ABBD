//go:build windows

package lsp

import (
	"os/exec"
	"strconv"
)

func ownTree(*exec.Cmd) {}

// killTree stops the server and everything it started. A server behind a
// .cmd shim runs under cmd.exe, and killing only that leaves node running.
func killTree(cmd *exec.Cmd) {
	if exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run() != nil {
		cmd.Process.Kill()
	}
}
