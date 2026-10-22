package ai_studio_sdk

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
	initMutex.Lock()
	grpcBroker, initialized = broker, true
	initMutex.Unlock()
	brokerConn.SetBroker(broker) // a new broker drops any earlier connection
	brokerConn.SetDialFunc(dial)
	SetServiceBrokerID(1)
	t.Cleanup(func() {
		initMutex.Lock()
		grpcBroker, initialized, serviceBrokerID = nil, false, 0
		initMutex.Unlock()
		brokerConn.SetDialFunc(nil)
		brokerConn.SetBroker(nil)
	})
}

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
	_, err := getServiceClient(ctx)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 400*time.Millisecond)
}

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
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := getServiceClient(context.Background())
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, dials.Load())
	assert.Same(t, conn, GetSharedBrokerConnection())
}

// See the gateway SDK's test of the same name.
func TestServiceCall_DialsWhenTheBrokerIDArrives(t *testing.T) {
	conn, err := grpc.NewClient("passthrough:///host", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	accepted := time.Now()
	var dials atomic.Int32
	withBrokerDial(t, func(*goplugin.GRPCBroker, uint32) (*grpc.ClientConn, error) {
		dials.Add(1)
		if time.Since(accepted) > 100*time.Millisecond {
			return nil, context.DeadlineExceeded
		}
		return conn, nil
	})

	time.Sleep(300 * time.Millisecond)
	_, err = getServiceClient(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, dials.Load())
}
