package services

import (
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Input length limits for submission fields
const (
	maxTextFieldLength    = 10000 // notes, privacy_justification, sla_expectation
	maxShortFieldLength   = 255   // primary_contact, secondary_contact
	maxURLFieldLength     = 2048  // documentation_url
	minPrivacyScore       = 0
	maxPrivacyScore       = 100
)

// payloadCredentialFields are the keys in ResourcePayload that contain secrets
var payloadCredentialFields = []string{"db_conn_api_key", "embed_api_key", "auth_key", "db_conn_string"}

func (s *Service) encryptSubmissionPayload(ctx context.Context, payload map[string]interface{}) {
	if s.Secrets == nil || payload == nil {
		return
	}
	for _, field := range payloadCredentialFields {
		if val, ok := payload[field]; ok {
			if str, ok := val.(string); ok && str != "" && str != "[redacted]" {
				encrypted, err := s.Secrets.EncryptValue(ctx, str)
				if err == nil {
					payload[field] = encrypted
				}
			}
		}
	}
}

func (s *Service) decryptSubmissionPayload(ctx context.Context, payload map[string]interface{}) {
	if s.Secrets == nil || payload == nil {
		return
	}
	for _, field := range payloadCredentialFields {
		if val, ok := payload[field]; ok {
			if str, ok := val.(string); ok {
				decrypted, err := s.Secrets.DecryptValue(ctx, str)
				if err == nil {
					payload[field] = decrypted
				}
			}
		}
	}
}

// validateDocumentationURL ensures the URL uses a safe protocol (http/https only).
// This prevents XSS via javascript: URIs being rendered as clickable links.
func validateDocumentationURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	parsed, err := neturl.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("documentation_url is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("documentation_url must use http or https protocol")
	}
	return nil
}

// validateSubmissionInput validates all user-provided submission fields for length
// and content safety. Call this before persisting any submission data.
func validateSubmissionInput(
	suggestedPrivacy int,
	privacyJustification string,
	primaryContact, secondaryContact, slaExpectation string,
	documentationURL, notes string,
	resourcePayload models.JSONMap,
) error {
	// Privacy score range
	if suggestedPrivacy < minPrivacyScore || suggestedPrivacy > maxPrivacyScore {
		return fmt.Errorf("suggested_privacy must be between %d and %d", minPrivacyScore, maxPrivacyScore)
	}

	// Text field lengths
	if len(notes) > maxTextFieldLength {
		return fmt.Errorf("notes must not exceed %d characters", maxTextFieldLength)
	}
	if len(privacyJustification) > maxTextFieldLength {
		return fmt.Errorf("privacy_justification must not exceed %d characters", maxTextFieldLength)
	}
	if len(slaExpectation) > maxTextFieldLength {
		return fmt.Errorf("sla_expectation must not exceed %d characters", maxTextFieldLength)
	}

	// Short field lengths
	if len(primaryContact) > maxShortFieldLength {
		return fmt.Errorf("primary_contact must not exceed %d characters", maxShortFieldLength)
	}
	if len(secondaryContact) > maxShortFieldLength {
		return fmt.Errorf("secondary_contact must not exceed %d characters", maxShortFieldLength)
	}

	// URL validation
	if len(documentationURL) > maxURLFieldLength {
		return fmt.Errorf("documentation_url must not exceed %d characters", maxURLFieldLength)
	}
	if err := validateDocumentationURL(documentationURL); err != nil {
		return err
	}

	// Resource payload size (configurable via MAX_RESOURCE_PAYLOAD_SIZE env var)
	if resourcePayload != nil {
		payloadBytes, err := json.Marshal(resourcePayload)
		if err != nil {
			return fmt.Errorf("invalid resource_payload: %w", err)
		}
		maxSize := 5 * 1024 * 1024 // default 5MB
		if appConf := config.Get(""); appConf != nil && appConf.MaxResourcePayloadSize > 0 {
			maxSize = appConf.MaxResourcePayloadSize
		}
		if len(payloadBytes) > maxSize {
			return fmt.Errorf("resource_payload must not exceed %d bytes", maxSize)
		}
	}

	return nil
}

// CreateSubmission creates a new submission (draft or submitted)
func (s *Service) CreateSubmission(submitterID uint, resourceType, status string, payload models.JSONMap,
	attestations models.JSONMap, suggestedPrivacy int, privacyJustification string,
	primaryContact, secondaryContact, slaExpectation string, dataCutoffDate *time.Time,
	documentationURL, notes string) (*models.Submission, error) {

	if resourceType != models.SubmissionResourceTypeDatasource && resourceType != models.SubmissionResourceTypeTool {
		return nil, fmt.Errorf("invalid resource type: must be '%s' or '%s'", models.SubmissionResourceTypeDatasource, models.SubmissionResourceTypeTool)
	}

	if err := validateSubmissionInput(suggestedPrivacy, privacyJustification, primaryContact, secondaryContact, slaExpectation, documentationURL, notes, payload); err != nil {
		return nil, err
	}

	if status == "" {
		status = models.SubmissionStatusDraft
	}
	if status != models.SubmissionStatusDraft && status != models.SubmissionStatusSubmitted {
		return nil, fmt.Errorf("initial status must be '%s' or '%s'", models.SubmissionStatusDraft, models.SubmissionStatusSubmitted)
	}

	submission := &models.Submission{
		ResourceType:         resourceType,
		Status:               status,
		SubmitterID:          submitterID,
		ResourcePayload:      payload,
		Attestations:         attestations,
		SuggestedPrivacy:     suggestedPrivacy,
		PrivacyJustification: privacyJustification,
		PrimaryContact:       primaryContact,
		SecondaryContact:     secondaryContact,
		SLAExpectation:       slaExpectation,
		DataCutoffDate:       dataCutoffDate,
		DocumentationURL:     documentationURL,
		Notes:                notes,
	}

	if status == models.SubmissionStatusSubmitted {
		now := time.Now()
		submission.SubmittedAt = &now
	}

	ctx := context.Background()
	s.encryptSubmissionPayload(ctx, submission.ResourcePayload)
	if err := submission.Create(s.DB); err != nil {
		return nil, err
	}
	s.decryptSubmissionPayload(ctx, submission.ResourcePayload)

	// Notify admins of new submission
	if status == models.SubmissionStatusSubmitted && s.NotificationService != nil {
		s.notifyAdminsOfSubmission(submission)
	}

	return submission, nil
}

// GetSubmissionByID retrieves a submission by ID
func (s *Service) GetSubmissionByID(id uint) (*models.Submission, error) {
	submission := models.NewSubmission()
	if err := submission.Get(s.DB, id); err != nil {
		return nil, err
	}
	s.decryptSubmissionPayload(context.Background(), submission.ResourcePayload)
	return submission, nil
}

// UpdateSubmission updates a draft or changes_requested submission
func (s *Service) UpdateSubmission(id uint, submitterID uint, payload models.JSONMap,
	attestations models.JSONMap, suggestedPrivacy int, privacyJustification string,
	primaryContact, secondaryContact, slaExpectation string, dataCutoffDate *time.Time,
	documentationURL, notes string) (*models.Submission, error) {

	submission, err := s.GetSubmissionByID(id)
	if err != nil {
		return nil, err
	}

	// Only the submitter can update their own submission
	if submission.SubmitterID != submitterID {
		return nil, fmt.Errorf("not authorized to update this submission")
	}

	// Only allow updates to draft or changes_requested submissions
	if submission.Status != models.SubmissionStatusDraft && submission.Status != models.SubmissionStatusChangesRequested {
		return nil, fmt.Errorf("can only update submissions in '%s' or '%s' status", models.SubmissionStatusDraft, models.SubmissionStatusChangesRequested)
	}

	if err := validateSubmissionInput(suggestedPrivacy, privacyJustification, primaryContact, secondaryContact, slaExpectation, documentationURL, notes, payload); err != nil {
		return nil, err
	}

	// Preserve original credentials when new payload contains "[redacted]" placeholders
	submission.ResourcePayload = mergePayloadPreservingCredentials(submission.ResourcePayload, payload)
	submission.Attestations = attestations
	submission.SuggestedPrivacy = suggestedPrivacy
	submission.PrivacyJustification = privacyJustification
	submission.PrimaryContact = primaryContact
	submission.SecondaryContact = secondaryContact
	submission.SLAExpectation = slaExpectation
	submission.DataCutoffDate = dataCutoffDate
	submission.DocumentationURL = documentationURL
	submission.Notes = notes

	ctx := context.Background()
	s.encryptSubmissionPayload(ctx, submission.ResourcePayload)
	if err := submission.UpdateWithLock(s.DB); err != nil {
		return nil, err
	}
	s.decryptSubmissionPayload(ctx, submission.ResourcePayload)
	return submission, nil
}

// SubmitSubmission moves a draft to submitted status
func (s *Service) SubmitSubmission(id, submitterID uint) (*models.Submission, error) {
	submission, err := s.GetSubmissionByID(id)
	if err != nil {
		return nil, err
	}

	if submission.SubmitterID != submitterID {
		return nil, fmt.Errorf("not authorized to submit this submission")
	}

	if submission.Status != models.SubmissionStatusDraft && submission.Status != models.SubmissionStatusChangesRequested {
		return nil, fmt.Errorf("can only submit from '%s' or '%s' status", models.SubmissionStatusDraft, models.SubmissionStatusChangesRequested)
	}

	// Validate resource payload has required fields for the resource type
	if err := validateResourcePayload(submission.ResourceType, submission.ResourcePayload); err != nil {
		return nil, err
	}

	// Validate required attestations are present
	if err := s.validateAttestations(submission); err != nil {
		return nil, err
	}

	now := time.Now()
	wasChangesRequested := submission.Status == models.SubmissionStatusChangesRequested
	submission.Status = models.SubmissionStatusSubmitted
	submission.SubmittedAt = &now

	if err := submission.UpdateWithLock(s.DB); err != nil {
		return nil, err
	}

	activityType := models.ActivityTypeSubmitted
	if wasChangesRequested {
		activityType = models.ActivityTypeResubmitted
	}
	s.RecordSubmissionActivity(submission.ID, submitterID, "", activityType, "", "")

	if s.NotificationService != nil {
		s.notifyAdminsOfSubmission(submission)
	}

	return submission, nil
}

// DeleteSubmission deletes a draft submission
func (s *Service) DeleteSubmission(id, submitterID uint) error {
	submission, err := s.GetSubmissionByID(id)
	if err != nil {
		return err
	}

	if submission.SubmitterID != submitterID {
		return fmt.Errorf("not authorized to delete this submission")
	}

	if submission.Status != models.SubmissionStatusDraft {
		return fmt.Errorf("can only delete submissions in '%s' status", models.SubmissionStatusDraft)
	}

	return submission.Delete(s.DB)
}

// GetSubmissionsBySubmitter retrieves submissions for a specific user
func (s *Service) GetSubmissionsBySubmitter(submitterID uint, status string, pageSize, pageNumber int) (models.Submissions, int64, int, error) {
	var submissions models.Submissions
	totalCount, totalPages, err := submissions.GetBySubmitter(s.DB, submitterID, status, pageSize, pageNumber)
	if err != nil {
		return nil, 0, 0, err
	}
	ctx := context.Background()
	for i := range submissions {
		s.decryptSubmissionPayload(ctx, submissions[i].ResourcePayload)
	}
	return submissions, totalCount, totalPages, nil
}

// --- Admin actions ---

// GetAllSubmissions retrieves all submissions with optional filters (admin)
func (s *Service) GetAllSubmissions(status, resourceType string, pageSize, pageNumber int) (models.Submissions, int64, int, error) {
	var submissions models.Submissions
	totalCount, totalPages, err := submissions.GetAll(s.DB, status, resourceType, pageSize, pageNumber)
	if err != nil {
		return nil, 0, 0, err
	}
	ctx := context.Background()
	for i := range submissions {
		s.decryptSubmissionPayload(ctx, submissions[i].ResourcePayload)
	}
	return submissions, totalCount, totalPages, nil
}

// GetSubmissionStatusCounts returns status counts for the admin dashboard
func (s *Service) GetSubmissionStatusCounts() (map[string]int64, error) {
	return models.GetSubmissionStatusCounts(s.DB)
}

// StartReview claims a submission for review (admin)
func (s *Service) StartReview(submissionID, reviewerID uint) (*models.Submission, error) {
	submission, err := s.GetSubmissionByID(submissionID)
	if err != nil {
		return nil, err
	}

	if submission.Status != models.SubmissionStatusSubmitted {
		return nil, fmt.Errorf("can only review submissions in '%s' status", models.SubmissionStatusSubmitted)
	}

	now := time.Now()
	submission.Status = models.SubmissionStatusInReview
	submission.ReviewerID = &reviewerID
	submission.ReviewStartedAt = &now

	if err := submission.UpdateWithLock(s.DB); err != nil {
		return nil, err
	}

	s.RecordSubmissionActivity(submission.ID, reviewerID, "", models.ActivityTypeReviewStarted, "", "")

	return submission, nil
}

// ApproveSubmission approves a submission and creates or updates the resource (admin).
// All operations are wrapped in a DB transaction to prevent orphaned resources.
func (s *Service) ApproveSubmission(submissionID, reviewerID uint, finalPrivacyScore int, catalogueIDs models.JSONMap, reviewNotes string) (*models.Submission, error) {
	submission, err := s.GetSubmissionByID(submissionID)
	if err != nil {
		return nil, err
	}

	if submission.Status != models.SubmissionStatusInReview && submission.Status != models.SubmissionStatusSubmitted {
		return nil, fmt.Errorf("can only approve submissions in '%s' or '%s' status", models.SubmissionStatusInReview, models.SubmissionStatusSubmitted)
	}

	// Begin transaction — all writes must succeed or all roll back
	tx := s.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	var resourceID uint

	if submission.IsUpdate && submission.TargetResourceID != nil {
		resourceID = *submission.TargetResourceID
		if err := s.snapshotAndUpdateResourceTx(tx, submission, reviewerID, finalPrivacyScore); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update resource: %w", err)
		}
	} else {
		resourceID, err = s.createResourceFromSubmissionTx(tx, submission, finalPrivacyScore)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}
	}

	now := time.Now()
	submission.Status = models.SubmissionStatusApproved
	submission.ReviewerID = &reviewerID
	submission.ResourceID = &resourceID
	submission.FinalPrivacyScore = &finalPrivacyScore
	submission.AssignedCatalogues = catalogueIDs
	submission.ReviewNotes = reviewNotes
	submission.ReviewCompletedAt = &now

	if err := submission.UpdateWithLock(tx); err != nil {
		tx.Rollback()
		return nil, err
	}

	// Record activity within the transaction
	activity := &models.SubmissionActivity{
		SubmissionID: submissionID,
		ActorID:      reviewerID,
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

	// Notifications are sent after commit (non-transactional, fire-and-forget)
	if s.NotificationService != nil {
		s.notifySubmitterOfDecision(submission, "approved")
	}

	return submission, nil
}

// RejectSubmission rejects a submission (admin)
func (s *Service) RejectSubmission(submissionID, reviewerID uint, feedback, reviewNotes string) (*models.Submission, error) {
	submission, err := s.GetSubmissionByID(submissionID)
	if err != nil {
		return nil, err
	}

	if submission.Status != models.SubmissionStatusInReview && submission.Status != models.SubmissionStatusSubmitted {
		return nil, fmt.Errorf("can only reject submissions in '%s' or '%s' status", models.SubmissionStatusInReview, models.SubmissionStatusSubmitted)
	}

	now := time.Now()
	submission.Status = models.SubmissionStatusRejected
	submission.ReviewerID = &reviewerID
	submission.SubmitterFeedback = feedback
	submission.ReviewNotes = reviewNotes
	submission.ReviewCompletedAt = &now

	if err := submission.UpdateWithLock(s.DB); err != nil {
		return nil, err
	}

	s.RecordSubmissionActivity(submissionID, reviewerID, "", models.ActivityTypeRejected, feedback, reviewNotes)

	if s.NotificationService != nil {
		s.notifySubmitterOfDecision(submission, "rejected")
	}

	return submission, nil
}

// RequestChanges requests changes from the submitter (admin)
func (s *Service) RequestChanges(submissionID, reviewerID uint, feedback, reviewNotes string) (*models.Submission, error) {
	submission, err := s.GetSubmissionByID(submissionID)
	if err != nil {
		return nil, err
	}

	if submission.Status != models.SubmissionStatusInReview && submission.Status != models.SubmissionStatusSubmitted {
		return nil, fmt.Errorf("can only request changes on submissions in '%s' or '%s' status", models.SubmissionStatusInReview, models.SubmissionStatusSubmitted)
	}

	submission.Status = models.SubmissionStatusChangesRequested
	submission.ReviewerID = &reviewerID
	submission.SubmitterFeedback = feedback
	submission.ReviewNotes = reviewNotes

	if err := submission.UpdateWithLock(s.DB); err != nil {
		return nil, err
	}

	s.RecordSubmissionActivity(submissionID, reviewerID, "", models.ActivityTypeChangesRequested, feedback, reviewNotes)

	if s.NotificationService != nil {
		s.notifySubmitterOfDecision(submission, "changes_requested")
	}

	return submission, nil
}

// createResourceFromSubmission creates the actual Datasource or Tool from the submission payload
func (s *Service) createResourceFromSubmission(submission *models.Submission, privacyScore int) (uint, error) {
	payload := submission.ResourcePayload

	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		return s.createDatasourceFromPayload(payload, submission.SubmitterID, privacyScore, true)
	case models.SubmissionResourceTypeTool:
		return s.createToolFromPayload(payload, submission.SubmitterID, privacyScore, true)
	default:
		return 0, fmt.Errorf("unsupported resource type: %s", submission.ResourceType)
	}
}

func (s *Service) createDatasourceFromPayload(payload models.JSONMap, submitterID uint, privacyScore int, communitySubmitted bool) (uint, error) {
	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}

	getBool := func(key string) bool {
		if v, ok := payload[key]; ok {
			if b, ok := v.(bool); ok {
				return b
			}
		}
		return false
	}

	var tagNames []string
	if tags, ok := payload["tags"]; ok {
		if tagList, ok := tags.([]interface{}); ok {
			for _, t := range tagList {
				if tagStr, ok := t.(string); ok {
					tagNames = append(tagNames, tagStr)
				}
			}
		}
	}

	ds, err := s.CreateDatasource(
		getString("name"),
		getString("short_description"),
		getString("long_description"),
		getString("icon"),
		getString("url"),
		privacyScore,
		submitterID,
		tagNames,
		getString("db_conn_string"),
		getString("db_source_type"),
		getString("db_conn_api_key"),
		getString("db_name"),
		getString("embed_vendor"),
		getString("embed_url"),
		getString("embed_api_key"),
		getString("embed_model"),
		getBool("active"),
	)
	if err != nil {
		return 0, err
	}

	// Mark as community submitted
	ds.CommunitySubmitted = communitySubmitted
	if err := ds.Update(s.DB); err != nil {
		return 0, err
	}

	return ds.ID, nil
}

func (s *Service) createToolFromPayload(payload models.JSONMap, submitterID uint, privacyScore int, communitySubmitted bool) (uint, error) {
	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}

	tool, err := s.CreateTool(
		getString("name"),
		getString("description"),
		getString("tool_type"),
		getString("oas_spec"),
		privacyScore,
		getString("auth_schema_name"),
		getString("auth_key"),
	)
	if err != nil {
		return 0, err
	}

	// Set operations
	if ops, ok := payload["available_operations"]; ok {
		if opStr, ok := ops.(string); ok && opStr != "" {
			tool.AvailableOperations = opStr
		}
	}

	// Mark as community submitted and set owner
	tool.CommunitySubmitted = communitySubmitted
	tool.UserID = submitterID
	if err := tool.Update(s.DB); err != nil {
		return 0, err
	}

	return tool.ID, nil
}

