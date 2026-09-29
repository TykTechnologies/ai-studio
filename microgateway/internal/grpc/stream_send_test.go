package grpc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
)

// overlapDetectingStream records whether two Sends ever ran at once, which a
// real gRPC stream does not allow.
type overlapDetectingStream struct {
	grpclib.ClientStream
	inSend  atomic.Int32
	overlap atomic.Bool
	sent    atomic.Int32
}

func (s *overlapDetectingStream) Send(*pb.EdgeMessage) error {
	if s.inSend.Add(1) > 1 {
		s.overlap.Store(true)
	}
	time.Sleep(200 * time.Microsecond)
	s.inSend.Add(-1)
	s.sent.Add(1)
	return nil
}

func (s *overlapDetectingStream) Recv() (*pb.ControlMessage, error) { select {} }

// Heartbeats, events and reload statuses are sent from different
// goroutines; they must never be on the stream at the same time.
func TestSimpleEdgeClient_SendsAreSerialised(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: config.HubSpokeConfig{EdgeID: "edge-1"}}, "test", "h", "t")
	raw := &overlapDetectingStream{}
	client.setStream(raw)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if i%2 == 0 {
					require.NoError(t, client.SendReloadStatus(&pb.ConfigurationReloadResponse{OperationId: "op", Phase: pb.ReloadPhase_PONG}))
				} else {
					require.NoError(t, client.send(&pb.EdgeMessage{}))
				}
			}
		}(i)
	}
	wg.Wait()
	assert.False(t, raw.overlap.Load(), "two Sends overlapped on one stream")
	assert.Equal(t, int32(160), raw.sent.Load())
}

func TestSimpleEdgeClient_SendWithoutStream(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: config.HubSpokeConfig{EdgeID: "edge-1"}}, "test", "h", "t")
	assert.Error(t, client.SendReloadStatus(&pb.ConfigurationReloadResponse{OperationId: "op"}))
	assert.Error(t, client.send(&pb.EdgeMessage{}))
}
