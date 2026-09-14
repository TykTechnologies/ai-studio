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
	// LastPushAt is when an administrator last issued a configuration push
	// (edge, namespace or global reload) for this namespace; nil until the
	// first push. The pending-changes preview lists what changed since it.
	LastPushAt *time.Time `json:"last_push_at"`
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

// MarkPushed records that a configuration push was issued for the namespace
// at the given time. The row is created when the namespace has no sync
// status yet (a push before any snapshot was generated), with an empty
// checksum that the next snapshot fills in.
func MarkNamespacePushed(db *gorm.DB, namespace string, at time.Time) error {
	result := db.Model(&NamespaceSyncStatus{}).
		Where("namespace = ?", namespace).
		Update("last_push_at", at)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	return db.Create(&NamespaceSyncStatus{
		Namespace:        namespace,
		LastConfigChange: at,
		LastPushAt:       &at,
	}).Error
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
