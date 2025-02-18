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
