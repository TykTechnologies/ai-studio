package plugin_sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdlog "log"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	mgwsdk "github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/sdk"
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
// It reuses the shared broker connection from either microgateway SDK or ai_studio_sdk to avoid dialing twice.
func getEventServiceClient() (pb.PluginEventServiceClient, error) {
	eventServiceMutex.Lock()
	defer eventServiceMutex.Unlock()

	if eventServiceClient != nil {
		stdlog.Printf("[EventSDK] getEventServiceClient: returning existing client")
		return eventServiceClient, nil
	}

	// Try to get the shared connection from Microgateway SDK first (for gateway runtime)
	// This avoids dialing the broker twice (which would fail)
	conn := mgwsdk.GetSharedBrokerConnection()
	if conn != nil {
		stdlog.Printf("[EventSDK] ✅ getEventServiceClient: using shared broker connection from Microgateway SDK")
		eventServiceClient = pb.NewPluginEventServiceClient(conn)
		log.Info().Msg("Event service client created via shared broker connection (microgateway)")
		return eventServiceClient, nil
	}

	// Try AI Studio SDK shared connection (for studio runtime)
	conn = ai_studio_sdk.GetSharedBrokerConnection()
	if conn != nil {
		stdlog.Printf("[EventSDK] ✅ getEventServiceClient: using shared broker connection from AI Studio SDK")
		eventServiceClient = pb.NewPluginEventServiceClient(conn)
		log.Info().Msg("Event service client created via shared broker connection (ai_studio)")
		return eventServiceClient, nil
	}

	// Fall back to dialing if no shared connection exists yet
	// This happens if the event service is used before any SDK call
	stdlog.Printf("[EventSDK] getEventServiceClient: no shared connection from either SDK, attempting to dial")

	// Prefer session broker if active (new session-based pattern)
	broker := GetSessionBroker()
	brokerID := GetSessionBrokerID()

	// Fall back to legacy global state if no session
	if broker == nil {
		broker = eventServiceBroker
	}
	if brokerID == 0 {
		brokerID = eventServiceBrokerID
	}

	stdlog.Printf("[EventSDK] getEventServiceClient: creating new client (session_broker=%v, session_brokerID=%d, legacy_broker=%v, legacy_brokerID=%d)",
		GetSessionBroker() != nil, GetSessionBrokerID(), eventServiceBroker != nil, eventServiceBrokerID)

	if broker == nil {
		return nil, fmt.Errorf("event service broker not initialized (no session or legacy broker available)")
	}

	if brokerID == 0 {
		return nil, fmt.Errorf("event service broker ID not set (no session or legacy broker ID)")
	}

	// Dial the brokered server where the event service is registered
	stdlog.Printf("[EventSDK] getEventServiceClient: dialing broker ID %d", brokerID)
	dialConn, err := broker.Dial(brokerID)
	if err != nil {
		stdlog.Printf("[EventSDK] ❌ getEventServiceClient: dial failed: %v", err)
		return nil, fmt.Errorf("failed to dial event service broker ID %d: %w", brokerID, err)
	}

	stdlog.Printf("[EventSDK] ✅ getEventServiceClient: dial successful, creating gRPC client")

	// CRITICAL: Share this connection with ai_studio_sdk so it doesn't try to dial again!
	// The go-plugin broker only accepts ONE connection per broker ID, so we must share.
	ai_studio_sdk.SetSharedBrokerConnection(dialConn)
	stdlog.Printf("[EventSDK] ✅ Shared broker connection with ai_studio_sdk")

	eventServiceClient = pb.NewPluginEventServiceClient(dialConn)
	log.Info().
		Uint32("broker_id", brokerID).
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
	stdlog.Printf("[EventSDK] Subscribe called: topic=%s pluginID=%s", topic, e.pluginID)

	if e.client == nil {
		stdlog.Printf("[EventSDK] ❌ Subscribe: client is nil!")
		return "", fmt.Errorf("event service not initialized")
	}

	if handler == nil {
		stdlog.Printf("[EventSDK] ❌ Subscribe: handler is nil!")
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

	stdlog.Printf("[EventSDK] Subscribe: starting runSubscription goroutine for subID=%s topic=%s", subID, topic)

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

	stdlog.Printf("[EventSDK] ✅ Subscribe: returning subID=%s for topic=%s", subID, topic)
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
	stdlog.Printf("[EventSDK] runSubscription starting: subID=%s topic=%s pluginID=%s", subID, sub.topic, req.PluginId)

	stream, err := e.client.Subscribe(ctx, req)
	if err != nil {
		stdlog.Printf("[EventSDK] ❌ Failed to create subscription stream: subID=%s topic=%s error=%v", subID, sub.topic, err)
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

	stdlog.Printf("[EventSDK] ✅ Subscription stream created successfully: subID=%s topic=%s", subID, sub.topic)

	for {
		select {
		case <-ctx.Done():
			stdlog.Printf("[EventSDK] Subscription context cancelled: subID=%s", subID)
			// Subscription cancelled
			return
		default:
			msg, err := stream.Recv()
			if err != nil {
				// Check if this was a clean shutdown
				select {
				case <-ctx.Done():
					stdlog.Printf("[EventSDK] Subscription stream closed (context done): subID=%s", subID)
					return
				default:
					stdlog.Printf("[EventSDK] Subscription stream ended with error: subID=%s error=%v", subID, err)
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

			stdlog.Printf("[EventSDK] 📨 Received event from stream: subID=%s eventID=%s topic=%s origin=%s", subID, msg.Id, msg.Topic, msg.Origin)

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
						stdlog.Printf("[EventSDK] ❌ Panic in event handler: subID=%s eventID=%s panic=%v", subID, event.ID, r)
						log.Error().
							Interface("panic", r).
							Str("subscription_id", subID).
							Str("event_id", event.ID).
							Msg("Panic in event handler")
					}
				}()
				stdlog.Printf("[EventSDK] Calling event handler: subID=%s eventID=%s topic=%s", subID, event.ID, event.Topic)
				sub.handler(event)
				stdlog.Printf("[EventSDK] Event handler completed: subID=%s eventID=%s", subID, event.ID)
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

// Events published before the broker connection exists are held, up to
// maxPendingEvents per plugin, and sent in order once it does. Beyond that
// they are dropped: Publish returns errEventDropped and a warning is logged,
// at most once per topic per dropWarnInterval.
const (
	maxPendingEvents = 64
	dropWarnInterval = time.Minute
	flushTimeout     = 5 * time.Second
)

// connectRetryInterval spaces connection attempts: dialling a broker whose
// server is not up yet can block for seconds. While events are buffered, a
// failed attempt schedules the next one, so they are sent even if the plugin
// publishes nothing more. A variable so tests can shorten it.
var connectRetryInterval = 2 * time.Second

// errEventDropped is returned by Publish when the event service is not ready
// and the pending buffer is full.
var errEventDropped = errors.New("event service not ready and its pending buffer is full: event dropped")

type pendingEvent struct {
	topic   string
	payload []byte
	dir     Direction
}

// lazyEventService wraps an EventService and initializes it on first use,
// because the gRPC broker connection does not exist yet when the service
// broker is created during plugin startup. Until the connection can be made,
// publishes are buffered (see maxPendingEvents) and subscriptions return an
// error the caller can retry.
//
// Connecting and sending the buffer happen outside mu, in one goroutine at a
// time (busy); other publishers meanwhile append to the buffer and return,
// so a slow dial or flush never stalls them. While the buffer is not empty,
// new events join it rather than overtake it, which keeps them in order.
type lazyEventService struct {
	runtime  RuntimeType
	pluginID string
	// connect returns the event service client; getEventServiceClient when nil.
	connect func() (pb.PluginEventServiceClient, error)

	mu          sync.Mutex
	inner       EventService // nil until the broker connection exists
	pending     []pendingEvent
	busy        bool // a goroutine is connecting or sending the buffer
	dropWarned  map[string]time.Time
	nextConnect time.Time   // no connection attempt before this
	retry       *time.Timer // scheduled drain while events wait for the broker
	closed      bool        // Cleanup ran; no more retries
}

// drain connects the event service if it is not connected (at most once per
// connectRetryInterval) and sends the buffered events in order. It returns
// at once if another goroutine is already doing so.
func (l *lazyEventService) drain() {
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		return
	}
	if l.inner == nil {
		now := time.Now()
		if now.Before(l.nextConnect) {
			l.scheduleRetry()
			l.mu.Unlock()
			return
		}
		l.busy = true
		l.mu.Unlock()

		connect := l.connect
		if connect == nil {
			connect = getEventServiceClient
		}
		client, err := connect()

		l.mu.Lock()
		if err != nil {
			l.nextConnect = now.Add(connectRetryInterval)
			l.busy = false
			l.scheduleRetry()
			l.mu.Unlock()
			log.Debug().Err(err).Str("plugin_id", l.pluginID).Msg("Event service not ready yet")
			return
		}
		l.inner = newEventService(client, l.pluginID)
		log.Debug().Str("plugin_id", l.pluginID).Msg("Event service initialized")
	}
	l.busy = true
	for len(l.pending) > 0 {
		batch, inner := l.pending, l.inner
		l.pending = nil
		l.mu.Unlock()
		for _, ev := range batch {
			ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
			if err := inner.PublishRaw(ctx, ev.topic, ev.payload, ev.dir); err != nil {
				log.Error().Err(err).Str("plugin_id", l.pluginID).Str("topic", ev.topic).Msg("Failed to send an event buffered while the event service was starting")
			}
			cancel()
		}
		l.mu.Lock()
	}
	l.busy = false
	l.mu.Unlock()
}

// scheduleRetry arranges one later drain while events are buffered, so they
// are not stranded when nothing else is published. The caller holds mu.
func (l *lazyEventService) scheduleRetry() {
	if l.retry != nil || l.closed || len(l.pending) == 0 {
		return
	}
	wait := time.Until(l.nextConnect)
	if wait < 0 {
		wait = 0
	}
	l.retry = time.AfterFunc(wait, func() {
		l.mu.Lock()
		l.retry = nil
		l.mu.Unlock()
		l.drain()
	})
}

func (l *lazyEventService) Publish(ctx context.Context, topic string, payload interface{}, dir Direction) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}
	return l.PublishRaw(ctx, topic, data, dir)
}

func (l *lazyEventService) PublishRaw(ctx context.Context, topic string, payload []byte, dir Direction) error {
	l.mu.Lock()
	if l.inner != nil && !l.busy && len(l.pending) == 0 {
		inner := l.inner
		l.mu.Unlock()
		return inner.PublishRaw(ctx, topic, payload, dir)
	}
	if len(l.pending) >= maxPendingEvents {
		l.warnDropped(topic)
		l.mu.Unlock()
		return errEventDropped
	}
	l.pending = append(l.pending, pendingEvent{topic: topic, payload: append([]byte(nil), payload...), dir: dir})
	l.mu.Unlock()
	l.drain()
	return nil
}

// warnDropped logs a dropped event, once per topic per dropWarnInterval.
// The caller holds mu.
func (l *lazyEventService) warnDropped(topic string) {
	now := time.Now()
	if last, ok := l.dropWarned[topic]; ok && now.Sub(last) < dropWarnInterval {
		return
	}
	if l.dropWarned == nil {
		l.dropWarned = map[string]time.Time{}
	}
	l.dropWarned[topic] = now
	log.Warn().Str("plugin_id", l.pluginID).Str("topic", topic).Int("buffered", len(l.pending)).
		Msg("Event service not ready and its buffer is full: dropping events on this topic")
}

// service returns the connected event service, or an error while the broker
// is not ready (the caller may retry).
func (l *lazyEventService) service() (EventService, error) {
	l.drain()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inner == nil {
		return nil, fmt.Errorf("event service not ready yet")
	}
	return l.inner, nil
}

func (l *lazyEventService) Subscribe(topic string, handler EventHandler) (string, error) {
	inner, err := l.service()
	if err != nil {
		return "", err
	}
	return inner.Subscribe(topic, handler)
}

func (l *lazyEventService) SubscribeAll(handler EventHandler) (string, error) {
	inner, err := l.service()
	if err != nil {
		return "", err
	}
	return inner.SubscribeAll(handler)
}

func (l *lazyEventService) Unsubscribe(subscriptionID string) error {
	l.mu.Lock()
	inner := l.inner
	l.mu.Unlock()
	if inner == nil {
		return nil // nothing was subscribed
	}
	return inner.Unsubscribe(subscriptionID)
}

// Cleanup cancels all active subscriptions and releases resources.
// This should be called during plugin shutdown.
func (l *lazyEventService) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.closed = true
	if l.retry != nil {
		l.retry.Stop()
		l.retry = nil
	}
	if impl, ok := l.inner.(*eventServiceImpl); ok {
		impl.cleanup()
	}
	if len(l.pending) > 0 {
		log.Warn().Str("plugin_id", l.pluginID).Int("events", len(l.pending)).
			Msg("Plugin shut down before the event service was ready: buffered events were not sent")
		l.pending = nil
	}
}
