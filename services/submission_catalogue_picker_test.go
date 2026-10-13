package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Approve dialog's catalogue picker (#504) sends
// assigned_catalogues: {"ids": [...]}. An approved data source joins the
// chosen data catalogues and none of the others the reviewer left out.
// (Community Edition also adds every new data source to Default, by policy;
// that catalogue is not part of this assertion.)
func TestApproveSubmission_DatasourceJoinsOnlyTheChosenCatalogues(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	submitter := createSubmissionTestUser(t, service, "picker-contributor@test.com")
	reviewer := createSubmissionTestAdmin(t, service, "picker-reviewer@test.com")

	chosen, err := service.CreateDataCatalogue("Community data", "", "", "")
	require.NoError(t, err)
	alsoChosen, err := service.CreateDataCatalogue("Research data", "", "", "")
	require.NoError(t, err)
	leftOut, err := service.CreateDataCatalogue("Finance data", "", "", "")
	require.NoError(t, err)

	submission, err := service.CreateSubmission(
		submitter.ID,
		models.SubmissionResourceTypeDatasource,
		models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Vectors", "db_source_type": "pgvector"},
		nil, 0, "public data",
		"picker-contributor@test.com", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// The exact JSON shape the dialog posts, after a round trip through
	// JSON (numbers arrive as float64).
	approved, err := service.ApproveSubmission(
		submission.ID, reviewer.ID, 10,
		models.JSONMap{"ids": []interface{}{float64(chosen.ID), float64(alsoChosen.ID)}},
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, approved.ResourceID)

	member := func(catalogueID uint) int64 {
		var n int64
		require.NoError(t, db.Table("data_catalogue_data_sources").
			Where("data_catalogue_id = ? AND datasource_id = ?", catalogueID, *approved.ResourceID).
			Count(&n).Error)
		return n
	}
	assert.EqualValues(t, 1, member(chosen.ID))
	assert.EqualValues(t, 1, member(alsoChosen.ID))
	assert.EqualValues(t, 0, member(leftOut.ID), "a catalogue the reviewer did not pick must not get the resource")
}
