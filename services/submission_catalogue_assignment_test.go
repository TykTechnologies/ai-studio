package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

// assigned_catalogues was accepted on approve, stored on the submission and
// serialised back -- and never read. A reviewer who chose catalogues got a
// resource that was in none of them, and the portal showed nothing until an
// administrator edited a catalogue by hand.

func TestCatalogueIDsFrom(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Empty(t, catalogueIDsFrom(nil))
	})

	t.Run("bare array of ids, as JSON round-trips them", func(t *testing.T) {
		// A JSONMap that has been through JSON carries numbers as float64.
		assigned := models.JSONMap{"ids": []interface{}{float64(2), float64(5)}}
		assert.ElementsMatch(t, []uint{2, 5}, catalogueIDsFrom(assigned))
	})

	t.Run("scalar value", func(t *testing.T) {
		assert.Equal(t, []uint{7}, catalogueIDsFrom(models.JSONMap{"catalogue": float64(7)}))
	})

	t.Run("ignores zero and non-numeric entries", func(t *testing.T) {
		assigned := models.JSONMap{"ids": []interface{}{float64(0), "nope", nil, float64(3)}}
		assert.Equal(t, []uint{3}, catalogueIDsFrom(assigned))
	})
}

func TestApproveSubmission_AssignsChosenCatalogues(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "contributor@test.com")
	reviewer := createSubmissionTestAdmin(t, service, "reviewer@test.com")

	catalogue, err := service.CreateToolCatalogue("Community tools", "", "", "")
	assert.NoError(t, err)

	submission, err := service.CreateSubmission(
		submitter.ID,
		models.SubmissionResourceTypeTool,
		models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name":        "Weather",
			"tool_type":   "REST",
			"description": "Weather lookup",
		},
		nil, 0, "public data",
		"contributor@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	approved, err := service.ApproveSubmission(
		submission.ID, reviewer.ID, 0,
		models.JSONMap{"ids": []interface{}{float64(catalogue.ID)}},
		"looks fine",
	)
	assert.NoError(t, err)
	assert.NotNil(t, approved.ResourceID)

	// The resource must actually be in the catalogue, not merely recorded as
	// having been assigned to it.
	var count int64
	err = db.Table("tool_catalogue_tools").
		Where("tool_catalogue_id = ? AND tool_id = ?", catalogue.ID, *approved.ResourceID).
		Count(&count).Error
	assert.NoError(t, err)
	assert.EqualValues(t, 1, count, "approved tool must be in the catalogue the reviewer chose")
}

func TestApproveSubmission_NoCataloguesIsStillAllowed(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "contributor2@test.com")
	reviewer := createSubmissionTestAdmin(t, service, "reviewer2@test.com")

	submission, err := service.CreateSubmission(
		submitter.ID,
		models.SubmissionResourceTypeTool,
		models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Unpublished", "tool_type": "REST"},
		nil, 0, "public data",
		"contributor2@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	// Approving without choosing catalogues is the deliberate two-step grant
	// and must keep working.
	approved, err := service.ApproveSubmission(submission.ID, reviewer.ID, 0, nil, "")
	assert.NoError(t, err)
	assert.NotNil(t, approved.ResourceID)
}


// Approval publishes into Default and leaves the resource inactive: publishing
// and going live are separate acts. Before this, an approved tool was created
// active and in no catalogue at all, while a hand-created data source
// auto-joined Default -- the platform published the sensitive thing
// automatically and withheld the reviewed one.
func TestApproveSubmission_PublishesToDefaultInactive(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "contributor3@test.com")
	reviewer := createSubmissionTestAdmin(t, service, "reviewer3@test.com")

	submission, err := service.CreateSubmission(
		submitter.ID,
		models.SubmissionResourceTypeTool,
		models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Weather", "tool_type": "REST", "active": true},
		nil, 0, "public data",
		"contributor3@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	approved, err := service.ApproveSubmission(submission.ID, reviewer.ID, 0, nil, "")
	assert.NoError(t, err)
	assert.NotNil(t, approved.ResourceID)

	tool, err := service.GetToolByID(*approved.ResourceID)
	assert.NoError(t, err)
	assert.False(t, tool.Active,
		"an approved tool is published but not live until an administrator activates it")

	defaultCatalogue, err := models.GetOrCreateDefaultToolCatalogue(db)
	assert.NoError(t, err)

	var count int64
	err = db.Table("tool_catalogue_tools").
		Where("tool_catalogue_id = ? AND tool_id = ?", defaultCatalogue.ID, tool.ID).
		Count(&count).Error
	assert.NoError(t, err)
	assert.EqualValues(t, 1, count,
		"an approved tool joins the Default catalogue, the way a datasource does")
}

func TestApproveSubmission_DatasourceIsInactiveRegardlessOfPayload(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "contributor4@test.com")
	reviewer := createSubmissionTestAdmin(t, service, "reviewer4@test.com")

	submission, err := service.CreateSubmission(
		submitter.ID,
		models.SubmissionResourceTypeDatasource,
		models.SubmissionStatusSubmitted,
		// A contributor asking for active must not decide this.
		models.JSONMap{"name": "Vectors", "db_source_type": "pgvector", "active": true},
		nil, 0, "public data",
		"contributor4@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	approved, err := service.ApproveSubmission(submission.ID, reviewer.ID, 0, nil, "")
	assert.NoError(t, err)

	ds, err := service.GetDatasourceByID(*approved.ResourceID)
	assert.NoError(t, err)
	assert.False(t, ds.Active,
		"approving a contribution is not the same as putting it live")
}
