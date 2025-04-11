package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestAppEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)

	// Create a test user
	user := &models.User{
		Email:         "test@example.com",
		Name:          "Test User",
		IsAdmin:       true,
		ShowPortal:    true,
		ShowChat:      true,
		EmailVerified: true,
	}
	err := user.Create(db)
	assert.NoError(t, err)

	// Test Create App
	createAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:            "Test App",
				Description:     "Test Description",
				UserID:          user.ID,
				DatasourceIDs:   []uint{},
				LLMIDs:          []uint{},
				MonthlyBudget:   nil,
				BudgetStartDate: nil,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]AppResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "app", response["data"].Type)
	assert.Equal(t, "Test App", response["data"].Attributes.Name)
	assert.Equal(t, "Test Description", response["data"].Attributes.Description)
	assert.Equal(t, user.ID, response["data"].Attributes.UserID)
	assert.Empty(t, response["data"].Attributes.DatasourceIDs)
	assert.Empty(t, response["data"].Attributes.LLMIDs)
}

func TestAppPagination(t *testing.T) {
	api, db := setupTestAPI(t)

	// Create a test user
	user := &models.User{
		Email:         "test@example.com",
		Name:          "Test User",
		IsAdmin:       true,
		ShowPortal:    true,
		ShowChat:      true,
		EmailVerified: true,
	}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create multiple apps
	for i := 0; i < 10; i++ {
		createAppInput := AppInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Name            string     `json:"name"`
					Description     string     `json:"description"`
					UserID          uint       `json:"user_id"`
					DatasourceIDs   []uint     `json:"datasource_ids"`
					LLMIDs          []uint     `json:"llm_ids"`
					MonthlyBudget   *float64   `json:"monthly_budget"`
					BudgetStartDate *time.Time `json:"budget_start_date"`
				} `json:"attributes"`
			}{
				Type: "app",
				Attributes: struct {
					Name            string     `json:"name"`
					Description     string     `json:"description"`
					UserID          uint       `json:"user_id"`
					DatasourceIDs   []uint     `json:"datasource_ids"`
					LLMIDs          []uint     `json:"llm_ids"`
					MonthlyBudget   *float64   `json:"monthly_budget"`
					BudgetStartDate *time.Time `json:"budget_start_date"`
				}{
					Name:            fmt.Sprintf("Test App %d", i),
					Description:     fmt.Sprintf("Test Description %d", i),
					UserID:          user.ID,
					DatasourceIDs:   []uint{},
					LLMIDs:          []uint{},
					MonthlyBudget:   nil,
					BudgetStartDate: nil,
				},
			},
		}

		w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	// Test pagination
	w := performRequest(api.router, "GET", "/api/v1/apps?page=2&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []AppResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	assert.Len(t, response.Data, 5)
}

func TestCreateAppPrivacyScoreMismatch(t *testing.T) {
	api, db := setupTestAPI(t)

	// Create a test user
	user := &models.User{
		Email:         "test@example.com",
		Name:          "Test User",
		IsAdmin:       true,
		ShowPortal:    true,
		ShowChat:      true,
		EmailVerified: true,
	}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create an LLM with privacy score 1
	llm := &models.LLM{
		Name:         "Test LLM",
		APIKey:       "test-api-key",
		APIEndpoint:  "https://test-endpoint.com",
		DefaultModel: "test-model",
		PrivacyScore: 1, // Low privacy score
	}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	// Create a datasource with privacy score 5
	datasource := &models.Datasource{
		Name:         "Test Datasource",
		PrivacyScore: 5, // High privacy score
		UserID:       user.ID,
	}
	err = db.Create(datasource).Error
	assert.NoError(t, err)

	// Try to create an app with mismatched privacy scores
	createAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:          "Privacy Mismatch App",
				Description:   "App with privacy score mismatch",
				UserID:        user.ID,
				DatasourceIDs: []uint{datasource.ID},
				LLMIDs:        []uint{llm.ID},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errorResponse models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	
	// Verify error message contains privacy score mismatch information
	assert.Len(t, errorResponse.Errors, 1)
	assert.Contains(t, errorResponse.Errors[0].Detail, "privacy score mismatch")
}

func TestUpdateAppPrivacyScoreMismatch(t *testing.T) {
	api, db := setupTestAPI(t)

	// Create a test user
	user := &models.User{
		Email:         "test@example.com",
		Name:          "Test User",
		IsAdmin:       true,
		ShowPortal:    true,
		ShowChat:      true,
		EmailVerified: true,
	}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create an LLM with privacy score 1
	llm := &models.LLM{
		Name:         "Test LLM",
		APIKey:       "test-api-key",
		APIEndpoint:  "https://test-endpoint.com",
		DefaultModel: "test-model",
		PrivacyScore: 1, // Low privacy score
	}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	// Create a datasource with privacy score 5
	datasource := &models.Datasource{
		Name:         "Test Datasource",
		PrivacyScore: 5, // High privacy score
		UserID:       user.ID,
	}
	err = db.Create(datasource).Error
	assert.NoError(t, err)

	// First create a valid app (without datasource)
	app := &models.App{
		Name:        "Test App",
		Description: "Test Description",
		UserID:      user.ID,
	}
	err = db.Create(app).Error
	assert.NoError(t, err)

	// Add the LLM to the app
	err = db.Model(app).Association("LLMs").Append(llm)
	assert.NoError(t, err)

	// Now try to update the app with a datasource that has a higher privacy score
	updateAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:          "Updated App",
				Description:   "Updated Description",
				UserID:        user.ID,
				DatasourceIDs: []uint{datasource.ID},
				LLMIDs:        []uint{llm.ID},
			},
		},
	}

	w := performRequest(api.router, "PUT", fmt.Sprintf("/api/v1/apps/%d", app.ID), updateAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errorResponse models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	
	// Verify error message contains privacy score mismatch information
	assert.Len(t, errorResponse.Errors, 1)
	assert.Contains(t, errorResponse.Errors[0].Detail, "privacy score mismatch")
}
