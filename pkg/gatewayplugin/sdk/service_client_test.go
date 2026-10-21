package sdk

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// withBrokerDial points the SDK at a fresh broker whose dials go to dial.
func withBrokerDial(t *testing.T, dial func(*goplugin.GRPCBroker, uint32) (*grpc.ClientConn, error)) {
	t.Helper()
	broker := &goplugin.GRPCBroker{}
	require.NoError(t, InitializeServiceAPI(nil, broker, 3))
	brokerConn.SetDialFunc(dial)
	SetServiceBrokerID(1)
	t.Cleanup(func() {
		brokerConn.SetDialFunc(nil)
		require.NoError(t, InitializeServiceAPI(nil, nil, 0))
	})
}

// A host service call honours its context even when the broker never hands
// over the connection: the plugin's request path is bounded by its own
// deadline, not by go-plugin's dial timeouts.
func TestServiceCall_BrokerNeverAnswersHonoursContext(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	withBrokerDial(t, func(*goplugin.GRPCBroker, uint32) (*grpc.ClientConn, error) {
		<-block
		return nil, context.DeadlineExceeded
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := GetLLM(ctx, 5)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 400*time.Millisecond)
}

// Concurrent first calls dial once and share the client, race-free.
func TestServiceCall_ConcurrentFirstCallsDialOnce(t *testing.T) {
	var dials atomic.Int32
	conn, err := grpc.NewClient("passthrough:///host", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	withBrokerDial(t, func(*goplugin.GRPCBroker, uint32) (*grpc.ClientConn, error) {
		dials.Add(1)
		time.Sleep(20 * time.Millisecond)
		return conn, nil
	})

	var wg sync.WaitGroup
	clients := make(chan interface{}, 50)
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := getServiceClient(context.Background())
			assert.NoError(t, err)
			clients <- c
		}()
	}
	wg.Wait()
	close(clients)
	var first interface{}
	for c := range clients {
		if first == nil {
			first = c
		}
		assert.Equal(t, first, c)
	}
	assert.EqualValues(t, 1, dials.Load())
	assert.Same(t, conn, GetSharedBrokerConnection())
}

// go-plugin hands over a broker ID's connection info only for a few seconds
// after the host accepts on it. The SDK dials as soon as it learns the ID,
// so a plugin whose first host call comes much later still connects.
func TestServiceCall_DialsWhenTheBrokerIDArrives(t *testing.T) {
	conn, err := grpc.NewClient("passthrough:///host", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	accepted := time.Now()
	var dials atomic.Int32
	withBrokerDial(t, func(*goplugin.GRPCBroker, uint32) (*grpc.ClientConn, error) {
		dials.Add(1)
		if time.Since(accepted) > 100*time.Millisecond {
			return nil, context.DeadlineExceeded // "timeout waiting for connection info"
		}
		return conn, nil
	})

	time.Sleep(300 * time.Millisecond) // the first host call comes late
	c, err := getServiceClient(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.EqualValues(t, 1, dials.Load(), "dialled once, when the broker ID was set")
}
