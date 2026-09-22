package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// DefaultNamespace is the canonical key of the global/default namespace in
// sync bookkeeping (namespace_sync_status rows, edge rows). Object tables
// keep "" for global objects; see CanonicalNamespace.
const DefaultNamespace = "default"

// CanonicalNamespace maps every spelling of the global/default namespace to
// DefaultNamespace: "" (object tables, the microgateway's EDGE_NAMESPACE
// default), "global" (the admin API) and "default" (edge registration, both
// editions). Any other namespace is returned trimmed. Every reader and
// writer of NamespaceSyncStatus goes through this so there is exactly one
// row per logical namespace.
func CanonicalNamespace(ns string) string {
	ns = strings.TrimSpace(ns)
	switch strings.ToLower(ns) {
	case "", "global", DefaultNamespace:
		return DefaultNamespace
	}
	return ns
}

// NamespaceAliases lists the stored spellings that mean the same namespace
// as ns, canonical first, for queries over rows written before
// canonicalisation (edge rows registered under "", legacy sync rows).
func NamespaceAliases(ns string) []string {
	canonical := CanonicalNamespace(ns)
	if canonical == DefaultNamespace {
		return []string{DefaultNamespace, "", "global"}
	}
	return []string{canonical}
}

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

// Upsert updates or creates the sync status for a namespace. The namespace
// is canonicalised; a legacy row under another spelling is updated in place
// rather than shadowed by a second row.
func (n *NamespaceSyncStatus) Upsert(db *gorm.DB) error {
	n.Namespace = CanonicalNamespace(n.Namespace)
	values := map[string]interface{}{
		"expected_checksum":  n.ExpectedChecksum,
		"config_version":     n.ConfigVersion,
		"last_config_change": n.LastConfigChange,
	}
	var existing NamespaceSyncStatus
	if err := existing.GetByNamespace(db, n.Namespace); err == nil {
		if err := db.Model(&existing).Updates(values).Error; err != nil {
			return err
		}
		n.ID = existing.ID
		n.CreatedAt = existing.CreatedAt
		n.LastPushAt = existing.LastPushAt
		return nil
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	return db.Create(n).Error
}

// MarkNamespacePushed records that a configuration push was issued for the
// namespace at the given time. The row is created when the namespace has no
// sync status yet (a push before any snapshot was generated), with an empty
// checksum that the next snapshot fills in; that first fill is not a
// configuration change (see grpc.ControlServer.updateNamespaceSyncStatus).
func MarkNamespacePushed(db *gorm.DB, namespace string, at time.Time) error {
	aliases := NamespaceAliases(namespace)
	result := db.Model(&NamespaceSyncStatus{}).
		Where("namespace IN ?", aliases).
		Update("last_push_at", at)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	return db.Create(&NamespaceSyncStatus{
		Namespace:        aliases[0],
		LastConfigChange: at,
		LastPushAt:       &at,
	}).Error
}

// GetByNamespace retrieves sync status for a namespace under any of its
// spellings, preferring the canonical row. The fallback keeps databases
// restored from before canonicalisation working until
// MergeLegacyNamespaceSyncStatus has run.
func (n *NamespaceSyncStatus) GetByNamespace(db *gorm.DB, namespace string) error {
	aliases := NamespaceAliases(namespace)
	if err := db.Where("namespace = ?", aliases[0]).First(n).Error; err == nil || err != gorm.ErrRecordNotFound {
		return err
	}
	if len(aliases) == 1 {
		return gorm.ErrRecordNotFound
	}
	return db.Where("namespace IN ?", aliases[1:]).Order("last_config_change DESC").First(n).Error
}

// MergeLegacyNamespaceSyncStatus folds rows keyed by a non-canonical
// spelling of the default namespace ("" or "global") into the "default"
// row, keeping the newest snapshot facts and the latest push stamp, and
// hard-deletes the legacy rows (a tombstone would still hold the unique
// index). Runs at startup; a no-op once nothing is left to merge.
func MergeLegacyNamespaceSyncStatus(db *gorm.DB) error {
	legacyKeys := NamespaceAliases(DefaultNamespace)[1:]
	var legacy []NamespaceSyncStatus
	if err := db.Where("namespace IN ?", legacyKeys).Find(&legacy).Error; err != nil {
		return err
	}
	if len(legacy) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var target NamespaceSyncStatus
		err := tx.Where("namespace = ?", DefaultNamespace).First(&target).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err == gorm.ErrRecordNotFound {
			// No canonical row yet: rename the legacy row with the newest
			// snapshot and fold the rest into it.
			target = legacy[0]
			for _, row := range legacy[1:] {
				if row.LastConfigChange.After(target.LastConfigChange) {
					target = row
				}
			}
			target.Namespace = DefaultNamespace
			if err := tx.Model(&NamespaceSyncStatus{}).Where("id = ?", target.ID).
				Update("namespace", DefaultNamespace).Error; err != nil {
				return err
			}
		}

		for _, row := range legacy {
			if row.ID == target.ID {
				continue
			}
			if row.ExpectedChecksum != "" && (target.ExpectedChecksum == "" || row.LastConfigChange.After(target.LastConfigChange)) {
				target.ExpectedChecksum = row.ExpectedChecksum
				target.ConfigVersion = row.ConfigVersion
				target.LastConfigChange = row.LastConfigChange
			}
			if row.LastPushAt != nil && (target.LastPushAt == nil || row.LastPushAt.After(*target.LastPushAt)) {
				at := *row.LastPushAt
				target.LastPushAt = &at
			}
		}

		if err := tx.Model(&NamespaceSyncStatus{}).Where("id = ?", target.ID).
			Updates(map[string]interface{}{
				"expected_checksum":  target.ExpectedChecksum,
				"config_version":     target.ConfigVersion,
				"last_config_change": target.LastConfigChange,
				"last_push_at":       target.LastPushAt,
			}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("namespace IN ? AND id <> ?", legacyKeys, target.ID).
			Delete(&NamespaceSyncStatus{}).Error
	})
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
