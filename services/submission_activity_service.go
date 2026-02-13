package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// RecordSubmissionActivity logs an action on a submission for audit trail
func (s *Service) RecordSubmissionActivity(submissionID, actorID uint, actorName, activityType, feedback, internalNote string) {
	activity := &models.SubmissionActivity{
		SubmissionID: submissionID,
		ActorID:      actorID,
		ActorName:    actorName,
		ActivityType: activityType,
		Feedback:     feedback,
		InternalNote: internalNote,
	}
	if err := activity.Create(s.DB); err != nil {
		logger.Warn(fmt.Sprintf("Failed to record submission activity: %v", err))
	}
}

// GetSubmissionActivities retrieves the audit trail for a submission
func (s *Service) GetSubmissionActivities(submissionID uint) (models.SubmissionActivities, error) {
	var activities models.SubmissionActivities
	if err := activities.GetBySubmission(s.DB, submissionID); err != nil {
		return nil, err
	}
	return activities, nil
}
