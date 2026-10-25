package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

// Every RecordSubmissionActivity call site passes an empty actorName, so before
// ResolveActorName existed the review history rendered "User #1" for every
// entry. These tests pin the resolution so the audit trail keeps naming people.

func TestResolveActorName_UsesUserName(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	admin := createSubmissionTestAdmin(t, service, "alice@test.com")

	assert.Equal(t, "Admin User", service.ResolveActorName(admin.ID, ""))
}

func TestResolveActorName_KeepsCallerSuppliedName(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	admin := createSubmissionTestAdmin(t, service, "alice@test.com")

	// A caller that already knows the name must not be second-guessed.
	assert.Equal(t, "Explicit Name", service.ResolveActorName(admin.ID, "Explicit Name"))
}

func TestResolveActorName_UnknownUserDoesNotFail(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	// A lookup failure must not fail the write; the UI falls back to "User #<id>".
	assert.Equal(t, "", service.ResolveActorName(99999, ""))
	assert.Equal(t, "", service.ResolveActorName(0, ""))
}

func TestRecordSubmissionActivity_StoresActorName(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	submission, err := service.CreateSubmission(
		user.ID,
		models.SubmissionResourceTypeDatasource,
		models.SubmissionStatusDraft,
		models.JSONMap{"name": "My Vector DB", "db_source_type": "pgvector"},
		nil, 5, "Contains product data only",
		"submitter@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	service.RecordSubmissionActivity(
		submission.ID, user.ID, "", models.ActivityTypeSubmitted, "", "",
	)

	activities, err := service.GetSubmissionActivities(submission.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, activities)

	found := false
	for _, a := range activities {
		if a.ActivityType == models.ActivityTypeSubmitted {
			assert.Equal(t, "Test User", a.ActorName, "activity must name the actor, not fall back to User #<id>")
			found = true
		}
	}
	assert.True(t, found, "expected a 'submitted' activity")
}

// The approval transaction holds its connection, so a name lookup on the
// plain DB inside it ran on a second connection: an empty in-memory database
// in tests (the activity was stored with ActorName ""), and a deadlock on a
// single-connection pool. The name is resolved before the transaction.
func TestApproveSubmission_ActivityStoresReviewerName(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	submission, err := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name":           "Community Vector DB",
			"db_source_type": "pgvector",
			"embed_vendor":   "openai",
			"embed_model":    "text-embedding-3-small",
		},
		nil, 5, "Low sensitivity",
		"contact@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)

	_, err = service.ApproveSubmission(submission.ID, admin.ID, 5, models.JSONMap{"catalogue_ids": []int{1}}, "Looks good")
	assert.NoError(t, err)

	activities, err := service.GetSubmissionActivities(submission.ID)
	assert.NoError(t, err)
	found := false
	for _, a := range activities {
		if a.ActivityType == models.ActivityTypeApproved {
			assert.Equal(t, "Admin User", a.ActorName, "the approval must name the reviewer")
			found = true
		}
	}
	assert.True(t, found, "expected an 'approved' activity")
}
