package services

import (
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

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
