//go:build linux

package studio

import "syscall"

// tcpUserTimeout is TCP_USER_TIMEOUT (linux/tcp.h), which package syscall
// does not name.
const tcpUserTimeout = 0x12

// setUserTimeout sets TCP_USER_TIMEOUT on a TCP socket before it connects.
func setUserTimeout(network, _ string, c syscall.RawConn) error {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil
	}
	var setErr error
	if err := c.Control(func(fd uintptr) {
		setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, int(deadPeerUserTimeout.Milliseconds()))
	}); err != nil {
		return err
	}
	return setErr
}
