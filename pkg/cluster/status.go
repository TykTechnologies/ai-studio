package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// NodeStatus is one live replica as the cluster status reports it.
type NodeStatus struct {
	NodeID    string    `json:"node_id"`
	Hostname  string    `json:"hostname"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
	LastSeen  time.Time `json:"last_seen"`
	// Edges is how many connected edges hold a stream to this replica.
	Edges int64 `json:"edges"`
	// Leader: this replica holds the leader lease.
	Leader bool `json:"leader"`
	// Self: the replica answering the request.
	Self bool `json:"self"`
}

// PushBacklog counts configuration push commands that are not finished.
type PushBacklog struct {
	Pending  int64 `json:"pending"`   // waiting for the edge's stream
	InFlight int64 `json:"in_flight"` // claimed or sent, waiting for the edge
	// OldestPending is when the oldest pending command was created.
	OldestPending *time.Time `json:"oldest_pending,omitempty"`
	// Unowned counts in-flight commands whose replica is no longer live;
	// a janitor returns them to pending within seconds, so a lasting
	// non-zero value means no replica's janitor runs.
	Unowned int64 `json:"unowned"`
}

// Status is the control plane as one replica sees it.
type Status struct {
	Self     string       `json:"self"`
	Database string       `json:"database"` // postgres | sqlite
	Nodes    []NodeStatus `json:"nodes"`
	// EdgesWithoutReplica counts connected edges whose owning replica is
	// not live: they reconnect elsewhere shortly.
	EdgesWithoutReplica int64 `json:"edges_without_replica"`

	Leader          string     `json:"leader"`
	LeaderExpiresAt *time.Time `json:"leader_expires_at,omitempty"`

	// EventLog and Relay are this replica's own.
	EventLog LogStats   `json:"event_log"`
	Relay    RelayStats `json:"relay"`

	Pushes PushBacklog `json:"pushes"`

	// Warnings explain anything that needs attention.
	Warnings []string `json:"warnings"`
}

// Snapshot reports the cluster from self's point of view. log and relay
// may be nil.
func Snapshot(ctx context.Context, db *gorm.DB, self string, log *Log, relay *Relay) (*Status, error) {
	db = db.WithContext(ctx)
	st := &Status{Self: self, Database: db.Dialector.Name(), Warnings: []string{}}

	live, err := LiveNodes(db)
	if err != nil {
		return nil, fmt.Errorf("cluster status: %w", err)
	}
	var owners []struct {
		OwnerNodeID string
		N           int64
	}
	if err := db.Model(&models.EdgeInstance{}).Select("owner_node_id, COUNT(*) AS n").
		Where("status = ?", models.EdgeStatusConnected).Group("owner_node_id").Scan(&owners).Error; err != nil {
		return nil, fmt.Errorf("cluster status: %w", err)
	}
	edges := map[string]int64{}
	for _, o := range owners {
		edges[o.OwnerNodeID] = o.N
	}

	leader, expires, err := Holder(ctx, db, LeaderLease)
	if err != nil {
		return nil, fmt.Errorf("cluster status: %w", err)
	}
	st.Leader, st.LeaderExpiresAt = leader, expires

	liveSet := map[string]bool{}
	for _, n := range live {
		liveSet[n.NodeID] = true
		st.Nodes = append(st.Nodes, NodeStatus{
			NodeID: n.NodeID, Hostname: n.Hostname, Version: n.Version,
			StartedAt: n.StartedAt, LastSeen: n.LastSeen,
			Edges: edges[n.NodeID], Leader: n.NodeID == leader, Self: n.NodeID == self,
		})
	}
	for owner, n := range edges {
		if !liveSet[owner] {
			st.EdgesWithoutReplica += n
		}
	}

	if log != nil {
		st.EventLog = log.Stats()
	}
	if relay != nil {
		st.Relay = relay.Stats()
	}

	var counts []struct {
		Status string
		N      int64
	}
	if err := db.Model(&models.EdgePushCommand{}).Select("status, COUNT(*) AS n").
		Where("status IN ?", []string{models.PushCommandPending, models.PushCommandClaimed, models.PushCommandSent}).
		Group("status").Scan(&counts).Error; err != nil {
		return nil, fmt.Errorf("cluster status: %w", err)
	}
	for _, c := range counts {
		if c.Status == models.PushCommandPending {
			st.Pushes.Pending = c.N
		} else {
			st.Pushes.InFlight += c.N
		}
	}
	if st.Pushes.Pending > 0 {
		var oldest models.EdgePushCommand
		if err := db.Select("created_at").Where("status = ?", models.PushCommandPending).Order("created_at").Take(&oldest).Error; err == nil {
			t := oldest.CreatedAt
			st.Pushes.OldestPending = &t
		}
	}
	if st.Pushes.InFlight > 0 && len(live) > 0 {
		ids := make([]string, 0, len(live))
		for _, n := range live {
			ids = append(ids, n.NodeID)
		}
		if err := db.Model(&models.EdgePushCommand{}).
			Where("status IN ? AND claimed_by NOT IN ?", []string{models.PushCommandClaimed, models.PushCommandSent}, ids).
			Count(&st.Pushes.Unowned).Error; err != nil {
			return nil, fmt.Errorf("cluster status: %w", err)
		}
	}

	// What needs attention.
	if st.Database == "postgres" {
		if log != nil && !st.EventLog.Enabled {
			st.Warnings = append(st.Warnings, "The cluster event log is not running on this replica: other replicas' changes do not reach it.")
		}
		if st.EventLog.LastError != "" {
			st.Warnings = append(st.Warnings, "Reading the cluster event log fails: "+st.EventLog.LastError)
		}
		if st.Relay.Dropped > 0 {
			st.Warnings = append(st.Warnings, fmt.Sprintf("%d event(s) could not be relayed to the other replicas since this replica started (see its log).", st.Relay.Dropped))
		}
	}
	if leader == "" {
		st.Warnings = append(st.Warnings, "No replica holds the leader lease: singleton jobs (budget aggregation, alerts, syncs, cleanups) are not running.")
	}
	if st.EdgesWithoutReplica > 0 {
		st.Warnings = append(st.Warnings, fmt.Sprintf("%d connected edge(s) are recorded against a replica that has stopped; they reconnect to a live replica shortly.", st.EdgesWithoutReplica))
	}
	if st.Pushes.Unowned > 0 {
		st.Warnings = append(st.Warnings, fmt.Sprintf("%d push command(s) are held by a replica that has stopped; a live replica returns them to pending shortly.", st.Pushes.Unowned))
	}
	if st.Pushes.OldestPending != nil && time.Since(*st.Pushes.OldestPending) > 10*time.Minute {
		st.Warnings = append(st.Warnings, "A configuration push has been waiting for its edge for over 10 minutes.")
	}
	return st, nil
}
