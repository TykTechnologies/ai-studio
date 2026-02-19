package models

import (
	"gorm.io/gorm"
)

const (
	ActivityTypeSubmitted        = "submitted"
	ActivityTypeReviewStarted    = "review_started"
	ActivityTypeApproved         = "approved"
	ActivityTypeRejected         = "rejected"
	ActivityTypeChangesRequested = "changes_requested"
	ActivityTypeResubmitted      = "resubmitted"
	ActivityTypeRolledBack       = "rolled_back"
)

// SubmissionActivity records each action taken on a submission for audit trail purposes
type SubmissionActivity struct {
	gorm.Model
	ID           uint   `json:"id" gorm:"primaryKey"`
	SubmissionID uint   `json:"submission_id" gorm:"index"`
	ActorID      uint   `json:"actor_id"`
	ActorName    string `json:"actor_name"`
	ActivityType string `json:"activity_type"` // submitted, review_started, approved, rejected, changes_requested, resubmitted
	Feedback     string `json:"feedback"`      // submitter-facing feedback
	InternalNote string `json:"internal_note"` // admin-only note
}

type SubmissionActivities []SubmissionActivity

func (a *SubmissionActivity) Create(db *gorm.DB) error {
	return db.Create(a).Error
}

// GetBySubmission retrieves all activities for a submission, ordered chronologically
func (a *SubmissionActivities) GetBySubmission(db *gorm.DB, submissionID uint) error {
	return db.Where("submission_id = ?", submissionID).
		Order("created_at ASC").
		Find(a).Error
}
