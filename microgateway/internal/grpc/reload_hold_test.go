package grpc

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// recordingReloadHandler records the pushes handed to it.
type recordingReloadHandler struct {
	mu  sync.Mutex
	ops []string
}

func (h *recordingReloadHandler) HandleReloadRequest(req *pb.ConfigurationReloadRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ops = append(h.ops, req.OperationId)
}

func (h *recordingReloadHandler) handled() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.ops...)
}

// Control delivers a waiting push as soon as the edge's stream opens, which
// is before cmd/microgateway sets its reload handler. The push is held and
// handed over when the handler is set, not dropped (M3). A push control
// sends again replaces the held one.
func TestSimpleEdgeClient_HoldsReloadsUntilHandlerIsSet(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: config.HubSpokeConfig{EdgeID: "edge-1"}}, "test", "h", "t")

	client.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-1"})
	client.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-2"})
	client.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-1"})

	h := &recordingReloadHandler{}
	client.SetReloadHandler(h)
	require.Eventually(t, func() bool { return len(h.handled()) == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"push-2", "push-1"}, h.handled())

	// Once set, pushes go straight to the handler.
	client.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push-3"})
	assert.Equal(t, []string{"push-2", "push-1", "push-3"}, h.handled())
}

// Only the most recent pushes are held.
func TestSimpleEdgeClient_HeldReloadsAreBounded(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: config.HubSpokeConfig{EdgeID: "edge-1"}}, "test", "h", "t")
	for i := 0; i < maxHeldReloads+5; i++ {
		client.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: string(rune('a' + i))})
	}
	h := &recordingReloadHandler{}
	client.SetReloadHandler(h)
	require.Eventually(t, func() bool { return len(h.handled()) == maxHeldReloads }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, string(rune('a'+5)), h.handled()[0], "the oldest were dropped")
}

// The handler is set from main while the stream's receive loop reads it;
// run with -race.
func TestSimpleEdgeClient_ReloadHandlerAccessIsSynchronised(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: config.HubSpokeConfig{EdgeID: "edge-1"}}, "test", "h", "t")
	h := &recordingReloadHandler{}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			client.HandleReloadRequest(&pb.ConfigurationReloadRequest{OperationId: "push"})
		}
	}()
	go func() {
		defer wg.Done()
		client.SetReloadHandler(h)
	}()
	wg.Wait()
	require.Eventually(t, func() bool { return len(h.handled()) >= 1 }, 5*time.Second, 5*time.Millisecond)
}

// The reconnect backoff doubles from EDGE_RECONNECT_INTERVAL but never
// waits more than 30 s (plus jitter), or the interval itself if longer (L3).
func TestSimpleEdgeClient_ReconnectBackoffIsCapped(t *testing.T) {
	assert.Equal(t, 30*time.Second, reconnectBackoffCap(2*time.Second))
	assert.Equal(t, 30*time.Second, reconnectBackoffCap(5*time.Second))
	assert.Equal(t, time.Minute, reconnectBackoffCap(time.Minute))

	client := NewSimpleEdgeClient(&config.Config{}, "test", "h", "t")
	base := 2 * time.Second
	var waited time.Duration
	for attempt := 1; attempt <= 20; attempt++ {
		d := client.calculateBackoffDelay(base, reconnectBackoffCap(base), 2.0, 0.1, attempt)
		assert.LessOrEqual(t, d, 33*time.Second, "attempt %d", attempt)
		if waited < 70*time.Second {
			waited += d
		}
	}
	// A 70 s outage: the edge is back within one capped interval.
	assert.Less(t, waited, 70*time.Second+33*time.Second)
}
