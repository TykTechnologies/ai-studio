package grpc

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

type recordingPayloads struct {
	mu  sync.Mutex
	got []*pb.PluginControlPayload
	err error
}

func (r *recordingPayloads) RouteEdgePayload(_ context.Context, p *pb.PluginControlPayload) error {
	return r.add(p)
}

func (r *recordingPayloads) ForwardEdgePayload(_ context.Context, p *pb.PluginControlPayload) error {
	return r.add(p)
}

func (r *recordingPayloads) add(p *pb.PluginControlPayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.got = append(r.got, p)
	return nil
}

func (r *recordingPayloads) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.got)
}

func pluginBatch() *pb.PluginControlBatch {
	return &pb.PluginControlBatch{
		EdgeId:        "edge-1",
		EdgeNamespace: "default",
		Payloads: []*pb.PluginControlPayload{
			{PluginId: 7, Payload: []byte("one"), EdgeId: "edge-1", CorrelationId: "c1"},
			{PluginId: 7, Payload: []byte("two"), EdgeId: "edge-1", CorrelationId: "c2"},
		},
	}
}

// A replica without plugins (a headless control plane) hands the edge's
// plugin payloads to its forwarder, which takes them to a replica that has
// them, and the edge learns they were accepted.
func TestSendPluginControlBatch_ForwardsWithoutPluginManager(t *testing.T) {
	server, _ := setupTestServer(t, nil)
	t.Cleanup(server.Stop)
	fwd := &recordingPayloads{}
	server.SetEdgePayloadForwarder(fwd)

	resp, err := server.SendPluginControlBatch(context.Background(), pluginBatch())
	require.NoError(t, err)
	assert.True(t, resp.Success, resp.Message)
	assert.Equal(t, uint64(2), resp.ProcessedCount)
	assert.Empty(t, resp.Errors)
	assert.Contains(t, resp.Message, "queued for the plugin host")
	assert.Equal(t, 2, fwd.count())
}

// A replica with plugins routes them itself, even with a forwarder set.
func TestSendPluginControlBatch_RoutesLocallyWhenItHasPlugins(t *testing.T) {
	server, _ := setupTestServer(t, nil)
	t.Cleanup(server.Stop)
	local, fwd := &recordingPayloads{}, &recordingPayloads{}
	server.SetPluginManager(local)
	server.SetEdgePayloadForwarder(fwd)

	resp, err := server.SendPluginControlBatch(context.Background(), pluginBatch())
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, 2, local.count())
	assert.Zero(t, fwd.count())
}

// A payload the forwarder cannot take fails on its own, in the response: a
// gRPC error would make the edge keep and resend the whole batch.
func TestSendPluginControlBatch_ForwardFailureIsPerPayload(t *testing.T) {
	server, _ := setupTestServer(t, nil)
	t.Cleanup(server.Stop)
	server.SetEdgePayloadForwarder(&recordingPayloads{err: errors.New("cluster log unavailable")})

	resp, err := server.SendPluginControlBatch(context.Background(), pluginBatch())
	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.Len(t, resp.Errors, 2)
	assert.Contains(t, resp.Errors[0].ErrorMessage, "cluster log unavailable")
	assert.Equal(t, "c1", resp.Errors[0].CorrelationId)
}
