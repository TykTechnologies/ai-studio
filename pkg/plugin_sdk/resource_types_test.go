package plugin_sdk

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockEventService records published events for testing
type mockEventService struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	Topic     string
	Payload   interface{}
	Direction Direction
}

func (m *mockEventService) Publish(ctx context.Context, topic string, payload interface{}, dir Direction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, publishedEvent{Topic: topic, Payload: payload, Direction: dir})
	return nil
}

func (m *mockEventService) PublishRaw(ctx context.Context, topic string, payload []byte, dir Direction) error {
	return m.Publish(ctx, topic, payload, dir)
}

func (m *mockEventService) Subscribe(topic string, handler EventHandler) (string, error) {
	return "sub-1", nil
}

func (m *mockEventService) SubscribeAll(handler EventHandler) (string, error) {
	return "sub-all", nil
}

func (m *mockEventService) Unsubscribe(subID string) error {
	return nil
}

// mockServiceBrokerForTest provides a mock service broker with our mock event service
type mockServiceBrokerForTest struct {
	eventSvc *mockEventService
}

func (m *mockServiceBrokerForTest) KV() KVService              { return nil }
func (m *mockServiceBrokerForTest) Logger() LogService          { return nil }
func (m *mockServiceBrokerForTest) Gateway() GatewayServices    { return nil }
func (m *mockServiceBrokerForTest) Studio() StudioServices      { return nil }
func (m *mockServiceBrokerForTest) Events() EventService {
	if m.eventSvc == nil {
		return nil
	}
	return m.eventSvc
}

func TestNotifyResourceInstanceChanged(t *testing.T) {
	mockEvents := &mockEventService{}
	broker := &mockServiceBrokerForTest{eventSvc: mockEvents}

	ctx := Context{
		Runtime:  RuntimeStudio,
		Services: broker,
		Context:  context.Background(),
	}

	t.Run("publishes event with correct topic and payload", func(t *testing.T) {
		err := NotifyResourceInstanceChanged(ctx, "mcp_servers", "server-42")
		assert.NoError(t, err)

		assert.Len(t, mockEvents.events, 1)
		evt := mockEvents.events[0]
		assert.Equal(t, ResourceInstanceChangedEvent, evt.Topic)
		assert.Equal(t, DirLocal, evt.Direction)

		payload, ok := evt.Payload.(resourceInstanceChangedPayload)
		assert.True(t, ok)
		assert.Equal(t, "mcp_servers", payload.ResourceTypeSlug)
		assert.Equal(t, "server-42", payload.InstanceID)
	})

	t.Run("returns error when event service is nil", func(t *testing.T) {
		nilBroker := &mockServiceBrokerForTest{eventSvc: nil}
		nilCtx := Context{
			Runtime:  RuntimeStudio,
			Services: nilBroker,
			Context:  context.Background(),
		}

		err := NotifyResourceInstanceChanged(nilCtx, "mcp_servers", "server-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event service not available")
	})
}

func TestResourceInstanceChangedEventConstant(t *testing.T) {
	assert.Equal(t, "system.plugin_resource.instance_changed", ResourceInstanceChangedEvent)
}
