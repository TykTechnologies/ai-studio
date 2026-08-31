package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// ResolveActorName returns a human-readable name for an activity actor.
//
// Every caller passes an empty actorName, which left ActorName empty on every
// row and made the review history render "User #1" instead of naming people.
// An audit trail that does not name people is most of the way to not being an
// audit trail, so resolve the name here rather than at each call site.
//
// A caller that already knows the name keeps it. A lookup failure is not worth
// failing the write for: the UI falls back to "User #<id>", which is what it
// rendered before.
func (s *Service) ResolveActorName(actorID uint, actorName string) string {
	if actorName != "" {
		return actorName
	}
	if actorID == 0 {
		return ""
	}

	user, err := s.GetUserByID(actorID)
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to resolve actor name for user %d: %v", actorID, err))
		return ""
	}
	if user.Name != "" {
		return user.Name
	}
	return user.Email
}

// RecordSubmissionActivity logs an action on a submission for audit trail
func (s *Service) RecordSubmissionActivity(submissionID, actorID uint, actorName, activityType, feedback, internalNote string) {
	activity := &models.SubmissionActivity{
		SubmissionID: submissionID,
		ActorID:      actorID,
		ActorName:    s.ResolveActorName(actorID, actorName),
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
