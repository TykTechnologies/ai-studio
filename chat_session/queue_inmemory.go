package chat_session

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InMemoryQueue implements MessageQueue using Go channels
// Maintains exact compatibility with current ChatSession channel behavior
type InMemoryQueue struct {
	sessionID      string
	outputMessages chan *ChatResponse
	outputStream   chan []byte
	errors         chan error
	llmResponses   chan *LLMResponseWrapper
	closed         bool
	closeMux       sync.RWMutex
}

// NewInMemoryQueue creates a new in-memory queue with specified buffer sizes
func NewInMemoryQueue(sessionID string, bufferSize int) *InMemoryQueue {
	return &InMemoryQueue{
		sessionID:      sessionID,
		outputMessages: make(chan *ChatResponse, bufferSize),
		outputStream:   make(chan []byte, bufferSize),
		errors:         make(chan error, bufferSize),
		llmResponses:   make(chan *LLMResponseWrapper, bufferSize),
		closed:         false,
	}
}

// PublishMessage sends a message, blocking until successful or context cancelled
func (q *InMemoryQueue) PublishMessage(ctx context.Context, msg *ChatResponse) error {
	q.closeMux.RLock()
	defer q.closeMux.RUnlock()

	if q.closed {
		return fmt.Errorf("queue closed")
	}

	select {
	case q.outputMessages <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishStream sends stream data, blocking until successful or context cancelled
func (q *InMemoryQueue) PublishStream(ctx context.Context, data []byte) error {
	q.closeMux.RLock()
	defer q.closeMux.RUnlock()

	if q.closed {
		return fmt.Errorf("queue closed")
	}

	select {
	case q.outputStream <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishError sends an error, blocking until successful or context cancelled
func (q *InMemoryQueue) PublishError(ctx context.Context, err error) error {
	q.closeMux.RLock()
	defer q.closeMux.RUnlock()

	if q.closed {
		return fmt.Errorf("queue closed")
	}

	select {
	case q.errors <- err:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishLLMResponse sends an LLM response, blocking until successful or context cancelled
func (q *InMemoryQueue) PublishLLMResponse(ctx context.Context, resp *LLMResponseWrapper) error {
	q.closeMux.RLock()
	defer q.closeMux.RUnlock()

	if q.closed {
		return fmt.Errorf("queue closed")
	}

	select {
	case q.llmResponses <- resp:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConsumeMessages returns the output messages channel
func (q *InMemoryQueue) ConsumeMessages(ctx context.Context) <-chan *ChatResponse {
	return q.outputMessages
}

// ConsumeStream returns the output stream channel
func (q *InMemoryQueue) ConsumeStream(ctx context.Context) <-chan []byte {
	return q.outputStream
}

// ConsumeErrors returns the errors channel
func (q *InMemoryQueue) ConsumeErrors(ctx context.Context) <-chan error {
	return q.errors
}

// ConsumeLLMResponses returns the LLM responses channel
func (q *InMemoryQueue) ConsumeLLMResponses(ctx context.Context) <-chan *LLMResponseWrapper {
	return q.llmResponses
}

// Close closes all channels and marks the queue as closed
func (q *InMemoryQueue) Close() error {
	q.closeMux.Lock()
	defer q.closeMux.Unlock()

	if q.closed {
		return nil // Already closed
	}

	q.closed = true
	close(q.outputMessages)
	close(q.outputStream)
	close(q.errors)
	close(q.llmResponses)

	return nil
}

// QueueDepth returns the current depth of all queues
func (q *InMemoryQueue) QueueDepth() (messages, stream, errors, llmResponses int) {
	q.closeMux.RLock()
	defer q.closeMux.RUnlock()

	if q.closed {
		return 0, 0, 0, 0
	}

	return len(q.outputMessages), len(q.outputStream), len(q.errors), len(q.llmResponses)
}

// DefaultQueueFactory creates InMemoryQueue instances
type DefaultQueueFactory struct {
	defaultBufferSize int
}

// NewDefaultQueueFactory creates a new factory with specified default buffer size
func NewDefaultQueueFactory(defaultBufferSize int) *DefaultQueueFactory {
	if defaultBufferSize <= 0 {
		defaultBufferSize = 100 // Match current ChatSession default
	}

	return &DefaultQueueFactory{
		defaultBufferSize: defaultBufferSize,
	}
}

// CreateQueue creates a new InMemoryQueue with configuration overrides
func (f *DefaultQueueFactory) CreateQueue(sessionID string, config map[string]interface{}) (MessageQueue, error) {
	bufferSize := f.defaultBufferSize

	if config != nil {
		if size, ok := config["bufferSize"].(int); ok && size > 0 {
			bufferSize = size
		}
	}

	return NewInMemoryQueue(sessionID, bufferSize), nil
}

// Helper function for creating a queue with default settings (backward compatibility)
func NewDefaultInMemoryQueue(sessionID string) MessageQueue {
	return NewInMemoryQueue(sessionID, 100)
}

// Helper function that can be used in sendStatus with proper timeout
func PublishWithTimeout(ctx context.Context, queue MessageQueue, publisher func(context.Context) error, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second // Default timeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return publisher(timeoutCtx)
}
