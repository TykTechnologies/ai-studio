package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// relayReplica is a control server whose bus is joined to the other
// replicas' through the cluster event log, as pkg/studio wires it.
func relayReplica(t *testing.T, db *gorm.DB, id string) *ControlServer {
	t.Helper()
	server := replicaServer(t, db, id)
	log := cluster.NewLog(db, id, cluster.LogOptions{PollInterval: 50 * time.Millisecond})
	require.NoError(t, log.Start(context.Background()))
	relay := cluster.NewRelay(log, server.GetEventBus(), cluster.RelayOptions{})
	relay.Start()
	t.Cleanup(func() { relay.Stop(); log.Stop() })
	return server
}

func edgeEvents(stream *fakeEdgeStream, topic string) []*pb.EventFrame {
	var out []*pb.EventFrame
	for _, m := range stream.sent() {
		if ev := m.GetEvent(); ev != nil && ev.Topic == topic {
			out = append(out, ev)
		}
	}
	return out
}

// An edge-bound event published on replica A (budget sync, a plugin's
// DirDown event) reaches edges streaming to replica B, once; object events
// reach B's bus but are never sent to edges.
func TestRelay_EdgeEventsReachEdgesOnEveryReplica(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		if db.Dialector.Name() != "postgres" {
			t.Skip("the cluster event log needs Postgres")
		}
		a := relayReplica(t, db, "node-a")
		b := relayReplica(t, db, "node-b")
		registerEdge(t, a, "edge-a", "default")
		registerEdge(t, b, "edge-b", "default")
		onA, stopA := connect(t, a, "edge-a", "default")
		defer stopA()
		onB, stopB := connect(t, b, "edge-b", "default")
		defer stopB()

		require.NoError(t, eventbridge.PublishDown(a.GetEventBus(), "control", "budget.sync", map[string]int{"seq": 1}))
		sys, err := eventbridge.NewEvent("system.llm.updated", "control", eventbridge.DirLocal, map[string]int{"object_id": 1})
		require.NoError(t, err)
		a.GetEventBus().Publish(sys)

		require.Eventually(t, func() bool { return len(edgeEvents(onB, "budget.sync")) == 1 }, 10*time.Second, 20*time.Millisecond,
			"the edge on B gets A's budget sync")
		require.Eventually(t, func() bool { return len(edgeEvents(onA, "budget.sync")) == 1 }, 5*time.Second, 20*time.Millisecond)

		time.Sleep(500 * time.Millisecond)
		assert.Len(t, edgeEvents(onA, "budget.sync"), 1, "once on A")
		assert.Len(t, edgeEvents(onB, "budget.sync"), 1, "once on B")
		assert.Empty(t, edgeEvents(onA, "system.llm.updated"), "object events stay in the control plane")
		assert.Empty(t, edgeEvents(onB, "system.llm.updated"))
	})
}
