package services

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func submitTool(t *testing.T, service *Service, userID uint, name string) *models.Submission {
	t.Helper()
	submission, err := service.CreateSubmission(
		userID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": name, "description": "d", "tool_type": "REST",
			"oas_spec": base64.StdEncoding.EncodeToString([]byte(validOASSpec)),
		},
		nil, 2, "", "contact@test.com", "", "", nil, "", "",
	)
	require.NoError(t, err)
	return submission
}

func countToolsWithSlug(t *testing.T, service *Service, toolSlug string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, service.DB.Model(&models.Tool{}).Where("slug = ?", toolSlug).Count(&n).Error)
	return n
}

// Approving a community submission refuses a tool name whose slug another
// tool has, for a new tool and for an update that renames one.
func TestSubmissionApprovalRefusesDuplicateToolSlug(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	existing, err := service.CreateTool("Weather API", "d", "REST", base64.StdEncoding.EncodeToString([]byte(validOASSpec)), 2, "", "")
	require.NoError(t, err)

	// A new tool.
	dup := submitTool(t, service, user.ID, "weather-api")
	_, err = service.ApproveSubmission(dup.ID, admin.ID, 2, nil, "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrSlugConflict), "got %v", err)
	assert.Equal(t, int64(1), countToolsWithSlug(t, service, "weather-api"))
	stored, err := service.GetSubmissionByID(dup.ID)
	require.NoError(t, err)
	assert.NotEqual(t, models.SubmissionStatusApproved, stored.Status)

	// An update that renames a community tool onto the slug.
	sub := submitTool(t, service, user.ID, "Forecast")
	approved, err := service.ApproveSubmission(sub.ID, admin.ID, 2, nil, "")
	require.NoError(t, err)
	toolID := *approved.ResourceID

	rename, err := service.CreateUpdateSubmission(user.ID, models.SubmissionResourceTypeTool, toolID,
		models.JSONMap{"name": "WEATHER api"}, nil, 2, "", "contact@test.com", "", "", nil, "", "", models.SubmissionStatusSubmitted)
	require.NoError(t, err)
	_, err = service.ApproveSubmission(rename.ID, admin.ID, 2, nil, "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrSlugConflict), "got %v", err)
	assert.Equal(t, int64(1), countToolsWithSlug(t, service, "weather-api"))

	// A rename to a free name brings its slug along (column updates run no
	// slug hook).
	rename, err = service.CreateUpdateSubmission(user.ID, models.SubmissionResourceTypeTool, toolID,
		models.JSONMap{"name": "Forecast Pro"}, nil, 2, "", "contact@test.com", "", "", nil, "", "", models.SubmissionStatusSubmitted)
	require.NoError(t, err)
	_, err = service.ApproveSubmission(rename.ID, admin.ID, 2, nil, "")
	require.NoError(t, err)
	tool, err := service.GetToolByID(toolID)
	require.NoError(t, err)
	assert.Equal(t, "forecast-pro", tool.Slug)
	assert.NotEqual(t, existing.ID, toolID)
}
