package plugins

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticClient is a PulseClientSource that always hands out the same client.
type staticClient struct {
	client pb.ConfigurationSyncServiceClient
}

func (s staticClient) GetGRPCClient() pb.ConfigurationSyncServiceClient { return s.client }

// swappableClient is a PulseClientSource whose client changes, like the edge
// client's across a reconnect. A nil client means disconnected.
type swappableClient struct {
	mu     sync.Mutex
	client pb.ConfigurationSyncServiceClient
}

func (s *swappableClient) GetGRPCClient() pb.ConfigurationSyncServiceClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

func (s *swappableClient) set(c pb.ConfigurationSyncServiceClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = c
}

func bufferAnalytics(t *testing.T, p *AnalyticsPulsePlugin, id string) {
	t.Helper()
	_, err := p.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{AppID: 1, RequestID: id, Timestamp: time.Now()}, nil)
	require.NoError(t, err)
}

// The pulse must send through the client the source hands out now, not the
// one it saw first: after a reconnect the first one is closed.
func TestAnalyticsPulse_SendsThroughCurrentClient(t *testing.T) {
	closed := &fakeSyncClient{failures: 1000} // a closed connection fails every call
	source := &swappableClient{client: closed}
	p := newRetryPlugin(closed, PulsePluginConfig{MaxRetries: 0})
	p.clients = source

	bufferAnalytics(t, p, "before-reconnect")
	p.sendPulse()
	assert.Equal(t, 1, closed.callCount(), "the first pulse goes to the first connection")

	fresh := &fakeSyncClient{}
	source.set(fresh)
	p.bufferMutex.Lock()
	p.lastError = nil // skip the backoff after the failed pulse
	p.bufferMutex.Unlock()
	bufferAnalytics(t, p, "after-reconnect")
	p.sendPulse()

	require.Equal(t, 1, fresh.callCount(), "the pulse must use the new connection")
	assert.Len(t, fresh.pulses[0].AnalyticsEvents, 2, "the record the failed pulse kept is sent too")
}

// While the edge is disconnected there is no client; the pulse fails like
// any transport error and keeps its data for the next one.
func TestAnalyticsPulse_NoConnectionKeepsData(t *testing.T) {
	source := &swappableClient{}
	p := newRetryPlugin(&fakeSyncClient{}, PulsePluginConfig{MaxRetries: 1})
	p.clients = source

	bufferAnalytics(t, p, "while-disconnected")
	p.sendPulse()

	p.bufferMutex.RLock()
	kept := len(p.analyticsBuffer)
	lastErr := p.lastError
	p.bufferMutex.RUnlock()
	assert.Equal(t, 1, kept)
	assert.ErrorIs(t, lastErr, errNotConnected)
}

// Stop must send what arrived while a pulse was in flight. sendPulse skips
// when a pulse is in flight, so Stop used to return with those records still
// in the buffer.
func TestAnalyticsPulse_StopWaitsForPulseInFlight(t *testing.T) {
	client := &slowSyncClient{delay: 200 * time.Millisecond}
	p := newRetryPlugin(&client.fakeSyncClient, PulsePluginConfig{})
	p.clients = staticClient{client}
	ctx, cancel := context.WithCancel(context.Background())
	p.ctx, p.cancel = ctx, cancel

	bufferAnalytics(t, p, "in-flight")
	go p.sendPulse()
	require.Eventually(t, p.sending.Load, time.Second, time.Millisecond)
	bufferAnalytics(t, p, "arrived-during-flight")

	require.NoError(t, p.Stop())
	assert.Equal(t, 2, client.recordCount(), "both records reach the hub")
}
