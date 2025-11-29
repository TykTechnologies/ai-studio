// plugins/sdk/event_service.go
// Host-side implementation of PluginEventService for microgateway.
// This server is registered on the go-plugin broker and called by plugins
// to publish events and subscribe to event streams.
package sdk

import (
	"context"
	"fmt"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/simonfxr/pubsub"
	"google.golang.org/grpc"
)

// PluginEventServer implements the PluginEventServiceServer interface.
// It bridges plugin event requests to the local eventbridge.Bus.
type PluginEventServer struct {
	pb.UnimplementedPluginEventServiceServer

	// bus is the local event bus where events are published and subscribed
	bus eventbridge.Bus

	// nodeID identifies this node (e.g., "edge-xyz" or "control")
	nodeID string

	// mu protects the subscriptions map
	mu sync.RWMutex

	// subscriptions tracks active stream subscriptions for cleanup
	// Key is the stream context, value is the pubsub subscription
	subscriptions map[string]*activePluginSubscription
}

// activePluginSubscription tracks an active subscription for a plugin
type activePluginSubscription struct {
	sub      *pubsub.Subscription
	pluginID string
	topic    string
	isAll    bool
}

// NewPluginEventServer creates a new PluginEventServer instance.
// The bus parameter is the local event bus to publish/subscribe to.
// The nodeID identifies this node for event origin tracking.
func NewPluginEventServer(bus eventbridge.Bus, nodeID string) *PluginEventServer {
	return &PluginEventServer{
		bus:           bus,
		nodeID:        nodeID,
		subscriptions: make(map[string]*activePluginSubscription),
	}
}

// Publish receives an event from a plugin and publishes it to the local bus.
// The event will be routed based on its direction field.
func (s *PluginEventServer) Publish(ctx context.Context, req *pb.PublishRequest) (*pb.PublishResponse, error) {
	if req.Topic == "" {
		return &pb.PublishResponse{
			Success:      false,
			ErrorMessage: "topic is required",
		}, nil
	}

	// Generate event ID
	eventID := uuid.NewString()

	// Determine origin - use plugin ID if provided, otherwise use node ID
	origin := s.nodeID
	if req.PluginId != "" {
		origin = fmt.Sprintf("plugin:%s@%s", req.PluginId, s.nodeID)
	}

	// Convert direction from proto int32 to eventbridge.Direction
	dir := eventbridge.DirectionFromInt32(req.Direction)

	// Create and publish event
	event := eventbridge.Event{
		ID:      eventID,
		Topic:   req.Topic,
		Origin:  origin,
		Dir:     dir,
		Payload: req.Payload,
	}

	s.bus.Publish(event)

	log.Debug().
		Str("event_id", eventID).
		Str("topic", req.Topic).
		Str("origin", origin).
		Str("direction", dir.String()).
		Str("plugin_id", req.PluginId).
		Int("payload_len", len(req.Payload)).
		Msg("Plugin published event to bus")

	return &pb.PublishResponse{
		Success: true,
		EventId: eventID,
	}, nil
}

// Subscribe creates a subscription and streams events back to the plugin.
// The stream remains open until the client disconnects.
func (s *PluginEventServer) Subscribe(req *pb.SubscribeRequest, stream grpc.ServerStreamingServer[pb.EventMessage]) error {
	log.Debug().
		Str("plugin_id", req.PluginId).
		Str("topic", req.Topic).
		Bool("subscribe_all", req.SubscribeAll).
		Str("node_id", s.nodeID).
		Msg("🔔 PluginEventServer.Subscribe called by plugin")

	// Validate request
	if !req.SubscribeAll && req.Topic == "" {
		log.Warn().Str("plugin_id", req.PluginId).Msg("Subscribe failed: no topic or subscribe_all specified")
		return fmt.Errorf("either topic or subscribe_all must be specified")
	}

	// Generate subscription ID for tracking
	subID := uuid.NewString()

	// Create a channel to receive events
	eventCh := make(chan eventbridge.Event, 100) // Buffer to prevent blocking

	// Create subscription handler
	handler := func(ev eventbridge.Event) {
		log.Debug().
			Str("subscription_id", subID).
			Str("plugin_id", req.PluginId).
			Str("event_id", ev.ID).
			Str("topic", ev.Topic).
			Str("origin", ev.Origin).
			Str("dir", ev.Dir.String()).
			Msg("PluginEventServer handler received event from bus")

		select {
		case eventCh <- ev:
			log.Debug().
				Str("subscription_id", subID).
				Str("plugin_id", req.PluginId).
				Str("event_id", ev.ID).
				Msg("PluginEventServer enqueued event to plugin channel")
		default:
			// Channel full, log and drop event
			log.Warn().
				Str("subscription_id", subID).
				Str("plugin_id", req.PluginId).
				Str("event_id", ev.ID).
				Msg("Plugin event channel full, dropping event")
		}
	}

	// Subscribe to bus
	var sub *pubsub.Subscription
	if req.SubscribeAll {
		sub = s.bus.SubscribeAll(handler)
	} else {
		sub = s.bus.Subscribe(req.Topic, handler)
	}

	// Track subscription for cleanup
	s.mu.Lock()
	s.subscriptions[subID] = &activePluginSubscription{
		sub:      sub,
		pluginID: req.PluginId,
		topic:    req.Topic,
		isAll:    req.SubscribeAll,
	}
	s.mu.Unlock()

	log.Debug().
		Str("subscription_id", subID).
		Str("plugin_id", req.PluginId).
		Str("topic", req.Topic).
		Bool("subscribe_all", req.SubscribeAll).
		Msg("Plugin subscribed to events")

	// Cleanup when stream closes
	defer func() {
		s.mu.Lock()
		if activeSub, exists := s.subscriptions[subID]; exists {
			s.bus.Unsubscribe(activeSub.sub)
			delete(s.subscriptions, subID)
		}
		s.mu.Unlock()
		close(eventCh)

		log.Debug().
			Str("subscription_id", subID).
			Str("plugin_id", req.PluginId).
			Msg("Plugin subscription cleaned up")
	}()

	// Stream events to plugin
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context cancelled
			return ctx.Err()

		case ev, ok := <-eventCh:
			if !ok {
				// Channel closed
				return nil
			}

			// Convert to proto message
			msg := &pb.EventMessage{
				Id:        ev.ID,
				Topic:     ev.Topic,
				Origin:    ev.Origin,
				Direction: int32(ev.Dir),
				Payload:   ev.Payload,
			}

			// Send to plugin
			if err := stream.Send(msg); err != nil {
				log.Debug().
					Err(err).
					Str("subscription_id", subID).
					Str("plugin_id", req.PluginId).
					Str("event_id", ev.ID).
					Msg("Failed to send event to plugin")
				return err
			}

			log.Trace().
				Str("subscription_id", subID).
				Str("plugin_id", req.PluginId).
				Str("event_id", ev.ID).
				Str("topic", ev.Topic).
				Msg("Event streamed to plugin")
		}
	}
}

// Unsubscribe removes a subscription by ID.
// Note: This is optional since subscriptions are automatically cleaned up
// when the Subscribe stream closes.
func (s *PluginEventServer) Unsubscribe(ctx context.Context, req *pb.UnsubscribeRequest) (*pb.UnsubscribeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if activeSub, exists := s.subscriptions[req.SubscriptionId]; exists {
		s.bus.Unsubscribe(activeSub.sub)
		delete(s.subscriptions, req.SubscriptionId)

		log.Debug().
			Str("subscription_id", req.SubscriptionId).
			Str("plugin_id", activeSub.pluginID).
			Msg("Plugin subscription unsubscribed")

		return &pb.UnsubscribeResponse{
			Success: true,
		}, nil
	}

	return &pb.UnsubscribeResponse{
		Success:      false,
		ErrorMessage: fmt.Sprintf("subscription %s not found", req.SubscriptionId),
	}, nil
}

// GetActiveSubscriptionCount returns the number of active subscriptions.
// Useful for monitoring and debugging.
func (s *PluginEventServer) GetActiveSubscriptionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscriptions)
}

// CleanupAllSubscriptions removes all active subscriptions.
// Called during shutdown.
func (s *PluginEventServer) CleanupAllSubscriptions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for subID, activeSub := range s.subscriptions {
		s.bus.Unsubscribe(activeSub.sub)
		log.Debug().
			Str("subscription_id", subID).
			Str("plugin_id", activeSub.pluginID).
			Msg("Cleaned up plugin subscription during shutdown")
	}
	s.subscriptions = make(map[string]*activePluginSubscription)
}
