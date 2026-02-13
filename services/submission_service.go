package services

import (
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// Input length limits for submission fields
const (
	maxTextFieldLength    = 10000 // notes, privacy_justification, sla_expectation
	maxShortFieldLength   = 255   // primary_contact, secondary_contact
	maxURLFieldLength     = 2048  // documentation_url
	minPrivacyScore       = 0
	maxPrivacyScore       = 100
)

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

	if err := submission.Create(s.DB); err != nil {
		return nil, err
	}

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

	if err := submission.UpdateWithLock(s.DB); err != nil {
		return nil, err
	}
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

// --- Transaction-aware resource creation (for ApproveSubmission) ---

func (s *Service) createResourceFromSubmissionTx(tx *gorm.DB, submission *models.Submission, privacyScore int) (uint, error) {
	payload := submission.ResourcePayload
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

	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		ds := &models.Datasource{
			Name: getString("name"), ShortDescription: getString("short_description"),
			LongDescription: getString("long_description"), Icon: getString("icon"),
			Url: getString("url"), PrivacyScore: privacyScore, UserID: submission.SubmitterID,
			DBConnString: getString("db_conn_string"), DBSourceType: getString("db_source_type"),
			DBConnAPIKey: getString("db_conn_api_key"), DBName: getString("db_name"),
			EmbedVendor: models.Vendor(getString("embed_vendor")), EmbedUrl: getString("embed_url"),
			EmbedAPIKey: getString("embed_api_key"), EmbedModel: getString("embed_model"),
			Active: getBool("active"), CommunitySubmitted: true,
		}
		if err := tx.Create(ds).Error; err != nil {
			return 0, err
		}
		return ds.ID, nil

	case models.SubmissionResourceTypeTool:
		tool := &models.Tool{
			Name: getString("name"), Description: getString("description"),
			ToolType: getString("tool_type"), OASSpec: getString("oas_spec"),
			PrivacyScore: privacyScore, AuthSchemaName: getString("auth_schema_name"),
			AuthKey: getString("auth_key"), AvailableOperations: getString("available_operations"),
			UserID: submission.SubmitterID, CommunitySubmitted: true,
		}
		if err := tx.Create(tool).Error; err != nil {
			return 0, err
		}
		return tool.ID, nil

	default:
		return 0, fmt.Errorf("unsupported resource type: %s", submission.ResourceType)
	}
}

func (s *Service) snapshotAndUpdateResourceTx(tx *gorm.DB, submission *models.Submission, reviewerID uint, privacyScore int) error {
	targetID := *submission.TargetResourceID

	currentVersion, err := models.GetLatestVersionNumber(tx, submission.ResourceType, targetID)
	if err != nil {
		return fmt.Errorf("failed to get version number: %w", err)
	}

	// Snapshot using struct marshaling (captures all fields automatically)
	var snapshotPayload models.JSONMap
	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		ds := &models.Datasource{}
		if err := tx.Preload("Tags").Preload("Files").First(ds, targetID).Error; err != nil {
			return fmt.Errorf("failed to snapshot datasource: %w", err)
		}
		snapshotPayload, err = structToJSONMap(ds)
		if err != nil {
			return fmt.Errorf("failed to marshal datasource snapshot: %w", err)
		}
	case models.SubmissionResourceTypeTool:
		tool := &models.Tool{}
		if err := tx.First(tool, targetID).Error; err != nil {
			return fmt.Errorf("failed to snapshot tool: %w", err)
		}
		snapshotPayload, err = structToJSONMap(tool)
		if err != nil {
			return fmt.Errorf("failed to marshal tool snapshot: %w", err)
		}
	default:
		return fmt.Errorf("unsupported resource type: %s", submission.ResourceType)
	}

	// Redact credentials before storing snapshot — credentials should never be in version history
	snapshotPayload = redactSnapshotCredentials(snapshotPayload)

	version := &models.SubmissionVersion{
		SubmissionID: submission.ID, ResourceID: targetID,
		ResourceType: submission.ResourceType, VersionNumber: currentVersion + 1,
		Payload: snapshotPayload, ChangedBy: submission.SubmitterID,
		ApprovedBy: reviewerID, ChangeNotes: submission.Notes,
	}
	if err := tx.Create(version).Error; err != nil {
		return fmt.Errorf("failed to create version snapshot: %w", err)
	}

	// Apply updates from payload — build updates map dynamically from non-empty string fields
	updates := payloadToUpdatesMap(submission.ResourcePayload)
	updates["privacy_score"] = privacyScore

	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		return tx.Model(&models.Datasource{}).Where("id = ?", targetID).Updates(updates).Error
	case models.SubmissionResourceTypeTool:
		return tx.Model(&models.Tool{}).Where("id = ?", targetID).Updates(updates).Error
	}
	return nil
}

// payloadToUpdatesMap converts a JSONMap payload into a map suitable for GORM Updates,
// including only non-empty string values and non-nil values.
func payloadToUpdatesMap(payload models.JSONMap) map[string]interface{} {
	updates := make(map[string]interface{})
	// Skip GORM metadata fields and credential fields (those are handled separately)
	skipFields := map[string]bool{
		// GORM metadata
		"id": true, "ID": true, "CreatedAt": true, "UpdatedAt": true, "DeletedAt": true,
		"created_at": true, "updated_at": true, "deleted_at": true,
		// Relationships (managed separately)
		"tags": true, "files": true, "file_stores": true, "filters": true,
		"dependencies": true, "apps": true, "metadata": true,
		// Ownership and status flags (must not be user-modifiable via payload)
		"user_id": true, "community_submitted": true, "submission_id": true,
		"slug": true, // auto-generated from name
	}
	for k, v := range payload {
		if skipFields[k] {
			continue
		}
		if str, ok := v.(string); ok {
			if str == "[redacted]" {
				continue // Skip redacted placeholders — originals preserved upstream
			}
			updates[k] = v // Empty strings are valid (intentional clearing)
		} else if v != nil {
			updates[k] = v
		}
	}
	return updates
}

// --- Update workflow: snapshot + apply ---

// CreateUpdateSubmission creates a submission that proposes changes to an existing published resource
func (s *Service) CreateUpdateSubmission(submitterID uint, resourceType string, targetResourceID uint,
	payload models.JSONMap, attestations models.JSONMap, suggestedPrivacy int, privacyJustification string,
	primaryContact, secondaryContact, slaExpectation string, dataCutoffDate *time.Time,
	documentationURL, notes string, status string) (*models.Submission, error) {

	// Verify the target resource exists and is community-submitted
	switch resourceType {
	case models.SubmissionResourceTypeDatasource:
		ds, err := s.GetDatasourceByID(targetResourceID)
		if err != nil {
			return nil, fmt.Errorf("target datasource not found: %w", err)
		}
		if ds.UserID != submitterID {
			return nil, fmt.Errorf("not authorized: you can only propose updates to resources you own")
		}
	case models.SubmissionResourceTypeTool:
		tool, err := s.GetToolByID(targetResourceID)
		if err != nil {
			return nil, fmt.Errorf("target tool not found: %w", err)
		}
		if tool.UserID != submitterID {
			return nil, fmt.Errorf("not authorized: you can only propose updates to resources you own")
		}
	default:
		return nil, fmt.Errorf("invalid resource type: %s", resourceType)
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
		IsUpdate:             true,
		TargetResourceID:     &targetResourceID,
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

	if err := submission.Create(s.DB); err != nil {
		return nil, err
	}

	if status == models.SubmissionStatusSubmitted && s.NotificationService != nil {
		s.notifyAdminsOfSubmission(submission)
	}

	return submission, nil
}

// snapshotAndUpdateResource takes a snapshot of the current resource state, then applies the update payload
func (s *Service) snapshotAndUpdateResource(submission *models.Submission, reviewerID uint, privacyScore int) error {
	targetID := *submission.TargetResourceID

	// Get current version number
	currentVersion, err := models.GetLatestVersionNumber(s.DB, submission.ResourceType, targetID)
	if err != nil {
		return fmt.Errorf("failed to get version number: %w", err)
	}

	// Snapshot current state
	var snapshotPayload models.JSONMap
	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		snapshotPayload, err = s.snapshotDatasource(targetID)
	case models.SubmissionResourceTypeTool:
		snapshotPayload, err = s.snapshotTool(targetID)
	default:
		return fmt.Errorf("unsupported resource type: %s", submission.ResourceType)
	}
	if err != nil {
		return fmt.Errorf("failed to snapshot resource: %w", err)
	}

	// Redact credentials before storing
	snapshotPayload = redactSnapshotCredentials(snapshotPayload)

	// Create version record
	version := &models.SubmissionVersion{
		SubmissionID:  submission.ID,
		ResourceID:    targetID,
		ResourceType:  submission.ResourceType,
		VersionNumber: currentVersion + 1,
		Payload:       snapshotPayload,
		ChangedBy:     submission.SubmitterID,
		ApprovedBy:    reviewerID,
		ChangeNotes:   submission.Notes,
	}
	if err := version.Create(s.DB); err != nil {
		return fmt.Errorf("failed to create version snapshot: %w", err)
	}

	// Apply the update
	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		return s.applyDatasourceUpdate(targetID, submission.ResourcePayload, privacyScore)
	case models.SubmissionResourceTypeTool:
		return s.applyToolUpdate(targetID, submission.ResourcePayload, privacyScore)
	}
	return nil
}

// structToJSONMap converts any struct to a models.JSONMap via JSON marshaling.
// This ensures all json-tagged fields are captured automatically.
func structToJSONMap(v interface{}) (models.JSONMap, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal struct: %w", err)
	}
	var result models.JSONMap
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}
	return result, nil
}

func (s *Service) snapshotDatasource(id uint) (models.JSONMap, error) {
	ds, err := s.GetDatasourceByID(id)
	if err != nil {
		return nil, err
	}
	return structToJSONMap(ds)
}

func (s *Service) snapshotTool(id uint) (models.JSONMap, error) {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return nil, err
	}
	return structToJSONMap(tool)
}

func (s *Service) applyDatasourceUpdate(id uint, payload models.JSONMap, privacyScore int) error {
	updates := payloadToUpdatesMap(payload)
	updates["privacy_score"] = privacyScore
	return s.DB.Model(&models.Datasource{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) applyToolUpdate(id uint, payload models.JSONMap, privacyScore int) error {
	updates := payloadToUpdatesMap(payload)
	updates["privacy_score"] = privacyScore
	return s.DB.Model(&models.Tool{}).Where("id = ?", id).Updates(updates).Error
}

// --- Version listing and rollback ---

// GetResourceVersions returns all version snapshots for a resource
func (s *Service) GetResourceVersions(resourceType string, resourceID uint) (models.SubmissionVersions, error) {
	var versions models.SubmissionVersions
	if err := versions.GetByResource(s.DB, resourceType, resourceID); err != nil {
		return nil, err
	}
	return versions, nil
}

// RollbackResource restores a resource to a previous version snapshot
func (s *Service) RollbackResource(resourceType string, resourceID uint, versionID uint, adminID uint) error {
	// Get the version to restore
	version := models.NewSubmissionVersion()
	if err := version.Get(s.DB, versionID); err != nil {
		return fmt.Errorf("version not found: %w", err)
	}

	// Verify it belongs to the right resource
	if version.ResourceType != resourceType || version.ResourceID != resourceID {
		return fmt.Errorf("version does not belong to the specified resource")
	}

	// Snapshot current state before rollback (so the rollback itself is reversible)
	currentVersion, err := models.GetLatestVersionNumber(s.DB, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("failed to get version number: %w", err)
	}

	var currentSnapshot models.JSONMap
	switch resourceType {
	case models.SubmissionResourceTypeDatasource:
		currentSnapshot, err = s.snapshotDatasource(resourceID)
	case models.SubmissionResourceTypeTool:
		currentSnapshot, err = s.snapshotTool(resourceID)
	default:
		return fmt.Errorf("unsupported resource type")
	}
	if err != nil {
		return fmt.Errorf("failed to snapshot current state: %w", err)
	}

	// Create a "pre-rollback" snapshot
	preRollback := &models.SubmissionVersion{
		SubmissionID:  version.SubmissionID,
		ResourceID:    resourceID,
		ResourceType:  resourceType,
		VersionNumber: currentVersion + 1,
		Payload:       currentSnapshot,
		ChangedBy:     adminID,
		ApprovedBy:    adminID,
		ChangeNotes:   fmt.Sprintf("Snapshot before rollback to version %d", version.VersionNumber),
	}
	if err := preRollback.Create(s.DB); err != nil {
		return fmt.Errorf("failed to create pre-rollback snapshot: %w", err)
	}

	// Apply the old version's payload
	switch resourceType {
	case models.SubmissionResourceTypeDatasource:
		privacyScore := 0
		if v, ok := version.Payload["privacy_score"]; ok {
			if f, ok := v.(float64); ok {
				privacyScore = int(f)
			}
		}
		if err := s.applyDatasourceUpdate(resourceID, version.Payload, privacyScore); err != nil {
			return fmt.Errorf("failed to apply rollback: %w", err)
		}
	case models.SubmissionResourceTypeTool:
		privacyScore := 0
		if v, ok := version.Payload["privacy_score"]; ok {
			if f, ok := v.(float64); ok {
				privacyScore = int(f)
			}
		}
		if err := s.applyToolUpdate(resourceID, version.Payload, privacyScore); err != nil {
			return fmt.Errorf("failed to apply rollback: %w", err)
		}
	}

	// Mark the version as rolled back to
	now := time.Now()
	version.RolledBackAt = &now
	version.RolledBackBy = &adminID
	s.DB.Save(version)

	return nil
}

// --- Resource payload validation ---

// validateResourcePayload checks that the payload contains required fields for the resource type.
// Called during SubmitSubmission to catch invalid submissions before they enter the review queue.
func validateResourcePayload(resourceType string, payload models.JSONMap) error {
	if payload == nil {
		return fmt.Errorf("resource_payload is required")
	}

	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}

	if getString("name") == "" {
		return fmt.Errorf("resource_payload must include a non-empty 'name' field")
	}

	switch resourceType {
	case models.SubmissionResourceTypeDatasource:
		if getString("db_source_type") == "" {
			return fmt.Errorf("datasource payload must include 'db_source_type'")
		}
		if getString("embed_vendor") == "" {
			return fmt.Errorf("datasource payload must include 'embed_vendor'")
		}
		if getString("embed_model") == "" {
			return fmt.Errorf("datasource payload must include 'embed_model'")
		}
	case models.SubmissionResourceTypeTool:
		if getString("oas_spec") == "" {
			return fmt.Errorf("tool payload must include 'oas_spec'")
		}
	}

	return nil
}

// --- Credential preservation ---

// credentialFields are payload keys that contain secrets and get redacted in API responses.
var credentialFields = []string{"db_conn_api_key", "embed_api_key", "auth_key", "db_conn_string"}

// mergePayloadPreservingCredentials returns the new payload, but for any credential field
// where the new value is "[redacted]", preserves the original value from the existing payload.
func mergePayloadPreservingCredentials(existing, incoming models.JSONMap) models.JSONMap {
	if incoming == nil {
		return existing
	}
	if existing == nil {
		return incoming
	}

	merged := make(models.JSONMap, len(incoming))
	for k, v := range incoming {
		merged[k] = v
	}

	for _, field := range credentialFields {
		newVal, hasNew := merged[field]
		if !hasNew {
			continue
		}
		if str, ok := newVal.(string); ok && str == "[redacted]" {
			// Preserve the original credential value
			if origVal, hasOrig := existing[field]; hasOrig {
				merged[field] = origVal
			}
		}
	}

	return merged
}

// --- Attestation validation ---

// validateAttestations checks that all required attestation templates for the resource type
// are acknowledged in the submission's attestations map.
func (s *Service) validateAttestations(submission *models.Submission) error {
	requiredTemplates, err := s.GetAttestationTemplatesByType(submission.ResourceType, true)
	if err != nil {
		return fmt.Errorf("failed to fetch attestation templates: %w", err)
	}

	// Filter to only required templates
	var required []models.AttestationTemplate
	for _, t := range requiredTemplates {
		if t.Required {
			required = append(required, t)
		}
	}

	if len(required) == 0 {
		return nil // No required attestations configured
	}

	// Parse the accepted attestation IDs from the submission
	acceptedIDs := make(map[uint]bool)
	if submission.Attestations != nil {
		if accepted, ok := submission.Attestations["accepted"]; ok {
			if acceptedList, ok := accepted.([]interface{}); ok {
				for _, item := range acceptedList {
					if m, ok := item.(map[string]interface{}); ok {
						if id, ok := m["template_id"]; ok {
							switch v := id.(type) {
							case float64:
								acceptedIDs[uint(v)] = true
							case int:
								acceptedIDs[uint(v)] = true
							}
						}
					}
				}
			}
		}
	}

	// Check each required template is acknowledged
	var missing []string
	for _, t := range required {
		if !acceptedIDs[t.ID] {
			missing = append(missing, t.Name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("required attestations not accepted: %s", strings.Join(missing, ", "))
	}

	return nil
}

// --- Snapshot credential redaction ---

// redactSnapshotCredentials removes sensitive credential values from a snapshot payload
// before storing to the submission_versions table. Credentials should never be stored
// in version history — they exist on the live resource only.
func redactSnapshotCredentials(payload models.JSONMap) models.JSONMap {
	if payload == nil {
		return nil
	}
	for _, field := range credentialFields {
		if _, ok := payload[field]; ok {
			payload[field] = "[redacted]"
		}
	}
	return payload
}

// --- Notification helpers ---

func (s *Service) notifyAdminsOfSubmission(submission *models.Submission) {
	title := fmt.Sprintf("New %s submission for review", submission.ResourceType)
	notificationID := fmt.Sprintf("submission_new_%d", submission.ID)

	if err := s.NotificationService.Notify(
		notificationID,
		title,
		"",
		map[string]interface{}{
			"submission_id":   submission.ID,
			"resource_type":   submission.ResourceType,
			"submitter_id":    submission.SubmitterID,
			"suggested_score": submission.SuggestedPrivacy,
		},
		models.NotifyAdmins,
	); err != nil {
		logger.Warn(fmt.Sprintf("Failed to notify admins of submission %d: %v", submission.ID, err))
	}
}

func (s *Service) notifySubmitterOfDecision(submission *models.Submission, decision string) {
	title := fmt.Sprintf("Your %s submission has been %s", submission.ResourceType, decision)
	notificationID := fmt.Sprintf("submission_%s_%d", decision, submission.ID)

	if err := s.NotificationService.Notify(
		notificationID,
		title,
		"",
		map[string]interface{}{
			"submission_id": submission.ID,
			"resource_type": submission.ResourceType,
			"decision":      decision,
			"feedback":      submission.SubmitterFeedback,
		},
		submission.SubmitterID,
	); err != nil {
		logger.Warn(fmt.Sprintf("Failed to notify submitter of submission %d decision: %v", submission.ID, err))
	}
}
