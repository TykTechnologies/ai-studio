package cluster

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	edgegrpc "github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	studiogrpc "github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// lossyAck is the edge's control client with one fault: when armed, the next
// analytics pulse reaches the control plane and is stored there, but its
// acknowledgement never arrives (onLoss stands for whatever took the control
// plane away in between).
type lossyAck struct {
	edge   *edgegrpc.SimpleEdgeClient
	armed  atomic.Bool
	onLoss func()
	// pulses counts the pulses that reached a control plane, lost the
	// acknowledgements dropped and acked those that reached the edge.
	pulses atomic.Int32
	lost   atomic.Int32
	acked  atomic.Int32
}

func (l *lossyAck) GetGRPCClient() pb.ConfigurationSyncServiceClient {
	c := l.edge.GetGRPCClient()
	if c == nil {
		return nil
	}
	return &lossyAckClient{ConfigurationSyncServiceClient: c, l: l}
}

type lossyAckClient struct {
	pb.ConfigurationSyncServiceClient
	l *lossyAck
}

func (c *lossyAckClient) SendAnalyticsPulse(ctx context.Context, in *pb.AnalyticsPulse, opts ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	resp, err := c.ConfigurationSyncServiceClient.SendAnalyticsPulse(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	c.l.pulses.Add(1)
	if c.l.armed.CompareAndSwap(true, false) {
		c.l.lost.Add(1)
		c.l.onLoss()
		return nil, status.Error(codes.Unavailable, "connection lost before the acknowledgement arrived")
	}
	c.l.acked.Add(1)
	return resp, nil
}

type hubAnalytics struct {
	logs, records, batches int64
	spend                  float64
	tokens                 int64
}

func countHubAnalytics(t *testing.T, db *gorm.DB) hubAnalytics {
	t.Helper()
	var c hubAnalytics
	require.NoError(t, db.Model(&models.ProxyLog{}).Count(&c.logs).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Count(&c.records).Error)
	require.NoError(t, db.Model(&models.EdgeAnalyticsBatch{}).Count(&c.batches).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Select("COALESCE(SUM(cost), 0)").Scan(&c.spend).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Select("COALESCE(SUM(total_tokens), 0)").Scan(&c.tokens).Error)
	return c
}

// The control plane stores an edge's analytics pulse and is lost before its
// acknowledgement reaches the edge. The edge sends the same batch again, to
// the control plane it moved to. The records, the spend and the token counts
// (what budget usage is computed from) are in the database once.
//
// A real edge and its analytics pulse plugin against two Studio replicas on
// one database; before batch identities the second replica stored the batch
// again.
func TestE2E_PulseAcknowledgementLost_BatchStoredOnce(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		fwd := newForwarder(t, a.addr)
		e := startEdge(t, "edge-1", "", fwd)
		waitOwner(t, b.db, "edge-1", "node-a")

		// The replicas' analytics go to the shared database.
		require.NoError(t, analytics.Migrate(b.db))
		analytics.ResetHandler()
		analytics.InitDefault(context.Background(), b.db)
		t.Cleanup(analytics.ResetHandler)

		source := &lossyAck{edge: e.client}
		// Site A goes away the moment it has stored the pulse: the edge's
		// connections drop and new ones reach replica B.
		source.onLoss = func() {
			fwd.route(b.addr)
			fwd.cut()
		}
		pulse, err := plugins.NewAnalyticsPulsePlugin("edge-1", "", source, map[string]interface{}{
			"interval_seconds":    1,
			"timeout_seconds":     5,
			"max_retries":         5,
			"retry_interval_secs": 1,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = pulse.Stop() })

		record := func(id string, cost float64, tokens int) {
			t.Helper()
			_, err := pulse.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
				AppID: 1, LLMID: 1, RequestID: id, ModelName: "gpt-4o", Vendor: "openai",
				PromptTokens: tokens, TotalTokens: tokens, Cost: cost, StatusCode: 200, Timestamp: time.Now(),
			}, nil)
			require.NoError(t, err)
		}

		record("r1", 120, 100)
		record("r2", 30, 20)
		source.armed.Store(true)
		// The pulse timer sends what is buffered: the pulse is stored by A,
		// its answer is lost, and the retry ladder sends the same batch
		// again once the edge is on B.
		require.NoError(t, pulse.Initialize(nil))

		waitOwner(t, b.db, "edge-1", "node-b")
		require.Eventually(t, func() bool { return source.acked.Load() >= 1 }, 20*time.Second, 20*time.Millisecond,
			"the batch was never sent again and acknowledged")
		assert.EqualValues(t, 1, source.lost.Load(), "one acknowledgement was lost")
		assert.EqualValues(t, 2, source.pulses.Load(), "the batch reached a control plane twice")

		want := hubAnalytics{logs: 2, records: 2, batches: 1, spend: 150, tokens: 120}
		time.Sleep(300 * time.Millisecond) // nothing arrives late from a background writer either
		assert.Equal(t, want, countHubAnalytics(t, b.db), "the batch whose acknowledgement was lost is stored once")

		// The edge's next batch is stored as usual, through B: a batch of
		// its own, so the edge held nothing of the first one back.
		record("r3", 50, 10)
		want = hubAnalytics{logs: 3, records: 3, batches: 2, spend: 200, tokens: 130}
		require.Eventually(t, func() bool { return countHubAnalytics(t, b.db) == want }, 20*time.Second, 50*time.Millisecond,
			"the next batch: %+v", countHubAnalytics(t, b.db))
	})
}

// A backlog of analytics on the edge (what gathers while pulses fail, as
// during a database switchover on the control plane's side) makes a pulse
// larger than the control plane's gRPC message limit. The control plane
// refuses such a message before any handler sees it. The edge used to send
// it again as it was, four times, and then discard everything above its
// buffer cap "while the control server is unreachable"; the load test lost
// 4% of a run's analytics that way. It now sends the batch in parts that
// fit: every record is in the database, once.
//
// A real edge and its pulse plugin against a real control server whose
// limit is set low (GRPC_MAX_MESSAGE_SIZE), so that the refusal is gRPC's
// own.
func TestE2E_PulseLargerThanTheControlPlanesMessageLimit_IsSentInParts(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		const limit = 48 * 1024
		a := startReplica(t, sdb, "node-a", func(c *studiogrpc.Config) { c.MaxMessageSize = limit })
		e := startEdge(t, "edge-1", "", newForwarder(t, a.addr))
		waitOwner(t, a.db, "edge-1", "node-a")

		require.NoError(t, analytics.Migrate(a.db))
		analytics.ResetHandler()
		analytics.InitDefault(context.Background(), a.db)
		t.Cleanup(analytics.ResetHandler)

		pulse, err := plugins.NewAnalyticsPulsePlugin("edge-1", "", e.client, map[string]interface{}{
			"interval_seconds":    3600, // the test sends
			"timeout_seconds":     10,
			"max_retries":         3,
			"retry_interval_secs": 1,
			"max_buffer_size":     5000,
		})
		require.NoError(t, err)
		require.NoError(t, pulse.Initialize(nil))

		// 1,500 requests of 0.01 each: about 160 KB as one pulse, three
		// times the limit, and below the buffer size, so one batch.
		const requests = 1500
		for i := 0; i < requests; i++ {
			_, err := pulse.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
				AppID: 1, LLMID: 1, RequestID: fmt.Sprintf("backlog-%04d", i), GatewayRequestID: fmt.Sprintf("backlog-%04d", i), ModelName: "gpt-4o", Vendor: "openai",
				PromptTokens: 10, TotalTokens: 10, Cost: 100, StatusCode: 200, Timestamp: time.Now(),
			}, nil)
			require.NoError(t, err)
		}
		// Stop sends what is buffered and waits for it.
		require.NoError(t, pulse.Stop())

		require.Eventually(t, func() bool { return countHubAnalytics(t, a.db).logs >= requests }, 30*time.Second, 100*time.Millisecond,
			"the control plane has %+v", countHubAnalytics(t, a.db))
		time.Sleep(300 * time.Millisecond) // nothing arrives late, or twice
		got := countHubAnalytics(t, a.db)
		assert.EqualValues(t, requests, got.logs, "every request's record, once")
		assert.EqualValues(t, requests, got.records)
		assert.EqualValues(t, requests*10, got.tokens)
		assert.Greater(t, got.batches, int64(3), "sent in parts")
		var distinct int64
		require.NoError(t, a.db.Model(&models.ProxyLog{}).Distinct("request_id").Count(&distinct).Error)
		assert.EqualValues(t, requests, distinct, "no record twice")
		stats := pulse.GetStats()
		assert.EqualValues(t, 0, stats["oversized_dropped"])
	})
}
