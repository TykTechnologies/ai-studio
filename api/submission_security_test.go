package api

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestRedactPayloadSecrets_RedactsAllSecretFields(t *testing.T) {
	payload := models.JSONMap{
		"name":            "Test DS",
		"db_source_type":  "pgvector",
		"db_conn_string":  "postgresql://secret-host:5432/db",
		"db_conn_api_key": "super-secret-key",
		"embed_api_key":   "sk-secret-embed-key",
		"auth_key":        "bearer-token-secret",
		"embed_vendor":    "openai",
		"embed_model":     "text-embedding-3-small",
	}

	redacted := redactPayloadSecrets(payload)

	// Secret fields should be redacted
	assert.Equal(t, "[redacted]", redacted["db_conn_string"])
	assert.Equal(t, "[redacted]", redacted["db_conn_api_key"])
	assert.Equal(t, "[redacted]", redacted["embed_api_key"])
	assert.Equal(t, "[redacted]", redacted["auth_key"])

	// Non-secret fields should be preserved
	assert.Equal(t, "Test DS", redacted["name"])
	assert.Equal(t, "pgvector", redacted["db_source_type"])
	assert.Equal(t, "openai", redacted["embed_vendor"])
	assert.Equal(t, "text-embedding-3-small", redacted["embed_model"])
}

func TestRedactPayloadSecrets_DoesNotModifyOriginal(t *testing.T) {
	payload := models.JSONMap{
		"db_conn_api_key": "original-secret",
		"name":            "Test",
	}

	redacted := redactPayloadSecrets(payload)

	// Original should be untouched
	assert.Equal(t, "original-secret", payload["db_conn_api_key"])
	// Redacted copy should have the secret replaced
	assert.Equal(t, "[redacted]", redacted["db_conn_api_key"])
}

func TestRedactPayloadSecrets_HandlesNilPayload(t *testing.T) {
	result := redactPayloadSecrets(nil)
	assert.Nil(t, result)
}

func TestRedactPayloadSecrets_HandlesEmptyPayload(t *testing.T) {
	result := redactPayloadSecrets(models.JSONMap{})
	assert.NotNil(t, result)
	assert.Len(t, result, 0)
}

func TestRedactPayloadSecrets_HandlesPayloadWithoutSecrets(t *testing.T) {
	payload := models.JSONMap{
		"name":        "Safe DS",
		"description": "No secrets here",
	}

	redacted := redactPayloadSecrets(payload)

	assert.Equal(t, "Safe DS", redacted["name"])
	assert.Equal(t, "No secrets here", redacted["description"])
	assert.Len(t, redacted, 2)
}

func TestSerializeSubmissionForPortal_OmitsReviewNotes(t *testing.T) {
	submission := &models.Submission{
		ResourceType:    "datasource",
		Status:          "rejected",
		ReviewNotes:     "Internal: this is suspicious",
		SubmitterFeedback: "Please improve documentation",
		ResourcePayload: models.JSONMap{"name": "Test"},
	}

	result := serializeSubmissionForPortal(submission)

	// review_notes should NOT be present
	_, hasReviewNotes := result["review_notes"]
	assert.False(t, hasReviewNotes, "portal serializer should not include review_notes")

	// submitter_feedback SHOULD be present
	assert.Equal(t, "Please improve documentation", result["submitter_feedback"])
}

func TestSerializeSubmissionForAdmin_IncludesReviewNotes(t *testing.T) {
	submission := &models.Submission{
		ResourceType:    "datasource",
		Status:          "rejected",
		ReviewNotes:     "Internal: this is suspicious",
		SubmitterFeedback: "Please improve documentation",
		ResourcePayload: models.JSONMap{"name": "Test"},
	}

	result := serializeSubmissionForAdmin(submission)

	// review_notes SHOULD be present for admin
	assert.Equal(t, "Internal: this is suspicious", result["review_notes"])
	assert.Equal(t, "Please improve documentation", result["submitter_feedback"])
}

func TestSerializeSubmission_RedactsPayloadSecrets(t *testing.T) {
	submission := &models.Submission{
		ResourceType: "datasource",
		Status:       "submitted",
		ResourcePayload: models.JSONMap{
			"name":            "Test DS",
			"db_conn_api_key": "secret-key",
			"embed_api_key":   "sk-secret",
		},
	}

	// Both admin and portal should redact payload secrets
	adminResult := serializeSubmissionForAdmin(submission)
	portalResult := serializeSubmissionForPortal(submission)

	adminPayload := adminResult["resource_payload"].(models.JSONMap)
	portalPayload := portalResult["resource_payload"].(models.JSONMap)

	assert.Equal(t, "[redacted]", adminPayload["db_conn_api_key"])
	assert.Equal(t, "[redacted]", adminPayload["embed_api_key"])
	assert.Equal(t, "Test DS", adminPayload["name"])

	assert.Equal(t, "[redacted]", portalPayload["db_conn_api_key"])
	assert.Equal(t, "[redacted]", portalPayload["embed_api_key"])
	assert.Equal(t, "Test DS", portalPayload["name"])
}
