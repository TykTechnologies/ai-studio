package plugins

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// Full batches spilled to the backlog (bead midsommar-clean-4x5y.49): while
// pulses fail, and while the backlog is being sent, a buffer that reaches
// max_buffer_size records is serialized onto the backlog's tail instead of
// waiting as Go structs.

// gateHub fails while failing is set; otherwise each call waits for a
// token on gate (when gate is set). It keeps what it was sent.
type gateHub struct {
	pb.ConfigurationSyncServiceClient
	mu      sync.Mutex
	failing bool
	gate    chan struct{}
	calls   int
	pulses  []*pb.AnalyticsPulse
	called  chan struct{} // one value per call, if set
}

func (h *gateHub) SendAnalyticsPulse(ctx context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	h.mu.Lock()
	h.calls++
	failing, gate, called := h.failing, h.gate, h.called
	h.mu.Unlock()
	if called != nil {
		called <- struct{}{}
	}
	if failing {
		return nil, errors.New("hub unreachable")
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	h.mu.Lock()
	h.pulses = append(h.pulses, pulse)
	h.mu.Unlock()
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

func (h *gateHub) set(f func(h *gateHub)) {
	h.mu.Lock()
	f(h)
	h.mu.Unlock()
}

func (h *gateHub) delivered() (ids []string, seqs []uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, pulse := range h.pulses {
		for _, ev := range pulse.AnalyticsEvents {
			ids = append(ids, ev.RequestId)
		}
		seqs = append(seqs, pulse.SequenceNumber)
	}
	return ids, seqs
}

func newSpillPlugin(hub *gateHub, cfg PulsePluginConfig) *AnalyticsPulsePlugin {
	cfg.IntervalSeconds = 3600
	cfg.TimeoutSeconds = 5
	cfg.EdgeRetentionHours = 24
	p := &AnalyticsPulsePlugin{
		config: &cfg, edgeID: "edge-1", edgeNamespace: "test",
		clients: staticClient{hub}, sequenceNumber: 1, lastPulseTime: time.Now(),
		retryInterval: time.Millisecond,
	}
	return p
}

// record goes through the request path, which starts spills and flushes.
func record(t *testing.T, p *AnalyticsPulsePlugin, prefix string, n int) (ids []string) {
	t.Helper()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%03d", prefix, i)
		_, err := p.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{RequestID: id, Vendor: "openai", Timestamp: time.Now()}, nil)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	return ids
}

// settle waits until no spill or flush started by the request path runs.
func settle(t *testing.T, p *AnalyticsPulsePlugin) {
	t.Helper()
	require.Eventually(t, func() bool { return !p.spillQueued.Load() && !p.flushQueued.Load() && !p.sending.Load() },
		5*time.Second, time.Millisecond)
}

func bufferedStructs(p *AnalyticsPulsePlugin) int {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()
	return p.totalBufferedLocked() - p.heldSizeLocked()
}

func backlogSeqs(p *AnalyticsPulsePlugin) (seqs []uint64) {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()
	for _, b := range p.backlog {
		seqs = append(seqs, b.seq)
	}
	return seqs
}

func assertUnique(t *testing.T, seqs []uint64) {
	t.Helper()
	seen := map[uint64]bool{}
	for _, s := range seqs {
		assert.False(t, seen[s], "sequence number %d used once", s)
		seen[s] = true
	}
}

// From the first failed attempt, while the batch in flight is still being
// retried, full batches go to the backlog; when the batch in flight fails
// for good it goes in front of them. Delivered later, every record arrives
// once, in the order recorded, each batch under its own sequence number.
func TestPulseSpill_DuringTheRetriesKeepsTheOrder(t *testing.T) {
	const batch = 10
	hub := &gateHub{failing: true, called: make(chan struct{}, 100)}
	p := newSpillPlugin(hub, PulsePluginConfig{MaxBufferSize: batch, MaxRetries: 1, MaxBacklogBytes: 1 << 20})
	p.retryInterval = 300 * time.Millisecond // the ladder: fail, wait, fail

	ids := bufferEvents(p, "a", 5, 0) // the batch that will be in flight
	done := make(chan struct{})
	go func() { defer close(done); p.sendPulse() }()
	<-hub.called // its first attempt has failed; the retry waits

	ids = append(ids, record(t, p, "b", 25)...) // 2 full batches and 5
	require.Eventually(t, func() bool { return p.spilledBatches.Load() == 2 && !p.spillQueued.Load() }, 5*time.Second, time.Millisecond)
	assert.Less(t, bufferedStructs(p), batch, "full batches do not wait as structs during the retries")

	<-done // the second attempt failed: the in-flight batch went to the front
	seqs := backlogSeqs(p)
	require.Len(t, seqs, 4, "in flight, two spilled, the rest")
	assertUnique(t, seqs)
	assert.Zero(t, bufferedStructs(p))

	hub.set(func(h *gateHub) { h.failing = false; h.called = nil })
	p.sendPulse()
	got, sent := hub.delivered()
	assert.Equal(t, ids, got, "every record once, in the order recorded")
	assert.Equal(t, seqs, sent, "the backlog's batches under their own numbers, oldest first")
}

// While the backlog is being sent, what arrives is spilled behind it in full
// batches instead of growing the buffers for the whole drain.
func TestPulseSpill_DuringTheDrainKeepsTheOrderAndTheBuffersSmall(t *testing.T) {
	const batch = 10
	hub := &gateHub{failing: true}
	p := newSpillPlugin(hub, PulsePluginConfig{MaxBufferSize: batch, MaxRetries: 0, MaxBacklogBytes: 1 << 20})

	ids := bufferEvents(p, "a", 30, 0)
	p.sendPulse()
	require.Len(t, backlogSeqs(p), 3, "the outage left three batches")

	gate := make(chan struct{})
	called := make(chan struct{}, 100)
	hub.set(func(h *gateHub) { h.failing = false; h.gate = gate; h.called = called })
	done := make(chan struct{})
	go func() { defer close(done); p.sendPulse() }()
	<-called // the drain has begun: the head is in flight

	ids = append(ids, record(t, p, "b", 45)...)
	require.Eventually(t, func() bool { return p.spilledBatches.Load() == 4 && !p.spillQueued.Load() }, 5*time.Second, time.Millisecond)
	assert.Less(t, bufferedStructs(p), batch, "the buffers stay below one batch during the drain")

	close(gate)
	<-done
	p.sendPulse() // what was left below a batch
	got, seqs := hub.delivered()
	assert.Equal(t, ids, got, "every record once, in the order recorded")
	assertUnique(t, seqs)
	bytes, batches := PulseBacklog()
	assert.Zero(t, bytes)
	assert.Zero(t, batches)
}

// Spilled batches count against max_backlog_bytes like any other: over the
// bound the oldest go, counted, and what is delivered is the newest, in order.
func TestPulseSpill_CountsAgainstTheBound(t *testing.T) {
	const batch = 50
	hub := &gateHub{failing: true}
	p := newSpillPlugin(hub, PulsePluginConfig{MaxBufferSize: batch, MaxRetries: 0, MaxBacklogBytes: 8 << 10})
	before := PulseDroppedRecords()["over_backlog"]

	ids := bufferEvents(p, "a", 10, 0)
	p.sendPulse() // fails: the outage has begun, spilling is on
	ids = append(ids, record(t, p, "b", 1000)...)
	settle(t, p)
	assert.Positive(t, p.spilledBatches.Load())
	bytes, _ := PulseBacklog()
	assert.LessOrEqual(t, bytes, int64(8<<10), "within the bound")
	dropped := PulseDroppedRecords()["over_backlog"] - before
	assert.Positive(t, dropped)

	hub.set(func(h *gateHub) { h.failing = false })
	p.sendPulse()
	p.sendPulse()
	got, seqs := hub.delivered()
	assert.EqualValues(t, len(ids), int(dropped)+len(got), "every record delivered or counted as dropped")
	assert.Equal(t, ids[len(ids)-len(got):], got, "the newest are kept, in order")
	assertUnique(t, seqs)
}

// A healthy control plane: full buffers are sent as before and nothing is
// serialized into the backlog.
func TestPulseSpill_HealthyControlPlaneSpillsNothing(t *testing.T) {
	hub := &gateHub{}
	p := newSpillPlugin(hub, PulsePluginConfig{MaxBufferSize: 10, MaxBacklogBytes: 1 << 20})
	defer p.stopTimer()
	ids := record(t, p, "h", 95)
	settle(t, p)
	p.sendPulse()
	assert.Zero(t, p.spilledBatches.Load())
	assert.Empty(t, backlogSeqs(p))
	got, _ := hub.delivered()
	assert.Equal(t, ids, got)
}

// With the backlog off (max_backlog_bytes 0) a failing control plane is
// handled as before: nothing is spilled, and what waits is trimmed to
// max_buffer_size (over_cap).
func TestPulseSpill_BacklogOffIsUnchanged(t *testing.T) {
	hub := &gateHub{failing: true}
	p := newSpillPlugin(hub, PulsePluginConfig{MaxBufferSize: 10, MaxRetries: 0, MaxBacklogBytes: 0})
	defer p.stopTimer()
	before := PulseDroppedRecords()["over_cap"]
	bufferEvents(p, "a", 10, 0)
	p.sendPulse()
	record(t, p, "b", 50)
	settle(t, p)
	p.sendPulse() // fails again: the held batch and the buffers are trimmed
	assert.Zero(t, p.spilledBatches.Load())
	assert.Empty(t, backlogSeqs(p))
	assert.Positive(t, PulseDroppedRecords()["over_cap"]-before, "trimmed to max_buffer_size, as before")
}
