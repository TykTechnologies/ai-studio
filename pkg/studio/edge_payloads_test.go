package studio

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// A headless replica relays what its edges send (their DirUp events arrive
// as DirLocal, marked FromEdge) besides what every replica relays; nothing
// else local.
func TestHeadlessRelayFilter(t *testing.T) {
	assert.True(t, headlessRelayFilter(eventbridge.Event{Topic: "metrics.report", Dir: eventbridge.DirLocal, FromEdge: true}))
	assert.True(t, headlessRelayFilter(eventbridge.Event{Topic: eventbridge.BudgetSyncTopic, Dir: eventbridge.DirDown}))
	assert.True(t, headlessRelayFilter(eventbridge.Event{Topic: "system.llm.updated", Dir: eventbridge.DirLocal}))
	assert.False(t, headlessRelayFilter(eventbridge.Event{Topic: "metrics.report", Dir: eventbridge.DirLocal}))
	// A full replica keeps the default: its own plugins already had its
	// edges' events.
	assert.False(t, cluster.RelayedByDefault(eventbridge.Event{Topic: "metrics.report", Dir: eventbridge.DirLocal, FromEdge: true}))

	// Object change events come from Studio, never from an edge: an edge
	// publishing one must not make other replicas reload plugins or clear
	// caches, from a full replica or a headless one.
	fromEdge := eventbridge.Event{Topic: "system.plugin.updated", Dir: eventbridge.DirLocal, FromEdge: true}
	assert.False(t, cluster.RelayedByDefault(fromEdge))
	assert.False(t, headlessRelayFilter(fromEdge))
}

// fakeLeadership answers IsLeader from a switch.
type fakeLeadership struct{ leading *atomic.Bool }

func (f fakeLeadership) IsLeader() bool                       { return f.leading.Load() }
func (f fakeLeadership) Signal(context.Context, string) error { return nil }

type payloadSink struct {
	mu  sync.Mutex
	got []*pb.PluginControlPayload
}

func (p *payloadSink) RouteEdgePayload(_ context.Context, pl *pb.PluginControlPayload) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got = append(p.got, pl)
	return nil
}

func (p *payloadSink) snapshot() []*pb.PluginControlPayload {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*pb.PluginControlPayload(nil), p.got...)
}

func logEvent(t *testing.T, id int64, pl *pb.PluginControlPayload) cluster.Event {
	t.Helper()
	b, err := proto.Marshal(pl)
	require.NoError(t, err)
	return cluster.Event{ID: id, Topic: pluginControlTopic, Origin: "headless", Payload: b}
}

// Only the leader routes a forwarded payload to its plugins, each log row
// once; the payload arrives as the edge sent it.
func TestEdgePayloadHost_LeaderRoutesEachRowOnce(t *testing.T) {
	leading := &atomic.Bool{}
	replicas.SetBackend(fakeLeadership{leading: leading})
	t.Cleanup(func() { replicas.SetBackend(nil) })

	sink := &payloadSink{}
	h := newEdgePayloadHost(nil, nil, sink)
	t.Cleanup(h.stop)

	sent := &pb.PluginControlPayload{PluginId: 7, Payload: []byte("stats"), EdgeId: "edge-1", EdgeNamespace: "eu", CorrelationId: "c1", Metadata: map[string]string{"k": "v"}}

	h.deliver(logEvent(t, 1, sent))
	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, sink.snapshot(), "a replica that does not lead leaves the payload to the leader")

	leading.Store(true)
	h.deliver(logEvent(t, 2, sent))
	h.deliver(logEvent(t, 2, sent)) // the log delivers at least once
	require.Eventually(t, func() bool { return len(sink.snapshot()) == 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	got := sink.snapshot()
	require.Len(t, got, 1, "a row is routed once")
	assert.True(t, proto.Equal(sent, got[0]), "the payload reaches the plugin as the edge sent it")
}

// The headless forwarder writes what the host reads.
func TestEdgePayloadForwarder_RoundTrip(t *testing.T) {
	sent := &pb.PluginControlPayload{PluginId: 3, Payload: []byte{0, 1, 2}, CorrelationId: "c9"}
	b, err := encodeEdgePayload(sent)
	require.NoError(t, err)
	back, err := decodeEdgePayload(b)
	require.NoError(t, err)
	assert.True(t, proto.Equal(sent, back))
}

// Handled ids are forgotten once the log can no longer deliver their rows,
// by time, however few payloads arrive.
func TestEdgePayloadHost_ForgetsOldIDs(t *testing.T) {
	h := newEdgePayloadHost(nil, nil, &payloadSink{})
	t.Cleanup(h.stop)
	h.mu.Lock()
	h.seen[1] = time.Now().Add(-2 * edgePayloadSeen)
	h.seen[2] = time.Now()
	h.mu.Unlock()

	h.forgetOld(time.Now().Add(-edgePayloadSeen))

	h.mu.Lock()
	defer h.mu.Unlock()
	assert.NotContains(t, h.seen, int64(1))
	assert.Contains(t, h.seen, int64(2))
}
