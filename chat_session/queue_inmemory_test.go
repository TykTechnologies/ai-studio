package chat_session

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestInMemoryQueue_PublishAndConsume(t *testing.T) {
	queue := NewInMemoryQueue("test-session", 10)
	defer queue.Close()

	ctx := context.Background()

	// Test publishing and consuming messages
	t.Run("PublishMessage", func(t *testing.T) {
		msg := &ChatResponse{Payload: "test message"}

		err := queue.PublishMessage(ctx, msg)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		select {
		case received := <-queue.ConsumeMessages(ctx):
			if received.Payload != msg.Payload {
				t.Errorf("Expected %q, got %q", msg.Payload, received.Payload)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected to receive message")
		}
	})

	t.Run("PublishStream", func(t *testing.T) {
		data := []byte("test stream data")

		err := queue.PublishStream(ctx, data)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		select {
		case received := <-queue.ConsumeStream(ctx):
			if string(received) != string(data) {
				t.Errorf("Expected %q, got %q", string(data), string(received))
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected to receive stream data")
		}
	})

	t.Run("PublishError", func(t *testing.T) {
		testErr := fmt.Errorf("test error")

		err := queue.PublishError(ctx, testErr)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		select {
		case received := <-queue.ConsumeErrors(ctx):
			if received.Error() != testErr.Error() {
				t.Errorf("Expected %q, got %q", testErr.Error(), received.Error())
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected to receive error")
		}
	})

	t.Run("PublishLLMResponse", func(t *testing.T) {
		resp := &LLMResponseWrapper{
			Response: nil, // nil is fine for test
			Opts:     nil,
		}

		err := queue.PublishLLMResponse(ctx, resp)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		select {
		case received := <-queue.ConsumeLLMResponses(ctx):
			if received != resp {
				t.Error("Expected to receive the same LLMResponseWrapper")
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected to receive LLM response")
		}
	})
}

func TestInMemoryQueue_ContextCancellation(t *testing.T) {
	// Create a queue with buffer size 1 to test blocking behavior
	queue := NewInMemoryQueue("test-session", 1)
	defer queue.Close()

	// Fill the buffer
	ctx := context.Background()
	msg := &ChatResponse{Payload: "first message"}
	err := queue.PublishMessage(ctx, msg)
	if err != nil {
		t.Fatalf("Expected no error filling buffer, got %v", err)
	}

	// Now the buffer is full, next publish should block
	cancelCtx, cancel := context.WithCancel(ctx)

	// Start a goroutine that will cancel the context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// This should block and then return context.Canceled
	msg2 := &ChatResponse{Payload: "second message"}
	err = queue.PublishMessage(cancelCtx, msg2)

	if err == nil {
		t.Error("Expected context cancellation error")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestInMemoryQueue_CloseOperations(t *testing.T) {
	queue := NewInMemoryQueue("test-session", 10)

	ctx := context.Background()

	// Close the queue
	err := queue.Close()
	if err != nil {
		t.Errorf("Expected no error closing queue, got %v", err)
	}

	// Publishing to closed queue should return error
	msg := &ChatResponse{Payload: "test"}
	err = queue.PublishMessage(ctx, msg)
	if err == nil {
		t.Error("Expected error when publishing to closed queue")
	}

	// Closing again should not error
	err = queue.Close()
	if err != nil {
		t.Errorf("Expected no error closing already closed queue, got %v", err)
	}
}

func TestInMemoryQueue_QueueDepth(t *testing.T) {
	queue := NewInMemoryQueue("test-session", 10)
	defer queue.Close()

	ctx := context.Background()

	// Initially empty
	msgs, stream, errors, llmResp := queue.QueueDepth()
	if msgs != 0 || stream != 0 || errors != 0 || llmResp != 0 {
		t.Errorf("Expected all depths to be 0, got %d, %d, %d, %d", msgs, stream, errors, llmResp)
	}

	// Add some items
	queue.PublishMessage(ctx, &ChatResponse{Payload: "test1"})
	queue.PublishMessage(ctx, &ChatResponse{Payload: "test2"})
	queue.PublishStream(ctx, []byte("stream"))
	queue.PublishError(ctx, fmt.Errorf("error"))

	msgs, stream, errors, llmResp = queue.QueueDepth()
	if msgs != 2 || stream != 1 || errors != 1 || llmResp != 0 {
		t.Errorf("Expected depths 2,1,1,0, got %d, %d, %d, %d", msgs, stream, errors, llmResp)
	}

	// After closing, depth should be 0
	queue.Close()
	msgs, stream, errors, llmResp = queue.QueueDepth()
	if msgs != 0 || stream != 0 || errors != 0 || llmResp != 0 {
		t.Errorf("Expected all depths to be 0 after close, got %d, %d, %d, %d", msgs, stream, errors, llmResp)
	}
}

func TestDefaultQueueFactory(t *testing.T) {
	t.Run("CreateQueue with default buffer size", func(t *testing.T) {
		factory := NewDefaultQueueFactory(50)
		queue, err := factory.CreateQueue("test-session", nil)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		inmemQueue, ok := queue.(*InMemoryQueue)
		if !ok {
			t.Error("Expected InMemoryQueue")
		}

		// Check that the buffer size is correct by checking channel capacity
		if cap(inmemQueue.outputMessages) != 50 {
			t.Errorf("Expected buffer size 50, got %d", cap(inmemQueue.outputMessages))
		}

		queue.Close()
	})

	t.Run("CreateQueue with config override", func(t *testing.T) {
		factory := NewDefaultQueueFactory(50)
		config := map[string]interface{}{
			"bufferSize": 200,
		}
		queue, err := factory.CreateQueue("test-session", config)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		inmemQueue, ok := queue.(*InMemoryQueue)
		if !ok {
			t.Error("Expected InMemoryQueue")
		}

		// Check that the buffer size override worked
		if cap(inmemQueue.outputMessages) != 200 {
			t.Errorf("Expected buffer size 200, got %d", cap(inmemQueue.outputMessages))
		}

		queue.Close()
	})

	t.Run("CreateQueue with zero buffer size defaults", func(t *testing.T) {
		factory := NewDefaultQueueFactory(0) // Should default to 100
		queue, err := factory.CreateQueue("test-session", nil)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		inmemQueue, ok := queue.(*InMemoryQueue)
		if !ok {
			t.Error("Expected InMemoryQueue")
		}

		// Check that it defaulted to 100
		if cap(inmemQueue.outputMessages) != 100 {
			t.Errorf("Expected buffer size 100, got %d", cap(inmemQueue.outputMessages))
		}

		queue.Close()
	})
}

func TestNewDefaultInMemoryQueue(t *testing.T) {
	queue := NewDefaultInMemoryQueue("test-session")
	defer queue.Close()

	inmemQueue, ok := queue.(*InMemoryQueue)
	if !ok {
		t.Error("Expected InMemoryQueue")
	}

	// Should have default buffer size of 100
	if cap(inmemQueue.outputMessages) != 100 {
		t.Errorf("Expected buffer size 100, got %d", cap(inmemQueue.outputMessages))
	}
}

func TestPublishWithTimeout(t *testing.T) {
	queue := NewInMemoryQueue("test-session", 1)
	defer queue.Close()

	ctx := context.Background()

	// Fill the buffer
	queue.PublishMessage(ctx, &ChatResponse{Payload: "first"})

	// Now test timeout behavior
	err := PublishWithTimeout(ctx, queue, func(ctx context.Context) error {
		return queue.PublishMessage(ctx, &ChatResponse{Payload: "second"})
	}, 50*time.Millisecond)

	if err == nil {
		t.Error("Expected timeout error")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}
