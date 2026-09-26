package plugins

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// slowSyncClient accepts every pulse after a delay, like a busy hub.
type slowSyncClient struct {
	fakeSyncClient
	delay   time.Duration
	records int
}

func (s *slowSyncClient) SendAnalyticsPulse(ctx context.Context, pulse *pb.AnalyticsPulse, opts ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	time.Sleep(s.delay)
	s.mu.Lock()
	s.records += len(pulse.AnalyticsEvents)
	s.mu.Unlock()
	return s.fakeSyncClient.SendAnalyticsPulse(ctx, pulse, opts...)
}

func (s *slowSyncClient) recordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records
}

// While a pulse is in flight a full buffer used to start a flush for every
// record added (each one stopping and re-creating the pulse timer), and every
// flush but the one in flight did nothing. Now a full buffer queues one flush,
// which runs once the pulse in flight has finished, and nothing is lost.
func TestAnalyticsPulse_FullBufferQueuesOneFlush(t *testing.T) {
	client := &slowSyncClient{delay: 50 * time.Millisecond}
	p := newRetryPlugin(&client.fakeSyncClient, PulsePluginConfig{MaxBufferSize: 10})
	p.grpcClient = client

	const n = 500
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := p.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{AppID: 1, RequestID: fmt.Sprint(i), Timestamp: time.Now()}, nil)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// Let the queued flush finish, then send what is left.
	assert.Eventually(t, func() bool { return !p.flushQueued.Load() && !p.sending.Load() }, 5*time.Second, 5*time.Millisecond)
	assert.NoError(t, p.Stop())

	assert.Equal(t, n, client.recordCount(), "every record reaches the hub")
	assert.LessOrEqual(t, client.callCount(), n/10, "pulses carry many records each")
}

// Stop must not leave a pulse timer armed behind it.
func TestAnalyticsPulse_StopLeavesNoTimer(t *testing.T) {
	client := &fakeSyncClient{}
	p := newRetryPlugin(client, PulsePluginConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	p.ctx, p.cancel = ctx, cancel
	p.schedulePulse()

	assert.NoError(t, p.Stop())
	p.schedulePulse() // a timer callback that fires after Stop

	p.timerMu.Lock()
	defer p.timerMu.Unlock()
	assert.Nil(t, p.pulseTimer)
}
