package models

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

const (
	SubmissionStatusDraft            = "draft"
	SubmissionStatusSubmitted        = "submitted"
	SubmissionStatusInReview         = "in_review"
	SubmissionStatusApproved         = "approved"
	SubmissionStatusRejected         = "rejected"
	SubmissionStatusChangesRequested = "changes_requested"

	SubmissionResourceTypeDatasource = "datasource"
	SubmissionResourceTypeTool       = "tool"
	SubmissionResourceTypePlugin     = "plugin"
)

type Submission struct {
	gorm.Model
	ID           uint   `json:"id" gorm:"primaryKey"`
	ResourceType string `json:"resource_type" gorm:"index"` // datasource | tool | plugin
	ResourceID   *uint  `json:"resource_id"`                 // set after approval creates the resource

	// Plugin resource type reference (only set when ResourceType == "plugin")
	PluginResourceTypeID *uint               `json:"plugin_resource_type_id"`
	PluginResourceType   *PluginResourceType `json:"plugin_resource_type,omitempty" gorm:"foreignKey:PluginResourceTypeID"`
	Status       string `json:"status" gorm:"index"`         // draft | submitted | in_review | approved | rejected | changes_requested
	LockVersion  int    `json:"lock_version"`                // optimistic concurrency control

	// Update workflow: when IsUpdate is true, this submission proposes changes to an existing resource
	IsUpdate         bool  `json:"is_update"`
	TargetResourceID *uint `json:"target_resource_id"` // the existing resource being updated

	SubmitterID uint  `json:"submitter_id" gorm:"index"`
	Submitter   *User `json:"submitter,omitempty" gorm:"foreignKey:SubmitterID"`
	ReviewerID  *uint `json:"reviewer_id"`
	Reviewer    *User `json:"reviewer,omitempty" gorm:"foreignKey:ReviewerID"`

	// Resource payload — stored as JSON, used to create the actual resource on approval
	ResourcePayload JSONMap `json:"resource_payload" gorm:"type:json"`

	// Governance metadata
	Attestations         JSONMap `json:"attestations" gorm:"type:json"` // array of {template_id, accepted_at, text}
	SuggestedPrivacy     int    `json:"suggested_privacy"`
	PrivacyJustification string `json:"privacy_justification"`

	// Support metadata
	PrimaryContact   string     `json:"primary_contact"`
	SecondaryContact string     `json:"secondary_contact"`
	SLAExpectation   string     `json:"sla_expectation"`
	DataCutoffDate   *time.Time `json:"data_cutoff_date"`
	DocumentationURL string     `json:"documentation_url"`
	Notes            string     `json:"notes"`

	// Review metadata
	ReviewNotes        string `json:"review_notes"`        // admin-facing notes
	SubmitterFeedback  string `json:"submitter_feedback"`  // submitter-facing feedback
	AssignedCatalogues JSONMap `json:"assigned_catalogues"` // array of catalogue IDs
	FinalPrivacyScore  *int   `json:"final_privacy_score"` // set by admin during review

	// Tracking timestamps
	SubmittedAt       *time.Time `json:"submitted_at"`
	ReviewStartedAt   *time.Time `json:"review_started_at"`
	ReviewCompletedAt *time.Time `json:"review_completed_at"`
}

type Submissions []Submission

func NewSubmission() *Submission {
	return &Submission{}
}

// payloadCredentialFields are the keys in ResourcePayload that contain secrets
var payloadCredentialFields = []string{"db_conn_api_key", "embed_api_key", "auth_key", "db_conn_string"}

// BeforeSave encrypts credential fields in ResourcePayload before writing to DB
func (s *Submission) BeforeSave(tx *gorm.DB) error {
	if s.ResourcePayload != nil {
		for _, field := range payloadCredentialFields {
			if val, ok := s.ResourcePayload[field]; ok {
				if str, ok := val.(string); ok && str != "" && str != "[redacted]" {
					s.ResourcePayload[field] = secrets.EncryptValue(str)
				}
			}
		}
	}
	return nil
}

// AfterFind decrypts credential fields in ResourcePayload after reading from DB
func (s *Submission) AfterFind(tx *gorm.DB) error {
	s.decryptPayloadFields()
	return nil
}

// decryptPayloadFields decrypts credential fields in ResourcePayload in-place.
func (s *Submission) decryptPayloadFields() {
	if s.ResourcePayload != nil {
		for _, field := range payloadCredentialFields {
			if val, ok := s.ResourcePayload[field]; ok {
				if str, ok := val.(string); ok {
					s.ResourcePayload[field] = secrets.DecryptValue(str)
				}
			}
		}
	}
}

func (s *Submission) Create(db *gorm.DB) error {
	if err := db.Create(s).Error; err != nil {
		return err
	}
	// BeforeSave encrypts fields in-place; decrypt so callers see plaintext.
	s.decryptPayloadFields()
	return nil
}

func (s *Submission) Get(db *gorm.DB, id uint) error {
	return db.Preload("Submitter").Preload("Reviewer").First(s, id).Error
}

func (s *Submission) Update(db *gorm.DB) error {
	if err := db.Save(s).Error; err != nil {
		return err
	}
	s.decryptPayloadFields()
	return nil
}

// UpdateWithLock performs an optimistic concurrency update.
// Returns an error if the lock_version has changed since the submission was read.
// Uses GORM's Select("*") to automatically include all struct fields — no manual map needed.
func (s *Submission) UpdateWithLock(db *gorm.DB) error {
	currentVersion := s.LockVersion
	s.LockVersion = currentVersion + 1
	s.UpdatedAt = time.Now()

	// Select("*") tells GORM to update all fields from the struct, Omit protects immutable fields.
	// This is maintainable: adding a new field to the Submission struct automatically includes it.
	result := db.Model(s).
		Where("id = ? AND lock_version = ? AND deleted_at IS NULL", s.ID, currentVersion).
		Select("*").
		Omit("id", "created_at", "deleted_at").
		Updates(s)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("concurrent modification detected: submission was modified by another request")
	}
	s.decryptPayloadFields()
	return nil
}

func (s *Submission) Delete(db *gorm.DB) error {
	return db.Delete(s).Error
}

// GetBySubmitter retrieves all submissions for a specific user
func (s *Submissions) GetBySubmitter(db *gorm.DB, submitterID uint, status string, pageSize, pageNumber int) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Submission{}).Where("submitter_id = ?", submitterID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	offset := (pageNumber - 1) * pageSize
	err := query.Preload("Submitter").Preload("Reviewer").
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(s).Error
	return totalCount, totalPages, err
}

// GetAll retrieves all submissions with optional filters (for admin)
func (s *Submissions) GetAll(db *gorm.DB, status, resourceType string, pageSize, pageNumber int) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Submission{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	offset := (pageNumber - 1) * pageSize
	err := query.Preload("Submitter").Preload("Reviewer").
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(s).Error
	return totalCount, totalPages, err
}

// GetStatusCounts returns counts grouped by status (for admin dashboard)
func GetSubmissionStatusCounts(db *gorm.DB) (map[string]int64, error) {
	type StatusCount struct {
		Status string
		Count  int64
	}
	var results []StatusCount
	if err := db.Model(&Submission{}).
		Select("status, count(*) as count").
		Group("status").
		Find(&results).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.Status] = r.Count
	}
	return counts, nil
}
