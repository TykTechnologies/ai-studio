package services

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Portal submissions may not carry $SECRET/ or $ENV/ references: once
// approved, the resource would resolve them and send the value to the
// endpoint the contributor submitted. Creating, updating and approving
// (for submissions saved before the check) all refuse them.
func TestSubmissionsRefuseReferences(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "contrib@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")
	spec := base64.StdEncoding.EncodeToString([]byte(validOASSpec))

	for _, payload := range []models.JSONMap{
		{"name": "T1", "tool_type": "REST", "oas_spec": spec, "auth_key": "$SECRET/PLATFORM_KEY"},
		{"name": "T2", "tool_type": "REST", "oas_spec": spec, "auth_key": "$ENV/TYK_AI_SECRET_KEY"},
		{"name": "T3", "tool_type": "REST", "oas_spec": spec, "nested": map[string]interface{}{"k": []interface{}{"$ENV/X"}}},
	} {
		_, err := service.CreateSubmission(user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted,
			payload, nil, 2, "", "c@test.com", "", "", nil, "", "")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSubmissionReference), "got %v", err)
	}

	// A plain submission is accepted; an update adding a reference is not.
	sub, err := service.CreateSubmission(user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusDraft,
		models.JSONMap{"name": "Plain", "tool_type": "REST", "oas_spec": spec, "auth_key": "plain-key"}, nil, 2, "", "c@test.com", "", "", nil, "", "")
	require.NoError(t, err)
	_, err = service.UpdateSubmission(sub.ID, user.ID,
		models.JSONMap{"name": "Plain", "tool_type": "REST", "oas_spec": spec, "auth_key": "$SECRET/PLATFORM_KEY"}, nil, 2, "", "c@test.com", "", "", nil, "", "")
	assert.True(t, errors.Is(err, ErrSubmissionReference), "got %v", err)

	// A submission stored with a reference before the check cannot be approved.
	legacy := &models.Submission{SubmitterID: user.ID, ResourceType: models.SubmissionResourceTypeTool,
		Status: models.SubmissionStatusSubmitted,
		ResourcePayload: models.JSONMap{"name": "Legacy", "tool_type": "REST", "oas_spec": spec, "auth_key": "$SECRET/PLATFORM_KEY"}}
	require.NoError(t, db.Create(legacy).Error)
	_, err = service.ApproveSubmission(legacy.ID, admin.ID, 2, nil, "")
	assert.True(t, errors.Is(err, ErrSubmissionReference), "got %v", err)
	var count int64
	require.NoError(t, db.Model(&models.Tool{}).Where("name = ?", "Legacy").Count(&count).Error)
	assert.Zero(t, count)
}
