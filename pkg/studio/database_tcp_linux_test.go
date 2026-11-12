//go:build linux

package studio

import (
	"context"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Database connections notice a host that has gone: keep-alives after 5 s
// (three probes 2 s apart) and a 10 s TCP_USER_TIMEOUT.
func TestDeadPeerDialerSocketOptions(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	conn, err := deadPeerDialer(0).DialContext(context.Background(), "tcp", l.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	raw, err := conn.(*net.TCPConn).SyscallConn()
	require.NoError(t, err)
	opt := func(level, name int) int {
		var v int
		var getErr error
		require.NoError(t, raw.Control(func(fd uintptr) { v, getErr = syscall.GetsockoptInt(int(fd), level, name) }))
		require.NoError(t, getErr)
		return v
	}
	assert.Equal(t, 1, opt(syscall.SOL_SOCKET, syscall.SO_KEEPALIVE))
	assert.Equal(t, 5, opt(syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE))
	assert.Equal(t, 2, opt(syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL))
	assert.Equal(t, 3, opt(syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT))
	assert.Equal(t, 10000, opt(syscall.IPPROTO_TCP, tcpUserTimeout))
}
