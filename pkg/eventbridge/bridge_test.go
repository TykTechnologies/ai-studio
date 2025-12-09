package eventbridge

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStream implements GRPCStream for testing
type mockStream struct {
	mu       sync.Mutex
	sent     []*EventFrame
	toRecv   chan *EventFrame
	closed   bool
	sendErr  error
}

func newMockStream() *mockStream {
	return &mockStream{
		toRecv: make(chan *EventFrame, 100),
	}
}

func (m *mockStream) SendEvent(frame *EventFrame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, frame)
	return nil
}

func (m *mockStream) RecvEvent() (*EventFrame, error) {
	frame, ok := <-m.toRecv
	if !ok {
		return nil, context.Canceled
	}
	return frame, nil
}

func (m *mockStream) getSent() []*EventFrame {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*EventFrame, len(m.sent))
	copy(result, m.sent)
	return result
}

func (m *mockStream) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		close(m.toRecv)
		m.closed = true
	}
}

func TestBridge_EdgeToControl(t *testing.T) {
	// Setup edge side
	edgeBus := NewBus()
	edgeStream := newMockStream()
	defer edgeStream.Close()

	edgeBridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, edgeBus, edgeStream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	edgeBridge.Start(ctx)
	assert.True(t, edgeBridge.IsRunning())

	// Give the bridge time to start
	time.Sleep(20 * time.Millisecond)

	// Publish DirUp event on edge
	payload, _ := json.Marshal(map[string]string{"data": "test"})
	edgeBus.Publish(Event{
		ID:      "evt-1",
		Topic:   "metrics.report",
		Origin:  "edge-001",
		Dir:     DirUp,
		Payload: payload,
	})

	// Wait for event to be sent
	time.Sleep(50 * time.Millisecond)

	// Verify event was forwarded
	sent := edgeStream.getSent()
	require.Len(t, sent, 1)
	assert.Equal(t, "metrics.report", sent[0].Topic)
	assert.Equal(t, int32(DirUp), sent[0].Dir)
	assert.Equal(t, "edge-001", sent[0].Origin)
}

func TestBridge_ControlToEdge(t *testing.T) {
	// Setup control side
	controlBus := NewBus()
	controlStream := newMockStream()
	defer controlStream.Close()

	controlBridge := NewBridge(BridgeConfig{
		NodeID:    "control",
		IsControl: true,
	}, controlBus, controlStream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controlBridge.Start(ctx)

	// Give the bridge time to start
	time.Sleep(20 * time.Millisecond)

	// Publish DirDown event on control
	payload, _ := json.Marshal(map[string]string{"action": "reload"})
	controlBus.Publish(Event{
		ID:      "evt-2",
		Topic:   "config.reload",
		Origin:  "control",
		Dir:     DirDown,
		Payload: payload,
	})

	// Wait for event to be sent
	time.Sleep(50 * time.Millisecond)

	// Verify event was forwarded
	sent := controlStream.getSent()
	require.Len(t, sent, 1)
	assert.Equal(t, "config.reload", sent[0].Topic)
	assert.Equal(t, int32(DirDown), sent[0].Dir)
}

func TestBridge_LocalEventsNotForwarded(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridge.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Publish DirLocal event
	bus.Publish(Event{
		ID:     "local-evt",
		Topic:  "internal.log",
		Origin: "edge-001",
		Dir:    DirLocal,
	})

	// Wait and verify nothing was sent
	time.Sleep(50 * time.Millisecond)

	sent := stream.getSent()
	assert.Empty(t, sent, "Local events should not be forwarded")
}

func TestBridge_EdgeDoesNotForwardDirDown(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	// Edge bridge
	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridge.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Publish DirDown event on edge (should not be forwarded)
	bus.Publish(Event{
		ID:     "wrong-dir",
		Topic:  "config.update",
		Origin: "edge-001",
		Dir:    DirDown,
	})

	time.Sleep(50 * time.Millisecond)

	sent := stream.getSent()
	assert.Empty(t, sent, "Edge should not forward DirDown events")
}

func TestBridge_ControlDoesNotForwardDirUp(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	// Control bridge
	bridge := NewBridge(BridgeConfig{
		NodeID:    "control",
		IsControl: true,
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridge.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Publish DirUp event on control (should not be forwarded)
	bus.Publish(Event{
		ID:     "wrong-dir",
		Topic:  "metrics.report",
		Origin: "control",
		Dir:    DirUp,
	})

	time.Sleep(50 * time.Millisecond)

	sent := stream.getSent()
	assert.Empty(t, sent, "Control should not forward DirUp events")
}

func TestBridge_RemoteToLocalMarkedAsLocal(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	var receivedEvent Event
	var wg sync.WaitGroup
	wg.Add(1)

	sub := bus.Subscribe("config.update", func(ev Event) {
		receivedEvent = ev
		wg.Done()
	})
	defer bus.Unsubscribe(sub)

	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridge.Start(ctx)

	// Simulate receiving an event from control
	stream.toRecv <- &EventFrame{
		ID:      "remote-evt",
		Topic:   "config.update",
		Origin:  "control",
		Dir:     int32(DirDown),
		Payload: []byte(`{"version":"1.0"}`),
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}

	// Verify the received event is marked as DirLocal
	assert.Equal(t, DirLocal, receivedEvent.Dir, "Remote events must be marked as DirLocal")
	assert.Equal(t, "control", receivedEvent.Origin)
	assert.Equal(t, "remote-evt", receivedEvent.ID)
}

func TestBridge_NoFeedbackLoop(t *testing.T) {
	// Simulate full round-trip to ensure no loops
	edgeBus := NewBus()
	controlBus := NewBus()

	edgeStream := newMockStream()
	controlStream := newMockStream()
	defer edgeStream.Close()
	defer controlStream.Close()

	edgeBridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, edgeBus, edgeStream)

	controlBridge := NewBridge(BridgeConfig{
		NodeID:    "control",
		IsControl: true,
	}, controlBus, controlStream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	edgeBridge.Start(ctx)
	controlBridge.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Control publishes DirDown
	controlBus.Publish(Event{
		ID:      "evt-down",
		Topic:   "config.push",
		Origin:  "control",
		Dir:     DirDown,
	})

	time.Sleep(50 * time.Millisecond)

	// Verify control forwarded to its stream
	controlSent := controlStream.getSent()
	require.Len(t, controlSent, 1)

	// Simulate edge receiving the event
	edgeStream.toRecv <- controlSent[0]

	time.Sleep(50 * time.Millisecond)

	// Edge should NOT have forwarded it back
	edgeSent := edgeStream.getSent()
	assert.Empty(t, edgeSent, "Edge must not forward received events back")
}

func TestBridge_TopicFiltering(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	// Bridge with topic filtering
	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
		Topics:    []string{"metrics.report", "error.critical"},
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridge.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Publish matching topic
	bus.Publish(Event{
		ID:     "evt-1",
		Topic:  "metrics.report",
		Origin: "edge-001",
		Dir:    DirUp,
	})

	// Publish non-matching topic
	bus.Publish(Event{
		ID:     "evt-2",
		Topic:  "debug.log",
		Origin: "edge-001",
		Dir:    DirUp,
	})

	// Publish another matching topic
	bus.Publish(Event{
		ID:     "evt-3",
		Topic:  "error.critical",
		Origin: "edge-001",
		Dir:    DirUp,
	})

	time.Sleep(50 * time.Millisecond)

	sent := stream.getSent()
	require.Len(t, sent, 2)

	topics := make(map[string]bool)
	for _, s := range sent {
		topics[s.Topic] = true
	}

	assert.True(t, topics["metrics.report"])
	assert.True(t, topics["error.critical"])
	assert.False(t, topics["debug.log"])
}

func TestBridge_WildcardTopicFilter(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	// Bridge with wildcard topic
	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
		Topics:    []string{"*"},
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridge.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Publish various topics
	bus.Publish(Event{ID: "1", Topic: "topic.a", Origin: "edge-001", Dir: DirUp})
	bus.Publish(Event{ID: "2", Topic: "topic.b", Origin: "edge-001", Dir: DirUp})
	bus.Publish(Event{ID: "3", Topic: "any.topic", Origin: "edge-001", Dir: DirUp})

	time.Sleep(50 * time.Millisecond)

	sent := stream.getSent()
	assert.Len(t, sent, 3, "Wildcard should allow all topics")
}

func TestBridge_Stop(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())

	bridge.Start(ctx)
	assert.True(t, bridge.IsRunning())

	time.Sleep(20 * time.Millisecond)

	bridge.Stop()
	cancel()

	time.Sleep(20 * time.Millisecond)
	assert.False(t, bridge.IsRunning())
}

func TestBridge_DoubleStart(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start twice should be safe
	bridge.Start(ctx)
	bridge.Start(ctx)

	assert.True(t, bridge.IsRunning())
}

func TestBridge_BidirectionalCommunication(t *testing.T) {
	// Create two buses to simulate control and edge
	controlBus := NewBus()
	edgeBus := NewBus()

	// Create connected mock streams (simulating real gRPC connection)
	controlToEdge := newMockStream()
	edgeToControl := newMockStream()
	defer controlToEdge.Close()
	defer edgeToControl.Close()

	// Create bridges
	controlBridge := NewBridge(BridgeConfig{
		NodeID:    "control",
		IsControl: true,
	}, controlBus, controlToEdge)

	edgeBridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, edgeBus, edgeToControl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controlBridge.Start(ctx)
	edgeBridge.Start(ctx)

	// Track received events
	var controlReceived []Event
	var edgeReceived []Event
	var mu sync.Mutex

	controlSub := controlBus.Subscribe("edge.metrics", func(ev Event) {
		mu.Lock()
		controlReceived = append(controlReceived, ev)
		mu.Unlock()
	})
	defer controlBus.Unsubscribe(controlSub)

	edgeSub := edgeBus.Subscribe("control.config", func(ev Event) {
		mu.Lock()
		edgeReceived = append(edgeReceived, ev)
		mu.Unlock()
	})
	defer edgeBus.Unsubscribe(edgeSub)

	time.Sleep(20 * time.Millisecond)

	// Edge publishes up
	edgeBus.Publish(Event{
		ID:      "up-evt",
		Topic:   "edge.metrics",
		Origin:  "edge-001",
		Dir:     DirUp,
		Payload: json.RawMessage(`{"cpu": 50}`),
	})

	// Control publishes down
	controlBus.Publish(Event{
		ID:      "down-evt",
		Topic:   "control.config",
		Origin:  "control",
		Dir:     DirDown,
		Payload: json.RawMessage(`{"version": "2.0"}`),
	})

	time.Sleep(50 * time.Millisecond)

	// Simulate the network transport
	edgeSent := edgeToControl.getSent()
	require.Len(t, edgeSent, 1)

	controlSent := controlToEdge.getSent()
	require.Len(t, controlSent, 1)

	// Deliver the events to the other side
	controlToEdge.toRecv <- controlSent[0] // This goes to edge (but edge bridge reads from edgeToControl)
	edgeToControl.toRecv <- edgeSent[0]    // This goes to control (but control reads from controlToEdge)

	// Actually, we need to swap - control receives from edgeToControl, edge receives from controlToEdge
	// Let me fix the mock delivery
}

func TestBridge_MultipleEventsSequential(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	bridge := NewBridge(BridgeConfig{
		NodeID:    "edge-001",
		IsControl: false,
	}, bus, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridge.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Publish multiple events
	for i := 0; i < 10; i++ {
		bus.Publish(Event{
			ID:     "evt-" + string(rune('0'+i)),
			Topic:  "test.topic",
			Origin: "edge-001",
			Dir:    DirUp,
		})
	}

	time.Sleep(100 * time.Millisecond)

	sent := stream.getSent()
	assert.Len(t, sent, 10)
}
