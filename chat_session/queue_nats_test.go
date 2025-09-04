package chat_session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/nats-io/nats.go"
)

// MockNATSConnection for testing without real NATS server
type MockNATSQueue struct {
	*InMemoryQueue
	config NATSConfig
	closed bool
}

func NewMockNATSQueue(sessionID string, config NATSConfig) *MockNATSQueue {
	inMemQueue := NewInMemoryQueue(sessionID, config.BufferSize)
	return &MockNATSQueue{
		InMemoryQueue: inMemQueue,
		config:        config,
		closed:        false,
	}
}

func (m *MockNATSQueue) Close() error {
	m.closed = true
	return m.InMemoryQueue.Close()
}

func TestDefaultNATSConfig(t *testing.T) {
	config := DefaultNATSConfig()

	// Verify default values match our Option 3 hybrid persistent configuration
	if config.URL != "nats://localhost:4222" {
		t.Errorf("Expected default URL to be nats://localhost:4222, got %s", config.URL)
	}

	if config.StorageType != "file" {
		t.Errorf("Expected default StorageType to be file, got %s", config.StorageType)
	}

	if config.RetentionPolicy != "interest" {
		t.Errorf("Expected default RetentionPolicy to be interest, got %s", config.RetentionPolicy)
	}

	if config.MaxAge != 2*time.Hour {
		t.Errorf("Expected default MaxAge to be 2h, got %v", config.MaxAge)
	}

	if config.MaxBytes != 100*1024*1024 {
		t.Errorf("Expected default MaxBytes to be 100MB, got %d", config.MaxBytes)
	}

	if !config.DurableConsumer {
		t.Error("Expected default DurableConsumer to be true")
	}

	if config.AckWait != 30*time.Second {
		t.Errorf("Expected default AckWait to be 30s, got %v", config.AckWait)
	}

	if config.MaxDeliver != 3 {
		t.Errorf("Expected default MaxDeliver to be 3, got %d", config.MaxDeliver)
	}

	if config.BufferSize != 100 {
		t.Errorf("Expected default BufferSize to be 100, got %d", config.BufferSize)
	}
}

func TestNATSQueueFactory(t *testing.T) {
	config := DefaultNATSConfig()
	factory := NewNATSQueueFactory(config)

	if factory == nil {
		t.Fatal("Expected factory to be created")
	}

	// Test configuration override
	sessionConfig := map[string]interface{}{
		"bufferSize": 200,
		"maxAge":     "1h",
	}

	// Since we can't easily test against real NATS without a server,
	// we'll test the factory creation logic indirectly through config
	if factory.config.BufferSize != 100 {
		t.Errorf("Expected factory config BufferSize to be 100, got %d", factory.config.BufferSize)
	}

	// Test that factory can be created successfully
	_, err := factory.CreateQueue("test-session", sessionConfig)
	// We expect this to fail without NATS server, which is fine for unit test
	if err == nil {
		t.Log("Factory created queue successfully (unexpected in unit test without NATS server)")
	} else {
		t.Logf("Expected error creating queue without NATS server: %v", err)
	}
}

func TestCreateQueueFactory(t *testing.T) {
	tests := []struct {
		name        string
		config      config.QueueConfig
		expectError bool
		expectType  string
	}{
		{
			name: "InMemory Queue Config",
			config: config.QueueConfig{
				Type:       "inmemory",
				BufferSize: 200,
			},
			expectError: false,
			expectType:  "inmemory",
		},
		{
			name: "NATS Queue Config",
			config: config.QueueConfig{
				Type:       "nats",
				BufferSize: 300,
				NATS: config.NATSConfig{
					URL:             "nats://test:4222",
					StorageType:     "memory",
					RetentionPolicy: "limits",
					MaxAge:          "1h",
					MaxBytes:        50 * 1024 * 1024,
					DurableConsumer: false,
					AckWait:         "15s",
					MaxDeliver:      5,
				},
			},
			expectError: false,
			expectType:  "nats",
		},
		{
			name: "Invalid Queue Type",
			config: config.QueueConfig{
				Type:       "invalid",
				BufferSize: 100,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, err := CreateQueueFactory(tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error for invalid config")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if factory == nil {
				t.Error("Expected factory to be created")
				return
			}

			// Test factory type indirectly
			switch tt.expectType {
			case "inmemory":
				// For inmemory, we can create and test the queue
				queue, err := factory.CreateQueue("test", nil)
				if err != nil {
					t.Errorf("Failed to create inmemory queue: %v", err)
					return
				}

				if _, ok := queue.(*InMemoryQueue); !ok {
					t.Error("Expected InMemoryQueue")
				}
				queue.Close()

			case "nats":
				// For NATS, we expect it to be NATSQueueFactory
				if _, ok := factory.(*NATSQueueFactory); !ok {
					t.Error("Expected NATSQueueFactory")
				}
			}
		})
	}
}

func TestCreateDefaultQueue(t *testing.T) {
	// Test with current configuration (should default to inmemory)
	queue, err := CreateDefaultQueue("test-session")
	if err != nil {
		t.Errorf("Failed to create default queue: %v", err)
		return
	}

	if queue == nil {
		t.Fatal("Expected queue to be created")
	}

	// Should be in-memory queue by default
	if _, ok := queue.(*InMemoryQueue); !ok {
		t.Error("Expected default queue to be InMemoryQueue")
	}

	queue.Close()
}

func TestNATSConfigurationConversion(t *testing.T) {
	// Test configuration conversion in createNATSQueueFactory
	cfg := config.QueueConfig{
		Type:       "nats",
		BufferSize: 500,
		NATS: config.NATSConfig{
			URL:             "nats://custom:4222",
			StorageType:     "memory",
			RetentionPolicy: "workqueue",
			MaxAge:          "30m",
			MaxBytes:        200 * 1024 * 1024,
			DurableConsumer: false,
			AckWait:         "45s",
			MaxDeliver:      2,
		},
	}

	factory, err := createNATSQueueFactory(cfg)
	if err != nil {
		t.Errorf("Failed to create NATS factory: %v", err)
		return
	}

	natsFactory, ok := factory.(*NATSQueueFactory)
	if !ok {
		t.Fatal("Expected NATSQueueFactory")
	}

	// Verify configuration conversion
	if natsFactory.config.URL != "nats://custom:4222" {
		t.Errorf("Expected URL nats://custom:4222, got %s", natsFactory.config.URL)
	}

	if natsFactory.config.StorageType != "memory" {
		t.Errorf("Expected StorageType memory, got %s", natsFactory.config.StorageType)
	}

	if natsFactory.config.RetentionPolicy != "workqueue" {
		t.Errorf("Expected RetentionPolicy workqueue, got %s", natsFactory.config.RetentionPolicy)
	}

	if natsFactory.config.MaxAge != 30*time.Minute {
		t.Errorf("Expected MaxAge 30m, got %v", natsFactory.config.MaxAge)
	}

	if natsFactory.config.MaxBytes != 200*1024*1024 {
		t.Errorf("Expected MaxBytes 200MB, got %d", natsFactory.config.MaxBytes)
	}

	if natsFactory.config.DurableConsumer != false {
		t.Errorf("Expected DurableConsumer false, got %t", natsFactory.config.DurableConsumer)
	}

	if natsFactory.config.AckWait != 45*time.Second {
		t.Errorf("Expected AckWait 45s, got %v", natsFactory.config.AckWait)
	}

	if natsFactory.config.MaxDeliver != 2 {
		t.Errorf("Expected MaxDeliver 2, got %d", natsFactory.config.MaxDeliver)
	}

	if natsFactory.config.BufferSize != 500 {
		t.Errorf("Expected BufferSize 500, got %d", natsFactory.config.BufferSize)
	}
}

func TestNATSConfigurationDefaults(t *testing.T) {
	// Test with invalid duration strings - should fall back to defaults
	cfg := config.QueueConfig{
		Type:       "nats",
		BufferSize: 100,
		NATS: config.NATSConfig{
			URL:             "nats://localhost:4222",
			StorageType:     "file",
			RetentionPolicy: "interest",
			MaxAge:          "invalid-duration",
			MaxBytes:        0,
			DurableConsumer: true,
			AckWait:         "invalid-duration",
			MaxDeliver:      3,
		},
	}

	factory, err := createNATSQueueFactory(cfg)
	if err != nil {
		t.Errorf("Failed to create NATS factory: %v", err)
		return
	}

	natsFactory, ok := factory.(*NATSQueueFactory)
	if !ok {
		t.Fatal("Expected NATSQueueFactory")
	}

	// Verify defaults are used for invalid values
	if natsFactory.config.MaxAge != 2*time.Hour {
		t.Errorf("Expected default MaxAge 2h for invalid duration, got %v", natsFactory.config.MaxAge)
	}

	if natsFactory.config.AckWait != 30*time.Second {
		t.Errorf("Expected default AckWait 30s for invalid duration, got %v", natsFactory.config.AckWait)
	}

	if natsFactory.config.MaxBytes != 100*1024*1024 {
		t.Errorf("Expected default MaxBytes 100MB for zero value, got %d", natsFactory.config.MaxBytes)
	}
}

func TestNewDefaultNATSQueue(t *testing.T) {
	// Test helper function
	queue, err := NewDefaultNATSQueue("test-session", "nats://test:4222")

	// We expect this to fail without a real NATS server
	if err == nil {
		t.Log("Unexpectedly succeeded in creating NATS queue without server")
		queue.Close()
	} else {
		t.Logf("Expected error creating NATS queue without server: %v", err)
	}
}

func TestQueueFactoryIntegration(t *testing.T) {
	// Test queue factory creation directly (without ChatSession complexity)
	factory := NewDefaultQueueFactory(100)

	queue, err := factory.CreateQueue("test-session", nil)
	if err != nil {
		t.Errorf("Failed to create queue: %v", err)
		return
	}

	if queue == nil {
		t.Fatal("Expected queue to be created")
	}

	// Verify it's an in-memory queue
	if _, ok := queue.(*InMemoryQueue); !ok {
		t.Error("Expected InMemoryQueue")
	}

	queue.Close()
}

func TestNATSConnection(t *testing.T) {
	// First, test basic NATS connection without JetStream complexity
	conn, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS server not available: %v", err)
		return
	}
	defer conn.Close()

	// Test JetStream context
	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("JetStream not available: %v", err)
	}

	// Test simple stream creation with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	streamName := "TEST_STREAM"
	subject := "test.subject"

	streamConfig := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subject},
		Storage:   nats.MemoryStorage, // Use memory storage for test
		Retention: nats.InterestPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  10 * 1024 * 1024,
	}

	// Try to add stream with context
	stream, err := js.AddStream(streamConfig, nats.Context(ctx))
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		t.Fatalf("Failed to create test stream: %v", err)
	}

	// Clean up
	if stream != nil {
		js.DeleteStream(streamName)
	}

	t.Log("Successfully connected to NATS with JetStream!")
}

func TestNATSQueueIntegration(t *testing.T) {
	// Test the actual NATS queue implementation
	config := DefaultNATSConfig()
	config.URL = "nats://localhost:4222"
	config.StorageType = "memory" // Use memory for faster tests
	config.MaxAge = 1 * time.Hour // Shorter for tests

	queue, err := NewNATSQueue("integration-test", config)
	if err != nil {
		t.Skipf("NATS queue creation failed: %v", err)
		return
	}
	defer queue.Close()

	// Test message publishing and consuming
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test publishing a message
	testMsg := &ChatResponse{Payload: "Hello NATS!"}
	err = queue.PublishMessage(ctx, testMsg)
	if err != nil {
		t.Fatalf("Failed to publish message: %v", err)
	}

	// Give a moment for message to be processed
	time.Sleep(100 * time.Millisecond)

	// Test consuming the message
	select {
	case receivedMsg := <-queue.ConsumeMessages(ctx):
		if receivedMsg.Payload != testMsg.Payload {
			t.Errorf("Expected %s, got %s", testMsg.Payload, receivedMsg.Payload)
		}
		t.Logf("Successfully published and consumed message: %s", receivedMsg.Payload)
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for message")
	}
}

func TestQueueSubjectGeneration(t *testing.T) {
	// Test subject generation logic
	config := DefaultNATSConfig()

	// We'll test this indirectly by checking that different message types
	// would generate different subjects
	sessionID := "test-session-123"

	expectedSubjects := map[string]string{
		MessageTypeChatResponse: fmt.Sprintf("chat.sessions.%s.%s", sessionID, MessageTypeChatResponse),
		MessageTypeStream:       fmt.Sprintf("chat.sessions.%s.%s", sessionID, MessageTypeStream),
		MessageTypeError:        fmt.Sprintf("chat.sessions.%s.%s", sessionID, MessageTypeError),
		MessageTypeLLMResponse:  fmt.Sprintf("chat.sessions.%s.%s", sessionID, MessageTypeLLMResponse),
	}

	// Create a mock queue to test subject generation
	queue := &NATSQueue{sessionID: sessionID, config: config}

	for msgType, expectedSubject := range expectedSubjects {
		subject := queue.getSubject(msgType)
		if subject != expectedSubject {
			t.Errorf("Expected subject %s for type %s, got %s", expectedSubject, msgType, subject)
		}
	}
}

// Integration test for message type constants
func TestMessageTypeConstants(t *testing.T) {
	expectedTypes := map[string]string{
		"chat_response": MessageTypeChatResponse,
		"stream":        MessageTypeStream,
		"error":         MessageTypeError,
		"llm_response":  MessageTypeLLMResponse,
	}

	for expected, actual := range expectedTypes {
		if actual != expected {
			t.Errorf("Expected constant %s to equal %s, got %s", expected, expected, actual)
		}
	}
}

// Benchmark tests for performance comparison
func BenchmarkInMemoryQueue(b *testing.B) {
	queue := NewInMemoryQueue("bench-session", 1000)
	defer queue.Close()

	ctx := context.Background()
	msg := &ChatResponse{Payload: "benchmark message"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			queue.PublishMessage(ctx, msg)
		}
	})
}

func BenchmarkQueueFactory(b *testing.B) {
	factory := NewDefaultQueueFactory(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sessionID := fmt.Sprintf("bench-session-%d", i)
		queue, err := factory.CreateQueue(sessionID, nil)
		if err != nil {
			b.Fatalf("Failed to create queue: %v", err)
		}
		queue.Close()
	}
}
