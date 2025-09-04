package chat_session

import (
	"context"
)

// MessageQueue abstracts the message passing mechanism for chat sessions.
// All implementations must guarantee message delivery (no silent drops).
type MessageQueue interface {
	// Publishing methods - all block until successful or context cancelled
	// Returns error only on context cancellation or queue closure
	PublishMessage(ctx context.Context, msg *ChatResponse) error
	PublishStream(ctx context.Context, data []byte) error
	PublishError(ctx context.Context, err error) error
	PublishLLMResponse(ctx context.Context, resp *LLMResponseWrapper) error

	// Consuming methods - returns channels for backward compatibility
	// Channels remain open until Close() is called
	ConsumeMessages(ctx context.Context) <-chan *ChatResponse
	ConsumeStream(ctx context.Context) <-chan []byte
	ConsumeErrors(ctx context.Context) <-chan error
	ConsumeLLMResponses(ctx context.Context) <-chan *LLMResponseWrapper

	// Lifecycle management
	Close() error

	// Metrics for monitoring (optional but useful for debugging)
	QueueDepth() (messages, stream, errors, llmResponses int)
}

// QueueFactory creates queue implementations based on configuration
type QueueFactory interface {
	CreateQueue(sessionID string, config map[string]interface{}) (MessageQueue, error)
}
