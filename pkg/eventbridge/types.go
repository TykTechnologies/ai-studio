// Package eventbridge provides an event bus system for communication between
// AI Studio (control) and Microgateway (edge) nodes using simonfxr/pubsub
// for in-process event buses and gRPC for cross-node event forwarding.
package eventbridge

import (
	"encoding/json"
)

// Direction controls event routing and prevents feedback loops.
// Events are routed based on their direction:
// - DirLocal: Events stay on the local bus, never forwarded
// - DirUp: Events flow from edge to control
// - DirDown: Events flow from control to edge(s)
type Direction int

const (
	// DirLocal indicates events that should only be processed locally
	DirLocal Direction = iota
	// DirUp indicates events flowing from edge to control
	DirUp
	// DirDown indicates events flowing from control to edge(s)
	DirDown
)

// String returns a human-readable representation of the Direction.
func (d Direction) String() string {
	switch d {
	case DirLocal:
		return "local"
	case DirUp:
		return "up"
	case DirDown:
		return "down"
	default:
		return "unknown"
	}
}

// DirectionFromInt32 converts a protobuf int32 to Direction.
func DirectionFromInt32(d int32) Direction {
	switch d {
	case 0:
		return DirLocal
	case 1:
		return DirUp
	case 2:
		return DirDown
	default:
		return DirLocal
	}
}

// Event is the canonical event type used on the local bus and for gRPC transport.
// All application code publishes and subscribes using this Event type.
type Event struct {
	// ID is a UUID for deduplication and tracing
	ID string `json:"id"`

	// Topic is the logical topic name (e.g., "config.update", "metrics.report")
	Topic string `json:"topic"`

	// Origin is the node ID that created the event (e.g., "control" or "edge-123")
	Origin string `json:"origin"`

	// Dir controls routing: DirLocal stays local, DirUp goes to control, DirDown goes to edges
	Dir Direction `json:"dir"`

	// Payload is the application-specific data as JSON
	Payload json.RawMessage `json:"payload"`
}

// EventFrame is the wire format for events over gRPC.
// This mirrors the protobuf EventFrame message structure.
type EventFrame struct {
	ID      string
	Topic   string
	Origin  string
	Dir     int32 // 0 = Local, 1 = Up, 2 = Down (matches Direction enum)
	Payload []byte
}

// ToEvent converts an EventFrame to an Event with DirLocal direction.
// This is used when receiving events from remote nodes to prevent re-forwarding.
func (f *EventFrame) ToEvent() Event {
	return Event{
		ID:      f.ID,
		Topic:   f.Topic,
		Origin:  f.Origin,
		Dir:     DirLocal, // Always mark as local to prevent loops
		Payload: f.Payload,
	}
}

// ToEventFrame converts an Event to an EventFrame for wire transmission.
func (e *Event) ToEventFrame() *EventFrame {
	return &EventFrame{
		ID:      e.ID,
		Topic:   e.Topic,
		Origin:  e.Origin,
		Dir:     int32(e.Dir),
		Payload: []byte(e.Payload),
	}
}
