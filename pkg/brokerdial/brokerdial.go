// Package brokerdial holds the one connection a plugin process opens to its
// host's brokered service server (KV, management, events, edge payloads).
//
// go-plugin hands a plugin a broker ID's connection info once, when the host
// accepts on that ID, keeps it for five seconds (GRPCBroker.timeoutWait) and
// lets one Dial consume it. So the connection must be dialled once, early,
// and shared by every service that needs it; a second Dial, or a first one
// made long after the host accepted, waits five seconds and fails.
//
// Each SDK holds its own Conn. Only one of them may be given the broker for
// a broker ID: two Conns dialling the same ID would race for go-plugin's
// one-shot connection info (on an edge only the Microgateway SDK has a
// broker; in Studio only the AI Studio SDK does).
//
// A Conn dials at most once at a time, honours the caller's context while
// the dial goes on in the background (so a caller that gives up does not
// waste the connection info), remembers a failure briefly instead of
// redialling on every call, and can be warmed as soon as the broker ID is
// known.
package brokerdial

import (
	"context"
	"errors"
	"sync"
	"time"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// RetryAfter is how long a failed dial is remembered before another is tried.
const RetryAfter = 2 * time.Second

// ErrNotReady is returned while the broker or the broker ID is not known.
var ErrNotReady = errors.New("plugin service broker not ready: no broker or broker ID yet")

// Conn is a plugin's connection to its host's brokered server. The zero
// value is ready to use.
type Conn struct {
	mu      sync.Mutex
	broker  *goplugin.GRPCBroker
	id      uint32
	gen     uint64 // bumped when the broker or ID changes
	conn    *grpc.ClientConn
	shared  bool          // conn came from Share: its owner closes it
	dialing chan struct{} // closed when the dial in flight ends
	lastErr error
	retryAt time.Time

	// dial and now are replaced in tests.
	dial func(b *goplugin.GRPCBroker, id uint32) (*grpc.ClientConn, error)
	now  func() time.Time
}

// SetDialFunc replaces how the connection is dialled (GRPCBroker.Dial), for
// tests of the SDKs that use a Conn.
func (c *Conn) SetDialFunc(dial func(b *goplugin.GRPCBroker, id uint32) (*grpc.ClientConn, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dial = dial
}

// SetBroker records the plugin side of the broker. A different broker drops
// the connection to the old one.
func (c *Conn) SetBroker(b *goplugin.GRPCBroker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broker != b {
		c.resetLocked()
	}
	c.broker = b
}

// SetID records the broker ID the host serves its services on. A different
// ID drops the connection to the old one, unless keepConn is set and a
// connection is open (a host that names the broker on every call but keeps
// serving the server the plugin is connected to).
func (c *Conn) SetID(id uint32, keepConn bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id == id {
		return
	}
	c.id = id
	if keepConn && c.conn != nil {
		return
	}
	c.resetLocked()
}

func (c *Conn) resetLocked() {
	c.gen++
	// A connection this Conn dialled is to a broker or ID no longer in
	// use; close it. One that was shared stays its owner's to close.
	if c.conn != nil && !c.shared {
		_ = c.conn.Close()
	}
	c.conn, c.shared = nil, false
	c.lastErr = nil
	c.retryAt = time.Time{}
	c.dialing = nil
}

// Share stores a connection something else dialled on this broker, when
// there is none yet.
func (c *Conn) Share(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		c.conn, c.shared = conn, true
	}
}

// Current returns the connection, or nil when none is open.
func (c *Conn) Current() *grpc.ClientConn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

// Warm starts a dial in the background when there is no connection, no dial
// in flight and no recent failure. It never blocks.
func (c *Conn) Warm() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startLocked()
}

// Get returns the connection, dialling it if needed. It waits for a dial at
// most until ctx ends; the dial itself goes on, and its connection is kept
// for the next call.
func (c *Conn) Get(ctx context.Context) (*grpc.ClientConn, error) {
	c.mu.Lock()
	if c.conn != nil {
		conn := c.conn
		c.mu.Unlock()
		return conn, nil
	}
	if c.broker == nil || c.id == 0 {
		c.mu.Unlock()
		return nil, ErrNotReady
	}
	done := c.startLocked()
	if done == nil {
		err := c.lastErr
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	select {
	case <-done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn, nil
	}
	if c.lastErr != nil {
		return nil, c.lastErr
	}
	return nil, ErrNotReady
}

// startLocked returns the channel of the dial in flight, starting one if it
// may; nil means a recent failure (c.lastErr) or nothing to dial yet.
func (c *Conn) startLocked() chan struct{} {
	if c.conn != nil || c.broker == nil || c.id == 0 {
		return nil
	}
	if c.dialing != nil {
		return c.dialing
	}
	now := c.clock()
	if c.lastErr != nil && now.Before(c.retryAt) {
		return nil
	}
	done := make(chan struct{})
	c.dialing = done
	b, id, gen := c.broker, c.id, c.gen
	dial := c.dial
	if dial == nil {
		dial = func(b *goplugin.GRPCBroker, id uint32) (*grpc.ClientConn, error) { return b.Dial(id) }
	}
	go func() {
		defer close(done)
		conn, err := dial(b, id)
		c.mu.Lock()
		defer c.mu.Unlock()
		if gen != c.gen {
			// The broker or ID changed while dialling: this connection
			// is to the old one.
			if conn != nil {
				_ = conn.Close()
			}
			return
		}
		c.dialing = nil
		if err != nil {
			c.lastErr = err
			c.retryAt = c.clock().Add(RetryAfter)
			return
		}
		c.conn, c.lastErr = conn, nil
	}()
	return done
}

func (c *Conn) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
