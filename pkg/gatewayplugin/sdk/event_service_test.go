// plugins/sdk/event_service_test.go
package sdk

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
	"google.golang.org/grpc"
)

// mockEventStream implements grpc.ServerStreamingServer[pb.EventMessage] for testing
type mockEventStream struct {
	grpc.ServerStream
	ctx      context.Context
	messages []*pb.EventMessage
	mu       sync.Mutex
	sendErr  error
}

func newMockEventStream(ctx context.Context) *mockEventStream {
	return &mockEventStream{
		ctx:      ctx,
		messages: make([]*pb.EventMessage, 0),
	}
}

func (m *mockEventStream) Context() context.Context {
	return m.ctx
}

func (m *mockEventStream) Send(msg *pb.EventMessage) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockEventStream) GetMessages() []*pb.EventMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages
}

func TestNewPluginEventServer(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	if server == nil {
		t.Fatal("NewPluginEventServer returned nil")
	}

	if server.nodeID != "test-node" {
		t.Errorf("Expected nodeID 'test-node', got '%s'", server.nodeID)
	}

	if server.bus == nil {
		t.Error("Expected bus to be set")
	}

	if server.subscriptions == nil {
		t.Error("Expected subscriptions map to be initialized")
	}
}

func TestPluginEventServer_Publish(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	// Subscribe to capture published events
	var receivedEvents []eventbridge.Event
	var mu sync.Mutex
	bus.Subscribe("test.topic", func(ev eventbridge.Event) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, ev)
	})

	// Test successful publish
	ctx := context.Background()
	resp, err := server.Publish(ctx, &pb.PublishRequest{
		Topic:     "test.topic",
		Payload:   []byte(`{"key": "value"}`),
		Direction: 0, // DirLocal
		PluginId:  "test-plugin",
	})

	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success=true, got false: %s", resp.ErrorMessage)
	}

	if resp.EventId == "" {
		t.Error("Expected event ID to be set")
	}

	// Wait for event to be processed
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 received event, got %d", len(receivedEvents))
	}

	ev := receivedEvents[0]
	if ev.Topic != "test.topic" {
		t.Errorf("Expected topic 'test.topic', got '%s'", ev.Topic)
	}

	if ev.Origin != "plugin:test-plugin@test-node" {
		t.Errorf("Expected origin 'plugin:test-plugin@test-node', got '%s'", ev.Origin)
	}
}

func TestPluginEventServer_Publish_EmptyTopic(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	ctx := context.Background()
	resp, err := server.Publish(ctx, &pb.PublishRequest{
		Topic:   "",
		Payload: []byte(`{}`),
	})

	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if resp.Success {
		t.Error("Expected success=false for empty topic")
	}

	if resp.ErrorMessage != "topic is required" {
		t.Errorf("Expected error message 'topic is required', got '%s'", resp.ErrorMessage)
	}
}

func TestPluginEventServer_Subscribe(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	// Create cancellable context for stream
	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockEventStream(ctx)

	// Start subscription in background
	done := make(chan error, 1)
	go func() {
		done <- server.Subscribe(&pb.SubscribeRequest{
			Topic:    "test.topic",
			PluginId: "test-plugin",
		}, stream)
	}()

	// Wait for subscription to be set up
	time.Sleep(50 * time.Millisecond)

	// Check subscription count
	if server.GetActiveSubscriptionCount() != 1 {
		t.Errorf("Expected 1 active subscription, got %d", server.GetActiveSubscriptionCount())
	}

	// Publish an event
	bus.Publish(eventbridge.Event{
		ID:      "test-event-1",
		Topic:   "test.topic",
		Origin:  "test-origin",
		Dir:     eventbridge.DirLocal,
		Payload: []byte(`{"test": true}`),
	})

	// Wait for event to be streamed
	time.Sleep(50 * time.Millisecond)

	// Check received messages
	messages := stream.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if msg.Id != "test-event-1" {
		t.Errorf("Expected event ID 'test-event-1', got '%s'", msg.Id)
	}

	if msg.Topic != "test.topic" {
		t.Errorf("Expected topic 'test.topic', got '%s'", msg.Topic)
	}

	// Cancel context to stop subscription
	cancel()

	// Wait for subscription to clean up
	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Error("Subscribe did not return after context cancellation")
	}

	// Check subscription was cleaned up
	time.Sleep(50 * time.Millisecond)
	if server.GetActiveSubscriptionCount() != 0 {
		t.Errorf("Expected 0 active subscriptions after cleanup, got %d", server.GetActiveSubscriptionCount())
	}
}

func TestPluginEventServer_SubscribeAll(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := newMockEventStream(ctx)

	// Start subscription in background
	done := make(chan error, 1)
	go func() {
		done <- server.Subscribe(&pb.SubscribeRequest{
			SubscribeAll: true,
			PluginId:     "test-plugin",
		}, stream)
	}()

	// Wait for subscription to be set up
	time.Sleep(50 * time.Millisecond)

	// Publish events to different topics
	bus.Publish(eventbridge.Event{
		ID:    "event-1",
		Topic: "topic.one",
	})
	bus.Publish(eventbridge.Event{
		ID:    "event-2",
		Topic: "topic.two",
	})

	// Wait for events to be streamed
	time.Sleep(50 * time.Millisecond)

	// Check we received both events
	messages := stream.GetMessages()
	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages for SubscribeAll, got %d", len(messages))
	}

	cancel()
	<-done
}

func TestPluginEventServer_Subscribe_InvalidRequest(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	ctx := context.Background()
	stream := newMockEventStream(ctx)

	// Test with neither topic nor subscribe_all
	err := server.Subscribe(&pb.SubscribeRequest{
		Topic:        "",
		SubscribeAll: false,
	}, stream)

	if err == nil {
		t.Error("Expected error for invalid request")
	}
}

func TestPluginEventServer_Unsubscribe(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	// Test unsubscribe with non-existent subscription
	ctx := context.Background()
	resp, err := server.Unsubscribe(ctx, &pb.UnsubscribeRequest{
		SubscriptionId: "non-existent",
	})

	if err != nil {
		t.Fatalf("Unsubscribe returned error: %v", err)
	}

	if resp.Success {
		t.Error("Expected success=false for non-existent subscription")
	}
}

func TestPluginEventServer_CleanupAllSubscriptions(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	// Create multiple subscriptions
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 3; i++ {
		stream := newMockEventStream(ctx)
		go func() {
			server.Subscribe(&pb.SubscribeRequest{
				Topic:    "test.topic",
				PluginId: "test-plugin",
			}, stream)
		}()
	}

	// Wait for subscriptions to be set up
	time.Sleep(50 * time.Millisecond)

	if server.GetActiveSubscriptionCount() != 3 {
		t.Errorf("Expected 3 active subscriptions, got %d", server.GetActiveSubscriptionCount())
	}

	// Cleanup all
	server.CleanupAllSubscriptions()

	if server.GetActiveSubscriptionCount() != 0 {
		t.Errorf("Expected 0 active subscriptions after cleanup, got %d", server.GetActiveSubscriptionCount())
	}
}

func TestPluginEventServer_DirectionMapping(t *testing.T) {
	bus := eventbridge.NewBus()
	server := NewPluginEventServer(bus, "test-node")

	tests := []struct {
		name      string
		direction int32
		expected  eventbridge.Direction
	}{
		{"DirLocal", 0, eventbridge.DirLocal},
		{"DirUp", 1, eventbridge.DirUp},
		{"DirDown", 2, eventbridge.DirDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received eventbridge.Direction
			bus.SubscribeAll(func(ev eventbridge.Event) {
				received = ev.Dir
			})

			server.Publish(context.Background(), &pb.PublishRequest{
				Topic:     "direction.test",
				Direction: tt.direction,
			})

			time.Sleep(10 * time.Millisecond)

			if received != tt.expected {
				t.Errorf("Expected direction %v, got %v", tt.expected, received)
			}
		})
	}
}
