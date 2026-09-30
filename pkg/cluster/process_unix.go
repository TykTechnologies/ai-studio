//go:build linux || darwin

package cluster

import (
	"errors"
	"syscall"
)

// processAlive reports whether pid exists in this pid namespace (a zombie
// counts as alive: nothing has reaped it yet).
func processAlive(pid int) bool {
	if pid <= 0 {
		return true
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
