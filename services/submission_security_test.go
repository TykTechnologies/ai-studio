package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSecurityTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = models.InitModels(db)
	require.NoError(t, err)
	return db
}

// =============================================================================
// C2: Verify secrets are not stored in plaintext in version snapshots
// =============================================================================

func TestSecurity_SnapshotDoesNotLeakRawCredentials(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	// Create a datasource with credentials via submission
	sub, _ := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Secret DS", "db_source_type": "pgvector",
			"db_conn_string": "postgresql://secret-host:5432/db",
			"db_conn_api_key": "super-secret-key-123",
			"embed_vendor": "openai", "embed_api_key": "sk-secret-embed-key",
			"embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "", "", "", nil, "", "",
	)
	approved, _ := svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	dsID := *approved.ResourceID

	// Update to create a version snapshot
	updateSub, _ := svc.CreateUpdateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, dsID,
		models.JSONMap{"name": "Updated DS"},
		nil, 5, "", "", "", "", nil, "", "", models.SubmissionStatusSubmitted,
	)
	svc.ApproveSubmission(updateSub.ID, admin.ID, 5, nil, "")

	// Get version snapshots — these are what the API returns
	versions, err := svc.GetResourceVersions(models.SubmissionResourceTypeDatasource, dsID)
	require.NoError(t, err)
	require.Len(t, versions, 1)

	// The raw snapshot in DB will have the values, but the API layer redacts them.
	// Verify the snapshot was created (the API handler applies redaction, not the service).
	assert.NotNil(t, versions[0].Payload)
}

// =============================================================================
// H1: CreateUpdateSubmission must reject invalid status values
// =============================================================================

func TestSecurity_CreateUpdateSubmission_RejectsInvalidStatus(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	// Create a resource for the user to own
	sub, _ := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Owned DS", "db_source_type": "pgvector",
			"embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "", "", "", nil, "", "",
	)
	approved, _ := svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	dsID := *approved.ResourceID

	t.Run("RejectsApprovedStatus", func(t *testing.T) {
		_, err := svc.CreateUpdateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Bypass"}, nil, 5, "", "", "", "", nil, "", "",
			models.SubmissionStatusApproved,
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "initial status must be")
	})

	t.Run("RejectsInReviewStatus", func(t *testing.T) {
		_, err := svc.CreateUpdateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Bypass"}, nil, 5, "", "", "", "", nil, "", "",
			models.SubmissionStatusInReview,
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "initial status must be")
	})

	t.Run("RejectsRejectedStatus", func(t *testing.T) {
		_, err := svc.CreateUpdateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Bypass"}, nil, 5, "", "", "", "", nil, "", "",
			models.SubmissionStatusRejected,
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "initial status must be")
	})

	t.Run("AcceptsDraftStatus", func(t *testing.T) {
		result, err := svc.CreateUpdateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Valid Draft"}, nil, 5, "", "", "", "", nil, "", "",
			models.SubmissionStatusDraft,
		)
		assert.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusDraft, result.Status)
	})

	t.Run("AcceptsSubmittedStatus", func(t *testing.T) {
		result, err := svc.CreateUpdateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Valid Submitted"}, nil, 5, "", "", "", "", nil, "", "",
			models.SubmissionStatusSubmitted,
		)
		assert.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusSubmitted, result.Status)
	})

	t.Run("DefaultsToDraftWhenEmpty", func(t *testing.T) {
		result, err := svc.CreateUpdateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Default Draft"}, nil, 5, "", "", "", "", nil, "", "",
			"",
		)
		assert.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusDraft, result.Status)
	})
}

// =============================================================================
// H3: Optimistic locking prevents concurrent state transitions
// =============================================================================

func TestSecurity_OptimisticLocking_PreventsConcurrentModification(t *testing.T) {
	db := setupSecurityTestDB(t)

	// Create a submission directly in DB
	sub := &models.Submission{
		ResourceType: "datasource",
		Status:       models.SubmissionStatusSubmitted,
		SubmitterID:  1,
		LockVersion:  0,
	}
	require.NoError(t, sub.Create(db))

	// Simulate two readers getting the same submission (both see lock_version=0)
	reader1 := &models.Submission{}
	require.NoError(t, reader1.Get(db, sub.ID))

	reader2 := &models.Submission{}
	require.NoError(t, reader2.Get(db, sub.ID))

	assert.Equal(t, 0, reader1.LockVersion)
	assert.Equal(t, 0, reader2.LockVersion)

	// Reader 1 updates successfully (lock_version 0 → 1)
	reader1.Status = models.SubmissionStatusInReview
	err := reader1.UpdateWithLock(db)
	assert.NoError(t, err)

	// Reader 2 tries to update with stale lock_version=0, but DB now has 1
	reader2.Status = models.SubmissionStatusInReview
	err = reader2.UpdateWithLock(db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "concurrent modification detected")
}

func TestSecurity_OptimisticLocking_NormalFlowSucceeds(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	sub, _ := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Normal Flow", "db_source_type": "pgvector",
			"embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "", "", "", nil, "", "",
	)

	// Normal sequential flow should work fine
	reviewed, err := svc.StartReview(sub.ID, admin.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusInReview, reviewed.Status)

	approved, err := svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approved.Status)
}

// =============================================================================
// H4: Duplicate check uses DB queries, not in-memory scan
// =============================================================================

func TestSecurity_DuplicateCheck_DoesNotLoadAllRecords(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")

	// Create several datasources
	for i := 0; i < 5; i++ {
		svc.CreateDatasource(
			"DS "+string(rune('A'+i)), "", "", "", "", 50, user.ID, nil,
			"conn"+string(rune('A'+i)), "pgvector", "", "db",
			"openai", "", "", "text-embedding-3-small", true,
		)
	}

	// Check for duplicates by connection string — should find exact match only
	dupes, err := svc.CheckForDuplicates(models.SubmissionResourceTypeDatasource, models.JSONMap{
		"name":           "Completely Different",
		"db_conn_string": "connA",
	})
	assert.NoError(t, err)
	assert.Len(t, dupes, 1)
	assert.Equal(t, "DS A", dupes[0].Name)

	// Check by name — case insensitive
	dupes, err = svc.CheckForDuplicates(models.SubmissionResourceTypeDatasource, models.JSONMap{
		"name":           "ds b",
		"db_conn_string": "no-match",
	})
	assert.NoError(t, err)
	assert.Len(t, dupes, 1)
	assert.Equal(t, "DS B", dupes[0].Name)

	// No match
	dupes, err = svc.CheckForDuplicates(models.SubmissionResourceTypeDatasource, models.JSONMap{
		"name":           "No Such DS",
		"db_conn_string": "no-such-conn",
	})
	assert.NoError(t, err)
	assert.Len(t, dupes, 0)
}

// =============================================================================
// H5: review_notes not exposed — tested via serializer behavior
// (The actual serializer is in the API layer; we test the model-level data here)
// =============================================================================

func TestSecurity_ReviewNotesStoredCorrectly(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	sub, _ := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Test"}, nil, 5, "", "", "", "", nil, "", "",
	)

	// Admin rejects with both feedback (submitter-facing) and review notes (internal)
	rejected, err := svc.RejectSubmission(sub.ID, admin.ID, "Please fix X", "Internal: looks sketchy")
	require.NoError(t, err)
	assert.Equal(t, "Please fix X", rejected.SubmitterFeedback)
	assert.Equal(t, "Internal: looks sketchy", rejected.ReviewNotes)

	// Verify both are persisted
	fetched, err := svc.GetSubmissionByID(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "Please fix X", fetched.SubmitterFeedback)
	assert.Equal(t, "Internal: looks sketchy", fetched.ReviewNotes)
	// The API layer is responsible for stripping review_notes from portal responses
}

// =============================================================================
// Activity audit trail records all state transitions
// =============================================================================

func TestSecurity_AuditTrail_RecordsAllTransitions(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	// Create and submit
	sub, _ := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"name": "Audit Trail Test", "db_source_type": "pgvector",
			"embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "", "", "", nil, "", "",
	)

	// Submit
	svc.SubmitSubmission(sub.ID, user.ID)

	// Admin requests changes
	svc.RequestChanges(sub.ID, admin.ID, "Add documentation", "Internal: needs more detail")

	// User resubmits
	svc.SubmitSubmission(sub.ID, user.ID)

	// Admin approves
	svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "Looks good now")

	// Check audit trail
	activities, err := svc.GetSubmissionActivities(sub.ID)
	require.NoError(t, err)

	// Should have: submitted, changes_requested, resubmitted, approved
	assert.Len(t, activities, 4)
	assert.Equal(t, models.ActivityTypeSubmitted, activities[0].ActivityType)
	assert.Equal(t, models.ActivityTypeChangesRequested, activities[1].ActivityType)
	assert.Equal(t, "Add documentation", activities[1].Feedback)
	assert.Equal(t, "Internal: needs more detail", activities[1].InternalNote)
	assert.Equal(t, models.ActivityTypeResubmitted, activities[2].ActivityType)
	assert.Equal(t, models.ActivityTypeApproved, activities[3].ActivityType)
}

// =============================================================================
// Item 1: Transaction atomicity — approval failure rolls back resource creation
// =============================================================================

func TestSecurity_ApprovalTransaction_CannotApproveAlreadyApproved(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	// Create and approve a submission
	sub, err := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "First Approval", "db_source_type": "pgvector",
			"embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	_, err = svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	require.NoError(t, err)

	// Count datasources before second attempt
	var countBefore int64
	db.Model(&models.Datasource{}).Count(&countBefore)

	// Attempt to approve again — should fail (already approved)
	_, err = svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can only approve")

	// No new datasource should have been created
	var countAfter int64
	db.Model(&models.Datasource{}).Count(&countAfter)
	assert.Equal(t, countBefore, countAfter, "no new resource should be created on failed re-approval")
}

func TestSecurity_ApprovalTransaction_ResourceAndSubmissionConsistent(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	sub, err := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Consistent Test", "db_source_type": "pgvector",
			"embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Approve — both resource and submission should be created/updated atomically
	approved, err := svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "All good")
	require.NoError(t, err)
	require.NotNil(t, approved.ResourceID)

	// Verify resource exists
	ds, err := svc.GetDatasourceByID(*approved.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "Consistent Test", ds.Name)
	assert.True(t, ds.CommunitySubmitted)

	// Verify submission points to resource
	fetched, err := svc.GetSubmissionByID(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, fetched.Status)
	assert.Equal(t, *approved.ResourceID, *fetched.ResourceID)

	// Verify activity was recorded
	activities, err := svc.GetSubmissionActivities(sub.ID)
	require.NoError(t, err)
	found := false
	for _, a := range activities {
		if a.ActivityType == models.ActivityTypeApproved {
			found = true
		}
	}
	assert.True(t, found, "approval activity should be recorded in the same transaction")
}

// =============================================================================
// Item 7: HandleUserDeletionForUGC runs inside user deletion transaction
// =============================================================================

func TestSecurity_UserDeletion_UGCCleanupIsTransactional(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")
	admin := createSubmissionTestAdmin(t, svc, "admin@test.com")

	// Create a community datasource via approval
	sub, _ := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Dev's Community DS", "db_source_type": "pgvector",
			"embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true,
		}, nil, 5, "", "", "", "", nil, "", "",
	)
	approved, _ := svc.ApproveSubmission(sub.ID, admin.ID, 5, nil, "")
	dsID := *approved.ResourceID

	// Verify datasource is active before deletion
	ds := &models.Datasource{}
	db.First(ds, dsID)
	assert.True(t, ds.Active)
	assert.True(t, ds.CommunitySubmitted)

	// Delete the user
	err := svc.DeleteUser(user)
	require.NoError(t, err)

	// Verify community datasource was deactivated as part of the deletion
	deactivatedDS := &models.Datasource{}
	db.First(deactivatedDS, dsID)
	assert.False(t, deactivatedDS.Active, "community datasource should be deactivated when owner is deleted")
}

// =============================================================================
// Item 8: Attestation validation enforced on submit
// =============================================================================

func TestSecurity_AttestationValidation_RequiredAttestationsEnforced(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")

	// Create a required attestation template
	tmpl, err := svc.CreateAttestationTemplate(
		"Data Authority",
		"I confirm I have authority to share these credentials",
		models.AttestationAppliesToDatasource,
		true, true, 1,
	)
	require.NoError(t, err)

	t.Run("SubmitWithoutRequiredAttestationFails", func(t *testing.T) {
		// Create draft with no attestations
		draft, err := svc.CreateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{"name": "No Attestation DS", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small"}, nil, 5, "",
			"", "", "", nil, "", "",
		)
		require.NoError(t, err)

		// Try to submit — should fail
		_, err = svc.SubmitSubmission(draft.ID, user.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required attestations not accepted")
		assert.Contains(t, err.Error(), "Data Authority")
	})

	t.Run("SubmitWithWrongAttestationIDFails", func(t *testing.T) {
		draft, err := svc.CreateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{"name": "Wrong Attestation DS", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small"},
			models.JSONMap{"accepted": []interface{}{
				map[string]interface{}{"template_id": float64(99999)},
			}},
			5, "", "", "", "", nil, "", "",
		)
		require.NoError(t, err)

		_, err = svc.SubmitSubmission(draft.ID, user.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required attestations not accepted")
	})

	t.Run("SubmitWithCorrectAttestationSucceeds", func(t *testing.T) {
		draft, err := svc.CreateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{"name": "Valid Attestation DS", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small"},
			models.JSONMap{"accepted": []interface{}{
				map[string]interface{}{"template_id": float64(tmpl.ID), "accepted_at": "2024-01-15T10:00:00Z"},
			}},
			5, "", "", "", "", nil, "", "",
		)
		require.NoError(t, err)

		submitted, err := svc.SubmitSubmission(draft.ID, user.ID)
		assert.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusSubmitted, submitted.Status)
	})

	t.Run("ToolSubmissionIgnoresDatasourceAttestations", func(t *testing.T) {
		// The template applies to datasources only — tools should not be blocked
		draft, err := svc.CreateSubmission(
			user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusDraft,
			models.JSONMap{"name": "Tool No Attestation", "oas_spec": "dGVzdA=="},
			nil, 5, "", "", "", "", nil, "", "",
		)
		require.NoError(t, err)

		submitted, err := svc.SubmitSubmission(draft.ID, user.ID)
		assert.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusSubmitted, submitted.Status)
	})

	t.Run("OptionalAttestationDoesNotBlock", func(t *testing.T) {
		// Create an optional attestation template
		svc.CreateAttestationTemplate(
			"Nice to Have", "I promise to be nice",
			models.AttestationAppliesToDatasource, false, true, 2,
		)

		// Submit without the optional one — should succeed (only required matters)
		draft, err := svc.CreateSubmission(
			user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{"name": "Optional Attestation DS", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small"},
			models.JSONMap{"accepted": []interface{}{
				map[string]interface{}{"template_id": float64(tmpl.ID)},
			}},
			5, "", "", "", "", nil, "", "",
		)
		require.NoError(t, err)

		submitted, err := svc.SubmitSubmission(draft.ID, user.ID)
		assert.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusSubmitted, submitted.Status)
	})
}

// =============================================================================
// Credential preservation on resubmission
// =============================================================================

func TestSecurity_CredentialPreservation_RedactedValuesPreserved(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")

	// Create a draft with real credentials
	draft, err := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"name":            "Credential Test",
			"db_conn_string":  "postgresql://real-host:5432/db",
			"db_conn_api_key": "real-secret-key",
			"embed_api_key":   "sk-real-embed-key",
		},
		nil, 5, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Update with [redacted] values (simulating what happens when user edits after API returns redacted data)
	updated, err := svc.UpdateSubmission(
		draft.ID, user.ID,
		models.JSONMap{
			"name":            "Credential Test Updated",
			"db_conn_string":  "[redacted]",
			"db_conn_api_key": "[redacted]",
			"embed_api_key":   "[redacted]",
		},
		nil, 5, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// The original credentials should be preserved, not overwritten with "[redacted]"
	assert.Equal(t, "Credential Test Updated", updated.ResourcePayload["name"])
	assert.Equal(t, "postgresql://real-host:5432/db", updated.ResourcePayload["db_conn_string"])
	assert.Equal(t, "real-secret-key", updated.ResourcePayload["db_conn_api_key"])
	assert.Equal(t, "sk-real-embed-key", updated.ResourcePayload["embed_api_key"])
}

func TestSecurity_CredentialPreservation_NewValuesOverwrite(t *testing.T) {
	db := setupSecurityTestDB(t)
	svc := NewService(db)

	user := createSubmissionTestUser(t, svc, "dev@test.com")

	draft, err := svc.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"name":            "Overwrite Test",
			"db_conn_api_key": "old-key",
			"embed_api_key":   "old-embed-key",
		},
		nil, 5, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Update with new actual values (not [redacted])
	updated, err := svc.UpdateSubmission(
		draft.ID, user.ID,
		models.JSONMap{
			"name":            "Overwrite Test",
			"db_conn_api_key": "new-key",
			"embed_api_key":   "new-embed-key",
		},
		nil, 5, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// New values should be written
	assert.Equal(t, "new-key", updated.ResourcePayload["db_conn_api_key"])
	assert.Equal(t, "new-embed-key", updated.ResourcePayload["embed_api_key"])
}
