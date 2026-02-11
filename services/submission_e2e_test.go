package services

import (
	"encoding/base64"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupE2ETestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = models.InitModels(db)
	require.NoError(t, err)
	return db
}

func e2eCreateUser(t *testing.T, svc *Service, email string) *models.User {
	user, err := svc.CreateUser(UserDTO{
		Email: email, Name: "Test User " + email, Password: "password123",
		IsAdmin: false, ShowChat: true, ShowPortal: true,
		EmailVerified: true, NotificationsEnabled: false, Groups: []uint{},
	})
	require.NoError(t, err)
	return user
}

func e2eCreateAdmin(t *testing.T, svc *Service, email string) *models.User {
	user, err := svc.CreateUser(UserDTO{
		Email: email, Name: "Admin " + email, Password: "password123",
		IsAdmin: true, ShowChat: true, ShowPortal: true,
		EmailVerified: true, NotificationsEnabled: true,
		AccessToSSOConfig: true, Groups: []uint{},
	})
	require.NoError(t, err)
	return user
}

// TestE2E_DatasourceSubmission_FullLifecycle exercises the complete happy path:
// draft → submit → review → approve → verify resource created → propose update → approve → verify updated → rollback
func TestE2E_DatasourceSubmission_FullLifecycle(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// ---- Step 1: Create attestation templates ----
	t.Log("Step 1: Admin creates attestation templates")
	tmpl, err := svc.CreateAttestationTemplate(
		"Data Authority", "I confirm I have authority to share these credentials",
		models.AttestationAppliesToDatasource, true, true, 1,
	)
	require.NoError(t, err)

	// Verify templates are retrievable by type
	templates, err := svc.GetAttestationTemplatesByType(models.AttestationAppliesToDatasource, true)
	require.NoError(t, err)
	assert.Len(t, templates, 1)
	assert.Equal(t, tmpl.ID, templates[0].ID)

	// ---- Step 2: Developer creates draft submission ----
	t.Log("Step 2: Developer creates draft datasource submission")
	dsPayload := models.JSONMap{
		"name":              "Product Embeddings",
		"short_description": "Vector DB with product review embeddings",
		"long_description":  "Contains 500k product review embeddings using text-embedding-3-small",
		"db_source_type":    "pgvector",
		"db_conn_string":    "postgresql://vectordb.internal:5432/products",
		"db_name":           "product_reviews",
		"embed_vendor":      "openai",
		"embed_url":         "https://api.openai.com/v1",
		"embed_model":       "text-embedding-3-small",
		"tags":              []string{"product", "reviews", "embeddings"},
		"active":            true,
	}

	draft, err := svc.CreateSubmission(
		developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		dsPayload,
		models.JSONMap{"attestations": []interface{}{
			map[string]interface{}{"template_id": tmpl.ID, "accepted": true},
		}},
		50, "Contains only public product review data",
		"dev@company.com", "team-lead@company.com",
		"99.9% uptime during business hours", nil,
		"https://docs.internal.com/product-embeddings", "First version of our product embedding DB",
	)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusDraft, draft.Status)
	assert.False(t, draft.IsUpdate)

	// ---- Step 3: Developer submits for review ----
	t.Log("Step 3: Developer submits draft for review")
	submitted, err := svc.SubmitSubmission(draft.ID, developer.ID)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusSubmitted, submitted.Status)
	assert.NotNil(t, submitted.SubmittedAt)

	// ---- Step 4: Admin sees the submission in their queue ----
	t.Log("Step 4: Admin lists pending submissions")
	submissions, totalCount, _, err := svc.GetAllSubmissions(models.SubmissionStatusSubmitted, "", 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), totalCount)
	assert.Equal(t, draft.ID, submissions[0].ID)

	// Check status counts
	counts, err := svc.GetSubmissionStatusCounts()
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts[models.SubmissionStatusSubmitted])

	// ---- Step 5: Admin starts review ----
	t.Log("Step 5: Admin claims submission for review")
	reviewed, err := svc.StartReview(draft.ID, admin.ID)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusInReview, reviewed.Status)
	assert.Equal(t, &admin.ID, reviewed.ReviewerID)

	// ---- Step 6: Admin approves ----
	t.Log("Step 6: Admin approves submission")
	approved, err := svc.ApproveSubmission(draft.ID, admin.ID, 45, models.JSONMap{"catalogue_ids": []int{1}}, "Good quality data source")
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approved.Status)
	assert.NotNil(t, approved.ResourceID)
	assert.NotNil(t, approved.ReviewCompletedAt)

	// ---- Step 7: Verify the datasource was created correctly ----
	t.Log("Step 7: Verify created datasource")
	ds, err := svc.GetDatasourceByID(*approved.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "Product Embeddings", ds.Name)
	assert.Equal(t, "pgvector", ds.DBSourceType)
	assert.Equal(t, 45, ds.PrivacyScore) // admin-set score
	assert.Equal(t, developer.ID, ds.UserID)
	assert.True(t, ds.CommunitySubmitted)

	// ---- Step 8: Developer proposes an update ----
	t.Log("Step 8: Developer proposes update to published resource")
	updateSub, err := svc.CreateUpdateSubmission(
		developer.ID, models.SubmissionResourceTypeDatasource, ds.ID,
		models.JSONMap{
			"name":              "Product Embeddings v2",
			"short_description": "Updated with 2024 reviews",
		},
		nil, 50, "", "dev@company.com", "", "", nil, "", "Added 2024 review data",
		models.SubmissionStatusSubmitted,
	)
	require.NoError(t, err)
	assert.True(t, updateSub.IsUpdate)
	assert.Equal(t, &ds.ID, updateSub.TargetResourceID)

	// ---- Step 9: Admin approves update → version created ----
	t.Log("Step 9: Admin approves update")
	approvedUpdate, err := svc.ApproveSubmission(updateSub.ID, admin.ID, 45, nil, "Update approved")
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approvedUpdate.Status)

	// Verify datasource was updated
	updatedDS, err := svc.GetDatasourceByID(ds.ID)
	require.NoError(t, err)
	assert.Equal(t, "Product Embeddings v2", updatedDS.Name)
	assert.Equal(t, "Updated with 2024 reviews", updatedDS.ShortDescription)

	// Verify version snapshot was created
	versions, err := svc.GetResourceVersions(models.SubmissionResourceTypeDatasource, ds.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 1)
	assert.Equal(t, "Product Embeddings", versions[0].Payload["name"])

	// ---- Step 10: Admin rolls back ----
	t.Log("Step 10: Admin rolls back to original version")
	err = svc.RollbackResource(models.SubmissionResourceTypeDatasource, ds.ID, versions[0].ID, admin.ID)
	require.NoError(t, err)

	rolledBackDS, err := svc.GetDatasourceByID(ds.ID)
	require.NoError(t, err)
	assert.Equal(t, "Product Embeddings", rolledBackDS.Name)

	// Verify pre-rollback snapshot exists (rollback is reversible)
	allVersions, err := svc.GetResourceVersions(models.SubmissionResourceTypeDatasource, ds.ID)
	require.NoError(t, err)
	assert.Len(t, allVersions, 2)

	// ---- Step 11: Developer can see their submissions ----
	t.Log("Step 11: Developer views their submissions")
	mySubs, count, _, err := svc.GetSubmissionsBySubmitter(developer.ID, "", 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count) // original + update
	assert.Len(t, mySubs, 2)
}

// TestE2E_ToolSubmission_RejectAndResubmit exercises the rejection and resubmission flow:
// submit → reject with feedback → developer revises → resubmit → approve
func TestE2E_ToolSubmission_RejectAndResubmit(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// Valid OpenAPI spec for testing
	validSpec := `
openapi: "3.0.0"
info:
  title: Weather API
  version: "1.0"
servers:
  - url: https://api.weather.example.com/v1
paths:
  /current:
    get:
      operationId: getCurrentWeather
      summary: Get current weather
      responses:
        "200":
          description: OK
  /forecast:
    get:
      operationId: getForecast
      summary: Get weather forecast
      responses:
        "200":
          description: OK
components:
  securitySchemes:
    ApiKeyAuth:
      type: apiKey
      in: header
      name: X-API-Key
`
	encodedSpec := base64.StdEncoding.EncodeToString([]byte(validSpec))

	// ---- Step 1: Developer validates spec before submitting ----
	t.Log("Step 1: Developer validates OAS spec")
	specResult, err := svc.ValidateOASSpec(encodedSpec)
	require.NoError(t, err)
	assert.True(t, specResult.Valid)
	assert.Len(t, specResult.Extracted.Operations, 2)
	assert.Contains(t, specResult.Extracted.Operations, "getCurrentWeather")

	// ---- Step 2: Developer submits tool ----
	t.Log("Step 2: Developer submits tool")
	toolPayload := models.JSONMap{
		"name":                 "Weather API",
		"description":          "Get weather data for any location",
		"tool_type":            "REST",
		"oas_spec":             encodedSpec,
		"auth_schema_name":     "ApiKeyAuth",
		"available_operations": "getCurrentWeather,getForecast",
	}

	submission, err := svc.CreateSubmission(
		developer.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted,
		toolPayload, nil, 20, "Public weather API, no sensitive data",
		"dev@company.com", "", "", nil,
		"https://docs.weather.example.com", "",
	)
	require.NoError(t, err)

	// ---- Step 3: Admin rejects with feedback ----
	t.Log("Step 3: Admin rejects — missing documentation")
	rejected, err := svc.RejectSubmission(submission.ID, admin.ID,
		"Please add rate limiting details and error code documentation to the notes field",
		"Internal: spec looks valid but docs are sparse",
	)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusRejected, rejected.Status)
	assert.Equal(t, "Please add rate limiting details and error code documentation to the notes field", rejected.SubmitterFeedback)

	// Verify developer can't resubmit a rejected submission (must create new one)
	_, err = svc.SubmitSubmission(submission.ID, developer.ID)
	assert.Error(t, err)

	// ---- Step 4: Developer creates a new improved submission ----
	t.Log("Step 4: Developer creates improved submission")
	improvedPayload := models.JSONMap{
		"name":                 "Weather API",
		"description":          "Get weather data for any location. Rate limit: 100 req/min. Errors: 401 (invalid key), 429 (rate limited), 503 (service unavailable)",
		"tool_type":            "REST",
		"oas_spec":             encodedSpec,
		"auth_schema_name":     "ApiKeyAuth",
		"available_operations": "getCurrentWeather,getForecast",
	}

	resubmission, err := svc.CreateSubmission(
		developer.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted,
		improvedPayload, nil, 20, "Public weather API",
		"dev@company.com", "", "", nil,
		"https://docs.weather.example.com",
		"Rate limit: 100 req/min. Error codes documented in description.",
	)
	require.NoError(t, err)

	// ---- Step 5: Admin approves ----
	t.Log("Step 5: Admin approves improved submission")
	approved, err := svc.ApproveSubmission(resubmission.ID, admin.ID, 15, nil, "Much better, approved")
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approved.Status)
	assert.NotNil(t, approved.ResourceID)

	// ---- Step 6: Verify tool created ----
	t.Log("Step 6: Verify tool was created")
	tool, err := svc.GetToolByID(*approved.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "Weather API", tool.Name)
	assert.True(t, tool.CommunitySubmitted)
	assert.Equal(t, developer.ID, tool.UserID)
	assert.Equal(t, 15, tool.PrivacyScore)
	assert.Contains(t, tool.Description, "Rate limit")
}

// TestE2E_ChangesRequestedFlow exercises the changes-requested loop:
// submit → request changes → developer updates → resubmit → approve
func TestE2E_ChangesRequestedFlow(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// ---- Step 1: Submit ----
	submission, err := svc.CreateSubmission(
		developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name":           "Customer DB",
			"db_source_type": "pgvector",
			"embed_vendor":   "openai",
			"embed_model":    "text-embedding-3-small",
			"active":         true,
		},
		nil, 70, "", "dev@company.com", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// ---- Step 2: Admin requests changes ----
	t.Log("Step 2: Admin requests changes")
	changed, err := svc.RequestChanges(submission.ID, admin.ID,
		"Privacy score seems low for customer data. Please justify or increase to 80+. Also add a secondary contact.",
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusChangesRequested, changed.Status)

	// ---- Step 3: Developer updates the submission ----
	t.Log("Step 3: Developer revises submission")
	updated, err := svc.UpdateSubmission(
		submission.ID, developer.ID,
		models.JSONMap{
			"name":           "Customer DB",
			"db_source_type": "pgvector",
			"embed_vendor":   "openai",
			"embed_model":    "text-embedding-3-small",
			"active":         true,
		},
		nil, 85, "Contains anonymized customer interaction data. PII has been stripped.",
		"dev@company.com", "team-lead@company.com", "", nil, "", "",
	)
	require.NoError(t, err)
	assert.Equal(t, 85, updated.SuggestedPrivacy)
	assert.Equal(t, "team-lead@company.com", updated.SecondaryContact)

	// ---- Step 4: Developer resubmits ----
	t.Log("Step 4: Developer resubmits")
	resubmitted, err := svc.SubmitSubmission(submission.ID, developer.ID)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusSubmitted, resubmitted.Status)

	// ---- Step 5: Admin approves ----
	t.Log("Step 5: Admin approves")
	approved, err := svc.ApproveSubmission(submission.ID, admin.ID, 85, nil, "Privacy justified, approved")
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approved.Status)
	assert.NotNil(t, approved.ResourceID)

	ds, err := svc.GetDatasourceByID(*approved.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "Customer DB", ds.Name)
	assert.Equal(t, 85, ds.PrivacyScore)
}

// TestE2E_DuplicateDetection verifies that duplicate resources are flagged
func TestE2E_DuplicateDetection(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// Create an existing datasource via submission
	sub, _ := svc.CreateSubmission(
		developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Product DB", "db_source_type": "pgvector",
			"db_conn_string": "postgresql://vectordb:5432/products",
			"db_name": "products", "embed_vendor": "openai",
			"embed_model": "text-embedding-3-small", "active": true,
		}, nil, 50, "", "", "", "", nil, "", "",
	)
	svc.ApproveSubmission(sub.ID, admin.ID, 45, nil, "")

	// ---- Check for duplicates: same connection string ----
	t.Log("Check: same connection string detected as duplicate")
	dupes, err := svc.CheckForDuplicates(models.SubmissionResourceTypeDatasource, models.JSONMap{
		"name":           "Another Product DB",
		"db_conn_string": "postgresql://vectordb:5432/products",
		"db_name":        "products",
	})
	require.NoError(t, err)
	assert.Len(t, dupes, 1)
	assert.Contains(t, dupes[0].MatchReason, "connection string")

	// ---- Check for duplicates: same name ----
	t.Log("Check: same name detected as duplicate")
	dupes, err = svc.CheckForDuplicates(models.SubmissionResourceTypeDatasource, models.JSONMap{
		"name":           "product db", // case-insensitive
		"db_conn_string": "postgresql://different-host:5432/other",
	})
	require.NoError(t, err)
	assert.Len(t, dupes, 1)
	assert.Contains(t, dupes[0].MatchReason, "name")

	// ---- Check for duplicates: no match ----
	t.Log("Check: no duplicate for different resource")
	dupes, err = svc.CheckForDuplicates(models.SubmissionResourceTypeDatasource, models.JSONMap{
		"name":           "Completely Different DB",
		"db_conn_string": "postgresql://other-host:5432/other",
	})
	require.NoError(t, err)
	assert.Len(t, dupes, 0)
}

// TestE2E_NominateFromExisting verifies the nominate shortcut
func TestE2E_NominateFromExisting(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")

	// Create a datasource that the developer owns
	ds, err := svc.CreateDatasource(
		"My Private DS", "Short desc", "Long desc", "", "",
		50, developer.ID, []string{"ml"},
		"postgresql://localhost:5432/vectors", "pgvector", "secret-key", "vectors",
		"openai", "https://api.openai.com/v1", "sk-key", "text-embedding-3-small",
		true,
	)
	require.NoError(t, err)

	// ---- Nominate the datasource ----
	t.Log("Nominate existing datasource")
	nomination, err := svc.NominateExistingDatasource(developer.ID, ds.ID)
	require.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusDraft, nomination.Status)
	assert.Equal(t, models.SubmissionResourceTypeDatasource, nomination.ResourceType)

	// Verify payload is pre-populated (but credentials are omitted)
	payload := nomination.ResourcePayload
	assert.Equal(t, "My Private DS", payload["name"])
	assert.Equal(t, "pgvector", payload["db_source_type"])
	assert.Equal(t, "openai", payload["embed_vendor"])
	// Credentials should NOT be in the payload
	assert.Nil(t, payload["db_conn_api_key"])
	assert.Nil(t, payload["embed_api_key"])

	// ---- Non-owner cannot nominate ----
	t.Log("Non-owner cannot nominate")
	otherUser := e2eCreateUser(t, svc, "other@company.com")
	_, err = svc.NominateExistingDatasource(otherUser.ID, ds.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized")
}

// TestE2E_OrphanManagement verifies that community resources are flagged when owner is deleted
func TestE2E_OrphanManagement(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// Create a community datasource via submission
	sub, _ := svc.CreateSubmission(
		developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name": "Dev's DS", "db_source_type": "pgvector",
			"embed_vendor": "openai", "embed_model": "text-embedding-3-small",
			"active": true,
		}, nil, 50, "", "", "", "", nil, "", "",
	)
	approved, _ := svc.ApproveSubmission(sub.ID, admin.ID, 45, nil, "")
	dsID := *approved.ResourceID

	// Verify it's active
	ds, _ := svc.GetDatasourceByID(dsID)
	assert.True(t, ds.CommunitySubmitted)

	// ---- Handle user deletion for UGC ----
	t.Log("Handle orphan management when user is deleted")
	err := svc.HandleUserDeletionForUGC(developer.ID)
	require.NoError(t, err)

	// Verify datasource was deactivated
	deactivatedDS := &models.Datasource{}
	svc.DB.First(deactivatedDS, dsID)
	assert.False(t, deactivatedDS.Active)
}

// TestE2E_SpecValidation_InSubmissionWorkflow verifies that spec validation integrates with the submission flow
func TestE2E_SpecValidation_InSubmissionWorkflow(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	// ---- Valid spec passes validation ----
	validSpec := `
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /test:
    get:
      operationId: testOp
      responses:
        "200":
          description: OK
`
	encoded := base64.StdEncoding.EncodeToString([]byte(validSpec))
	result, err := svc.ValidateOASSpec(encoded)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Len(t, result.Extracted.Operations, 1)

	// ---- Invalid spec fails validation with actionable errors ----
	invalidSpec := `
openapi: "3.0.0"
info:
  title: Bad API
  version: "1.0"
paths:
  /test:
    get:
      summary: No operationId!
      responses:
        "200":
          description: OK
`
	encoded = base64.StdEncoding.EncodeToString([]byte(invalidSpec))
	result, err = svc.ValidateOASSpec(encoded)
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
	// Error should mention operationID
	foundOpError := false
	for _, e := range result.Errors {
		if e.Field == "paths" || e.Field == "servers" {
			foundOpError = true
		}
	}
	assert.True(t, foundOpError, "expected structured error about paths or operationID")
}

// =============================================================================
// FAILURE SCENARIO E2E TESTS
// =============================================================================

// TestE2E_InvalidStateTransitions verifies that the state machine rejects invalid transitions
func TestE2E_InvalidStateTransitions(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// Create a submission in each state for testing
	draft, _ := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{"name": "Draft DS"}, nil, 50, "", "", "", "", nil, "", "")

	submitted, _ := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Submitted DS", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
		nil, 50, "", "", "", "", nil, "", "")

	// Get one to approved state
	toApprove, _ := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "To Approve", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
		nil, 50, "", "", "", "", nil, "", "")
	approved, _ := svc.ApproveSubmission(toApprove.ID, admin.ID, 45, nil, "")

	// Get one to rejected state
	toReject, _ := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "To Reject"}, nil, 50, "", "", "", "", nil, "", "")
	svc.RejectSubmission(toReject.ID, admin.ID, "No good", "")

	t.Run("CannotApproveDraft", func(t *testing.T) {
		_, err := svc.ApproveSubmission(draft.ID, admin.ID, 50, nil, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only approve")
	})

	t.Run("CannotRejectDraft", func(t *testing.T) {
		_, err := svc.RejectSubmission(draft.ID, admin.ID, "reason", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only reject")
	})

	t.Run("CannotRequestChangesDraft", func(t *testing.T) {
		_, err := svc.RequestChanges(draft.ID, admin.ID, "feedback", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only request changes")
	})

	t.Run("CannotReviewDraft", func(t *testing.T) {
		_, err := svc.StartReview(draft.ID, admin.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only review")
	})

	t.Run("CannotApproveAlreadyApproved", func(t *testing.T) {
		_, err := svc.ApproveSubmission(approved.ID, admin.ID, 50, nil, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only approve")
	})

	t.Run("CannotRejectAlreadyApproved", func(t *testing.T) {
		_, err := svc.RejectSubmission(approved.ID, admin.ID, "reason", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only reject")
	})

	t.Run("CannotResubmitRejected", func(t *testing.T) {
		_, err := svc.SubmitSubmission(toReject.ID, developer.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only submit")
	})

	t.Run("CannotUpdateSubmittedSubmission", func(t *testing.T) {
		_, err := svc.UpdateSubmission(submitted.ID, developer.ID, models.JSONMap{}, nil, 50, "", "", "", "", nil, "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only update")
	})

	t.Run("CannotDeleteSubmittedSubmission", func(t *testing.T) {
		err := svc.DeleteSubmission(submitted.ID, developer.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only delete")
	})
}

// TestE2E_AuthorizationBoundaries verifies that users cannot act on each other's submissions
func TestE2E_AuthorizationBoundaries(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	alice := e2eCreateUser(t, svc, "alice@company.com")
	bob := e2eCreateUser(t, svc, "bob@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// Alice creates a draft
	aliceDraft, _ := svc.CreateSubmission(alice.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{"name": "Alice's DS"}, nil, 50, "", "", "", "", nil, "", "")

	// Alice creates and gets a resource approved
	aliceSub, _ := svc.CreateSubmission(alice.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Alice's Published DS", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
		nil, 50, "", "", "", "", nil, "", "")
	aliceApproved, _ := svc.ApproveSubmission(aliceSub.ID, admin.ID, 45, nil, "")

	t.Run("BobCannotUpdateAlicesDraft", func(t *testing.T) {
		_, err := svc.UpdateSubmission(aliceDraft.ID, bob.ID, models.JSONMap{}, nil, 50, "", "", "", "", nil, "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})

	t.Run("BobCannotSubmitAlicesDraft", func(t *testing.T) {
		_, err := svc.SubmitSubmission(aliceDraft.ID, bob.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})

	t.Run("BobCannotDeleteAlicesDraft", func(t *testing.T) {
		err := svc.DeleteSubmission(aliceDraft.ID, bob.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})

	t.Run("BobCannotProposeUpdateToAlicesResource", func(t *testing.T) {
		_, err := svc.CreateUpdateSubmission(
			bob.ID, models.SubmissionResourceTypeDatasource, *aliceApproved.ResourceID,
			models.JSONMap{"name": "Hijacked"}, nil, 50, "", "", "", "", nil, "", "", "",
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})

	t.Run("AliceCanOnlySeeOwnSubmissions", func(t *testing.T) {
		// Bob creates his own submission
		svc.CreateSubmission(bob.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{"name": "Bob's DS"}, nil, 50, "", "", "", "", nil, "", "")

		aliceSubs, count, _, err := svc.GetSubmissionsBySubmitter(alice.ID, "", 10, 1)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), count) // only Alice's 2 submissions
		for _, sub := range aliceSubs {
			assert.Equal(t, alice.ID, sub.SubmitterID)
		}
	})
}

// TestE2E_UpdateWorkflow_FailureScenarios verifies update workflow edge cases
func TestE2E_UpdateWorkflow_FailureScenarios(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// Create an admin-curated (non-community) datasource
	adminDS, err := svc.CreateDatasource(
		"Admin DS", "Short", "Long", "", "", 50, admin.ID, nil,
		"conn", "pgvector", "", "db", "openai", "", "", "text-embedding-3-small", true,
	)
	require.NoError(t, err)

	t.Run("CannotUpdateNonOwnedResource", func(t *testing.T) {
		_, err := svc.CreateUpdateSubmission(
			developer.ID, models.SubmissionResourceTypeDatasource, adminDS.ID,
			models.JSONMap{"name": "Hijacked"}, nil, 50, "", "", "", "", nil, "", "", "",
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})

	t.Run("CannotUpdateNonExistentResource", func(t *testing.T) {
		_, err := svc.CreateUpdateSubmission(
			developer.ID, models.SubmissionResourceTypeDatasource, 99999,
			models.JSONMap{"name": "Ghost"}, nil, 50, "", "", "", "", nil, "", "", "",
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("CannotUpdateWithInvalidResourceType", func(t *testing.T) {
		_, err := svc.CreateUpdateSubmission(
			developer.ID, "invalid_type", 1,
			models.JSONMap{}, nil, 50, "", "", "", "", nil, "", "", "",
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource type")
	})

	t.Run("RollbackWithWrongVersionFails", func(t *testing.T) {
		// Create a community DS, approve, update, then try rollback with wrong version
		sub, _ := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
			models.JSONMap{"name": "Rollback Test", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
			nil, 50, "", "", "", "", nil, "", "")
		approved, _ := svc.ApproveSubmission(sub.ID, admin.ID, 45, nil, "")
		dsID := *approved.ResourceID

		// Update to create a version
		updateSub, _ := svc.CreateUpdateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, dsID,
			models.JSONMap{"name": "Updated"}, nil, 50, "", "", "", "", nil, "", "", models.SubmissionStatusSubmitted)
		svc.ApproveSubmission(updateSub.ID, admin.ID, 50, nil, "")

		// Try rollback with non-existent version
		err := svc.RollbackResource(models.SubmissionResourceTypeDatasource, dsID, 99999, admin.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestE2E_SubmissionCreation_EdgeCases verifies edge cases in submission creation
func TestE2E_SubmissionCreation_EdgeCases(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")

	t.Run("InvalidResourceType", func(t *testing.T) {
		_, err := svc.CreateSubmission(developer.ID, "llm", models.SubmissionStatusDraft,
			models.JSONMap{}, nil, 50, "", "", "", "", nil, "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource type")
	})

	t.Run("InvalidInitialStatus", func(t *testing.T) {
		_, err := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusApproved,
			models.JSONMap{}, nil, 50, "", "", "", "", nil, "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "initial status must be")
	})

	t.Run("CanCreateWithEmptyPayload", func(t *testing.T) {
		// Drafts should allow empty payload (user fills in incrementally)
		sub, err := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{}, nil, 0, "", "", "", "", nil, "", "")
		assert.NoError(t, err)
		assert.NotZero(t, sub.ID)
	})

	t.Run("GetNonExistentSubmission", func(t *testing.T) {
		_, err := svc.GetSubmissionByID(99999)
		assert.Error(t, err)
	})

	t.Run("DeleteNonExistentSubmission", func(t *testing.T) {
		err := svc.DeleteSubmission(99999, developer.ID)
		assert.Error(t, err)
	})
}

// TestE2E_NominationFailures verifies nomination edge cases
func TestE2E_NominationFailures(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	otherUser := e2eCreateUser(t, svc, "other@company.com")

	t.Run("NominateNonExistentDatasource", func(t *testing.T) {
		_, err := svc.NominateExistingDatasource(developer.ID, 99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("NominateNonExistentTool", func(t *testing.T) {
		_, err := svc.NominateExistingTool(developer.ID, 99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("NominateDatasourceNotInYourApps", func(t *testing.T) {
		// Create a datasource owned by someone else with no app association
		ds, _ := svc.CreateDatasource("Other's DS", "", "", "", "", 50, otherUser.ID, nil,
			"conn", "pgvector", "", "db", "openai", "", "", "text-embedding-3-small", true)

		_, err := svc.NominateExistingDatasource(developer.ID, ds.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})

	t.Run("NominateToolNotInYourApps", func(t *testing.T) {
		// Create a tool owned by someone else
		tool, _ := svc.CreateTool("Other's Tool", "desc", "REST", "", 50, "", "")
		tool.UserID = otherUser.ID
		tool.Update(svc.DB)

		_, err := svc.NominateExistingTool(developer.ID, tool.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authorized")
	})
}

// TestE2E_ConcurrentSubmissionsForSameResource verifies that multiple update submissions
// for the same resource are handled correctly
func TestE2E_ConcurrentSubmissionsForSameResource(t *testing.T) {
	db := setupE2ETestDB(t)
	svc := NewService(db)

	developer := e2eCreateUser(t, svc, "dev@company.com")
	admin := e2eCreateAdmin(t, svc, "admin@company.com")

	// Create a community datasource
	sub, _ := svc.CreateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Shared DS", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "active": true},
		nil, 50, "", "", "", "", nil, "", "")
	approved, _ := svc.ApproveSubmission(sub.ID, admin.ID, 45, nil, "")
	dsID := *approved.ResourceID

	// Developer submits two updates for the same resource
	update1, err := svc.CreateUpdateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, dsID,
		models.JSONMap{"name": "Update 1"}, nil, 50, "", "", "", "", nil, "", "First update", models.SubmissionStatusSubmitted)
	require.NoError(t, err)

	update2, err := svc.CreateUpdateSubmission(developer.ID, models.SubmissionResourceTypeDatasource, dsID,
		models.JSONMap{"name": "Update 2"}, nil, 50, "", "", "", "", nil, "", "Second update", models.SubmissionStatusSubmitted)
	require.NoError(t, err)

	// Admin approves update1 first
	_, err = svc.ApproveSubmission(update1.ID, admin.ID, 45, nil, "")
	require.NoError(t, err)

	ds, _ := svc.GetDatasourceByID(dsID)
	assert.Equal(t, "Update 1", ds.Name)

	// Admin can still approve update2 (applies on top of update1)
	_, err = svc.ApproveSubmission(update2.ID, admin.ID, 45, nil, "")
	require.NoError(t, err)

	ds, _ = svc.GetDatasourceByID(dsID)
	assert.Equal(t, "Update 2", ds.Name)

	// Both updates created version snapshots
	versions, _ := svc.GetResourceVersions(models.SubmissionResourceTypeDatasource, dsID)
	assert.Len(t, versions, 2)
}
