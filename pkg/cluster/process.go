package cluster

import (
	"os"
	"sync"
)

// processInfo says where a replica's process runs. Each node records its
// own in cluster_nodes, so that a replica restarted after a crash can
// recognise the lease its dead predecessor still holds (see
// Leadership.predecessorGone) instead of waiting out the lease's TTL.
// Hostname and PID also make up the default node ID, but a host may pass
// any ID (Options.NodeID), so they are kept apart.
type processInfo struct {
	Hostname string
	// BootID identifies the running kernel: it differs between machines
	// and after every reboot, and is shared by every container on one
	// machine. Empty where it cannot be read; such a node's leases are
	// never taken over early.
	BootID string
	// PIDNamespace identifies the namespace PID is valid in ("host" where
	// the OS has none). Two processes can check each other's pid only when
	// it is the same.
	PIDNamespace string
	PID          int
}

// thisProcess describes the calling process.
func thisProcess() processInfo {
	host, _ := os.Hostname()
	return processInfo{Hostname: host, BootID: bootID(), PIDNamespace: pidNamespace(), PID: os.Getpid()}
}

// nodesHere are the nodes registered by this process and not stopped: a
// holder that shares this process's pid is alive exactly when it is here.
var nodesHere sync.Map // node ID -> struct{}
