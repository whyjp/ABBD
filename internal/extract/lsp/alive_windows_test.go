//go:build windows

package lsp

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func alive(p *os.Process) bool {
	out, _ := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(p.Pid), "/NH").Output()
	return strings.Contains(string(out), strconv.Itoa(p.Pid))
}
