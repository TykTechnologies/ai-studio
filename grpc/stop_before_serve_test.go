package grpc

import (
	"net"
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
