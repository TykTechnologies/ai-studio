package eventbridge

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirection_String(t *testing.T) {
	tests := []struct {
		dir      Direction
		expected string
	}{
		{DirLocal, "local"},
		{DirUp, "up"},
		{DirDown, "down"},
		{Direction(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.dir.String())
	}
}

func TestDirectionFromInt32(t *testing.T) {
	tests := []struct {
		input    int32
		expected Direction
	}{
		{0, DirLocal},
		{1, DirUp},
		{2, DirDown},
		{99, DirLocal}, // Unknown defaults to local
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, DirectionFromInt32(tt.input))
	}
}

func TestBus_PublishSubscribe(t *testing.T) {
	bus := NewBus()

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	sub := bus.Subscribe("test.topic", func(ev Event) {
		received = ev
		wg.Done()
	})
	defer bus.Unsubscribe(sub)

	payload, err := json.Marshal(map[string]string{"key": "value"})
	require.NoError(t, err)

	bus.Publish(Event{
		ID:      "test-id",
		Topic:   "test.topic",
		Origin:  "test-node",
		Dir:     DirLocal,
		Payload: payload,
	})

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}

	assert.Equal(t, "test-id", received.ID)
	assert.Equal(t, "test.topic", received.Topic)
	assert.Equal(t, "test-node", received.Origin)
	assert.Equal(t, DirLocal, received.Dir)
}

func TestBus_SubscribeAll(t *testing.T) {
	bus := NewBus()

	var events []Event
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	sub := bus.SubscribeAll(func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
		wg.Done()
	})
	defer bus.Unsubscribe(sub)

	bus.Publish(Event{ID: "1", Topic: "topic.a", Origin: "node", Dir: DirLocal})
	bus.Publish(Event{ID: "2", Topic: "topic.b", Origin: "node", Dir: DirLocal})

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for events")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, events, 2)

	// Check both events received (order may vary)
	ids := make(map[string]bool)
	for _, ev := range events {
		ids[ev.ID] = true
	}
	assert.True(t, ids["1"])
	assert.True(t, ids["2"])
}

func TestBus_TopicIsolation(t *testing.T) {
	bus := NewBus()

	var eventsA []Event
	var eventsB []Event
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	subA := bus.Subscribe("topic.a", func(ev Event) {
		mu.Lock()
		eventsA = append(eventsA, ev)
		mu.Unlock()
		wg.Done()
	})
	defer bus.Unsubscribe(subA)

	subB := bus.Subscribe("topic.b", func(ev Event) {
		mu.Lock()
		eventsB = append(eventsB, ev)
		mu.Unlock()
		wg.Done()
	})
	defer bus.Unsubscribe(subB)

	bus.Publish(Event{ID: "1", Topic: "topic.a", Origin: "node", Dir: DirLocal})
	bus.Publish(Event{ID: "2", Topic: "topic.b", Origin: "node", Dir: DirLocal})

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for events")
	}

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, eventsA, 1)
	assert.Equal(t, "1", eventsA[0].ID)

	assert.Len(t, eventsB, 1)
	assert.Equal(t, "2", eventsB[0].ID)
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := NewBus()

	callCount := 0
	var mu sync.Mutex

	sub := bus.Subscribe("test.topic", func(ev Event) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	// First publish should be received
	bus.Publish(Event{ID: "1", Topic: "test.topic", Origin: "node", Dir: DirLocal})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount)
	mu.Unlock()

	// Unsubscribe
	bus.Unsubscribe(sub)

	// Second publish should not be received
	bus.Publish(Event{ID: "2", Topic: "test.topic", Origin: "node", Dir: DirLocal})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount)
	mu.Unlock()
}

func TestPublishLocal(t *testing.T) {
	bus := NewBus()

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	sub := bus.Subscribe("test.local", func(ev Event) {
		received = ev
		wg.Done()
	})
	defer bus.Unsubscribe(sub)

	err := PublishLocal(bus, "test-node", "test.local", map[string]string{"data": "value"})
	require.NoError(t, err)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}

	assert.Equal(t, "test.local", received.Topic)
	assert.Equal(t, "test-node", received.Origin)
	assert.Equal(t, DirLocal, received.Dir)
	assert.NotEmpty(t, received.ID)
}

func TestPublishUp(t *testing.T) {
	bus := NewBus()

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	sub := bus.Subscribe("metrics.report", func(ev Event) {
		received = ev
		wg.Done()
	})
	defer bus.Unsubscribe(sub)

	err := PublishUp(bus, "edge-001", "metrics.report", map[string]int{"count": 42})
	require.NoError(t, err)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}

	assert.Equal(t, "metrics.report", received.Topic)
	assert.Equal(t, "edge-001", received.Origin)
	assert.Equal(t, DirUp, received.Dir)
}

func TestPublishDown(t *testing.T) {
	bus := NewBus()

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	sub := bus.Subscribe("config.reload", func(ev Event) {
		received = ev
		wg.Done()
	})
	defer bus.Unsubscribe(sub)

	err := PublishDown(bus, "control", "config.reload", map[string]string{"action": "reload"})
	require.NoError(t, err)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}

	assert.Equal(t, "config.reload", received.Topic)
	assert.Equal(t, "control", received.Origin)
	assert.Equal(t, DirDown, received.Dir)
}

func TestNewEvent(t *testing.T) {
	ev, err := NewEvent("test.topic", "node-1", DirUp, map[string]string{"key": "value"})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.ID)
	assert.Equal(t, "test.topic", ev.Topic)
	assert.Equal(t, "node-1", ev.Origin)
	assert.Equal(t, DirUp, ev.Dir)

	var payload map[string]string
	err = json.Unmarshal(ev.Payload, &payload)
	require.NoError(t, err)
	assert.Equal(t, "value", payload["key"])
}

func TestNewEventRaw(t *testing.T) {
	rawPayload := json.RawMessage(`{"precomputed": true}`)
	ev := NewEventRaw("test.topic", "node-1", DirDown, rawPayload)

	assert.NotEmpty(t, ev.ID)
	assert.Equal(t, "test.topic", ev.Topic)
	assert.Equal(t, "node-1", ev.Origin)
	assert.Equal(t, DirDown, ev.Dir)
	assert.JSONEq(t, `{"precomputed": true}`, string(ev.Payload))
}

func TestEventFrame_ToEvent(t *testing.T) {
	frame := &EventFrame{
		ID:      "frame-id",
		Topic:   "remote.topic",
		Origin:  "remote-node",
		Dir:     int32(DirDown),
		Payload: []byte(`{"data": "test"}`),
	}

	ev := frame.ToEvent()

	assert.Equal(t, "frame-id", ev.ID)
	assert.Equal(t, "remote.topic", ev.Topic)
	assert.Equal(t, "remote-node", ev.Origin)
	assert.Equal(t, DirLocal, ev.Dir) // Should always be DirLocal
	assert.JSONEq(t, `{"data": "test"}`, string(ev.Payload))
}

func TestEvent_ToEventFrame(t *testing.T) {
	ev := Event{
		ID:      "event-id",
		Topic:   "local.topic",
		Origin:  "local-node",
		Dir:     DirUp,
		Payload: json.RawMessage(`{"data": "test"}`),
	}

	frame := ev.ToEventFrame()

	assert.Equal(t, "event-id", frame.ID)
	assert.Equal(t, "local.topic", frame.Topic)
	assert.Equal(t, "local-node", frame.Origin)
	assert.Equal(t, int32(DirUp), frame.Dir)
	assert.Equal(t, []byte(`{"data": "test"}`), frame.Payload)
}

func TestBus_ConcurrentPublish(t *testing.T) {
	bus := NewBus()

	const numPublishers = 10
	const eventsPerPublisher = 100

	var received int64
	var mu sync.Mutex

	sub := bus.SubscribeAll(func(ev Event) {
		mu.Lock()
		received++
		mu.Unlock()
	})
	defer bus.Unsubscribe(sub)

	var wg sync.WaitGroup
	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func(pubID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				bus.Publish(Event{
					ID:     "concurrent-test",
					Topic:  "concurrent.topic",
					Origin: "node",
					Dir:    DirLocal,
				})
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Allow events to be processed

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, int64(numPublishers*eventsPerPublisher), received)
}
