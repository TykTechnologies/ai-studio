package services

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
)

// MCP server submissions (Enterprise): the payload is validated against the
// Tyk connection it names, and approval is two-phase because creating the
// proxy on the Dashboard is neither transactional nor idempotent.

// SubmissionApproveOptions carries reviewer choices that only apply to some
// resource types.
type SubmissionApproveOptions struct {
	// GatewayTags overrides the submitter's requested deployment target.
	GatewayTags *[]string
	// Publish publishes the created MCP server to the portal straight away.
	Publish bool
}

func (s *Service) mcpSubmissionsAvailable() error {
	if s.TykMCP == nil || !tykmcp.IsEnterpriseAvailable() {
		return fmt.Errorf("%w: MCP server submissions need the Tyk Dashboard integration", tykmcp.ErrEnterpriseFeature)
	}
	if st := s.TykMCP.Status(); !st.Enabled {
		return fmt.Errorf("%w: MCP server submissions are unavailable while the Tyk Dashboard integration is disabled", tykmcp.ErrDisabled)
	}
	return nil
}

// validateMCPSubmissionPayload checks an mcp_server payload against the
// connection it names. No Dashboard call is made.
func (s *Service) validateMCPSubmissionPayload(payload models.JSONMap) error {
	if err := s.mcpSubmissionsAvailable(); err != nil {
		return err
	}
	in := tykmcp.RegisterInputFromPayload(payload)
	if in.ConnectionID == 0 {
		return fmt.Errorf("connection_id is required for an MCP server submission")
	}
	if in.Description == "" {
		return fmt.Errorf("description is required for an MCP server submission")
	}
	return s.TykMCP.ValidateSubmissionInput(context.Background(), in)
}

func (s *Service) mcpActor(userID uint) tykmcp.Actor {
	actor := tykmcp.Actor{UserID: userID, CanExecute: true}
	var u models.User
	if err := s.DB.First(&u, userID).Error; err == nil {
		actor.Email = u.Email
		actor.Name = u.Name
	}
	return actor
}

// approveMCPSubmission creates the proxy (or the handoff record) first, in
// its own short write, then runs the approval transaction. A retry after a
// failure between the two finds ResourceID set and skips the create.
func (s *Service) approveMCPSubmission(submission *models.Submission, reviewerID uint, finalPrivacyScore int, reviewNotes string, opts SubmissionApproveOptions) (*models.Submission, error) {
	if err := s.mcpSubmissionsAvailable(); err != nil {
		return nil, err
	}
	ctx := context.Background()
	actor := s.mcpActor(reviewerID)
	if submission.ResourceID == nil {
		in := tykmcp.RegisterInputFromPayload(submission.ResourcePayload)
		score := finalPrivacyScore
		in.PrivacyScore = &score
		in.Publish = false
		if opts.GatewayTags != nil {
			in.GatewayTags = *opts.GatewayTags
			if len(in.GatewayTags) == 0 {
				in.ConfirmNoGatewayTags = true
			}
		}
		srv, err := s.TykMCP.RegisterFromSubmission(ctx, actor, tykmcp.SubmissionRegistration{
			SubmissionID: submission.ID, SubmitterID: submission.SubmitterID, Input: in,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create the MCP server: %w", err)
		}
		id := srv.ID
		submission.ResourceID = &id
		submission.ExternalResourceID = srv.TykAPIID
		// Phase 1 is durable on its own: a failure below must not repeat it.
		if err := submission.UpdateWithLock(s.DB); err != nil {
			return nil, err
		}
		// Reload: the save hook encrypted the payload in memory, and a second
		// save of the same struct would encrypt the ciphertext again.
		submission, err = s.GetSubmissionByID(submission.ID)
		if err != nil {
			return nil, err
		}
	}

	// Resolved before the transaction: SQLite holds a single connection.
	actorName := s.ResolveActorName(reviewerID, "")
	tx := s.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	now := time.Now()
	submission.Status = models.SubmissionStatusApproved
	submission.ReviewerID = &reviewerID
	submission.FinalPrivacyScore = &finalPrivacyScore
	submission.ReviewNotes = reviewNotes
	submission.ReviewCompletedAt = &now
	if err := submission.UpdateWithLock(tx); err != nil {
		tx.Rollback()
		return nil, err
	}
	activity := &models.SubmissionActivity{
		SubmissionID: submission.ID,
		ActorID:      reviewerID,
		ActorName:    actorName,
		ActivityType: models.ActivityTypeApproved,
		InternalNote: reviewNotes,
	}
	if err := activity.Create(tx); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to record activity: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	if opts.Publish && submission.ResourceID != nil {
		// Best effort: a proxy that is not active on the Dashboard yet (handoff)
		// cannot be published; the reviewer sees that on the server page.
		if _, err := s.TykMCP.PublishServer(ctx, actor, *submission.ResourceID); err != nil {
			s.RecordSubmissionActivity(submission.ID, reviewerID, "", models.ActivityTypeApproved, "", "Not published: "+err.Error())
		}
	}
	if s.NotificationService != nil {
		s.notifySubmitterOfDecision(submission, "approved")
	}
	return submission, nil
}
