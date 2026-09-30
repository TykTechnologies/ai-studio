package models

import "time"

// ClusterNode is one running Studio replica. Each replica refreshes its row
// every few seconds (pkg/cluster); a row whose LastSeen is older than the
// liveness window belongs to a replica that has stopped or lost the
// database, and anything it claimed is up for others to take over.
type ClusterNode struct {
	NodeID    string    `json:"node_id" gorm:"primaryKey;size:128"`
	Hostname  string    `json:"hostname" gorm:"size:255"`
	Version   string    `json:"version" gorm:"size:64"`
	StartedAt time.Time `json:"started_at"`
	LastSeen  time.Time `json:"last_seen" gorm:"index"`
	// PID, BootID (the running kernel) and PIDNamespace locate the
	// replica's process, so a replica restarted on the same host after a
	// crash can tell that the lease holder is its dead predecessor and
	// take over at once (pkg/cluster.Leadership). Empty on rows written
	// before they existed.
	PID          int    `json:"pid" gorm:"column:pid"`
	BootID       string `json:"boot_id" gorm:"column:boot_id;size:64"`
	PIDNamespace string `json:"pid_namespace" gorm:"column:pid_namespace;size:64"`
	// Label names the replica for operators (a full Studio "studio" or
	// "dashboard", a headless control plane "mdcb-<host>"). Empty on rows
	// written before labels existed.
	Label string `json:"label" gorm:"column:label;size:64"`
	// LeaderEligible is false for a replica that never takes the leader
	// lease (a headless control plane). Nil on rows written before it
	// existed, which means eligible. A pointer without a gorm default: a
	// bool with default:true would turn an explicit false into true.
	LeaderEligible *bool `json:"leader_eligible" gorm:"column:leader_eligible"`
}

// CanLead reports whether the replica may take the leader lease.
func (n ClusterNode) CanLead() bool { return n.LeaderEligible == nil || *n.LeaderEligible }

// ClusterEvent is one entry of the cluster event log: an event every other
// replica must see (pkg/cluster). Readers track the ids they have handled;
// rows are pruned after the retention window.
type ClusterEvent struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Topic     string    `json:"topic" gorm:"size:255;not null"`
	Origin    string    `json:"origin" gorm:"size:128;not null"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;index"`
}

// ClusterLease is a named lease held by at most one replica at a time: the
// cluster's leader for singleton work (pkg/cluster.Leadership). Taking and
// renewing it are conditional writes against the database's clock.
type ClusterLease struct {
	Name       string    `json:"name" gorm:"primaryKey;size:64"`
	Holder     string    `json:"holder" gorm:"size:128;not null"`
	AcquiredAt time.Time `json:"acquired_at"`
	RenewedAt  time.Time `json:"renewed_at"`
	ExpiresAt  time.Time `json:"expires_at" gorm:"not null"`
}
