package grpc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credentialChangedFrame is a credential.changed event as it arrives on an
// edge's stream.
func credentialChangedFrame(t *testing.T) *pb.EventFrame {
	t.Helper()
	payload, err := json.Marshal(eventbridge.CredentialChanged{CredentialID: 11, Hash: "v1:00", Active: false})
	require.NoError(t, err)
	return &pb.EventFrame{Id: "evt-1", Topic: eventbridge.CredentialChangedTopic, Origin: "control",
		Dir: int32(eventbridge.DirDown), Payload: payload}
}

func edgeWithBridge(t *testing.T) *SimpleEdgeClient {
	t.Helper()
	c := NewSimpleEdgeClient(&config.Config{HubSpoke: config.HubSpokeConfig{EdgeID: "edge-1", EdgeNamespace: "ns"}}, "test", "hash", "time")
	c.setupEventBridge()
	t.Cleanup(c.stopEventBridge)
	return c
}

// An edge that does not act on credential.changed (one from before the
// event existed, or one with no token cache to drop) takes the event
// without error: it reaches the bus, where nothing listens to it.
func TestCredentialChangedEvent_IgnoredByAnEdgeWithoutAHandler(t *testing.T) {
	c := edgeWithBridge(t)
	seen := make(chan eventbridge.Event, 1)
	sub := c.GetEventBus().SubscribeAll(func(e eventbridge.Event) { seen <- e })
	defer c.GetEventBus().Unsubscribe(sub)

	require.NoError(t, c.processControlMessage(&pb.ControlMessage{Message: &pb.ControlMessage_Event{Event: credentialChangedFrame(t)}}))
	select {
	case e := <-seen:
		assert.Equal(t, eventbridge.CredentialChangedTopic, e.Topic)
	case <-time.After(5 * time.Second):
		t.Fatal("the event did not reach the edge's bus")
	}
}

// With a handler subscribed, as cmd/microgateway wires the hybrid gateway
// service, the event reaches it with its payload intact.
func TestCredentialChangedEvent_ReachesTheSubscribedHandler(t *testing.T) {
	c := edgeWithBridge(t)
	got := make(chan eventbridge.CredentialChanged, 1)
	sub := c.GetEventBus().Subscribe(eventbridge.CredentialChangedTopic, func(e eventbridge.Event) {
		change, err := eventbridge.ParseCredentialChanged(e.Payload)
		if err == nil {
			got <- change
		}
	})
	defer c.GetEventBus().Unsubscribe(sub)

	require.NoError(t, c.processControlMessage(&pb.ControlMessage{Message: &pb.ControlMessage_Event{Event: credentialChangedFrame(t)}}))
	select {
	case change := <-got:
		assert.EqualValues(t, 11, change.CredentialID)
		assert.False(t, change.Active)
	case <-time.After(5 * time.Second):
		t.Fatal("the handler was not called")
	}
}
