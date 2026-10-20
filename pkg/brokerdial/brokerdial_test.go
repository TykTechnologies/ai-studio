package brokerdial

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func testClientConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///broker", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// fakeDialer answers dials when released, counting them.
type fakeDialer struct {
	calls   atomic.Int32
	release chan struct{}
	conn    *grpc.ClientConn
	err     error
}

func (f *fakeDialer) dial(*goplugin.GRPCBroker, uint32) (*grpc.ClientConn, error) {
	f.calls.Add(1)
	if f.release != nil {
		<-f.release
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.conn, nil
}

func readyConn(f *fakeDialer) *Conn {
	c := &Conn{}
	c.SetDialFunc(f.dial)
	c.SetBroker(&goplugin.GRPCBroker{})
	c.SetID(7, false)
	return c
}

// A caller's deadline bounds the wait, not the dial: the dial goes on, and
// its connection serves the next call without dialling again.
func TestConn_GetHonoursContextAndKeepsTheDial(t *testing.T) {
	f := &fakeDialer{release: make(chan struct{}), conn: testClientConn(t)}
	c := readyConn(f)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Get(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 400*time.Millisecond)

	close(f.release)
	got, err := c.Get(context.Background())
	require.NoError(t, err)
	assert.Same(t, f.conn, got)
	assert.EqualValues(t, 1, f.calls.Load(), "the dial the first caller gave up on was kept")
}

// Concurrent first calls share one dial.
func TestConn_ConcurrentCallsDialOnce(t *testing.T) {
	f := &fakeDialer{release: make(chan struct{}), conn: testClientConn(t)}
	c := readyConn(f)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := c.Get(context.Background())
			if err == nil && conn != f.conn {
				err = errors.New("wrong connection")
			}
			errs <- err
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(f.release)
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}
	assert.EqualValues(t, 1, f.calls.Load())
}

// A failed dial is remembered for RetryAfter: calls in between fail at once
// instead of each waiting out another dial.
func TestConn_FailureIsCachedThenRetried(t *testing.T) {
	now := time.Unix(1000, 0)
	f := &fakeDialer{err: errors.New("timeout waiting for connection info")}
	c := readyConn(f)
	c.now = func() time.Time { return now }

	_, err := c.Get(context.Background())
	require.Error(t, err)
	_, err = c.Get(context.Background())
	require.Error(t, err)
	assert.EqualValues(t, 1, f.calls.Load(), "no redial within RetryAfter")

	now = now.Add(RetryAfter)
	f.err, f.conn = nil, testClientConn(t)
	got, err := c.Get(context.Background())
	require.NoError(t, err)
	assert.Same(t, f.conn, got)
	assert.EqualValues(t, 2, f.calls.Load())
}

// A dial to an old broker ID does not become the connection for a new one.
func TestConn_IDChangeDiscardsTheDialInFlight(t *testing.T) {
	f := &fakeDialer{release: make(chan struct{}), conn: testClientConn(t)}
	c := readyConn(f)
	c.Warm()
	c.SetID(8, false)
	close(f.release)
	time.Sleep(20 * time.Millisecond)
	assert.Nil(t, c.Current(), "the dial for ID 7 was discarded")

	got, err := c.Get(context.Background())
	require.NoError(t, err)
	assert.Same(t, f.conn, got)
	assert.EqualValues(t, 2, f.calls.Load())
}

// keepConn leaves an open connection in use when the ID changes.
func TestConn_KeepConnOnIDChange(t *testing.T) {
	f := &fakeDialer{conn: testClientConn(t)}
	c := readyConn(f)
	_, err := c.Get(context.Background())
	require.NoError(t, err)
	c.SetID(9, true)
	assert.Same(t, f.conn, c.Current())
	c.SetID(10, false)
	assert.Nil(t, c.Current())
}

// Warm dials in the background without blocking, and a shared connection is
// used without dialling.
func TestConn_WarmAndShare(t *testing.T) {
	f := &fakeDialer{release: make(chan struct{}), conn: testClientConn(t)}
	c := readyConn(f)
	start := time.Now()
	c.Warm()
	assert.Less(t, time.Since(start), 50*time.Millisecond)
	close(f.release)
	require.Eventually(t, func() bool { return c.Current() == f.conn }, time.Second, 5*time.Millisecond)

	shared := testClientConn(t)
	other := &Conn{}
	other.SetDialFunc(f.dial)
	other.SetBroker(&goplugin.GRPCBroker{})
	other.SetID(7, false)
	other.Share(shared)
	got, err := other.Get(context.Background())
	require.NoError(t, err)
	assert.Same(t, shared, got)
	assert.EqualValues(t, 1, f.calls.Load())
}

// A connection dropped for a new broker ID is closed if this Conn dialled
// it, and left open if it was shared by its owner.
func TestConn_ResetClosesOnlyItsOwnConnection(t *testing.T) {
	own := testClientConn(t)
	f := &fakeDialer{conn: own}
	c := readyConn(f)
	_, err := c.Get(context.Background())
	require.NoError(t, err)
	c.SetID(8, false)
	assert.Equal(t, connectivity.Shutdown, own.GetState())

	shared := testClientConn(t)
	other := readyConn(&fakeDialer{})
	other.Share(shared)
	other.SetID(8, false)
	assert.NotEqual(t, connectivity.Shutdown, shared.GetState())
}

func TestConn_NotReadyWithoutBrokerOrID(t *testing.T) {
	c := &Conn{}
	_, err := c.Get(context.Background())
	assert.ErrorIs(t, err, ErrNotReady)
	c.SetBroker(&goplugin.GRPCBroker{})
	_, err = c.Get(context.Background())
	assert.ErrorIs(t, err, ErrNotReady)
}
