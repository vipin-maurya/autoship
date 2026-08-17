//go:build !windows

package state

import (
	"os"
	"syscall"
)

// processAlive reports whether pid names a running process, via the classic
// signal-0 probe.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
