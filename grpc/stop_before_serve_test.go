package grpc

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A host may call Stop before Serve has built its gRPC server (Stop right
// after starting Serve in a goroutine). Serve must then return instead of
// serving for ever: Stop found no server to stop.
func TestServeAfterStopReturns(t *testing.T) {
	control, _ := setupTestServer(t, nil)
	control.config.TLSEnabled = false
	control.Stop()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- control.Serve(lis) }()

	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve kept serving after Stop")
	}
	// The listener is closed: nothing accepts on it any more.
	_, err = net.DialTimeout("tcp", lis.Addr().String(), time.Second)
	require.Error(t, err)
}

// pausingListener holds Serve between building its gRPC server and serving:
// Serve logs the listener's address in that window.
type pausingListener struct {
	net.Listener
	reached, release chan struct{}
	once             sync.Once
}

func (l *pausingListener) Addr() net.Addr {
	l.once.Do(func() {
		close(l.reached)
		<-l.release
	})
	return l.Listener.Addr()
}

// Stop can also land after Serve built its gRPC server but before it
// serves: grpc-go then answers Serve with ErrServerStopped, which is the
// shutdown asked for, not a failure.
func TestStopBetweenServerBuiltAndServingIsNotAnError(t *testing.T) {
	control, _ := setupTestServer(t, nil)
	control.config.TLSEnabled = false

	inner, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	lis := &pausingListener{Listener: inner, reached: make(chan struct{}), release: make(chan struct{})}
	served := make(chan error, 1)
	go func() { served <- control.Serve(lis) }()

	<-lis.reached
	control.Stop()
	close(lis.release)

	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve kept serving after Stop")
	}
}
