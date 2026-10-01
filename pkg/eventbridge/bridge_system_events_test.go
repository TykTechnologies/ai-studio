package eventbridge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Object change events (system.*) come from Studio. A control node drops
// the ones an edge sends instead of putting them on its bus, where they
// would reload the gateway, clear caches or reach webhooks and plugins as if
// Studio had changed an object; the edge's other events still arrive.
func TestBridge_ControlDropsSystemEventsFromEdges(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	var mu sync.Mutex
	var topics []string
	sub := bus.SubscribeAll(func(ev Event) {
		mu.Lock()
		topics = append(topics, ev.Topic)
		mu.Unlock()
	})
	defer bus.Unsubscribe(sub)

	bridge := NewBridge(BridgeConfig{NodeID: "control", IsControl: true}, bus, stream)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bridge.Start(ctx)

	stream.toRecv <- &EventFrame{ID: "1", Topic: "system.plugin.updated", Origin: "edge-1", Dir: int32(DirUp), Payload: []byte(`{"object_id":3}`)}
	stream.toRecv <- &EventFrame{ID: "2", Topic: "system.llm.deleted", Origin: "edge-1", Dir: int32(DirUp), Payload: []byte(`{}`)}
	stream.toRecv <- &EventFrame{ID: "3", Topic: "metrics.report", Origin: "edge-1", Dir: int32(DirUp), Payload: []byte(`{}`)}

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(topics) >= 1
	}, time.Second, 10*time.Millisecond)
	// Frames are handled in order: once the third is in, the first two had
	// their chance.
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"metrics.report"}, topics)
}

// An edge still takes system.* events from its control plane: they are how
// it learns about object changes.
func TestBridge_EdgeAcceptsSystemEventsFromControl(t *testing.T) {
	bus := NewBus()
	stream := newMockStream()
	defer stream.Close()

	got := make(chan string, 1)
	sub := bus.Subscribe("system.llm.updated", func(ev Event) { got <- ev.Topic })
	defer bus.Unsubscribe(sub)

	bridge := NewBridge(BridgeConfig{NodeID: "edge-1", IsControl: false}, bus, stream)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bridge.Start(ctx)

	stream.toRecv <- &EventFrame{ID: "1", Topic: "system.llm.updated", Origin: "control", Dir: int32(DirDown), Payload: []byte(`{}`)}
	select {
	case topic := <-got:
		assert.Equal(t, "system.llm.updated", topic)
	case <-time.After(time.Second):
		t.Fatal("the edge did not receive the control plane's system event")
	}
}
