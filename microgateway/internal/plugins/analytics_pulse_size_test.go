package plugins

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// limitedHub is a control plane with a gRPC receive limit: a pulse above it
// is refused the way grpc-go refuses it, before any handler runs. It keeps
// what it accepted, once per sequence number as the real one does.
type limitedHub struct {
	pb.ConfigurationSyncServiceClient
	mu       sync.Mutex
	limit    int // bytes; 0: none
	calls    int
	refused  int
	accepted []*pb.AnalyticsPulse
	seen     map[uint64]bool
}

func (h *limitedHub) SendAnalyticsPulse(_ context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	if size := proto.Size(pulse); h.limit > 0 && size > h.limit {
		h.refused++
		return nil, status.Errorf(codes.ResourceExhausted, "grpc: received message larger than max (%d vs. %d)", size, h.limit)
	}
	if h.seen == nil {
		h.seen = map[uint64]bool{}
	}
	if !h.seen[pulse.SequenceNumber] {
		h.seen[pulse.SequenceNumber] = true
		h.accepted = append(h.accepted, pulse)
	}
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

// requestIDs are the analytics events the hub stored, in the order stored.
func (h *limitedHub) requestIDs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var ids []string
	for _, p := range h.accepted {
		for _, ev := range p.AnalyticsEvents {
			ids = append(ids, ev.RequestId)
		}
	}
	return ids
}

func (h *limitedHub) pulses() []*pb.AnalyticsPulse {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*pb.AnalyticsPulse(nil), h.accepted...)
}

func newSizePlugin(hub pb.ConfigurationSyncServiceClient, maxBuffer int) *AnalyticsPulsePlugin {
	cfg := PulsePluginConfig{MaxBufferSize: maxBuffer, TimeoutSeconds: 1, EdgeRetentionHours: 24, IntervalSeconds: 3600, MaxRetries: 3,
		IncludeRequestResponseData: true, IncludeProxySummaries: true}
	return &AnalyticsPulsePlugin{
		config: &cfg, edgeID: "edge-1", edgeNamespace: "test", clients: staticClient{hub},
		sequenceNumber: 1, lastPulseTime: time.Now(), retryInterval: time.Millisecond, bootID: "boot-1",
	}
}

// bufferEvents adds n analytics events (ids prefix-0, prefix-1, ...) with a
// request body of bodyBytes each, without the buffer-full trigger.
func bufferEvents(p *AnalyticsPulsePlugin, prefix string, n, bodyBytes int) (ids []string) {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()
	body := strings.Repeat("x", bodyBytes)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%d", prefix, i)
		ids = append(ids, id)
		p.analyticsBuffer = append(p.analyticsBuffer, database.AnalyticsEvent{RequestID: id, TimeStamp: time.Now()})
		p.analyticsMetadata = append(p.analyticsMetadata, AnalyticsMetadata{RequestID: id, RequestBody: body})
	}
	return ids
}

func setMaxPulseBytes(t *testing.T, n int) {
	prev := maxPulseBytes
	maxPulseBytes = n
	t.Cleanup(func() { maxPulseBytes = prev })
}

// A backlog larger than one pulse (what gathers while one slow pulse is in
// flight, or after pulses failed) is sent as several pulses of at most
// max_buffer_size records, one after the other, each under its own sequence
// number. It used to go as one message of whatever size it had reached.
// What is left below a full pulse waits for the next one, as always.
func TestAnalyticsPulse_BacklogIsSentAsSeveralPulses(t *testing.T) {
	hub := &limitedHub{}
	p := newSizePlugin(hub, 100)
	ids := bufferEvents(p, "req", 250, 10)

	p.sendPulse()

	pulses := hub.pulses()
	require.Len(t, pulses, 2, "two full pulses; 50 records wait for the next")
	for i, pulse := range pulses {
		assert.EqualValues(t, 100, pulse.TotalRecords)
		assert.EqualValues(t, i+1, pulse.SequenceNumber)
		assert.Equal(t, "boot-1", pulse.BootId)
	}
	assert.Equal(t, ids[:200], hub.requestIDs(), "oldest first, each once")
	assert.Equal(t, 50, p.totalBufferedLocked())

	p.sendPulse()
	assert.Equal(t, ids, hub.requestIDs())
	assert.Zero(t, p.totalBufferedLocked())
}

// Every kind of record gets its share of a pulse, so none waits behind
// another when there is a backlog.
func TestAnalyticsPulse_BatchTakesEveryKindInProportion(t *testing.T) {
	hub := &limitedHub{}
	p := newSizePlugin(hub, 30)
	for i := 0; i < 20; i++ {
		bufferOneOfEach(p, fmt.Sprintf("r%d", i), time.Now()) // 5 records each: 100 in all
	}

	p.sendPulse()

	pulses := hub.pulses()
	require.Len(t, pulses, 3, "three pulses of 30; 10 records wait")
	for _, pulse := range pulses {
		assert.Len(t, pulse.AnalyticsEvents, 6)
		assert.Len(t, pulse.BudgetEvents, 6)
		assert.Len(t, pulse.ProxySummaries, 6)
		assert.Len(t, pulse.ComplianceEvents, 6)
		assert.Len(t, pulse.ToolCalls, 6)
	}
	assert.Equal(t, 10, p.totalBufferedLocked())
}

// Records are counted when a pulse is put together, not weighed: with
// stored request and response bodies a pulse within the record limit can
// still be too large for one gRPC message. It is then sent in parts that
// fit, the older part under the pulse's sequence number, and nothing is
// lost or sent twice.
func TestAnalyticsPulse_PulseAboveTheByteLimitIsSentInParts(t *testing.T) {
	setMaxPulseBytes(t, 4000)
	hub := &limitedHub{}
	p := newSizePlugin(hub, 100)
	ids := bufferEvents(p, "big", 40, 500) // about 22 KB in one message

	p.sendPulse()

	assert.Equal(t, ids, hub.requestIDs(), "every record, once, in order")
	for _, pulse := range hub.pulses() {
		assert.LessOrEqual(t, proto.Size(pulse), 4000)
	}
	assert.Greater(t, len(hub.pulses()), 5)
	assert.Zero(t, p.totalBufferedLocked())
	assert.Zero(t, hub.refused, "the edge kept to its own limit; the control plane refused nothing")
	assert.Zero(t, p.oversizedDropped.Load())
}

// The control plane's limit can be below the edge's. A pulse it refuses for
// its size (ResourceExhausted) is not sent again as it is and is not an
// outage: the control plane answered. The edge sends the batch in parts and
// discards nothing, although the backlog is far above max_buffer_size, the
// cap that applies while the control plane cannot be reached. This is the
// failure of the load test: a 19 MB pulse refused four times, then 268,829
// records discarded "while the control server is unreachable".
func TestAnalyticsPulse_PulseRefusedForItsSizeIsSplitNotDiscarded(t *testing.T) {
	hub := &limitedHub{limit: 3000}
	p := newSizePlugin(hub, 20)
	ids := bufferEvents(p, "req", 200, 300) // ten times the cap, 20 records are about 7 KB

	for i := 0; i < 5 && p.totalBufferedLocked() > 0; i++ {
		p.sendPulse()
	}

	assert.Equal(t, ids, hub.requestIDs(), "every record reached the control plane, once, in order")
	assert.Zero(t, p.totalBufferedLocked())
	assert.Nil(t, p.lastError, "a pulse refused for its size is not a failed control plane")
	hub.mu.Lock()
	defer hub.mu.Unlock()
	assert.Greater(t, hub.refused, 0)
	assert.Less(t, hub.refused, len(hub.accepted)*2, "a refused message is split, not sent again as it is")
	seqs := map[uint64]bool{}
	for _, pulse := range hub.accepted {
		assert.False(t, seqs[pulse.SequenceNumber], "sequence number %d used for two batches", pulse.SequenceNumber)
		seqs[pulse.SequenceNumber] = true
		assert.LessOrEqual(t, proto.Size(pulse), 3000)
	}
}

// One record that is too large by itself can never be delivered. It is
// dropped and counted, with an error in the log, and does not hold up the
// records around it.
func TestAnalyticsPulse_RecordTooLargeByItselfIsDropped(t *testing.T) {
	setMaxPulseBytes(t, 2000)
	hub := &limitedHub{}
	p := newSizePlugin(hub, 100)
	before := bufferEvents(p, "before", 3, 10)
	bufferEvents(p, "huge", 1, 5000)
	after := bufferEvents(p, "after", 3, 10)

	p.sendPulse()

	assert.Equal(t, append(before, after...), hub.requestIDs())
	assert.EqualValues(t, 1, p.oversizedDropped.Load())
	assert.Zero(t, p.totalBufferedLocked())
}

// halve always makes a snapshot of two or more records smaller, whatever
// kinds it holds, and loses nothing.
func TestPulseSnapshotHalve(t *testing.T) {
	p := newSizePlugin(&limitedHub{}, 0)
	bufferOneOfEach(p, "a", time.Now())
	one := p.takeBatchLocked(0)
	require.Equal(t, 5, one.size())
	first, second := one.halve()
	assert.Equal(t, 2, first.size())
	assert.Equal(t, 3, second.size())
	assert.Len(t, first.analytics, 1)
	assert.Len(t, first.metadata, 1)

	bufferEvents(p, "e", 3, 1)
	three := p.takeBatchLocked(0)
	first, second = three.halve()
	assert.Len(t, first.analytics, 1)
	assert.Len(t, second.analytics, 2)
	assert.Len(t, second.metadata, 2)
	assert.Equal(t, "e-1", second.metadata[0].RequestID)

	single, none := first.halve()
	assert.Equal(t, 1, single.size())
	assert.True(t, none.empty())
}

// What the edge gives up is counted by reason, for the metric an operator
// reads after an outage, and the log line says why pulses were failing: an
// unreachable control plane and one that answers but cannot store are
// different incidents although the edge keeps records the same way.
func TestAnalyticsPulse_DroppedRecordsAreCountedAndTheReasonIsNamed(t *testing.T) {
	before := PulseDroppedRecords()
	hub := &fakeSyncClient{failures: 100}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: 10, MaxRetries: 0})
	bufferEvents(p, "req", 25, 1)

	p.sendPulse() // takes 10, fails, holds them; 15 are buffered: 15 over the cap

	after := PulseDroppedRecords()
	assert.EqualValues(t, 15, after["over_cap"]-before["over_cap"])
	assert.EqualValues(t, 0, after["expired"]-before["expired"])
	assert.Equal(t, 10, p.totalBufferedLocked())
	assert.Contains(t, after, "oversized")

	assert.Equal(t, "the edge has no connection to a control plane", pulseFailureReason(errNotConnected))
	assert.Contains(t, pulseFailureReason(status.Error(codes.Unavailable, "analytics pulse not stored; retry")), "answered but could not store")
	assert.Equal(t, "the control plane could not be reached", pulseFailureReason(status.Error(codes.Unavailable, "connection refused")))
	assert.Contains(t, pulseFailureReason(status.Error(codes.DeadlineExceeded, "context deadline exceeded")), "did not answer")
	assert.Contains(t, pulseFailureReason(context.DeadlineExceeded), "did not answer")
	assert.Equal(t, "the control plane refused or failed the call", pulseFailureReason(status.Error(codes.Internal, "boom")))
}
