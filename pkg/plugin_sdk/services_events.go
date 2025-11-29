package plugin_sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"

	pb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
)

// Global state for event service client access (similar pattern to ai_studio_sdk)
var (
	eventServiceClient   pb.PluginEventServiceClient
	eventServiceBroker   *goplugin.GRPCBroker
	eventServiceBrokerID uint32
	eventServiceMutex    sync.Mutex
)

// SetEventServiceBroker stores the broker for event service access.
// This is called during plugin initialization.
func SetEventServiceBroker(broker *goplugin.GRPCBroker) {
	eventServiceMutex.Lock()
	defer eventServiceMutex.Unlock()
	eventServiceBroker = broker
}

// SetEventServiceBrokerID stores the broker ID for dialing the event service.
// This is called when the plugin receives the broker ID from config.
func SetEventServiceBrokerID(brokerID uint32) {
	eventServiceMutex.Lock()
	defer eventServiceMutex.Unlock()
	eventServiceBrokerID = brokerID
	// Reset client so it will be recreated with new broker ID
	eventServiceClient = nil
}

// getEventServiceClient creates and returns the event service client.
func getEventServiceClient() (pb.PluginEventServiceClient, error) {
	eventServiceMutex.Lock()
	defer eventServiceMutex.Unlock()

	if eventServiceClient != nil {
		return eventServiceClient, nil
	}

	if eventServiceBroker == nil {
		return nil, fmt.Errorf("event service broker not initialized")
	}

	if eventServiceBrokerID == 0 {
		return nil, fmt.Errorf("event service broker ID not set")
	}

	// Dial the brokered server where the event service is registered
	conn, err := eventServiceBroker.Dial(eventServiceBrokerID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial event service broker ID %d: %w", eventServiceBrokerID, err)
	}

	eventServiceClient = pb.NewPluginEventServiceClient(conn)
	log.Info().
		Uint32("broker_id", eventServiceBrokerID).
		Msg("Event service client created via broker dial")

	return eventServiceClient, nil
}

// eventServiceImpl implements EventService for plugins using the go-plugin broker.
// It communicates with the host's event service via gRPC.
type eventServiceImpl struct {
	client   pb.PluginEventServiceClient
	pluginID string

	mu            sync.RWMutex
	subscriptions map[string]*activeSubscription
}

// activeSubscription tracks an active event subscription
type activeSubscription struct {
	topic    string
	handler  EventHandler
	cancel   context.CancelFunc
	all      bool // true if this is a SubscribeAll subscription
}

// newEventService creates a new event service implementation.
// client is the gRPC client for the PluginEventService.
// pluginID is used for origin tracking in published events.
func newEventService(client pb.PluginEventServiceClient, pluginID string) EventService {
	return &eventServiceImpl{
		client:        client,
		pluginID:      pluginID,
		subscriptions: make(map[string]*activeSubscription),
	}
}

// Publish sends an event to the event bus.
func (e *eventServiceImpl) Publish(ctx context.Context, topic string, payload interface{}, dir Direction) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}
	return e.PublishRaw(ctx, topic, data, dir)
}

// PublishRaw sends an event with a pre-encoded JSON payload.
func (e *eventServiceImpl) PublishRaw(ctx context.Context, topic string, payload []byte, dir Direction) error {
	if e.client == nil {
		return fmt.Errorf("event service not initialized")
	}

	resp, err := e.client.Publish(ctx, &pb.PublishRequest{
		Topic:     topic,
		Payload:   payload,
		Direction: int32(dir),
		PluginId:  e.pluginID,
	})
	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("event publish failed: %s", resp.ErrorMessage)
	}

	log.Trace().
		Str("topic", topic).
		Str("event_id", resp.EventId).
		Str("dir", dir.String()).
		Msg("Event published from plugin")

	return nil
}

// Subscribe registers a handler for events on a specific topic.
func (e *eventServiceImpl) Subscribe(topic string, handler EventHandler) (string, error) {
	if e.client == nil {
		return "", fmt.Errorf("event service not initialized")
	}

	if handler == nil {
		return "", fmt.Errorf("event handler cannot be nil")
	}

	subID := uuid.NewString()
	ctx, cancel := context.WithCancel(context.Background())

	sub := &activeSubscription{
		topic:   topic,
		handler: handler,
		cancel:  cancel,
		all:     false,
	}

	e.mu.Lock()
	e.subscriptions[subID] = sub
	e.mu.Unlock()

	// Start background goroutine to receive events
	go e.runSubscription(ctx, subID, sub, &pb.SubscribeRequest{
		Topic:        topic,
		SubscribeAll: false,
		PluginId:     e.pluginID,
	})

	log.Debug().
		Str("subscription_id", subID).
		Str("topic", topic).
		Msg("Plugin subscribed to event topic")

	return subID, nil
}

// SubscribeAll registers a handler for all events regardless of topic.
func (e *eventServiceImpl) SubscribeAll(handler EventHandler) (string, error) {
	if e.client == nil {
		return "", fmt.Errorf("event service not initialized")
	}

	if handler == nil {
		return "", fmt.Errorf("event handler cannot be nil")
	}

	subID := uuid.NewString()
	ctx, cancel := context.WithCancel(context.Background())

	sub := &activeSubscription{
		topic:   "*",
		handler: handler,
		cancel:  cancel,
		all:     true,
	}

	e.mu.Lock()
	e.subscriptions[subID] = sub
	e.mu.Unlock()

	// Start background goroutine to receive events
	go e.runSubscription(ctx, subID, sub, &pb.SubscribeRequest{
		Topic:        "",
		SubscribeAll: true,
		PluginId:     e.pluginID,
	})

	log.Debug().
		Str("subscription_id", subID).
		Msg("Plugin subscribed to all events")

	return subID, nil
}

// Unsubscribe removes a subscription by ID.
func (e *eventServiceImpl) Unsubscribe(subscriptionID string) error {
	e.mu.Lock()
	sub, ok := e.subscriptions[subscriptionID]
	if ok {
		delete(e.subscriptions, subscriptionID)
	}
	e.mu.Unlock()

	if !ok {
		return fmt.Errorf("subscription not found: %s", subscriptionID)
	}

	// Cancel the subscription's context to stop the goroutine
	sub.cancel()

	log.Debug().
		Str("subscription_id", subscriptionID).
		Str("topic", sub.topic).
		Msg("Plugin unsubscribed from events")

	return nil
}

// runSubscription runs in a goroutine to receive events from the gRPC stream.
func (e *eventServiceImpl) runSubscription(ctx context.Context, subID string, sub *activeSubscription, req *pb.SubscribeRequest) {
	stream, err := e.client.Subscribe(ctx, req)
	if err != nil {
		log.Error().
			Err(err).
			Str("subscription_id", subID).
			Str("topic", sub.topic).
			Msg("Failed to create event subscription stream")

		// Clean up the subscription
		e.mu.Lock()
		delete(e.subscriptions, subID)
		e.mu.Unlock()
		return
	}

	for {
		select {
		case <-ctx.Done():
			// Subscription cancelled
			return
		default:
			msg, err := stream.Recv()
			if err != nil {
				// Check if this was a clean shutdown
				select {
				case <-ctx.Done():
					return
				default:
					log.Debug().
						Err(err).
						Str("subscription_id", subID).
						Msg("Event subscription stream ended")

					// Clean up the subscription
					e.mu.Lock()
					delete(e.subscriptions, subID)
					e.mu.Unlock()
					return
				}
			}

			// Convert proto message to SDK Event
			event := Event{
				ID:      msg.Id,
				Topic:   msg.Topic,
				Origin:  msg.Origin,
				Dir:     DirectionFromInt32(msg.Direction),
				Payload: msg.Payload,
			}

			// Call the handler in the current goroutine
			// The handler is expected to handle panics if needed
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Error().
							Interface("panic", r).
							Str("subscription_id", subID).
							Str("event_id", event.ID).
							Msg("Panic in event handler")
					}
				}()
				sub.handler(event)
			}()
		}
	}
}

// cleanup cancels all active subscriptions.
// This is called when the plugin is shutting down.
func (e *eventServiceImpl) cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for subID, sub := range e.subscriptions {
		sub.cancel()
		log.Debug().
			Str("subscription_id", subID).
			Msg("Cleaned up event subscription")
	}
	e.subscriptions = make(map[string]*activeSubscription)
}

// stubEventService is a no-op implementation used when the event service is unavailable
type stubEventService struct{}

func (s *stubEventService) Publish(ctx context.Context, topic string, payload interface{}, dir Direction) error {
	log.Warn().Str("topic", topic).Msg("Event service not available - event dropped")
	return nil
}

func (s *stubEventService) PublishRaw(ctx context.Context, topic string, payload []byte, dir Direction) error {
	log.Warn().Str("topic", topic).Msg("Event service not available - event dropped")
	return nil
}

func (s *stubEventService) Subscribe(topic string, handler EventHandler) (string, error) {
	return "", fmt.Errorf("event service not available")
}

func (s *stubEventService) SubscribeAll(handler EventHandler) (string, error) {
	return "", fmt.Errorf("event service not available")
}

func (s *stubEventService) Unsubscribe(subscriptionID string) error {
	return nil
}

// lazyEventService wraps an EventService and lazily initializes it on first use.
// This is necessary because the gRPC broker connection is not available when
// the service broker is created during plugin startup.
type lazyEventService struct {
	runtime  RuntimeType
	pluginID string

	mu          sync.Mutex
	initialized bool
	inner       EventService
}

// getInner returns the underlying event service, initializing it if necessary.
func (l *lazyEventService) getInner() EventService {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.initialized {
		return l.inner
	}

	// Try to create the event service client
	client, err := getEventServiceClient()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize event service - using stub")
		l.inner = &stubEventService{}
	} else {
		l.inner = newEventService(client, l.pluginID)
		log.Debug().Str("plugin_id", l.pluginID).Msg("Event service initialized")
	}

	l.initialized = true
	return l.inner
}

func (l *lazyEventService) Publish(ctx context.Context, topic string, payload interface{}, dir Direction) error {
	return l.getInner().Publish(ctx, topic, payload, dir)
}

func (l *lazyEventService) PublishRaw(ctx context.Context, topic string, payload []byte, dir Direction) error {
	return l.getInner().PublishRaw(ctx, topic, payload, dir)
}

func (l *lazyEventService) Subscribe(topic string, handler EventHandler) (string, error) {
	return l.getInner().Subscribe(topic, handler)
}

func (l *lazyEventService) SubscribeAll(handler EventHandler) (string, error) {
	return l.getInner().SubscribeAll(handler)
}

func (l *lazyEventService) Unsubscribe(subscriptionID string) error {
	return l.getInner().Unsubscribe(subscriptionID)
}
