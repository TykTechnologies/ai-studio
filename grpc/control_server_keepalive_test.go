package grpc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// HTTP/2 frame types and the error code a gRPC server sends when a client
// pings more often than its keepalive enforcement policy allows.
const (
	h2FrameSettings         = 0x4
	h2FramePing             = 0x6
	h2FrameGoAway           = 0x7
	h2ErrEnhanceYourCalm    = 0xb
	h2ClientConnectionStart = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
)

// serveOnLoopback serves s on a loopback listener and stops it with the test.
func serveOnLoopback(t *testing.T, s *ControlServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Serve(lis)
	}()
	t.Cleanup(func() {
		s.Stop()
		<-done
	})
	return lis.Addr().String()
}

// pingEdgeLike opens a bare HTTP/2 connection (no stream, as an edge between
// reconnects or before its stream opens) and sends a PING every interval, as
// an edge's keepalive (30 s, PermitWithoutStream) does. It returns the GOAWAY
// error code the server answered with, or -1 if the connection stayed up.
func pingEdgeLike(t *testing.T, addr string, pings int, interval time.Duration) int {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	writeFrame := func(typ byte, payload []byte) error {
		hdr := make([]byte, 9)
		hdr[0], hdr[1], hdr[2] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
		hdr[3] = typ
		_, err := conn.Write(append(hdr, payload...))
		return err
	}
	_, err = conn.Write([]byte(h2ClientConnectionStart))
	require.NoError(t, err)
	require.NoError(t, writeFrame(h2FrameSettings, nil))

	goAway := make(chan int, 1)
	go func() {
		hdr := make([]byte, 9)
		for {
			if _, err := io.ReadFull(conn, hdr); err != nil {
				goAway <- -1
				return
			}
			n := int(hdr[0])<<16 | int(hdr[1])<<8 | int(hdr[2])
			payload := make([]byte, n)
			if _, err := io.ReadFull(conn, payload); err != nil {
				goAway <- -1
				return
			}
			if hdr[3] == h2FrameGoAway && n >= 8 {
				goAway <- int(binary.BigEndian.Uint32(payload[4:8]))
				return
			}
		}
	}()

	for i := 0; i < pings; i++ {
		if err := writeFrame(h2FramePing, make([]byte, 8)); err != nil && !errors.Is(err, net.ErrClosed) {
			break
		}
		select {
		case code := <-goAway:
			return code
		case <-time.After(interval):
		}
	}
	select {
	case code := <-goAway:
		return code
	case <-time.After(200 * time.Millisecond):
		return -1
	}
}

// An edge pings every 30 s even without an open stream. The server must
// allow that: with grpc-go's default policy (at most one ping per 5 minutes,
// none without a stream) it answers the third ping with GOAWAY
// ENHANCE_YOUR_CALM ("too_many_pings") and drops the connection.
func TestServe_AllowsEdgeKeepalivePings(t *testing.T) {
	s, _ := setupTestServer(t, &Config{
		GRPCHost:  "127.0.0.1",
		AuthToken: testAuthToken,
		// Scaled down with the ping interval below: the edge's 30 s pings
		// against the 10 s default.
		KeepaliveMinTime: 50 * time.Millisecond,
	})
	addr := serveOnLoopback(t, s)

	assert.Equal(t, -1, pingEdgeLike(t, addr, 5, 150*time.Millisecond),
		"the server sent GOAWAY to a client pinging within its keepalive policy")
}

// The enforcement policy still stops a client that pings too often.
func TestServe_RejectsPingFlood(t *testing.T) {
	s, _ := setupTestServer(t, &Config{
		GRPCHost:         "127.0.0.1",
		AuthToken:        testAuthToken,
		KeepaliveMinTime: time.Minute,
	})
	addr := serveOnLoopback(t, s)

	assert.Equal(t, h2ErrEnhanceYourCalm, pingEdgeLike(t, addr, 5, 10*time.Millisecond))
}

// An edge sends plugin payloads of up to 1 MB each, several to a batch, and
// its client allows 16 MB messages. The server used to keep gRPC's 4 MB
// receive limit and refuse such a batch as a whole.
func TestServe_AcceptsMessagesOverFourMB(t *testing.T) {
	s, _ := setupTestServer(t, &Config{GRPCHost: "127.0.0.1", AuthToken: testAuthToken})
	addr := serveOnLoopback(t, s)

	conn, err := grpclib.NewClient(addr,
		grpclib.WithTransportCredentials(insecure.NewCredentials()),
		grpclib.WithDefaultCallOptions(grpclib.MaxCallSendMsgSize(16*1024*1024)))
	require.NoError(t, err)
	defer conn.Close()

	batch := &pb.PluginControlBatch{EdgeId: "edge-1", SequenceNumber: 1}
	for i := 0; i < 6; i++ {
		batch.Payloads = append(batch.Payloads, &pb.PluginControlPayload{
			PluginId: 1, Payload: make([]byte, 1024*1024), CorrelationId: fmt.Sprint(i),
		})
	}
	batch.TotalPayloads = uint32(len(batch.Payloads))
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+testAuthToken)
	resp, err := pb.NewConfigurationSyncServiceClient(conn).SendPluginControlBatch(ctx, batch)
	require.NoError(t, err, "a 6 MB batch must reach the handler")
	assert.Len(t, resp.Errors, 6, "no plugin manager here, so each payload reports an error")

	// Beyond the limit the server still refuses.
	batch.Payloads = append(batch.Payloads, &pb.PluginControlPayload{PluginId: 1, Payload: make([]byte, 11*1024*1024)})
	_, err = pb.NewConfigurationSyncServiceClient(conn).SendPluginControlBatch(ctx, batch,
		grpclib.MaxCallSendMsgSize(32*1024*1024))
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

// The defaults admit an edge's keepalive (30 s pings, also without a
// stream) and its 16 MB messages.
func TestServerOptions_Defaults(t *testing.T) {
	o := (&Config{}).serverTuning()
	assert.Equal(t, 10*time.Second, o.keepaliveMinTime)
	assert.LessOrEqual(t, o.keepaliveMinTime, 30*time.Second, "edges ping every 30 s")
	assert.Equal(t, 30*time.Second, o.keepaliveTime)
	assert.Equal(t, 5*time.Second, o.keepaliveTimeout)
	assert.Equal(t, 16*1024*1024, o.maxMessageSize)
	assert.Zero(t, o.maxConnectionAge, "connection ageing is off unless configured")
}
