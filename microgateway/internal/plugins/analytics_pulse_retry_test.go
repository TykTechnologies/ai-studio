package plugins

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeSyncClient fails the first `failures` SendAnalyticsPulse calls and then
// accepts everything, recording every pulse it was offered.
type fakeSyncClient struct {
	pb.ConfigurationSyncServiceClient
	mu       sync.Mutex
	failures int
	calls    int
	pulses   []*pb.AnalyticsPulse
}

func (f *fakeSyncClient) SendAnalyticsPulse(_ context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.pulses = append(f.pulses, pulse)
	if f.calls <= f.failures {
		return nil, errors.New("hub unreachable")
	}
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

func (f *fakeSyncClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newRetryPlugin(client *fakeSyncClient, cfg PulsePluginConfig) *AnalyticsPulsePlugin {
	if cfg.MaxBufferSize == 0 {
		cfg.MaxBufferSize = 100
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 1
	}
	if cfg.EdgeRetentionHours == 0 {
		cfg.EdgeRetentionHours = 24
	}
	// sendPulseNow reschedules the timer; keep it far away so a test never
	// races a timer-driven pulse.
	cfg.IntervalSeconds = 3600
	cfg.IncludeProxySummaries = true
	return &AnalyticsPulsePlugin{
		config:         &cfg,
		edgeID:         "edge-1",
		edgeNamespace:  "test",
		clients:        staticClient{client},
		sequenceNumber: 1,
		lastPulseTime:  time.Now(),
		retryInterval:  time.Millisecond,
	}
}

func bufferOneOfEach(p *AnalyticsPulsePlugin, id string, ts time.Time) {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()
	p.analyticsBuffer = append(p.analyticsBuffer, database.AnalyticsEvent{RequestID: id, TimeStamp: ts})
	p.analyticsMetadata = append(p.analyticsMetadata, AnalyticsMetadata{RequestID: id})
	p.budgetBuffer = append(p.budgetBuffer, BudgetUsageBuffer{AppID: 1, Cost: 1, Timestamp: ts})
	p.proxyBuffer = append(p.proxyBuffer, ProxyLogBuffer{Vendor: id, LastRequest: ts})
	p.complianceBuffer = append(p.complianceBuffer, ComplianceEventBuffer{FilterName: id, Timestamp: ts})
	p.toolCallBuffer = append(p.toolCallBuffer, ToolCallBuffer{OperationID: id, Timestamp: ts})
}

// A pulse the hub never received used to be gone for good: the buffers were
// cleared before the send and nothing put the data back on failure. It is
// held, every kind of record, apart from what arrives afterwards.
func TestAnalyticsPulse_FailedPulseIsHeldAheadOfNewerData(t *testing.T) {
	client := &fakeSyncClient{failures: 100}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 1})
	now := time.Now()
	bufferOneOfEach(p, "old", now.Add(-time.Minute))

	p.sendPulse()
	assert.Equal(t, 2, client.callCount(), "one attempt plus MaxRetries retries")

	// Something arrives while the hub is down.
	bufferOneOfEach(p, "new", now)

	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()
	require.NotNil(t, p.held, "the failed batch is held")
	held := p.held.snap
	require.Len(t, held.analytics, 1)
	assert.Equal(t, "old", held.analytics[0].RequestID)
	require.Len(t, held.metadata, 1)
	assert.Equal(t, "old", held.metadata[0].RequestID, "metadata stays paired with its event")
	require.Len(t, held.budget, 1)
	require.Len(t, held.proxy, 1)
	assert.Equal(t, "old", held.proxy[0].Vendor)
	require.Len(t, held.compliance, 1)
	assert.Equal(t, "old", held.compliance[0].FilterName)
	require.Len(t, held.toolCalls, 1)
	assert.Equal(t, "old", held.toolCalls[0].OperationID)
	assert.Equal(t, uint64(1), p.held.seq, "it keeps the sequence number it was sent under")

	require.Len(t, p.analyticsBuffer, 1, "newer data is buffered apart from it")
	assert.Equal(t, "new", p.analyticsBuffer[0].RequestID)
	assert.Equal(t, 10, p.totalBufferedLocked(), "both count as buffered")
	assert.Error(t, p.lastError)
	assert.Equal(t, uint64(2), p.sequenceNumber, "the failed pulse consumed a sequence number")
}

// A batch that was not acknowledged is sent again as the same batch: the
// same records under the same sequence number and boot ID, never merged with
// what arrived since. The hub stores a batch once by that identity; merged
// and renumbered, as it used to be, a batch the hub had in fact stored (its
// answer lost with the control plane) was stored again.
func TestAnalyticsPulse_RetriedBatchKeepsItsIdentityAndBoundaries(t *testing.T) {
	client := &fakeSyncClient{failures: 2}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 0})
	p.bootID = "boot-1"
	now := time.Now()
	bufferOneOfEach(p, "first", now.Add(-time.Minute))

	p.sendPulse() // fails
	bufferOneOfEach(p, "second", now.Add(-30*time.Second))
	p.sendPulse() // fails again: still the first batch, alone
	bufferOneOfEach(p, "third", now)
	p.sendPulse() // the hub is back

	require.Equal(t, 4, client.callCount(), "two failures, then the held batch and the newer one")
	for i := 0; i < 3; i++ {
		pulse := client.pulses[i]
		assert.Equal(t, uint64(1), pulse.SequenceNumber, "attempt %d", i+1)
		assert.Equal(t, "boot-1", pulse.BootId)
		require.Len(t, pulse.AnalyticsEvents, 1, "attempt %d carries only the first batch's records", i+1)
		assert.Equal(t, "first", pulse.AnalyticsEvents[0].RequestId)
		assert.Len(t, pulse.BudgetEvents, 1)
		assert.Len(t, pulse.ComplianceEvents, 1)
		assert.Len(t, pulse.ToolCalls, 1)
		assert.EqualValues(t, 5, pulse.TotalRecords)
	}
	next := client.pulses[3]
	assert.Equal(t, uint64(2), next.SequenceNumber, "what arrived meanwhile is the next batch")
	assert.Equal(t, "boot-1", next.BootId)
	require.Len(t, next.AnalyticsEvents, 2)
	assert.Equal(t, "second", next.AnalyticsEvents[0].RequestId)
	assert.Equal(t, "third", next.AnalyticsEvents[1].RequestId)

	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()
	assert.Nil(t, p.held)
	assert.Equal(t, 0, p.totalBufferedLocked(), "everything was delivered")
	assert.NoError(t, p.lastError)
	assert.Equal(t, uint64(2), p.totalPulsesSent)
	assert.Equal(t, uint64(15), p.totalRecordsSent)
}

// A held batch may lose records (retention, the buffer cap) but keeps its
// sequence number and never gains any: if the hub stored the first attempt,
// it acknowledges the smaller repeat without storing it; if it did not, it
// stores what is left.
func TestAnalyticsPulse_TrimmedHeldBatchKeepsItsSequenceNumber(t *testing.T) {
	client := &fakeSyncClient{failures: 2}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 0, MaxBufferSize: 3})
	now := time.Now()
	p.bufferMutex.Lock()
	for _, id := range []string{"1", "2", "3"} {
		p.analyticsBuffer = append(p.analyticsBuffer, database.AnalyticsEvent{RequestID: id, TimeStamp: now})
		p.analyticsMetadata = append(p.analyticsMetadata, AnalyticsMetadata{RequestID: id})
	}
	p.bufferMutex.Unlock()
	p.sendPulse() // fails: 1, 2, 3 held under sequence 1

	p.bufferMutex.Lock()
	for _, id := range []string{"4", "5"} {
		p.analyticsBuffer = append(p.analyticsBuffer, database.AnalyticsEvent{RequestID: id, TimeStamp: now})
		p.analyticsMetadata = append(p.analyticsMetadata, AnalyticsMetadata{RequestID: id})
	}
	p.bufferMutex.Unlock()
	p.sendPulse() // fails: 5 records against a cap of 3, the held batch gives up 1 and 2
	p.sendPulse() // delivered

	require.Equal(t, 4, client.callCount())
	last := client.pulses[2]
	assert.Equal(t, uint64(1), last.SequenceNumber)
	require.Len(t, last.AnalyticsEvents, 1, "the held batch was trimmed, oldest first")
	assert.Equal(t, "3", last.AnalyticsEvents[0].RequestId)
	fresh := client.pulses[3]
	assert.Equal(t, uint64(2), fresh.SequenceNumber)
	require.Len(t, fresh.AnalyticsEvents, 2)
	assert.Equal(t, "4", fresh.AnalyticsEvents[0].RequestId)
	assert.Equal(t, "5", fresh.AnalyticsEvents[1].RequestId)
}

// A held batch whose records have all expired is dropped: there is nothing
// to send again, and the next pulse is a new batch.
func TestAnalyticsPulse_ExpiredHeldBatchIsDropped(t *testing.T) {
	client := &fakeSyncClient{failures: 1}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 0, EdgeRetentionHours: 1})
	now := time.Now()
	bufferOneOfEach(p, "stale", now.Add(-2*time.Hour))

	p.sendPulse()
	p.bufferMutex.RLock()
	assert.Nil(t, p.held)
	assert.Equal(t, 0, p.totalBufferedLocked())
	p.bufferMutex.RUnlock()

	bufferOneOfEach(p, "fresh", now)
	p.sendPulse()
	require.Equal(t, 2, client.callCount())
	assert.Equal(t, uint64(2), client.pulses[1].SequenceNumber)
	assert.Equal(t, "fresh", client.pulses[1].AnalyticsEvents[0].RequestId)
}

// Every plugin instance draws a boot ID of its own and sends it with each
// pulse; sequence numbers start at 1 again after a restart, and the boot ID
// keeps the two runs' batches apart on the hub.
func TestAnalyticsPulse_BootIDIsPerProcess(t *testing.T) {
	a, err := NewAnalyticsPulsePlugin("edge-1", "test", staticClient{&fakeSyncClient{}}, map[string]interface{}{})
	require.NoError(t, err)
	b, err := NewAnalyticsPulsePlugin("edge-1", "test", staticClient{&fakeSyncClient{}}, map[string]interface{}{})
	require.NoError(t, err)
	require.NotEmpty(t, a.bootID)
	assert.NotEqual(t, a.bootID, b.bootID)

	pulse := a.buildPulseMessage(nil, nil, nil, nil, nil, nil, 7)
	assert.Equal(t, a.bootID, pulse.BootId)
	assert.Equal(t, uint64(7), pulse.SequenceNumber)

	// Capture-only pulses carry no boot ID: their events have idempotency
	// keys of their own on the hub.
	assert.Empty(t, a.capturePulse(nil).BootId)
}

func TestAnalyticsPulse_RetrySucceedsWithinLadder(t *testing.T) {
	client := &fakeSyncClient{failures: 2}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 3})
	bufferOneOfEach(p, "a", time.Now())

	p.sendPulse()

	assert.Equal(t, 3, client.callCount())
	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()
	assert.Equal(t, 0, p.totalBufferedLocked(), "delivered data is released")
	assert.NoError(t, p.lastError)
	assert.Equal(t, uint64(1), p.totalPulsesSent)
	assert.Equal(t, uint64(5), p.totalRecordsSent)
}

func TestAnalyticsPulse_RestoreDropsRecordsPastRetention(t *testing.T) {
	client := &fakeSyncClient{failures: 100}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 0, EdgeRetentionHours: 1})
	now := time.Now()
	bufferOneOfEach(p, "stale", now.Add(-2*time.Hour))
	bufferOneOfEach(p, "fresh", now.Add(-time.Minute))

	p.sendPulse()

	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()
	require.NotNil(t, p.held)
	held := p.held.snap
	require.Len(t, held.analytics, 1)
	assert.Equal(t, "fresh", held.analytics[0].RequestID)
	require.Len(t, held.metadata, 1)
	assert.Equal(t, "fresh", held.metadata[0].RequestID)
	assert.Len(t, held.budget, 1)
	assert.Len(t, held.proxy, 1)
	assert.Len(t, held.compliance, 1)
	assert.Len(t, held.toolCalls, 1)
}

func TestAnalyticsPulse_RestoreCapsBufferDroppingOldest(t *testing.T) {
	client := &fakeSyncClient{failures: 100}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 0, MaxBufferSize: 4})
	now := time.Now()
	p.bufferMutex.Lock()
	for _, id := range []string{"1", "2", "3"} {
		p.analyticsBuffer = append(p.analyticsBuffer, database.AnalyticsEvent{RequestID: id, TimeStamp: now})
		p.analyticsMetadata = append(p.analyticsMetadata, AnalyticsMetadata{RequestID: id})
	}
	p.budgetBuffer = append(p.budgetBuffer, BudgetUsageBuffer{AppID: 1, Timestamp: now})
	p.bufferMutex.Unlock()

	p.sendPulse()

	// Two more arrive while the hub is down: 6 records against a cap of 4.
	p.bufferMutex.Lock()
	for _, id := range []string{"4", "5"} {
		p.analyticsBuffer = append(p.analyticsBuffer, database.AnalyticsEvent{RequestID: id, TimeStamp: now})
		p.analyticsMetadata = append(p.analyticsMetadata, AnalyticsMetadata{RequestID: id})
	}
	p.bufferMutex.Unlock()

	p.sendPulse()

	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()
	assert.Equal(t, 4, p.totalBufferedLocked())
	// The held batch is the oldest: it gives up its first two events.
	require.NotNil(t, p.held)
	held := p.held.snap
	require.Len(t, held.analytics, 1, "analytics events are trimmed before budget records")
	assert.Equal(t, "3", held.analytics[0].RequestID, "the oldest events were dropped")
	assert.Equal(t, "3", held.metadata[0].RequestID)
	assert.Len(t, held.budget, 1, "budget records survive the trim")
	require.Len(t, p.analyticsBuffer, 2)
	assert.Equal(t, "4", p.analyticsBuffer[0].RequestID)
	assert.Equal(t, "5", p.analyticsBuffer[1].RequestID)
}

func TestAnalyticsPulse_BufferFullTriggerSuppressedDuringBackoff(t *testing.T) {
	client := &fakeSyncClient{failures: 100}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 0, MaxBufferSize: 2})
	p.retryInterval = time.Hour // the backoff must outlive the test
	bufferOneOfEach(p, "a", time.Now())

	p.sendPulse()
	require.Equal(t, 1, client.callCount())

	// The buffer is over its cap, but the last pulse failed a moment ago.
	_, err := p.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{RequestID: "b", Timestamp: time.Now()}, nil)
	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, client.callCount(), "no pulse fired while in backoff")

	// Once the backoff has passed a full buffer triggers a pulse again.
	p.bufferMutex.Lock()
	p.lastErrorTime = time.Now().Add(-2 * time.Hour)
	p.bufferMutex.Unlock()
	_, err = p.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{RequestID: "c", Timestamp: time.Now()}, nil)
	require.NoError(t, err)
	assert.Eventually(t, func() bool { return client.callCount() >= 2 }, time.Second, 5*time.Millisecond)
}

func TestAnalyticsPulse_OnlyOnePulseInFlight(t *testing.T) {
	client := &fakeSyncClient{}
	p := newRetryPlugin(client, PulsePluginConfig{})
	bufferOneOfEach(p, "a", time.Now())

	require.True(t, p.sending.CompareAndSwap(false, true))
	p.sendPulse() // a pulse is "already running": this call must not send
	assert.Equal(t, 0, client.callCount())
	p.sending.Store(false)

	p.sendPulse()
	assert.Equal(t, 1, client.callCount())
}

func TestAnalyticsPulse_StoppingCutsTheRetryLadderShort(t *testing.T) {
	client := &fakeSyncClient{failures: 100}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 5})
	p.retryInterval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	p.ctx, p.cancel = ctx, cancel
	bufferOneOfEach(p, "a", time.Now())

	cancel()
	done := make(chan struct{})
	go func() { p.sendPulse(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendPulse sat through the retry ladder after the plugin was stopped")
	}
	assert.Equal(t, 1, client.callCount(), "one attempt, then give up")
	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()
	assert.Equal(t, 5, p.totalBufferedLocked(), "the unsent data is still buffered")
}
