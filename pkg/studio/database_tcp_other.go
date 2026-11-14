//go:build !linux

package studio

import "syscall"

// setUserTimeout: TCP_USER_TIMEOUT is Linux only; elsewhere keep-alives
// alone notice a host that has gone.
func setUserTimeout(string, string, syscall.RawConn) error { return nil }
