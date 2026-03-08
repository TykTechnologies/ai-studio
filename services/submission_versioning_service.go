package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// --- Transaction-aware resource creation (for ApproveSubmission) ---

// createResourceFromSubmissionTx creates a Datasource or Tool using the canonical service methods
// with the provided transaction, ensuring all validation, hooks, and catalogue assignment run.
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

	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		ds, err := s.CreateDatasourceWithDB(tx,
			getString("name"), getString("short_description"), getString("long_description"),
			getString("icon"), getString("url"), privacyScore, submission.SubmitterID, tagNames,
			getString("db_conn_string"), getString("db_source_type"), getString("db_conn_api_key"),
			getString("db_name"), getString("embed_vendor"), getString("embed_url"),
			getString("embed_api_key"), getString("embed_model"), getBool("active"),
		)
		if err != nil {
			return 0, err
		}
		// Mark as community submitted
		ds.CommunitySubmitted = true
		if err := tx.Model(ds).Update("community_submitted", true).Error; err != nil {
			return 0, err
		}
		return ds.ID, nil

	case models.SubmissionResourceTypeTool:
		tool, err := s.CreateToolWithDB(tx,
			getString("name"), getString("description"), getString("tool_type"),
			getString("oas_spec"), privacyScore, getString("auth_schema_name"), getString("auth_key"),
		)
		if err != nil {
			return 0, err
		}
		// Set operations, community flag, owner, and active
		updates := map[string]interface{}{
			"community_submitted":  true,
			"user_id":              submission.SubmitterID,
			"active":               true,
			"available_operations": getString("available_operations"),
		}
		if err := tx.Model(tool).Updates(updates).Error; err != nil {
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
		// Ownership, status, and admin-controlled flags (must not be user-modifiable via payload)
		"user_id": true, "community_submitted": true, "submission_id": true,
		"slug": true,   // auto-generated from name
		"active": true, // admin-controlled, not settable via UGC
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

	s.encryptSubmissionPayload(context.Background(), submission.ID, submission.ResourcePayload)
	if err := submission.Create(s.DB); err != nil {
		return nil, err
	}
	s.decryptSubmissionPayload(context.Background(), submission.ResourcePayload)

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
