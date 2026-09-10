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

// submissionTypeLabel returns a human-readable label for the submission's
// resource type. Plugin resource types use the type's registered name when it
// has been preloaded (e.g. "Agent"), so notifications read naturally.
func submissionTypeLabel(submission *models.Submission) string {
	switch submission.ResourceType {
	case models.SubmissionResourceTypeDatasource:
		return "data source"
	case models.SubmissionResourceTypeTool:
		return "tool"
	case models.SubmissionResourceTypePlugin:
		if submission.PluginResourceType != nil && submission.PluginResourceType.Name != "" {
			return submission.PluginResourceType.Name
		}
		return "plugin resource"
	default:
		return submission.ResourceType
	}
}

// submissionResourceName extracts the display name from the submission payload
// (all supported payload shapes carry a top-level "name").
func submissionResourceName(submission *models.Submission) string {
	if submission.ResourcePayload == nil {
		return ""
	}
	if v, ok := submission.ResourcePayload["name"].(string); ok {
		return v
	}
	return ""
}

func (s *Service) notifyAdminsOfSubmission(submission *models.Submission) {
	label := submissionTypeLabel(submission)
	title := fmt.Sprintf("New %s submission for review", label)
	notificationID := fmt.Sprintf("submission_new_%d", submission.ID)

	content := fmt.Sprintf("A new **%s** submission is waiting for review.\n\n", label)
	if name := submissionResourceName(submission); name != "" {
		content += fmt.Sprintf("- **Name:** %s\n", name)
	}
	content += fmt.Sprintf("- **Suggested privacy score:** %d\n", submission.SuggestedPrivacy)
	content += fmt.Sprintf("\n[Open the submission queue](/admin/submissions/%d)", submission.ID)

	if err := s.NotificationService.NotifyDirect(
		notificationID,
		"submission",
		title,
		content,
		models.NotifyAdmins,
	); err != nil {
		logger.Warn(fmt.Sprintf("Failed to notify admins of submission %d: %v", submission.ID, err))
	}
}

func (s *Service) notifySubmitterOfDecision(submission *models.Submission, decision string) {
	label := submissionTypeLabel(submission)
	title := fmt.Sprintf("Your %s submission has been %s", label, decision)
	notificationID := fmt.Sprintf("submission_%s_%d", decision, submission.ID)

	content := fmt.Sprintf("Your **%s** submission", label)
	if name := submissionResourceName(submission); name != "" {
		content += fmt.Sprintf(" **%s**", name)
	}
	content += fmt.Sprintf(" has been **%s**.\n", strings.ReplaceAll(decision, "_", " "))
	if submission.SubmitterFeedback != "" {
		content += fmt.Sprintf("\n**Feedback from the reviewer:**\n\n%s\n", submission.SubmitterFeedback)
	}
	content += fmt.Sprintf("\n[View your contribution](/portal/submissions/%d)", submission.ID)

	if err := s.NotificationService.NotifyDirect(
		notificationID,
		"submission",
		title,
		content,
		submission.SubmitterID,
	); err != nil {
		logger.Warn(fmt.Sprintf("Failed to notify submitter of submission %d decision: %v", submission.ID, err))
	}
}
