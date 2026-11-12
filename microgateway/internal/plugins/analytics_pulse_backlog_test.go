package plugins

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// The backlog of undelivered pulses (bead midsommar-clean-4x5y.36, option B):
// while pulses fail, an edge keeps what it could not deliver as serialized
// pulses, up to max_backlog_bytes, and sends them first, oldest first, when
// a control plane answers again.

// recoverHub makes the fake hub accept every pulse from now on, and forgets
// what it was sent so far.
func recoverHub(hub *fakeSyncClient) {
	hub.mu.Lock()
	hub.failures = 0
	hub.calls = 0
	hub.pulses = nil
	hub.mu.Unlock()
}

// delivered lists, in order, what the hub received: the request IDs of the
// analytics events, and the sequence number and size of every pulse.
func delivered(hub *fakeSyncClient) (ids []string, seqs []uint64, sizes []int, records []int) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for _, pulse := range hub.pulses {
		for _, ev := range pulse.AnalyticsEvents {
			ids = append(ids, ev.RequestId)
		}
		seqs = append(seqs, pulse.SequenceNumber)
		sizes = append(sizes, proto.Size(pulse))
		records = append(records, pulseRecords(pulse))
	}
	return ids, seqs, sizes, records
}

// An outage longer than the retries, at load: every round's pulse fails
// after its retry ladder (the 15 s of the packaged settings), and nothing is
// lost while the backlog is within its bound. When a control plane answers,
// the backlog drains in pulses of at most max_buffer_size records and
// within the pulse size bound, every record once, oldest first.
func TestPulseBacklog_OutageLosesNothingBelowTheBound(t *testing.T) {
	setMaxPulseBytes(t, 6<<10) // small, so batches are split by size too
	const cap = 30
	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: cap, MaxRetries: 3, MaxBacklogBytes: 64 << 20, IncludeRequestResponseData: true})
	dropped := PulseDroppedRecords()

	var ids []string
	for round := 0; round < 20; round++ {
		ids = append(ids, bufferEvents(p, fmt.Sprintf("r%02d", round), 100, 64)...)
		p.sendPulse()
		p.bufferMutex.Lock()
		assert.Zero(t, p.totalBufferedLocked(), "after a failed pulse everything buffered is in the backlog")
		p.bufferMutex.Unlock()
	}
	bytes, batches := PulseBacklog()
	assert.Positive(t, bytes)
	assert.Positive(t, batches)
	after := PulseDroppedRecords()
	assert.Equal(t, dropped["over_backlog"], after["over_backlog"], "nothing over the bound")
	assert.Equal(t, dropped["over_cap"], after["over_cap"], "no trim to max_buffer_size")

	recoverHub(hub)
	p.sendPulse()

	got, seqs, sizes, records := delivered(hub)
	assert.Equal(t, ids, got, "every record, once, oldest first")
	for i := range sizes {
		assert.LessOrEqual(t, sizes[i], maxPulseBytes, "pulse %d within the pulse size bound", i)
		assert.LessOrEqual(t, records[i], cap, "pulse %d within max_buffer_size records", i)
	}
	seen := map[uint64]bool{}
	for _, s := range seqs {
		assert.False(t, seen[s], "each batch under its own sequence number")
		seen[s] = true
	}
	bytes, batches = PulseBacklog()
	assert.Zero(t, bytes)
	assert.Zero(t, batches)
}

// With the default pulse size bound (8 MB), no pulse of a drained backlog
// comes near the control plane's 16 MiB gRPC limit, even with bodies.
func TestPulseBacklog_DrainedPulsesStayUnderTheGRPCLimit(t *testing.T) {
	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: 10000, MaxRetries: 1, MaxBacklogBytes: 256 << 20, IncludeRequestResponseData: true})
	var ids []string
	for round := 0; round < 4; round++ {
		ids = append(ids, bufferEvents(p, fmt.Sprintf("b%d", round), 5000, 4096)...)
		p.sendPulse()
	}
	recoverHub(hub)
	p.sendPulse()
	got, _, sizes, _ := delivered(hub)
	assert.Equal(t, ids, got)
	require.NotEmpty(t, sizes)
	for i, n := range sizes {
		assert.Less(t, n, 16<<20, "pulse %d under the control plane's 16 MiB limit", i)
		assert.LessOrEqual(t, n, maxPulseBytes, "pulse %d within the edge's own bound", i)
	}
}

// Over max_backlog_bytes the oldest batches go first, counted as
// over_backlog; the newest are delivered.
func TestPulseBacklog_OverTheBoundDropsTheOldestCounted(t *testing.T) {
	const cap = 50
	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: cap, MaxRetries: 1, MaxBacklogBytes: 20 << 10})
	before := PulseDroppedRecords()["over_backlog"]

	var ids []string
	for round := 0; round < 20; round++ {
		ids = append(ids, bufferEvents(p, fmt.Sprintf("o%02d", round), 100, 0)...)
		p.sendPulse()
		bytes, _ := PulseBacklog()
		assert.LessOrEqual(t, bytes, int64(20<<10), "the backlog stays within its bound")
	}
	droppedN := PulseDroppedRecords()["over_backlog"] - before
	assert.Positive(t, droppedN, "the oldest were discarded, and counted")

	recoverHub(hub)
	p.sendPulse()
	got, _, _, _ := delivered(hub)
	assert.EqualValues(t, len(ids), int(droppedN)+len(got), "every record is either delivered or counted as dropped")
	assert.Equal(t, ids[len(ids)-len(got):], got, "the newest are kept")
}

// Batches whose newest record is older than edge_retention_hours expire,
// counted as expired.
func TestPulseBacklog_ExpiredBatchesAreDropped(t *testing.T) {
	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: 10, MaxRetries: 1, MaxBacklogBytes: 1 << 20, EdgeRetentionHours: 1})
	before := PulseDroppedRecords()["expired"]
	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 3; i++ {
		bufferOneOfEach(p, fmt.Sprintf("old-%d", i), old)
	}
	p.sendPulse()
	assert.EqualValues(t, 15, PulseDroppedRecords()["expired"]-before, "three requests' five records each")
	bytes, batches := PulseBacklog()
	assert.Zero(t, bytes)
	assert.Zero(t, batches)
}

// lossyAckHub stores every pulse but loses the first acknowledgement, as a
// control plane that dies between its commit and its answer.
type lossyAckHub struct {
	pb.ConfigurationSyncServiceClient
	mu     sync.Mutex
	lost   bool
	pulses []*pb.AnalyticsPulse
}

func (h *lossyAckHub) SendAnalyticsPulse(_ context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pulses = append(h.pulses, pulse)
	if !h.lost {
		h.lost = true
		return nil, fmt.Errorf("connection reset after the commit")
	}
	return &pb.AnalyticsPulseResponse{Success: true}, nil
}

// A batch whose acknowledgement was lost is sent again under the same
// sequence number (and boot ID), so the control plane, which stores a batch
// once by them, does not store it twice.
func TestPulseBacklog_ResendKeepsTheSequenceNumber(t *testing.T) {
	hub := &lossyAckHub{}
	p := newRetryPlugin(nil, PulsePluginConfig{MaxBufferSize: 10, MaxRetries: 0, MaxBacklogBytes: 1 << 20})
	p.clients = staticClient{hub}
	p.bootID = "boot-1"
	bufferEvents(p, "a", 5, 0)
	p.sendPulse() // stored, acknowledgement lost
	p.sendPulse() // sent again from the backlog

	hub.mu.Lock()
	defer hub.mu.Unlock()
	require.Len(t, hub.pulses, 2)
	assert.Equal(t, hub.pulses[0].SequenceNumber, hub.pulses[1].SequenceNumber)
	assert.Equal(t, "boot-1", hub.pulses[1].BootId)
	assert.Equal(t, pulseRecords(hub.pulses[0]), pulseRecords(hub.pulses[1]))
}

// The setting: unset in the pulse file it is 256 MiB; 0 turns the backlog
// off (a failed pulse is trimmed to max_buffer_size, as before 2.3);
// negative is refused.
func TestPulseBacklog_Setting(t *testing.T) {
	cfg, err := parsePluginConfig(map[string]interface{}{"interval_seconds": 10})
	require.NoError(t, err)
	assert.EqualValues(t, 256<<20, cfg.MaxBacklogBytes)

	cfg, err = parsePluginConfig(map[string]interface{}{"max_backlog_bytes": 0})
	require.NoError(t, err)
	assert.Zero(t, cfg.MaxBacklogBytes)

	cfg, err = parsePluginConfig(map[string]interface{}{"max_backlog_bytes": 64 << 20})
	require.NoError(t, err)
	assert.EqualValues(t, 64<<20, cfg.MaxBacklogBytes)

	_, err = parsePluginConfig(map[string]interface{}{"max_backlog_bytes": -1})
	assert.Error(t, err)
}

// In real time: an outage longer than 15 s while requests keep being
// recorded from several goroutines (as the gateway records them), with the
// packaged retry ladder's shape (several attempts seconds apart). Nothing
// is lost below the bound, and on recovery every record arrives once.
func TestPulseBacklog_RealTimeOutageAtLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("takes about 20 s")
	}
	const cap = 1000
	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: cap, MaxRetries: 3, MaxBacklogBytes: 256 << 20})
	p.retryInterval = 3 * time.Second // a ladder of four attempts takes 9 s
	before := PulseDroppedRecords()

	stop := make(chan struct{})
	var mu sync.Mutex
	var ids []string
	var writers sync.WaitGroup
	for w := 0; w < 4; w++ {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				got := bufferEvents(p, fmt.Sprintf("w%d-%06d", w, i), 1, 0)
				mu.Lock()
				ids = append(ids, got...)
				mu.Unlock()
				time.Sleep(2 * time.Millisecond) // about 500 requests/s per writer
			}
		}(w)
	}

	started := time.Now()
	for time.Since(started) < 16*time.Second {
		p.sendPulse() // each failed ladder moves what arrived into the backlog
	}
	close(stop)
	writers.Wait()
	outage := time.Since(started)
	after := PulseDroppedRecords()
	assert.Equal(t, before["over_backlog"], after["over_backlog"])
	assert.Equal(t, before["over_cap"], after["over_cap"])

	recoverHub(hub)
	p.sendPulse()
	p.sendPulse() // what was still in the buffers when the writers stopped

	got, _, sizes, records := delivered(hub)
	mu.Lock()
	defer mu.Unlock()
	t.Logf("outage %s, %d requests recorded, %d pulses on recovery", outage.Round(time.Second), len(ids), len(sizes))
	assert.Greater(t, outage, 15*time.Second)
	require.Equal(t, len(ids), len(got), "every record arrives")
	seen := make(map[string]int, len(got))
	for _, id := range got {
		seen[id]++
	}
	for _, id := range ids {
		assert.Equal(t, 1, seen[id], "%s once", id)
	}
	for i := range sizes {
		assert.LessOrEqual(t, records[i], cap)
		assert.LessOrEqual(t, sizes[i], maxPulseBytes)
	}
}

// splitPulse partitions the records: each once, the older half first, the
// first half keeping the sequence number; capture events only in the first.
func TestPulseBacklog_SplitPulse(t *testing.T) {
	m := &pb.AnalyticsPulse{EdgeId: "e", SequenceNumber: 7, BootId: "b",
		CaptureEvents: []*pb.CaptureEventProto{{}}}
	for i := 0; i < 5; i++ {
		m.AnalyticsEvents = append(m.AnalyticsEvents, &pb.AnalyticsEvent{RequestId: fmt.Sprint("a", i)})
		m.BudgetEvents = append(m.BudgetEvents, &pb.BudgetUsageEvent{})
	}
	first, second := splitPulse(m, 8)
	require.NotNil(t, second)
	assert.Equal(t, pulseRecords(m), pulseRecords(first)+pulseRecords(second))
	assert.Equal(t, 5, pulseRecords(first))
	assert.EqualValues(t, 7, first.SequenceNumber)
	assert.EqualValues(t, 8, second.SequenceNumber)
	assert.Equal(t, "b", second.BootId)
	assert.Len(t, first.CaptureEvents, 1)
	assert.Empty(t, second.CaptureEvents)
	var ids []string
	for _, h := range []*pb.AnalyticsPulse{first, second} {
		for _, ev := range h.AnalyticsEvents {
			ids = append(ids, ev.RequestId)
		}
	}
	assert.Equal(t, []string{"a0", "a1", "a2", "a3", "a4"}, ids)

	one := &pb.AnalyticsPulse{AnalyticsEvents: []*pb.AnalyticsEvent{{}}}
	f, s := splitPulse(one, 9)
	assert.Same(t, one, f)
	assert.Nil(t, s)
}

// When the backlog first discards records in an outage, the log says how to
// raise the bound, once; after a delivered pulse ends the outage, the next
// one says it again.
func TestPulseBacklog_FullHintOncePerOutage(t *testing.T) {
	var logged bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&logged)
	t.Cleanup(func() { log.Logger = prev })
	hint := "raise max_backlog_bytes"

	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: 50, MaxRetries: 0, MaxBacklogBytes: 8 << 10})
	outage := func(prefix string) {
		for round := 0; round < 10; round++ {
			bufferEvents(p, fmt.Sprintf("%s%02d", prefix, round), 100, 0)
			p.sendPulse()
		}
	}
	outage("x")
	assert.Equal(t, 1, strings.Count(logged.String(), hint), "once in the first outage, though records were discarded at several pulses")
	assert.Greater(t, strings.Count(logged.String(), "Analytics records discarded"), 1)

	recoverHub(hub)
	p.sendPulse()
	hub.mu.Lock()
	hub.failures = 1 << 30
	hub.mu.Unlock()
	outage("y")
	assert.Equal(t, 2, strings.Count(logged.String(), hint), "and once more in the next outage")
}

// Batches the backlog drops do not stay reachable through its backing
// array: their messages can be collected, so memory follows the bound.
func TestPulseBacklog_DroppedBatchesAreReleased(t *testing.T) {
	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: 50, MaxRetries: 0, MaxBacklogBytes: 8 << 10, EdgeRetentionHours: 1})
	for round := 0; round < 10; round++ {
		bufferEvents(p, fmt.Sprintf("m%02d", round), 100, 0)
		p.sendPulse()
	}
	old := time.Now().Add(-2 * time.Hour)
	bufferOneOfEach(p, "expiring", old)
	p.sendPulse()

	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()
	full := p.backlog[:cap(p.backlog)]
	for i := len(p.backlog); i < len(full); i++ {
		assert.Nil(t, full[i].msg, "a dropped entry past the end holds no message")
	}
	var live int64
	for _, b := range p.backlog {
		live += int64(len(b.msg))
	}
	assert.Equal(t, p.backlogBytes, live)
}
