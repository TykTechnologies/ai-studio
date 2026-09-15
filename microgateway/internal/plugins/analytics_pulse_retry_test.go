package plugins

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
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
		grpcClient:     client,
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
// cleared before the send and nothing put the data back on failure.
func TestAnalyticsPulse_FailedPulseRestoresEveryBufferAheadOfNewerData(t *testing.T) {
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
	require.Len(t, p.analyticsBuffer, 2)
	assert.Equal(t, "old", p.analyticsBuffer[0].RequestID)
	assert.Equal(t, "new", p.analyticsBuffer[1].RequestID)
	require.Len(t, p.analyticsMetadata, 2)
	assert.Equal(t, "old", p.analyticsMetadata[0].RequestID, "metadata stays paired with its event")
	require.Len(t, p.budgetBuffer, 2)
	require.Len(t, p.proxyBuffer, 2)
	assert.Equal(t, "old", p.proxyBuffer[0].Vendor)
	require.Len(t, p.complianceBuffer, 2)
	assert.Equal(t, "old", p.complianceBuffer[0].FilterName)
	require.Len(t, p.toolCallBuffer, 2)
	assert.Equal(t, "old", p.toolCallBuffer[0].OperationID)
	assert.Error(t, p.lastError)
	assert.Equal(t, uint64(2), p.sequenceNumber, "the failed pulse consumed a sequence number")
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
	require.Len(t, p.analyticsBuffer, 1)
	assert.Equal(t, "fresh", p.analyticsBuffer[0].RequestID)
	require.Len(t, p.analyticsMetadata, 1)
	assert.Equal(t, "fresh", p.analyticsMetadata[0].RequestID)
	assert.Len(t, p.budgetBuffer, 1)
	assert.Len(t, p.proxyBuffer, 1)
	assert.Len(t, p.complianceBuffer, 1)
	assert.Len(t, p.toolCallBuffer, 1)
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
	require.Len(t, p.analyticsBuffer, 3, "analytics events are trimmed before budget records")
	assert.Equal(t, "3", p.analyticsBuffer[0].RequestID, "the oldest events were dropped")
	assert.Equal(t, "5", p.analyticsBuffer[2].RequestID)
	assert.Equal(t, "3", p.analyticsMetadata[0].RequestID)
	assert.Len(t, p.budgetBuffer, 1, "budget records survive the trim")
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
