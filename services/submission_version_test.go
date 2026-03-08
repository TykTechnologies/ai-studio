package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForVersions(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = models.InitModels(db)
	assert.NoError(t, err)
	return db
}

func TestCreateUpdateSubmission(t *testing.T) {
	db := setupTestDBForVersions(t)
	service := NewService(db)

	user := createSubmissionTestUser(t, service, "owner@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	// Create a datasource via a regular submission + approval
	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Original DS", "short_description": "Original",
			"db_source_type": "pgvector", "embed_vendor": "openai",
			"embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "contact@test.com", "", "", nil, "", "",
	)
	approved, err := service.ApproveSubmission(submission.ID, admin.ID, 5, nil, "")
	assert.NoError(t, err)
	dsID := *approved.ResourceID

	t.Run("OwnerCanCreateUpdateSubmission", func(t *testing.T) {
		updateSub, err := service.CreateUpdateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Updated DS", "short_description": "Updated description"},
			nil, 5, "", "contact@test.com", "", "", nil, "", "Updated the description",
			models.SubmissionStatusSubmitted,
		)
		assert.NoError(t, err)
		assert.True(t, updateSub.IsUpdate)
		assert.Equal(t, &dsID, updateSub.TargetResourceID)
	})

	t.Run("NonOwnerCannotCreateUpdateSubmission", func(t *testing.T) {
		otherUser := createSubmissionTestUser(t, service, "other@test.com")
		_, err := service.CreateUpdateSubmission(
			otherUser.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Hijacked"}, nil, 5, "", "", "", "", nil, "", "", "",
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})
}

func TestApproveUpdateSubmission_CreatesVersion(t *testing.T) {
	db := setupTestDBForVersions(t)
	service := NewService(db)

	user := createSubmissionTestUser(t, service, "owner@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	// Create original datasource
	sub, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Original DS", "short_description": "Original desc",
			"db_source_type": "pgvector", "embed_vendor": "openai",
			"embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "contact@test.com", "", "", nil, "", "",
	)
	approved, _ := service.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	dsID := *approved.ResourceID

	// Verify original state
	ds, _ := service.GetDatasourceByID(context.Background(), dsID)
	assert.Equal(t, "Original DS", ds.Name)

	// Submit an update
	updateSub, _ := service.CreateUpdateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, dsID,
		models.JSONMap{"name": "Updated DS", "short_description": "New description"},
		nil, 6, "", "contact@test.com", "", "", nil, "", "Changed name and desc",
		models.SubmissionStatusSubmitted,
	)

	// Approve the update
	approvedUpdate, err := service.ApproveSubmission(updateSub.ID, admin.ID, 6, nil, "Update looks good")
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approvedUpdate.Status)

	// Verify datasource was updated
	updatedDS, _ := service.GetDatasourceByID(context.Background(), dsID)
	assert.Equal(t, "Updated DS", updatedDS.Name)
	assert.Equal(t, "New description", updatedDS.ShortDescription)
	assert.Equal(t, 6, updatedDS.PrivacyScore)

	// Verify a version snapshot was created
	versions, err := service.GetResourceVersions(models.SubmissionResourceTypeDatasource, dsID)
	assert.NoError(t, err)
	assert.Len(t, versions, 1)
	assert.Equal(t, 1, versions[0].VersionNumber)
	assert.Equal(t, "Original DS", versions[0].Payload["name"])
}

func TestApproveUpdateSubmission_Tool(t *testing.T) {
	db := setupTestDBForVersions(t)
	service := NewService(db)

	user := createSubmissionTestUser(t, service, "owner@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	// Create original tool
	sub, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Weather API", "description": "Get weather",
			"tool_type": "REST", "oas_spec": "dGVzdA==",
		}, nil, 2, "", "contact@test.com", "", "", nil, "", "",
	)
	approved, _ := service.ApproveSubmission(sub.ID, admin.ID, 2, nil, "")
	toolID := *approved.ResourceID

	// Submit an update
	updateSub, _ := service.CreateUpdateSubmission(
		user.ID, models.SubmissionResourceTypeTool, toolID,
		models.JSONMap{"name": "Weather API v2", "description": "Updated weather data"},
		nil, 3, "", "contact@test.com", "", "", nil, "", "v2 update",
		models.SubmissionStatusSubmitted,
	)

	// Approve the update
	_, err := service.ApproveSubmission(updateSub.ID, admin.ID, 2, nil, "")
	assert.NoError(t, err)

	// Verify tool was updated
	tool, _ := service.GetToolByID(context.Background(), toolID)
	assert.Equal(t, "Weather API v2", tool.Name)

	// Verify version snapshot
	versions, _ := service.GetResourceVersions(models.SubmissionResourceTypeTool, toolID)
	assert.Len(t, versions, 1)
	assert.Equal(t, "Weather API", versions[0].Payload["name"])
}

func TestRollbackResource(t *testing.T) {
	db := setupTestDBForVersions(t)
	service := NewService(db)

	user := createSubmissionTestUser(t, service, "owner@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	// Create and approve original
	sub, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Original DS", "short_description": "Original desc",
			"db_source_type": "pgvector", "embed_vendor": "openai",
			"embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "contact@test.com", "", "", nil, "", "",
	)
	approved, _ := service.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	dsID := *approved.ResourceID

	// Submit and approve an update
	updateSub, _ := service.CreateUpdateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, dsID,
		models.JSONMap{"name": "Updated DS"},
		nil, 6, "", "contact@test.com", "", "", nil, "", "",
		models.SubmissionStatusSubmitted,
	)
	service.ApproveSubmission(updateSub.ID, admin.ID, 6, nil, "")

	// Verify updated state
	ds, _ := service.GetDatasourceByID(context.Background(), dsID)
	assert.Equal(t, "Updated DS", ds.Name)

	// Get the version to rollback to
	versions, _ := service.GetResourceVersions(models.SubmissionResourceTypeDatasource, dsID)
	assert.Len(t, versions, 1)

	// Rollback
	err := service.RollbackResource(models.SubmissionResourceTypeDatasource, dsID, versions[0].ID, admin.ID)
	assert.NoError(t, err)

	// Verify rolled back
	rolledBack, _ := service.GetDatasourceByID(context.Background(), dsID)
	assert.Equal(t, "Original DS", rolledBack.Name)

	// Verify a pre-rollback snapshot was also created (rollback is reversible)
	allVersions, _ := service.GetResourceVersions(models.SubmissionResourceTypeDatasource, dsID)
	assert.Len(t, allVersions, 2)
	assert.Equal(t, 2, allVersions[0].VersionNumber) // newest first
	assert.Equal(t, "Updated DS", allVersions[0].Payload["name"])
}

func TestRollbackResource_WrongResource(t *testing.T) {
	db := setupTestDBForVersions(t)
	service := NewService(db)

	user := createSubmissionTestUser(t, service, "owner@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	// Create two datasources
	sub1, _ := service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "DS1", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
		nil, 5, "", "", "", "", nil, "", "")
	approved1, _ := service.ApproveSubmission(sub1.ID, admin.ID, 5, nil, "")
	ds1ID := *approved1.ResourceID

	sub2, _ := service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "DS2", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
		nil, 5, "", "", "", "", nil, "", "")
	approved2, _ := service.ApproveSubmission(sub2.ID, admin.ID, 5, nil, "")
	ds2ID := *approved2.ResourceID

	// Update DS1 to create a version
	updateSub, _ := service.CreateUpdateSubmission(user.ID, models.SubmissionResourceTypeDatasource, ds1ID,
		models.JSONMap{"name": "Updated DS1"}, nil, 5, "", "", "", "", nil, "", "", models.SubmissionStatusSubmitted)
	service.ApproveSubmission(updateSub.ID, admin.ID, 5, nil, "")

	versions, _ := service.GetResourceVersions(models.SubmissionResourceTypeDatasource, ds1ID)

	// Try to rollback DS2 using DS1's version — should fail
	err := service.RollbackResource(models.SubmissionResourceTypeDatasource, ds2ID, versions[0].ID, admin.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong")
}

func TestMultipleUpdates_IncrementingVersions(t *testing.T) {
	db := setupTestDBForVersions(t)
	service := NewService(db)

	user := createSubmissionTestUser(t, service, "owner@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	// Create original
	sub, _ := service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "v1", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
		nil, 5, "", "", "", "", nil, "", "")
	approved, _ := service.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	dsID := *approved.ResourceID

	// Three updates
	for _, name := range []string{"v2", "v3", "v4"} {
		updateSub, _ := service.CreateUpdateSubmission(user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": name}, nil, 5, "", "", "", "", nil, "", "", models.SubmissionStatusSubmitted)
		service.ApproveSubmission(updateSub.ID, admin.ID, 5, nil, "")
	}

	// Should have 3 version snapshots
	versions, _ := service.GetResourceVersions(models.SubmissionResourceTypeDatasource, dsID)
	assert.Len(t, versions, 3)
	assert.Equal(t, 3, versions[0].VersionNumber)
	assert.Equal(t, 2, versions[1].VersionNumber)
	assert.Equal(t, 1, versions[2].VersionNumber)

	// Latest state should be v4
	ds, _ := service.GetDatasourceByID(context.Background(), dsID)
	assert.Equal(t, "v4", ds.Name)

	// Version payloads should capture the state before each update
	assert.Equal(t, "v3", versions[0].Payload["name"]) // before v4 was applied
	assert.Equal(t, "v2", versions[1].Payload["name"]) // before v3 was applied
	assert.Equal(t, "v1", versions[2].Payload["name"]) // before v2 was applied
}
