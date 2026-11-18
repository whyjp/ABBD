//go:build !windows

package lsp

import (
	"os"
	"syscall"
)

func alive(p *os.Process) bool { return p.Signal(syscall.Signal(0)) == nil }
