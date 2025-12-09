package eventbridge

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/simonfxr/pubsub"
)

// Bus is the interface for the local event bus.
// It provides publish/subscribe functionality for Event objects.
type Bus interface {
	// Subscribe registers a callback for events on a specific topic.
	// Returns a subscription that can be used to unsubscribe.
	Subscribe(topic string, fn func(Event)) *pubsub.Subscription

	// SubscribeAll registers a callback for all events regardless of topic.
	// Returns a subscription that can be used to unsubscribe.
	SubscribeAll(fn func(Event)) *pubsub.Subscription

	// Unsubscribe removes a subscription.
	Unsubscribe(sub *pubsub.Subscription)

	// Publish sends an event to all subscribers of the event's topic.
	Publish(event Event)
}

// PubSubBus wraps simonfxr/pubsub as a Bus implementation.
// It is safe for concurrent use.
type PubSubBus struct {
	ps              *pubsub.Bus
	wildcardTopic   string
	mu              sync.RWMutex
	wildcardSubs    []*wildcardSubscription
}

type wildcardSubscription struct {
	sub *pubsub.Subscription
	fn  func(Event)
}

// NewBus creates a new PubSubBus instance.
func NewBus() *PubSubBus {
	return &PubSubBus{
		ps:            pubsub.NewBus(),
		wildcardTopic: "__eventbridge_wildcard__",
		wildcardSubs:  make([]*wildcardSubscription, 0),
	}
}

// Subscribe registers a callback for events on a specific topic.
func (b *PubSubBus) Subscribe(topic string, fn func(Event)) *pubsub.Subscription {
	return b.ps.Subscribe(topic, fn)
}

// SubscribeAll registers a callback for all events.
// This is implemented by subscribing to a special wildcard topic that
// receives all published events.
func (b *PubSubBus) SubscribeAll(fn func(Event)) *pubsub.Subscription {
	sub := b.ps.Subscribe(b.wildcardTopic, fn)

	b.mu.Lock()
	b.wildcardSubs = append(b.wildcardSubs, &wildcardSubscription{
		sub: sub,
		fn:  fn,
	})
	b.mu.Unlock()

	return sub
}

// Unsubscribe removes a subscription.
func (b *PubSubBus) Unsubscribe(sub *pubsub.Subscription) {
	b.ps.Unsubscribe(sub)

	// Remove from wildcard subs if present
	b.mu.Lock()
	for i, ws := range b.wildcardSubs {
		if ws.sub == sub {
			b.wildcardSubs = append(b.wildcardSubs[:i], b.wildcardSubs[i+1:]...)
			break
		}
	}
	b.mu.Unlock()
}

// Publish sends an event to all subscribers of the event's topic
// and to all wildcard subscribers.
func (b *PubSubBus) Publish(event Event) {
	// Publish to specific topic subscribers
	b.ps.Publish(event.Topic, event)

	// Also publish to wildcard subscribers
	b.ps.Publish(b.wildcardTopic, event)
}

// PublishLocal is a helper to publish local-only events that won't be forwarded.
func PublishLocal(bus Bus, nodeID, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	bus.Publish(Event{
		ID:      uuid.NewString(),
		Topic:   topic,
		Origin:  nodeID,
		Dir:     DirLocal,
		Payload: data,
	})
	return nil
}

// PublishUp is a helper to publish events from edge to control.
func PublishUp(bus Bus, nodeID, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	bus.Publish(Event{
		ID:      uuid.NewString(),
		Topic:   topic,
		Origin:  nodeID,
		Dir:     DirUp,
		Payload: data,
	})
	return nil
}

// PublishDown is a helper to publish events from control to edges.
func PublishDown(bus Bus, nodeID, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	bus.Publish(Event{
		ID:      uuid.NewString(),
		Topic:   topic,
		Origin:  nodeID,
		Dir:     DirDown,
		Payload: data,
	})
	return nil
}

// NewEvent creates a new event with a generated UUID.
func NewEvent(topic, origin string, dir Direction, payload interface{}) (Event, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}

	return Event{
		ID:      uuid.NewString(),
		Topic:   topic,
		Origin:  origin,
		Dir:     dir,
		Payload: data,
	}, nil
}

// NewEventRaw creates a new event with pre-marshaled JSON payload.
func NewEventRaw(topic, origin string, dir Direction, payload json.RawMessage) Event {
	return Event{
		ID:      uuid.NewString(),
		Topic:   topic,
		Origin:  origin,
		Dir:     dir,
		Payload: payload,
	}
}
