package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
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

// validateDocumentationURL ensures the URL uses a safe protocol (http/https only).
// This prevents XSS via javascript: URIs being rendered as clickable links.
func validateDocumentationURL(url string) error {
	if url == "" {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(url))
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("documentation_url must start with http:// or https://")
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

	submission.ResourcePayload = payload
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

// ApproveSubmission approves a submission and creates or updates the resource (admin)
func (s *Service) ApproveSubmission(submissionID, reviewerID uint, finalPrivacyScore int, catalogueIDs models.JSONMap, reviewNotes string) (*models.Submission, error) {
	submission, err := s.GetSubmissionByID(submissionID)
	if err != nil {
		return nil, err
	}

	if submission.Status != models.SubmissionStatusInReview && submission.Status != models.SubmissionStatusSubmitted {
		return nil, fmt.Errorf("can only approve submissions in '%s' or '%s' status", models.SubmissionStatusInReview, models.SubmissionStatusSubmitted)
	}

	if finalPrivacyScore < minPrivacyScore || finalPrivacyScore > maxPrivacyScore {
		return nil, fmt.Errorf("final_privacy_score must be between %d and %d", minPrivacyScore, maxPrivacyScore)
	}

	var resourceID uint

	if submission.IsUpdate && submission.TargetResourceID != nil {
		// Update workflow: snapshot current state, then apply changes
		resourceID = *submission.TargetResourceID
		if err := s.snapshotAndUpdateResource(submission, reviewerID, finalPrivacyScore); err != nil {
			return nil, fmt.Errorf("failed to update resource: %w", err)
		}
	} else {
		// New submission: create the resource
		resourceID, err = s.createResourceFromSubmission(submission, finalPrivacyScore)
		if err != nil {
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

	if err := submission.UpdateWithLock(s.DB); err != nil {
		return nil, err
	}

	s.RecordSubmissionActivity(submissionID, reviewerID, "", models.ActivityTypeApproved, "", reviewNotes)

	// Notify submitter
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

func (s *Service) snapshotDatasource(id uint) (models.JSONMap, error) {
	ds, err := s.GetDatasourceByID(id)
	if err != nil {
		return nil, err
	}
	return models.JSONMap{
		"name":              ds.Name,
		"short_description": ds.ShortDescription,
		"long_description":  ds.LongDescription,
		"icon":              ds.Icon,
		"url":               ds.Url,
		"privacy_score":     ds.PrivacyScore,
		"db_conn_string":    ds.DBConnString,
		"db_source_type":    ds.DBSourceType,
		"db_conn_api_key":   ds.DBConnAPIKey,
		"db_name":           ds.DBName,
		"embed_vendor":      string(ds.EmbedVendor),
		"embed_url":         ds.EmbedUrl,
		"embed_api_key":     ds.EmbedAPIKey,
		"embed_model":       ds.EmbedModel,
		"active":            ds.Active,
	}, nil
}

func (s *Service) snapshotTool(id uint) (models.JSONMap, error) {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return nil, err
	}
	return models.JSONMap{
		"name":                 tool.Name,
		"description":          tool.Description,
		"tool_type":            tool.ToolType,
		"oas_spec":             tool.OASSpec,
		"privacy_score":        tool.PrivacyScore,
		"auth_key":             tool.AuthKey,
		"auth_schema_name":     tool.AuthSchemaName,
		"available_operations": tool.AvailableOperations,
	}, nil
}

func (s *Service) applyDatasourceUpdate(id uint, payload models.JSONMap, privacyScore int) error {
	ds, err := s.GetDatasourceByID(id)
	if err != nil {
		return err
	}

	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}

	if v := getString("name"); v != "" {
		ds.Name = v
	}
	if v := getString("short_description"); v != "" {
		ds.ShortDescription = v
	}
	if v := getString("long_description"); v != "" {
		ds.LongDescription = v
	}
	if v := getString("icon"); v != "" {
		ds.Icon = v
	}
	if v := getString("url"); v != "" {
		ds.Url = v
	}
	ds.PrivacyScore = privacyScore
	if v := getString("db_conn_string"); v != "" {
		ds.DBConnString = v
	}
	if v := getString("db_source_type"); v != "" {
		ds.DBSourceType = v
	}
	if v := getString("db_name"); v != "" {
		ds.DBName = v
	}
	if v := getString("embed_vendor"); v != "" {
		ds.EmbedVendor = models.Vendor(v)
	}
	if v := getString("embed_url"); v != "" {
		ds.EmbedUrl = v
	}
	if v := getString("embed_model"); v != "" {
		ds.EmbedModel = v
	}

	return ds.Update(s.DB)
}

func (s *Service) applyToolUpdate(id uint, payload models.JSONMap, privacyScore int) error {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return err
	}

	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}

	if v := getString("name"); v != "" {
		tool.Name = v
	}
	if v := getString("description"); v != "" {
		tool.Description = v
	}
	if v := getString("oas_spec"); v != "" {
		tool.OASSpec = v
	}
	if v := getString("available_operations"); v != "" {
		tool.AvailableOperations = v
	}
	tool.PrivacyScore = privacyScore

	return tool.Update(s.DB)
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
