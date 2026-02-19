package models

import (
	"time"

	"gorm.io/gorm"
)

// NamespaceSyncStatus tracks the expected configuration checksum for each namespace
type NamespaceSyncStatus struct {
	gorm.Model
	Namespace        string    `gorm:"uniqueIndex;not null" json:"namespace"`
	ExpectedChecksum string    `gorm:"size:64;not null" json:"expected_checksum"`
	ConfigVersion    string    `gorm:"size:64;not null" json:"config_version"`
	LastConfigChange time.Time `gorm:"not null" json:"last_config_change"`
}

// TableName specifies the table name for the NamespaceSyncStatus model
func (NamespaceSyncStatus) TableName() string {
	return "namespace_sync_status"
}

// Upsert updates or creates the sync status for a namespace
func (n *NamespaceSyncStatus) Upsert(db *gorm.DB) error {
	return db.Where("namespace = ?", n.Namespace).
		Assign(map[string]interface{}{
			"expected_checksum":  n.ExpectedChecksum,
			"config_version":     n.ConfigVersion,
			"last_config_change": n.LastConfigChange,
		}).FirstOrCreate(n).Error
}

// GetByNamespace retrieves sync status for a specific namespace
func (n *NamespaceSyncStatus) GetByNamespace(db *gorm.DB, namespace string) error {
	return db.Where("namespace = ?", namespace).First(n).Error
}

// GetAll retrieves all namespace sync statuses
func (n *NamespaceSyncStatus) GetAll(db *gorm.DB) ([]NamespaceSyncStatus, error) {
	var statuses []NamespaceSyncStatus
	err := db.Order("namespace ASC").Find(&statuses).Error
	return statuses, err
}

// Delete removes the sync status for a namespace
func (n *NamespaceSyncStatus) Delete(db *gorm.DB) error {
	return db.Delete(n).Error
}
