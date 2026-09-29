package models

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// EdgeInstance represents a registered edge instance in hub-and-spoke mode
type EdgeInstance struct {
	gorm.Model
	ID             uint                   `json:"id" gorm:"primaryKey"`
	EdgeID         string                 `json:"edge_id" gorm:"uniqueIndex;not null"`
	Namespace      string                 `json:"namespace" gorm:"default:'';index:idx_edge_instances_namespace"`
	Version        string                 `json:"version"`
	BuildHash      string                 `json:"build_hash"`
	Metadata       map[string]interface{} `json:"metadata" gorm:"serializer:json"`
	LastHeartbeat  *time.Time             `json:"last_heartbeat" gorm:"index:idx_edge_instances_heartbeat"`
	Status         string                 `json:"status" gorm:"default:'registered';index:idx_edge_instances_status"`
	SessionID      string                 `json:"session_id"`
	// OwnerNodeID is the Studio replica holding the edge's configuration
	// stream, and StreamSessionID identifies that stream. Writes that end a
	// stream are conditional on StreamSessionID, so a replica the edge has
	// left cannot overwrite the state its new replica recorded.
	OwnerNodeID     string `json:"owner_node_id" gorm:"size:128;index:idx_edge_instances_owner"`
	StreamSessionID string `json:"stream_session_id" gorm:"size:64"`
	// Sync tracking fields
	LoadedChecksum string     `json:"loaded_checksum" gorm:"size:64"`
	LoadedVersion  string     `json:"loaded_version" gorm:"size:64"`
	SyncStatus     string     `json:"sync_status" gorm:"size:20;default:'unknown';index:idx_edge_instances_sync_status"`
	LastSyncAck    *time.Time `json:"last_sync_ack"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

type EdgeInstances []EdgeInstance

// Edge instance status constants
const (
	EdgeStatusRegistered   = "registered"
	EdgeStatusConnected    = "connected"
	EdgeStatusDisconnected = "disconnected"
	EdgeStatusUnhealthy    = "unhealthy"
)

// Edge sync status constants
const (
	EdgeSyncStatusInSync  = "in_sync"
	EdgeSyncStatusPending = "pending"
	EdgeSyncStatusUnknown = "unknown"
	EdgeSyncStatusStale   = "stale"
)

// NewEdgeInstance creates a new EdgeInstance
func NewEdgeInstance() *EdgeInstance {
	return &EdgeInstance{
		Status: EdgeStatusRegistered,
	}
}

// Get retrieves an edge instance by ID
func (e *EdgeInstance) Get(db *gorm.DB, id uint) error {
	return db.First(e, id).Error
}

// GetByEdgeID retrieves an edge instance by edge ID
func (e *EdgeInstance) GetByEdgeID(db *gorm.DB, edgeID string) error {
	return db.Where("edge_id = ?", edgeID).First(e).Error
}

// Create creates a new edge instance
func (e *EdgeInstance) Create(db *gorm.DB) error {
	return db.Create(e).Error
}

// Update updates an existing edge instance
func (e *EdgeInstance) Update(db *gorm.DB) error {
	return db.Save(e).Error
}

// Delete permanently deletes an edge instance
// We use hard delete (not soft delete) to allow edges to re-register with the same edge_id
func (e *EdgeInstance) Delete(db *gorm.DB) error {
	return db.Unscoped().Delete(e).Error
}

// UpdateHeartbeat updates the last heartbeat timestamp
func (e *EdgeInstance) UpdateHeartbeat(db *gorm.DB) error {
	now := time.Now()
	e.LastHeartbeat = &now
	return db.Model(e).Update("last_heartbeat", now).Error
}

// UpdateStatus updates the edge instance status
func (e *EdgeInstance) UpdateStatus(db *gorm.DB, status string) error {
	e.Status = status
	return db.Model(e).Update("status", status).Error
}

// IsHealthy checks if the edge instance is considered healthy
func (e *EdgeInstance) IsHealthy(maxAge time.Duration) bool {
	if e.LastHeartbeat == nil {
		return false
	}
	return time.Since(*e.LastHeartbeat) <= maxAge
}

// ListEdgesInNamespace returns all edges in a specific namespace, under
// any spelling of it (see NamespaceAliases).
func (edges *EdgeInstances) ListEdgesInNamespace(db *gorm.DB, namespace string) error {
	return db.Where("namespace IN ?", NamespaceAliases(namespace)).Order("created_at DESC").Find(edges).Error
}

// ListActiveEdges returns all active (connected/registered) edges
func (edges *EdgeInstances) ListActiveEdges(db *gorm.DB) error {
	return db.Where("status IN ?", []string{EdgeStatusConnected, EdgeStatusRegistered}).
		Order("created_at DESC").Find(edges).Error
}

// ListEdgesByStatus returns edges with a specific status
func (edges *EdgeInstances) ListEdgesByStatus(db *gorm.DB, status string) error {
	return db.Where("status = ?", status).Order("created_at DESC").Find(edges).Error
}

// CountEdgesInNamespace returns the count of edges in a specific namespace
func (e *EdgeInstance) CountEdgesInNamespace(db *gorm.DB, namespace string) (int64, error) {
	var count int64
	err := db.Model(&EdgeInstance{}).Where("namespace = ?", namespace).Count(&count).Error
	return count, err
}

// CountActiveEdges returns the count of active edges
func (e *EdgeInstance) CountActiveEdges(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&EdgeInstance{}).
		Where("status IN ?", []string{EdgeStatusConnected, EdgeStatusRegistered}).
		Count(&count).Error
	return count, err
}

// CleanupStaleEdges marks edges as disconnected if they haven't sent heartbeat in maxAge
func (e *EdgeInstance) CleanupStaleEdges(db *gorm.DB, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	return db.Model(&EdgeInstance{}).
		Where("status = ? AND (last_heartbeat IS NULL OR last_heartbeat < ?)", EdgeStatusConnected, cutoff).
		Update("status", EdgeStatusDisconnected).Error
}

// UpdateSyncStatus updates the sync status for an edge
func (e *EdgeInstance) UpdateSyncStatus(db *gorm.DB, checksum, version, status string) error {
	updates := map[string]interface{}{
		"loaded_checksum": checksum,
		"loaded_version":  version,
		"sync_status":     status,
	}
	if status == EdgeSyncStatusInSync {
		now := time.Now()
		e.LastSyncAck = &now
		updates["last_sync_ack"] = now
	}
	e.LoadedChecksum = checksum
	e.LoadedVersion = version
	e.SyncStatus = status
	return db.Model(e).Updates(updates).Error
}

// MarkEdgesAsPendingInNamespace marks all active edges in a namespace as pending sync
func (e *EdgeInstance) MarkEdgesAsPendingInNamespace(db *gorm.DB, namespace string) error {
	return db.Model(&EdgeInstance{}).
		Where("namespace IN ? AND status IN ?", NamespaceAliases(namespace), []string{EdgeStatusConnected, EdgeStatusRegistered}).
		Update("sync_status", EdgeSyncStatusPending).Error
}

// FirstInSyncWithChecksum returns an active edge in the namespace that
// reports the given checksum as loaded and in sync, or nil when there is
// none. The pending-changes preview uses its LastSyncAck as the reference
// point when the namespace has a checksum but no recorded push.
func (e *EdgeInstance) FirstInSyncWithChecksum(db *gorm.DB, namespace, checksum string) (*EdgeInstance, error) {
	if checksum == "" {
		return nil, nil
	}
	var edge EdgeInstance
	err := db.Where("namespace IN ? AND status IN ? AND sync_status = ? AND loaded_checksum = ?",
		NamespaceAliases(namespace), []string{EdgeStatusConnected, EdgeStatusRegistered},
		EdgeSyncStatusInSync, checksum).
		Order("last_sync_ack DESC").First(&edge).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &edge, nil
}

// MarkStaleEdges marks edges that have been pending sync for too long as stale
func (e *EdgeInstance) MarkStaleEdges(db *gorm.DB, staleThreshold time.Duration) error {
	cutoff := time.Now().Add(-staleThreshold)
	return db.Model(&EdgeInstance{}).
		Where("sync_status = ? AND (last_sync_ack IS NULL OR last_sync_ack < ?)", EdgeSyncStatusPending, cutoff).
		Where("status IN ?", []string{EdgeStatusConnected, EdgeStatusRegistered}).
		Update("sync_status", EdgeSyncStatusStale).Error
}

// CountEdgesBySyncStatus returns the count of edges by sync status in a namespace
func (e *EdgeInstance) CountEdgesBySyncStatus(db *gorm.DB, namespace string) (map[string]int64, error) {
	type Result struct {
		SyncStatus string
		Count      int64
	}
	var results []Result

	query := db.Model(&EdgeInstance{}).
		Select("sync_status, COUNT(*) as count").
		Where("status IN ?", []string{EdgeStatusConnected, EdgeStatusRegistered})

	if namespace != "" {
		query = query.Where("namespace IN ?", NamespaceAliases(namespace))
	}

	err := query.Group("sync_status").Scan(&results).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.SyncStatus] = r.Count
	}
	return counts, nil
}

// ListEdgesBySyncStatus returns edges with a specific sync status
func (edges *EdgeInstances) ListEdgesBySyncStatus(db *gorm.DB, syncStatus string) error {
	return db.Where("sync_status = ? AND status IN ?", syncStatus, []string{EdgeStatusConnected, EdgeStatusRegistered}).
		Order("created_at DESC").Find(edges).Error
}
// ClaimEdgeStream records that nodeID now holds the edge's configuration
// stream, identified by session. The newest stream always wins: an edge has
// one live stream at a time, so a new one means the old one is gone.
// It reports whether the edge exists.
func ClaimEdgeStream(db *gorm.DB, edgeID, nodeID, session string) (bool, error) {
	now := time.Now()
	res := db.Model(&EdgeInstance{}).Where("edge_id = ?", edgeID).Updates(map[string]interface{}{
		"owner_node_id":     nodeID,
		"stream_session_id": session,
		"status":            EdgeStatusConnected,
		"last_heartbeat":    now,
	})
	return res.RowsAffected > 0, res.Error
}

// ReleaseEdgeStream records that the stream identified by session has
// ended, but only if it is still the edge's current stream: when the edge
// has already reconnected (to this replica or another), the newer stream's
// state is left alone. It reports whether it changed anything.
func ReleaseEdgeStream(db *gorm.DB, edgeID, session string) (bool, error) {
	if session == "" {
		return false, nil
	}
	res := db.Model(&EdgeInstance{}).Where("edge_id = ? AND stream_session_id = ?", edgeID, session).Updates(map[string]interface{}{
		"owner_node_id":     "",
		"stream_session_id": "",
		"status":            EdgeStatusDisconnected,
	})
	return res.RowsAffected > 0, res.Error
}

// TouchEdgeStream records a heartbeat received on the stream identified by
// session. If the row has lost its owner (a stale sweep cleared it, or the
// row was written by something else), the heartbeat proves this stream is
// the edge's live one, so it re-claims it. It never takes over from a
// different session. It reports whether it re-claimed.
func TouchEdgeStream(db *gorm.DB, edgeID, nodeID, session string) (reclaimed bool, err error) {
	now := time.Now()
	if err := db.Model(&EdgeInstance{}).Where("edge_id = ?", edgeID).Update("last_heartbeat", now).Error; err != nil {
		return false, err
	}
	if session == "" {
		return false, nil
	}
	res := db.Model(&EdgeInstance{}).
		Where("edge_id = ?", edgeID).
		Where("(stream_session_id = '' OR stream_session_id IS NULL OR stream_session_id = ?)", session).
		Where("(owner_node_id <> ? OR owner_node_id IS NULL OR stream_session_id <> ? OR stream_session_id IS NULL OR status <> ?)", nodeID, session, EdgeStatusConnected).
		Updates(map[string]interface{}{
			"owner_node_id":     nodeID,
			"stream_session_id": session,
			"status":            EdgeStatusConnected,
		})
	return res.RowsAffected > 0, res.Error
}
