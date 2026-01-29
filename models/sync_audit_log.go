package models

import (
	"gorm.io/gorm"
)

// Sync audit event types
const (
	SyncEventConfigChanged    = "config_changed"
	SyncEventEdgeAck          = "edge_ack"
	SyncEventEdgeOutOfSync    = "edge_out_of_sync"
	SyncEventEdgeConnected    = "edge_connected"
	SyncEventEdgeDisconnected = "edge_disconnected"
	SyncEventEdgeStale        = "edge_stale"
)

// SyncAuditLog tracks sync events between control plane and edge gateways
type SyncAuditLog struct {
	gorm.Model
	EventType     string  `gorm:"size:50;not null;index" json:"event_type"`
	Namespace     string  `gorm:"size:255;not null;index" json:"namespace"`
	EdgeID        *string `gorm:"size:255;index" json:"edge_id,omitempty"`
	Checksum      string  `gorm:"size:64" json:"checksum"`
	ConfigVersion string  `gorm:"size:64" json:"config_version"`
	Details       string  `gorm:"type:text" json:"details"`
}

// TableName specifies the table name for the SyncAuditLog model
func (SyncAuditLog) TableName() string {
	return "sync_audit_log"
}

// Create creates a new sync audit log entry
func (s *SyncAuditLog) Create(db *gorm.DB) error {
	return db.Create(s).Error
}

// GetByNamespace retrieves audit logs for a specific namespace
func (s *SyncAuditLog) GetByNamespace(db *gorm.DB, namespace string, limit int) ([]SyncAuditLog, error) {
	var logs []SyncAuditLog
	query := db.Where("namespace = ?", namespace).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&logs).Error
	return logs, err
}

// GetByEdgeID retrieves audit logs for a specific edge
func (s *SyncAuditLog) GetByEdgeID(db *gorm.DB, edgeID string, limit int) ([]SyncAuditLog, error) {
	var logs []SyncAuditLog
	query := db.Where("edge_id = ?", edgeID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&logs).Error
	return logs, err
}

// GetRecent retrieves the most recent audit logs
func (s *SyncAuditLog) GetRecent(db *gorm.DB, limit int) ([]SyncAuditLog, error) {
	var logs []SyncAuditLog
	err := db.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// GetFiltered retrieves audit logs with optional filters
func (s *SyncAuditLog) GetFiltered(db *gorm.DB, namespace, edgeID, eventType string, limit int) ([]SyncAuditLog, error) {
	var logs []SyncAuditLog
	query := db.Model(&SyncAuditLog{}).Order("created_at DESC")

	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}
	if edgeID != "" {
		query = query.Where("edge_id = ?", edgeID)
	}
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	return logs, err
}

// CleanupOldLogs removes audit logs older than the specified number of days
func (s *SyncAuditLog) CleanupOldLogs(db *gorm.DB, daysToKeep int) error {
	return db.Exec("DELETE FROM sync_audit_log WHERE created_at < datetime('now', ?)",
		"-"+string(rune(daysToKeep))+" days").Error
}
