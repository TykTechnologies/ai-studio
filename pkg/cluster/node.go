// Package cluster lets several Studio replicas share one database: it
// registers each replica (Node) so others can tell live replicas from dead
// ones, and carries events every replica must see (Log). See
// features/ClusterControlPlane.md.
//
// On Postgres it is always on, whether one replica runs or ten: a deployment
// that forgot to switch it on would fail silently. On SQLite, which only one
// process can serve, the node registry still runs (one row) and the event
// log is inert.
package cluster

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
	"unicode"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// Defaults for the node registry. A node counts as live while its row was
// refreshed within LivenessWindow; that is four refreshes, so one slow
// database round trip never makes a live node look dead. Rows of nodes that
// stopped without removing theirs (a crash, a kill) are deleted once they
// are NodeRetention old, by whichever node prunes first: nothing refers to
// a node that long dead (its leases, claims and edge streams were taken
// over long before), and until then the rows show what ran recently.
var (
	HeartbeatInterval = 5 * time.Second
	LivenessWindow    = 20 * time.Second
	NodeRetention     = time.Hour
)

// pruneEvery is how many refreshes pass between two prunes of dead rows.
const pruneEvery = 12

// NewNodeID returns an ID for this process: hostname, pid and a random
// suffix. The suffix matters in containers, where every process may be pid
// 1 and a restarted replica must not inherit its predecessor's claims.
func NewNodeID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "studio"
	}
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(b))
}

// NodeOptions describe a replica to the others and to operators.
type NodeOptions struct {
	// Label names the replica on the status page and the Edge Gateways
	// page ("studio", "dashboard", "mdcb-eu-1"): at most MaxLabelLength
	// characters, no control characters. Empty leaves it unlabelled.
	Label string
	// NeverLeads records that the replica never takes the leader lease (a
	// headless control plane). It is recorded for operators; the replica's
	// own code decides whether it contends for the lease.
	NeverLeads bool
}

// MaxLabelLength bounds NodeOptions.Label (the column's size).
const MaxLabelLength = 64

func (o NodeOptions) validate() error {
	if len([]rune(o.Label)) > MaxLabelLength {
		return fmt.Errorf("node label is longer than %d characters", MaxLabelLength)
	}
	for _, r := range o.Label {
		if unicode.IsControl(r) {
			return fmt.Errorf("node label %q contains a control character", o.Label)
		}
	}
	return nil
}

// Node is this replica's entry in the registry.
type Node struct {
	db       *gorm.DB
	id       string
	proc     processInfo
	version  string
	label    string
	canLead  bool
	started  time.Time
	interval time.Duration

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// StartNode registers the replica and keeps its row fresh until Stop. The
// first write happens before it returns, so a failure to reach the database
// is reported rather than discovered later. At most one NodeOptions is used;
// without one the replica is unlabelled and may lead.
func StartNode(db *gorm.DB, id, version string, opts ...NodeOptions) (*Node, error) {
	var o NodeOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("cluster: register node %s: %w", id, err)
	}
	n := &Node{
		label:   o.Label,
		canLead: !o.NeverLeads,
		// A refresh is one statement; gorm's default transaction around it
		// would make it three, on every replica, every few seconds.
		db:       db.Session(&gorm.Session{SkipDefaultTransaction: true}),
		id:       id,
		proc:     thisProcess(),
		version:  version,
		started:  time.Now().UTC(),
		interval: HeartbeatInterval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	if err := n.beat(); err != nil {
		return nil, fmt.Errorf("cluster: register node %s: %w", id, err)
	}
	nodesHere.Store(id, struct{}{})
	n.prune()
	go n.loop()
	logger.Infof("Cluster node %s registered", id)
	return n, nil
}

// ID is the node's ID.
func (n *Node) ID() string { return n.id }

func (n *Node) loop() {
	defer close(n.done)
	safe.Loop("cluster node heartbeat", n.stop, n.heartbeat)
}

func (n *Node) heartbeat() {
	t := time.NewTicker(n.interval)
	defer t.Stop()
	failing := false
	beats := 0
	for {
		select {
		case <-n.stop:
			return
		case <-t.C:
			if beats++; beats%pruneEvery == 0 {
				n.prune()
			}
			if err := n.beat(); err != nil {
				if !failing {
					logger.Warnf("Cluster node %s could not refresh its registration (other replicas will consider it dead after %s): %v", n.id, LivenessWindow, err)
				}
				failing = true
				continue
			}
			if failing {
				logger.Infof("Cluster node %s refreshes its registration again", n.id)
				failing = false
			}
		}
	}
}

func (n *Node) beat() error {
	return n.db.Model(&models.ClusterNode{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"last_seen": nowExpr(n.db), "hostname": n.proc.Hostname, "version": n.version,
			"label": n.label, "leader_eligible": n.canLead,
		}),
	}).Create(map[string]interface{}{
		"node_id":         n.id,
		"hostname":        n.proc.Hostname,
		"version":         n.version,
		"label":           n.label,
		"leader_eligible": n.canLead,
		"started_at":    n.started,
		"last_seen":     nowExpr(n.db),
		"pid":           n.proc.PID,
		"boot_id":       n.proc.BootID,
		"pid_namespace": n.proc.PIDNamespace,
	}).Error
}

// prune deletes the rows of nodes silent for NodeRetention. Every node runs
// it now and then; the delete is conditional, so running it twice at once
// is harmless.
func (n *Node) prune() {
	res := n.db.Where("last_seen < ?", sinceExpr(n.db, NodeRetention)).Delete(&models.ClusterNode{})
	if res.Error != nil {
		logger.Warnf("Cluster node %s could not delete the registrations of long-stopped replicas: %v", n.id, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		logger.Infof("Cluster: deleted the registrations of %d replica(s) stopped for more than %s", res.RowsAffected, NodeRetention)
	}
}

// Stop ends the refreshes and removes the node's row, so other replicas
// know at once that it is gone rather than after the liveness window.
func (n *Node) Stop(ctx context.Context) {
	n.stopOnce.Do(func() {
		close(n.stop)
		<-n.done
		nodesHere.Delete(n.id)
		if err := n.db.WithContext(ctx).Where("node_id = ?", n.id).Delete(&models.ClusterNode{}).Error; err != nil {
			logger.Warnf("Cluster node %s could not remove its registration; others will see it expire: %v", n.id, err)
		}
	})
}

// LiveNodes returns the nodes whose registration is fresh, by the database's
// clock (never the caller's, so clock skew between replicas cannot make a
// live node look dead).
func LiveNodes(db *gorm.DB) ([]models.ClusterNode, error) {
	var nodes []models.ClusterNode
	err := db.Model(&models.ClusterNode{}).Where("last_seen > ?", sinceExpr(db, LivenessWindow)).Order("node_id").Find(&nodes).Error
	return nodes, err
}

// IsLive reports whether nodeID has a fresh registration.
func IsLive(db *gorm.DB, nodeID string) (bool, error) {
	if nodeID == "" {
		return false, nil
	}
	var count int64
	err := db.Model(&models.ClusterNode{}).Where("node_id = ? AND last_seen > ?", nodeID, sinceExpr(db, LivenessWindow)).Count(&count).Error
	return count > 0, err
}

// nowExpr is the database's current time.
func nowExpr(db *gorm.DB) clause.Expr {
	if db.Dialector.Name() == "postgres" {
		return gorm.Expr("now()")
	}
	return gorm.Expr("CURRENT_TIMESTAMP")
}

// sinceExpr is the database's current time minus d.
func sinceExpr(db *gorm.DB, d time.Duration) clause.Expr {
	if db.Dialector.Name() == "postgres" {
		return gorm.Expr("now() - make_interval(secs => ?)", d.Seconds())
	}
	return gorm.Expr("datetime('now', ?)", fmt.Sprintf("-%d seconds", int(d.Seconds())))
}
