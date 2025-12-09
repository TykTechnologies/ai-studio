package eventbridge

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/simonfxr/pubsub"
)

// GRPCStream is the interface for sending/receiving event frames over gRPC.
// This abstraction allows the bridge to work with different stream implementations.
type GRPCStream interface {
	// SendEvent sends an event frame to the remote end
	SendEvent(frame *EventFrame) error
	// RecvEvent receives an event frame from the remote end (blocking)
	RecvEvent() (*EventFrame, error)
}

// Bridge connects a local event bus to a gRPC stream, forwarding events
// based on their direction while preventing feedback loops.
//
// The bridge runs two goroutines:
// - localToRemote: subscribes to local bus and forwards events over gRPC
// - remoteToLocal: reads from gRPC and republishes into local bus
//
// Feedback loops are prevented by:
// 1. Remote events are always marked as DirLocal when republished
// 2. Outbound loop only forwards events matching the node's role:
//   - Control node only forwards DirDown events
//   - Edge nodes only forward DirUp events
type Bridge struct {
	nodeID    string
	isControl bool
	bus       Bus
	stream    GRPCStream
	topics    []string // Topics to export (empty = all)

	mu      sync.RWMutex
	running bool
	cancel  context.CancelFunc
	sub     *pubsub.Subscription
}

// BridgeConfig configures the bridge behavior.
type BridgeConfig struct {
	// NodeID is the identifier for this node ("control" or "edge-xxx")
	NodeID string
	// IsControl indicates if this is the control node (true) or an edge (false)
	IsControl bool
	// Topics is an optional list of topics to export. If empty, all topics are exported.
	Topics []string
}

// NewBridge creates a new bridge instance.
// The bridge is not started until Start() is called.
func NewBridge(config BridgeConfig, bus Bus, stream GRPCStream) *Bridge {
	return &Bridge{
		nodeID:    config.NodeID,
		isControl: config.IsControl,
		bus:       bus,
		stream:    stream,
		topics:    config.Topics,
	}
}

// Start begins the bridge loops. This method is non-blocking.
// Call Stop() to halt the bridge.
func (b *Bridge) Start(ctx context.Context) {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.running = true

	ctx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.mu.Unlock()

	go b.localToRemote(ctx)
	go b.remoteToLocal(ctx)

	log.Debug().
		Str("node_id", b.nodeID).
		Bool("is_control", b.isControl).
		Msg("Event bridge started")
}

// Stop halts the bridge loops.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		b.cancel()
	}
	b.running = false

	log.Debug().
		Str("node_id", b.nodeID).
		Msg("Event bridge stopped")
}

// IsRunning returns whether the bridge is currently running.
func (b *Bridge) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.running
}

// localToRemote subscribes to the local bus and forwards events to the remote end.
// Events are filtered based on:
// - Direction: DirLocal events are never forwarded
// - Role: Control only forwards DirDown, edges only forward DirUp
// - Topics: If configured, only matching topics are forwarded
func (b *Bridge) localToRemote(ctx context.Context) {
	log.Debug().
		Str("node_id", b.nodeID).
		Bool("is_control", b.isControl).
		Msg("Bridge localToRemote goroutine started - subscribing to bus")

	sub := b.bus.SubscribeAll(func(ev Event) {
		log.Debug().
			Str("node_id", b.nodeID).
			Bool("is_control", b.isControl).
			Str("topic", ev.Topic).
			Str("event_id", ev.ID).
			Str("dir", ev.Dir.String()).
			Str("origin", ev.Origin).
			Msg("Bridge received event from local bus")

		// Direction filtering: never forward local events
		if ev.Dir == DirLocal {
			log.Debug().
				Str("node_id", b.nodeID).
				Str("event_id", ev.ID).
				Msg("Bridge skipping event - direction is local")
			return
		}

		// Role-based filtering
		if b.isControl && ev.Dir != DirDown {
			log.Debug().
				Str("node_id", b.nodeID).
				Str("event_id", ev.ID).
				Str("dir", ev.Dir.String()).
				Msg("Bridge skipping event - control node only forwards DirDown")
			return
		}
		if !b.isControl && ev.Dir != DirUp {
			log.Debug().
				Str("node_id", b.nodeID).
				Str("event_id", ev.ID).
				Str("dir", ev.Dir.String()).
				Msg("Bridge skipping event - edge node only forwards DirUp")
			return
		}

		// Topic filtering (if configured)
		if len(b.topics) > 0 && !b.topicMatches(ev.Topic) {
			log.Debug().
				Str("node_id", b.nodeID).
				Str("event_id", ev.ID).
				Str("topic", ev.Topic).
				Msg("Bridge skipping event - topic does not match filter")
			return
		}

		frame := ev.ToEventFrame()

		log.Debug().
			Str("node_id", b.nodeID).
			Str("topic", ev.Topic).
			Str("event_id", ev.ID).
			Str("dir", ev.Dir.String()).
			Msg("Bridge forwarding event to remote via stream")

		if err := b.stream.SendEvent(frame); err != nil {
			log.Error().
				Err(err).
				Str("node_id", b.nodeID).
				Str("topic", ev.Topic).
				Str("event_id", ev.ID).
				Msg("Bridge failed to send event to remote")
		} else {
			log.Debug().
				Str("node_id", b.nodeID).
				Str("topic", ev.Topic).
				Str("event_id", ev.ID).
				Str("dir", ev.Dir.String()).
				Msg("Bridge event successfully forwarded to remote")
		}
	})

	b.mu.Lock()
	b.sub = sub
	b.mu.Unlock()

	<-ctx.Done()

	b.bus.Unsubscribe(sub)
}

// remoteToLocal receives events from the remote end and republishes them locally.
// All received events are marked as DirLocal to prevent re-forwarding.
func (b *Bridge) remoteToLocal(ctx context.Context) {
	log.Debug().
		Str("node_id", b.nodeID).
		Bool("is_control", b.isControl).
		Msg("Bridge remoteToLocal goroutine started - waiting for events from remote")

	for {
		select {
		case <-ctx.Done():
			log.Debug().
				Str("node_id", b.nodeID).
				Msg("Bridge remoteToLocal stopping - context done")
			return
		default:
			frame, err := b.stream.RecvEvent()
			if err != nil {
				// Check if context was cancelled
				select {
				case <-ctx.Done():
					log.Debug().
						Str("node_id", b.nodeID).
						Msg("Bridge remoteToLocal stopping - context cancelled during recv")
					return
				default:
				}

				log.Debug().
					Err(err).
					Str("node_id", b.nodeID).
					Msg("Bridge failed to receive event from remote, bridge stopping")
				return
			}

			if frame == nil {
				continue
			}

			log.Debug().
				Str("node_id", b.nodeID).
				Str("topic", frame.Topic).
				Str("event_id", frame.ID).
				Str("origin", frame.Origin).
				Int32("dir", frame.Dir).
				Msg("Bridge received event frame from remote")

			// Convert to local event - CRITICAL: mark as DirLocal to prevent loops
			ev := frame.ToEvent()

			log.Debug().
				Str("node_id", b.nodeID).
				Str("topic", ev.Topic).
				Str("event_id", ev.ID).
				Str("origin", ev.Origin).
				Str("dir", ev.Dir.String()).
				Msg("Bridge publishing remote event to local bus")

			b.bus.Publish(ev)

			log.Debug().
				Str("node_id", b.nodeID).
				Str("topic", ev.Topic).
				Str("event_id", ev.ID).
				Str("origin", ev.Origin).
				Msg("Bridge event received from remote and published locally")
		}
	}
}

// topicMatches checks if a topic matches the configured topic filter.
func (b *Bridge) topicMatches(topic string) bool {
	for _, t := range b.topics {
		if t == topic || t == "*" {
			return true
		}
	}
	return false
}
