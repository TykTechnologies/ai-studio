package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForSubmissions(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func createSubmissionTestUser(t *testing.T, service *Service, email string) *models.User {
	user, err := service.CreateUser(UserDTO{
		Email: email, Name: "Test User", Password: "password123",
		IsAdmin: false, ShowChat: true, ShowPortal: true,
		EmailVerified: true, NotificationsEnabled: false,
		AccessToSSOConfig: false, Groups: []uint{},
	})
	assert.NoError(t, err)
	return user
}

func createSubmissionTestAdmin(t *testing.T, service *Service, email string) *models.User {
	user, err := service.CreateUser(UserDTO{
		Email: email, Name: "Admin User", Password: "password123",
		IsAdmin: true, ShowChat: true, ShowPortal: true,
		EmailVerified: true, NotificationsEnabled: true,
		AccessToSSOConfig: true, Groups: []uint{},
	})
	assert.NoError(t, err)
	return user
}

func TestCreateSubmission_Draft(t *testing.T) {
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
	assert.NotNil(t, submission)
	assert.NotZero(t, submission.ID)
	assert.Equal(t, models.SubmissionStatusDraft, submission.Status)
	assert.Equal(t, user.ID, submission.SubmitterID)
	assert.Nil(t, submission.SubmittedAt)
}

func TestCreateSubmission_Submitted(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	submission, err := service.CreateSubmission(
		user.ID,
		models.SubmissionResourceTypeTool,
		models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Weather API", "tool_type": "REST"},
		nil, 3, "Public API",
		"submitter@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusSubmitted, submission.Status)
	assert.NotNil(t, submission.SubmittedAt)
}

func TestCreateSubmission_InvalidResourceType(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	_, err := service.CreateSubmission(
		user.ID, "invalid_type", models.SubmissionStatusDraft,
		models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource type")
}

func TestCreateSubmission_InvalidInitialStatus(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	_, err := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusApproved,
		models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initial status must be")
}

func TestUpdateSubmission_DraftOnly(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{"name": "Original"}, nil, 5, "", "contact@test.com", "", "", nil, "", "",
	)

	updated, err := service.UpdateSubmission(
		submission.ID, user.ID,
		models.JSONMap{"name": "Updated"}, nil, 6, "Updated justification",
		"new-contact@test.com", "", "", nil, "", "",
	)
	assert.NoError(t, err)
	assert.Equal(t, 6, updated.SuggestedPrivacy)
	assert.Equal(t, "new-contact@test.com", updated.PrimaryContact)
}

func TestUpdateSubmission_WrongUser(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user1 := createSubmissionTestUser(t, service, "user1@test.com")
	user2 := createSubmissionTestUser(t, service, "user2@test.com")

	submission, _ := service.CreateSubmission(
		user1.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)

	_, err := service.UpdateSubmission(
		submission.ID, user2.ID, models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized")
}

func TestUpdateSubmission_CannotUpdateSubmittedStatus(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)

	_, err := service.UpdateSubmission(
		submission.ID, user.ID, models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can only update submissions")
}

func TestSubmitSubmission(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{"name": "Submit Test", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small"},
		nil, 5, "", "", "", "", nil, "", "",
	)

	submitted, err := service.SubmitSubmission(submission.ID, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusSubmitted, submitted.Status)
	assert.NotNil(t, submitted.SubmittedAt)
}

func TestSubmitSubmission_WrongUser(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user1 := createSubmissionTestUser(t, service, "user1@test.com")
	user2 := createSubmissionTestUser(t, service, "user2@test.com")

	submission, _ := service.CreateSubmission(
		user1.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)

	_, err := service.SubmitSubmission(submission.ID, user2.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized")
}

func TestDeleteSubmission_DraftOnly(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)

	err := service.DeleteSubmission(submission.ID, user.ID)
	assert.NoError(t, err)

	_, err = service.GetSubmissionByID(submission.ID)
	assert.Error(t, err)
}

func TestDeleteSubmission_CannotDeleteSubmitted(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "",
	)

	err := service.DeleteSubmission(submission.ID, user.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can only delete")
}

func TestGetSubmissionsBySubmitter(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft, models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "")
	service.CreateSubmission(user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted, models.JSONMap{}, nil, 3, "", "", "", "", nil, "", "")
	service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft, models.JSONMap{}, nil, 6, "", "", "", "", nil, "", "")

	// All submissions
	submissions, totalCount, _, err := service.GetSubmissionsBySubmitter(user.ID, "", 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), totalCount)
	assert.Len(t, submissions, 3)

	// Filter by status
	submissions, totalCount, _, err = service.GetSubmissionsBySubmitter(user.ID, models.SubmissionStatusDraft, 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), totalCount)
	assert.Len(t, submissions, 2)
}

func TestAdminReviewWorkflow(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name":           "Test DS",
			"db_source_type": "pgvector",
			"embed_vendor":   "openai",
			"embed_model":    "text-embedding-3-small",
		},
		nil, 5, "Low sensitivity data",
		"contact@test.com", "", "", nil, "", "",
	)

	t.Run("StartReview", func(t *testing.T) {
		reviewed, err := service.StartReview(submission.ID, admin.ID)
		assert.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusInReview, reviewed.Status)
		assert.Equal(t, &admin.ID, reviewed.ReviewerID)
		assert.NotNil(t, reviewed.ReviewStartedAt)
	})

	t.Run("StartReview_AlreadyInReview", func(t *testing.T) {
		_, err := service.StartReview(submission.ID, admin.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can only review submissions")
	})
}

func TestAdminApproveSubmission_Datasource(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name":             "Community Vector DB",
			"short_description": "Product embeddings",
			"db_source_type":   "pgvector",
			"db_conn_string":   "postgresql://localhost:5432/vectors",
			"embed_vendor":     "openai",
			"embed_model":      "text-embedding-3-small",
			"active":           true,
		},
		nil, 5, "Low sensitivity",
		"contact@test.com", "", "", nil, "", "",
	)

	approved, err := service.ApproveSubmission(submission.ID, admin.ID, 5, models.JSONMap{"catalogue_ids": []int{1}}, "Looks good")
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approved.Status)
	assert.NotNil(t, approved.ResourceID)
	assert.NotNil(t, approved.ReviewCompletedAt)
	finalScore := 5
	assert.Equal(t, &finalScore, approved.FinalPrivacyScore)

	// Verify the datasource was created
	ds, err := service.GetDatasourceByID(*approved.ResourceID)
	assert.NoError(t, err)
	assert.Equal(t, "Community Vector DB", ds.Name)
	assert.True(t, ds.CommunitySubmitted)
	assert.Equal(t, 5, ds.PrivacyScore)
	assert.Equal(t, user.ID, ds.UserID)
}

func TestAdminApproveSubmission_Tool(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted,
		models.JSONMap{
			"name":        "Weather API",
			"description": "Get weather data",
			"tool_type":   "REST",
			"oas_spec":    "dGVzdA==", // base64 of "test"
		},
		nil, 2, "Public API",
		"contact@test.com", "", "", nil, "", "",
	)

	approved, err := service.ApproveSubmission(submission.ID, admin.ID, 2, nil, "Approved")
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusApproved, approved.Status)
	assert.NotNil(t, approved.ResourceID)

	// Verify the tool was created
	tool, err := service.GetToolByID(*approved.ResourceID)
	assert.NoError(t, err)
	assert.Equal(t, "Weather API", tool.Name)
	assert.True(t, tool.CommunitySubmitted)
	assert.Equal(t, user.ID, tool.UserID)
	assert.Equal(t, 2, tool.PrivacyScore)
}

func TestAdminRejectSubmission(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Bad DS"}, nil, 5, "", "", "", "", nil, "", "",
	)

	rejected, err := service.RejectSubmission(submission.ID, admin.ID, "Insufficient documentation", "Internal: not enough info")
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusRejected, rejected.Status)
	assert.Equal(t, "Insufficient documentation", rejected.SubmitterFeedback)
	assert.Equal(t, "Internal: not enough info", rejected.ReviewNotes)
	assert.NotNil(t, rejected.ReviewCompletedAt)
	assert.Nil(t, rejected.ResourceID)
}

func TestAdminRequestChanges(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")

	submission, _ := service.CreateSubmission(
		user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted,
		models.JSONMap{"name": "Needs Work", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small"},
		nil, 5, "", "", "", "", nil, "", "",
	)

	changed, err := service.RequestChanges(submission.ID, admin.ID, "Please add documentation URL", "")
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusChangesRequested, changed.Status)
	assert.Equal(t, "Please add documentation URL", changed.SubmitterFeedback)

	// Submitter can update after changes requested
	updated, err := service.UpdateSubmission(
		submission.ID, user.ID,
		models.JSONMap{"name": "Needs Work", "db_source_type": "pgvector", "embed_vendor": "openai", "embed_model": "text-embedding-3-small", "documentation_url": "https://docs.example.com"},
		nil, 5, "", "", "", "", nil, "https://docs.example.com", "",
	)
	assert.NoError(t, err)
	assert.Equal(t, "https://docs.example.com", updated.DocumentationURL)

	// Submitter can resubmit
	resubmitted, err := service.SubmitSubmission(submission.ID, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.SubmissionStatusSubmitted, resubmitted.Status)
}

func TestGetAllSubmissions_AdminFiltering(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft, models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "")
	service.CreateSubmission(user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted, models.JSONMap{}, nil, 3, "", "", "", "", nil, "", "")
	service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted, models.JSONMap{}, nil, 6, "", "", "", "", nil, "", "")

	// No filter
	all, count, _, err := service.GetAllSubmissions("", "", 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)
	assert.Len(t, all, 3)

	// Filter by status
	submitted, count, _, err := service.GetAllSubmissions(models.SubmissionStatusSubmitted, "", 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.Len(t, submitted, 2)

	// Filter by resource type
	tools, count, _, err := service.GetAllSubmissions("", models.SubmissionResourceTypeTool, 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Len(t, tools, 1)
}

func TestGetSubmissionStatusCounts(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	user := createSubmissionTestUser(t, service, "submitter@test.com")

	service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft, models.JSONMap{}, nil, 5, "", "", "", "", nil, "", "")
	service.CreateSubmission(user.ID, models.SubmissionResourceTypeTool, models.SubmissionStatusSubmitted, models.JSONMap{}, nil, 3, "", "", "", "", nil, "", "")
	service.CreateSubmission(user.ID, models.SubmissionResourceTypeDatasource, models.SubmissionStatusSubmitted, models.JSONMap{}, nil, 6, "", "", "", "", nil, "", "")

	counts, err := service.GetSubmissionStatusCounts()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), counts[models.SubmissionStatusDraft])
	assert.Equal(t, int64(2), counts[models.SubmissionStatusSubmitted])
}
