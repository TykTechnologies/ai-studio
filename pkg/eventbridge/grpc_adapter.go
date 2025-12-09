package eventbridge

import (
	"context"
	"sync"
)

// ProtoEventFrame is an interface that matches the generated pb.EventFrame
// This allows the adapter to work with the proto types without importing them directly
type ProtoEventFrame interface {
	GetId() string
	GetTopic() string
	GetOrigin() string
	GetDir() int32
	GetPayload() []byte
}

// EventFrameCreator is a function type for creating proto EventFrame objects
type EventFrameCreator func(id, topic, origin string, dir int32, payload []byte) interface{}

// StreamAdapter wraps a gRPC bidirectional stream to implement GRPCStream.
// It provides a channel-based interface for receiving events that can be
// populated from the main message processing loop.
type StreamAdapter struct {
	// SendFunc is called to send an event frame over gRPC
	// The frame parameter is an *EventFrame from this package
	SendFunc func(frame *EventFrame) error

	// eventChan receives events from the main message loop
	eventChan chan *EventFrame

	// mu protects closed state
	mu     sync.RWMutex
	closed bool
}

// NewStreamAdapter creates a new StreamAdapter.
// bufferSize controls the channel buffer for incoming events.
func NewStreamAdapter(sendFunc func(frame *EventFrame) error, bufferSize int) *StreamAdapter {
	if bufferSize < 1 {
		bufferSize = 100
	}
	return &StreamAdapter{
		SendFunc:  sendFunc,
		eventChan: make(chan *EventFrame, bufferSize),
	}
}

// SendEvent implements GRPCStream.SendEvent by calling the configured SendFunc.
func (a *StreamAdapter) SendEvent(frame *EventFrame) error {
	return a.SendFunc(frame)
}

// RecvEvent implements GRPCStream.RecvEvent by reading from the event channel.
// This blocks until an event is available or the adapter is closed.
func (a *StreamAdapter) RecvEvent() (*EventFrame, error) {
	frame, ok := <-a.eventChan
	if !ok {
		return nil, context.Canceled
	}
	return frame, nil
}

// EnqueueEvent adds an event to the receive channel from the main message loop.
// This should be called when an event message is received on the gRPC stream.
// Returns false if the adapter is closed or the channel is full.
func (a *StreamAdapter) EnqueueEvent(frame *EventFrame) bool {
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return false
	}
	a.mu.RUnlock()

	select {
	case a.eventChan <- frame:
		return true
	default:
		// Channel full, drop event
		return false
	}
}

// EnqueueProtoEvent converts a proto EventFrame to our EventFrame and enqueues it.
// This is a convenience method for use in message processing loops.
func (a *StreamAdapter) EnqueueProtoEvent(protoFrame ProtoEventFrame) bool {
	frame := &EventFrame{
		ID:      protoFrame.GetId(),
		Topic:   protoFrame.GetTopic(),
		Origin:  protoFrame.GetOrigin(),
		Dir:     protoFrame.GetDir(),
		Payload: protoFrame.GetPayload(),
	}
	return a.EnqueueEvent(frame)
}

// Close closes the event channel, causing RecvEvent to return an error.
func (a *StreamAdapter) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.closed {
		a.closed = true
		close(a.eventChan)
	}
}

// IsClosed returns whether the adapter has been closed.
func (a *StreamAdapter) IsClosed() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.closed
}
