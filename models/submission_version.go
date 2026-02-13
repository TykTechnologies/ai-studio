package models

import (
	"time"

	"gorm.io/gorm"
)

// SubmissionVersion stores a snapshot of a resource's state before an update is applied.
// This enables rollback to any previous version.
type SubmissionVersion struct {
	gorm.Model
	ID             uint   `json:"id" gorm:"primaryKey"`
	SubmissionID   uint   `json:"submission_id" gorm:"index"` // FK to the update submission that triggered this snapshot
	ResourceID     uint   `json:"resource_id" gorm:"index"`
	ResourceType   string `json:"resource_type"` // datasource | tool
	VersionNumber  int    `json:"version_number"`
	Payload        JSONMap `json:"payload" gorm:"type:json"` // snapshot of resource state before the update
	ChangedBy      uint   `json:"changed_by"`                // user who proposed the change
	ApprovedBy     uint   `json:"approved_by"`               // admin who approved
	ChangeNotes    string `json:"change_notes"`
	RolledBackAt   *time.Time `json:"rolled_back_at"`        // set if this version was restored via rollback
	RolledBackBy   *uint      `json:"rolled_back_by"`        // admin who performed rollback
}

type SubmissionVersions []SubmissionVersion

func NewSubmissionVersion() *SubmissionVersion {
	return &SubmissionVersion{}
}

func (v *SubmissionVersion) Create(db *gorm.DB) error {
	return db.Create(v).Error
}

func (v *SubmissionVersion) Get(db *gorm.DB, id uint) error {
	return db.First(v, id).Error
}

// GetByResource retrieves all versions for a specific resource, ordered by version number descending
func (v *SubmissionVersions) GetByResource(db *gorm.DB, resourceType string, resourceID uint) error {
	return db.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("version_number DESC").
		Find(v).Error
}

// GetBySubmission retrieves all versions created by a specific update submission
func (v *SubmissionVersions) GetBySubmission(db *gorm.DB, submissionID uint) error {
	return db.Where("submission_id = ?", submissionID).
		Order("version_number DESC").
		Find(v).Error
}

// GetLatestVersion returns the highest version number for a resource
func GetLatestVersionNumber(db *gorm.DB, resourceType string, resourceID uint) (int, error) {
	var maxVersion int
	err := db.Model(&SubmissionVersion{}).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Select("COALESCE(MAX(version_number), 0)").
		Scan(&maxVersion).Error
	return maxVersion, err
}
