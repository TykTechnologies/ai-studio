package services

import (
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

// The assignment used to fetch each catalogue and append to it one id at a
// time -- 2N queries inside the approval transaction, which holds locks for
// as long as it runs. These pin the two behaviours a bulk rewrite can quietly
// lose: every chosen catalogue still gets the resource, and a catalogue id
// that does not exist is still an error rather than a silent omission.

func TestApproveSubmission_AssignsEveryChosenCatalogue(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "contributor5@test.com")
	reviewer := createSubmissionTestAdmin(t, service, "reviewer5@test.com")

	first, err := service.CreateToolCatalogue("Community tools", "", "", "")
	assert.NoError(t, err)
	second, err := service.CreateToolCatalogue("Partner tools", "", "", "")
	assert.NoError(t, err)
	third, err := service.CreateToolCatalogue("Internal tools", "", "", "")
	assert.NoError(t, err)

	submission, err := service.CreateSubmission(
		submitter.ID,
		models.SubmissionResourceTypeTool,
		models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Weather", "tool_type": "REST"},
		nil, 0, "public data",
		"contributor5@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	approved, err := service.ApproveSubmission(
		submission.ID, reviewer.ID, 0,
		models.JSONMap{"ids": []interface{}{
			float64(first.ID), float64(second.ID), float64(third.ID),
		}},
		"looks fine",
	)
	require.NoError(t, err)
	require.NotNil(t, approved.ResourceID)

	for _, catalogue := range []uint{first.ID, second.ID, third.ID} {
		var count int64
		err = db.Table("tool_catalogue_tools").
			Where("tool_catalogue_id = ? AND tool_id = ?", catalogue, *approved.ResourceID).
			Count(&count).Error
		assert.NoError(t, err)
		assert.EqualValues(t, 1, count,
			"tool must be in catalogue %d, not only the first one", catalogue)
	}
}

func TestApproveSubmission_UnknownCatalogueIsAnError(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "contributor6@test.com")
	reviewer := createSubmissionTestAdmin(t, service, "reviewer6@test.com")

	catalogue, err := service.CreateToolCatalogue("Community tools", "", "", "")
	assert.NoError(t, err)

	submission, err := service.CreateSubmission(
		submitter.ID,
		models.SubmissionResourceTypeTool,
		models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Weather", "tool_type": "REST"},
		nil, 0, "public data",
		"contributor6@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	// A real catalogue alongside one that does not exist: the missing id must
	// fail the approval rather than being dropped and the rest published.
	_, err = service.ApproveSubmission(
		submission.ID, reviewer.ID, 0,
		models.JSONMap{"ids": []interface{}{float64(catalogue.ID), float64(4242)}},
		"looks fine",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "4242", "the error names the catalogue that is missing")
}

func TestAssignSubmissionCatalogues_QueryCount(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "contributor7@test.com")

	ids := make([]interface{}, 0, 8)
	for i := 0; i < 8; i++ {
		catalogue, err := service.CreateToolCatalogue(fmt.Sprintf("Catalogue %d", i), "", "", "")
		require.NoError(t, err)
		ids = append(ids, float64(catalogue.ID))
	}

	tool := &models.Tool{Name: "Weather", ToolType: "REST"}
	require.NoError(t, db.Create(tool).Error)

	submission := &models.Submission{
		SubmitterID:        submitter.ID,
		ResourceType:       models.SubmissionResourceTypeTool,
		Status:             models.SubmissionStatusSubmitted,
		AssignedCatalogues: models.JSONMap{"ids": ids},
	}

	counter := &QueryCountLogger{}
	counted := db.Session(&gorm.Session{Logger: counter})

	require.NoError(t, service.assignSubmissionCatalogues(counted, submission, tool.ID))

	// One SELECT for the catalogues and one INSERT for the memberships. The
	// per-id version cost two queries per catalogue, so eight catalogues meant
	// sixteen; the point of the pin is that this number does not move with the
	// number of catalogues chosen.
	assert.LessOrEqual(t, counter.QueryCount, 4,
		"8 catalogues must not mean 16 queries: %v", counter.Queries)
}
