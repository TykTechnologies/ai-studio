package studio

import (
	"net"
	"time"
)

// A pooled connection to a database host that has gone (a failed node, a
// deleted pod whose address now drops packets) would otherwise wait for an
// answer for as long as TCP retransmits, many minutes: neither pgx nor
// database/sql has a read timeout. With these settings the connection
// fails within seconds, so the statement is held and tried again on the
// new primary (pkg/dbhold), or answered 503, inside the hold window. A
// server that is alive answers keep-alives and acknowledges data whatever
// a query costs, so long queries are not affected.
const (
	// deadPeerKeepAliveIdle, Interval and Count: an idle connection, or one
	// waiting for an answer, is probed after 5 s and dropped after three
	// unanswered probes 2 s apart (about 11 s).
	deadPeerKeepAliveIdle     = 5 * time.Second
	deadPeerKeepAliveInterval = 2 * time.Second
	deadPeerKeepAliveCount    = 3
	// deadPeerUserTimeout bounds how long sent data may stay unacknowledged
	// (TCP_USER_TIMEOUT, Linux only).
	deadPeerUserTimeout = 10 * time.Second
)

// deadPeerDialer dials database connections with those settings, and
// connectTimeout (0: none) for the dial itself.
func deadPeerDialer(connectTimeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: connectTimeout,
		KeepAliveConfig: net.KeepAliveConfig{
			Enable:   true,
			Idle:     deadPeerKeepAliveIdle,
			Interval: deadPeerKeepAliveInterval,
			Count:    deadPeerKeepAliveCount,
		},
		Control: setUserTimeout,
	}
}
