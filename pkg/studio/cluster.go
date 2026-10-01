package studio

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// clusterParts is this process's membership of the Studio replicas sharing
// its database (pkg/cluster): a full Studio (New) and a headless control
// plane (NewControlPlane) both join the same way.
type clusterParts struct {
	// clusterNode registers this replica; clusterLog carries events between
	// replicas (pkg/cluster).
	clusterNode *cluster.Node
	clusterLog  *cluster.Log
	// relay carries edge-bound and object-change bus events to the other
	// replicas through clusterLog.
	relay *cluster.Relay
	// leadership is this replica's claim on the leader lease, which
	// singleton background work runs under. Nil: this replica never leads
	// (a headless control plane).
	leadership *cluster.Leadership
	// unsubscribeSignals stops delivering other replicas' signals.
	unsubscribeSignals func()
	// signals writes this replica's replica signals in the background.
	signals *signalSender
}

// Labels a replica gets when its host names none.
const (
	DefaultNodeLabel         = "studio"
	DefaultControlPlaneLabel = "control-plane"
)

// nodeLabel is label, or def when it is empty.
func nodeLabel(label, def string) string {
	if label == "" {
		return def
	}
	return label
}

// joinCluster registers this replica and starts reading the event log. With
// one replica (or SQLite) this is one row and an idle reader.
func (c *clusterParts) joinCluster(ctx context.Context, db *gorm.DB, nodeID, version string, node cluster.NodeOptions) (err error) {
	if c.clusterNode, err = cluster.StartNode(db, nodeID, version, node); err != nil {
		return fmt.Errorf("studio: %w", err)
	}
	c.clusterLog = cluster.NewLog(db, nodeID, cluster.LogOptions{})
	if err := c.clusterLog.Start(ctx); err != nil {
		return fmt.Errorf("studio: %w", err)
	}
	return nil
}

// startRelay starts carrying bus events between this replica and the others.
func (c *clusterParts) startRelay(bus eventbridge.Bus, opts cluster.RelayOptions) {
	c.relay = cluster.NewRelay(c.clusterLog, bus, opts)
	c.relay.Start()
}

// stopCluster leaves the cluster, in this order: the relay (events still
// queued are written to the log on the way out), the log, replica signals,
// the leader lease (handed over now rather than after its TTL), and last the
// node row, whose removal tells the other replicas at once that this one is
// gone.
func (c *clusterParts) stopCluster(ctx context.Context) {
	if c.relay != nil {
		c.relay.Stop()
	}
	if c.clusterLog != nil {
		c.clusterLog.Stop()
	}
	replicas.SetBackend(nil)
	if c.signals != nil {
		c.signals.close()
	}
	if c.unsubscribeSignals != nil {
		c.unsubscribeSignals()
	}
	if c.leadership != nil {
		c.leadership.Stop()
	}
	if c.clusterNode != nil {
		c.clusterNode.Stop(ctx)
	}
}
