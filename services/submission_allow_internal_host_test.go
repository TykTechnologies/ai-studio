package services

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// w4e: portal submissions never carry the "Allow internal host" switch:
// creating, updating and approving refuse a payload that names it, approval
// creates the resource with it off, and an update submission never applies
// it.
func TestSubmissionsNeverCarryAllowInternalHost(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "contrib@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")
	spec := base64.StdEncoding.EncodeToString([]byte(validOASSpec))

	for _, v := range []any{true, false} {
		_, err := service.CreateSubmission(user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted,
			models.JSONMap{"name": "T", "tool_type": "REST", "oas_spec": spec, "allow_internal_host": v}, nil, 2, "", "c@test.com", "", "", nil, "", "")
		assert.True(t, errors.Is(err, ErrSubmissionAllowInternalHost), "got %v", err)
	}
	_, err := service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "D", "embed_allow_internal_host": true}, nil, 2, "", "c@test.com", "", "", nil, "", "")
	assert.True(t, errors.Is(err, ErrSubmissionAllowInternalHost), "got %v", err)

	sub, err := service.CreateSubmission(user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusDraft,
		models.JSONMap{"name": "Plain", "tool_type": "REST", "oas_spec": spec}, nil, 2, "", "c@test.com", "", "", nil, "", "")
	require.NoError(t, err)
	_, err = service.UpdateSubmission(sub.ID, user.ID,
		models.JSONMap{"name": "Plain", "tool_type": "REST", "oas_spec": spec, "allow_internal_host": true}, nil, 2, "", "c@test.com", "", "", nil, "", "")
	assert.True(t, errors.Is(err, ErrSubmissionAllowInternalHost), "got %v", err)

	// One stored before the check cannot be approved.
	legacy := &models.Submission{SubmitterID: user.ID, ResourceType: models.SubmissionResourceTypeTool,
		Status: models.SubmissionStatusSubmitted,
		ResourcePayload: models.JSONMap{"name": "Legacy", "tool_type": "REST", "oas_spec": spec, "allow_internal_host": true}}
	require.NoError(t, db.Create(legacy).Error)
	_, err = service.ApproveSubmission(legacy.ID, admin.ID, 2, nil, "")
	assert.True(t, errors.Is(err, ErrSubmissionAllowInternalHost), "got %v", err)

	// Applying an update never sets it.
	updates := payloadToUpdatesMap(models.JSONMap{"name": "x", "allow_internal_host": true, "embed_allow_internal_host": true})
	assert.NotContains(t, updates, "allow_internal_host")
	assert.NotContains(t, updates, "embed_allow_internal_host")
}
