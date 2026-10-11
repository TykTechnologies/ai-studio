package grpc

import (
	"net"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// The connected flag is written by the goroutine that opens a stream and by
// the receive loop when the stream ends, and read by the heartbeat worker.
// A short heartbeat interval makes the worker read it while the stream is
// opened and closed, so `go test -race` reports any unsynchronised access
// (it used to be a plain bool, which failed TestE2E_OfflineEdges at random).
// Logging is off: every goroutine writing debug lines to one file
// descriptor synchronises them as far as the race detector can tell, which
// hid the race whenever the log writes happened to interleave.
func TestSimpleEdgeClient_ConnectedFlagIsRaceFree(t *testing.T) {
	level := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.Disabled)
	defer zerolog.SetGlobalLevel(level)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := probe.Addr().String()
	probe.Close()

	fc := &fakeControl{}
	srv := startFakeControl(t, addr, fc)

	client := NewSimpleEdgeClient(&config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:            "race-edge",
			EdgeNamespace:     "test",
			ControlEndpoint:   addr,
			AllowInsecure:     true,
			HeartbeatInterval: 2 * time.Millisecond,
		},
	}, "test", "hash", "time")
	client.reconnectInterval = 20 * time.Millisecond
	require.NoError(t, client.Start())
	defer client.Stop()
	require.Eventually(t, func() bool {
		return fc.streams.Load() == 1
	}, 5*time.Second, 10*time.Millisecond, "initial subscription")

	// Heartbeats while connected.
	time.Sleep(50 * time.Millisecond)

	// Ending the stream clears the flag from the receive loop while the
	// heartbeat worker keeps reading it.
	srv.Stop()
	time.Sleep(50 * time.Millisecond)

	srv = startFakeControl(t, addr, fc)
	defer srv.Stop()
	require.Eventually(t, func() bool {
		return fc.streams.Load() >= 2
	}, 10*time.Second, 20*time.Millisecond, "edge never re-subscribed")
	time.Sleep(50 * time.Millisecond)
}
