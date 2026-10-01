package eventbridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// receiveOne starts a bridge with the given role, feeds it one frame from
// its peer and returns the event it republished on the local bus.
func receiveOne(t *testing.T, isControl bool) Event {
	t.Helper()
	bus := NewBus()
	stream := newMockStream()
	t.Cleanup(stream.Close)

	got := make(chan Event, 1)
	sub := bus.Subscribe("metrics.report", func(ev Event) { got <- ev })
	t.Cleanup(func() { bus.Unsubscribe(sub) })

	bridge := NewBridge(BridgeConfig{NodeID: "node", IsControl: isControl}, bus, stream)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bridge.Start(ctx)

	stream.toRecv <- &EventFrame{ID: "e1", Topic: "metrics.report", Origin: "peer", Dir: int32(DirUp), Payload: []byte(`{}`)}
	select {
	case ev := <-got:
		return ev
	case <-time.After(time.Second):
		t.Fatal("the bridge did not republish the peer's event")
		return Event{}
	}
}

// A control node's bridge marks what its edge sent it, so a control plane
// can tell an edge's events (DirLocal once republished, like its own) from
// those published on it; an edge's bridge marks nothing.
func TestBridge_ControlMarksEventsFromItsEdge(t *testing.T) {
	fromEdge := receiveOne(t, true)
	assert.True(t, fromEdge.FromEdge)
	assert.Equal(t, DirLocal, fromEdge.Dir, "still local: never forwarded back to edges")

	fromControl := receiveOne(t, false)
	assert.False(t, fromControl.FromEdge)
}

// The mark is local: it travels neither to edges nor through the cluster
// relay, whose copies are JSON.
func TestEvent_FromEdgeIsNotSerialized(t *testing.T) {
	b, err := json.Marshal(Event{ID: "e1", Topic: "t", FromEdge: true})
	require.NoError(t, err)
	var back Event
	require.NoError(t, json.Unmarshal(b, &back))
	assert.False(t, back.FromEdge)
	assert.NotContains(t, string(b), "edge")
}
