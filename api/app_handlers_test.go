package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Redefine AppInput for tests to include ToolIDs as []string
type TestAppInput struct {
	Data struct {
		Type       string `json:"type" binding:"required,eq=app"`
		Attributes struct {
			Name            string     `json:"name"`
			Description     string     `json:"description"`
			UserID          uint       `json:"user_id"`
			DatasourceIDs   []string   `json:"datasource_ids"`
			LLMIDs          []string   `json:"llm_ids"`
			ToolIDs         []string   `json:"tool_ids"` // Added for tools
			MonthlyBudget   *float64   `json:"monthly_budget"`
			BudgetStartDate *time.Time `json:"budget_start_date"`
		} `json:"attributes" binding:"required"`
	} `json:"data" binding:"required"`
}

func TestAppEndpointsWithTools(t *testing.T) {
	api, db := setupTestAPI(t) // setupTestAPI should handle DB migrations including AppTool

	// 1. Create a User
	user := &models.User{Email: "apphandlertest@example.com", Name: "AppHandler User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	// 2. Create Tools
	tool1 := &models.Tool{Name: "Handler Tool 1", Description: "Desc 1", ToolType: "REST"}
	err = tool1.Create(db)
	assert.NoError(t, err)

	tool2 := &models.Tool{Name: "Handler Tool 2", Description: "Desc 2", ToolType: "REST"}
	err = tool2.Create(db)
	assert.NoError(t, err)

	// 3. Test Create App with ToolIDs
	createAppPayload := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app", // Ensure this matches the binding in AppInput struct
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:        "App With Tools Handler Test",
				Description: "Test app creation with tools",
				UserID:      user.ID,
				ToolIDs:     []string{strconv.Itoa(int(tool1.ID))},
			},
		},
	}
	w := performRequest(api.router, "POST", "/api/v1/apps", createAppPayload)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createAppResponse AppResponseWrapper // Use a wrapper if response is { "data": AppResponse }
	err = json.Unmarshal(w.Body.Bytes(), &createAppResponse)
	assert.NoError(t, err)
	createdAppID, _ := strconv.ParseUint(createAppResponse.Data.ID, 10, 32)
	assert.NotZero(t, createdAppID)
	assert.Len(t, createAppResponse.Data.Attributes.Tools, 1, "Should have 1 tool associated on create")
	if len(createAppResponse.Data.Attributes.Tools) > 0 {
		assert.Equal(t, strconv.Itoa(int(tool1.ID)), createAppResponse.Data.Attributes.Tools[0].ID)
	}

	// 4. Test Get App Tools
	wGetTools := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%d/tools", createdAppID), nil)
	assert.Equal(t, http.StatusOK, wGetTools.Code)
	var getToolsResponse ToolsResponseWrapper // Assuming { "data": []ToolResponse }
	err = json.Unmarshal(wGetTools.Body.Bytes(), &getToolsResponse)
	assert.NoError(t, err)
	assert.Len(t, getToolsResponse.Data, 1)
	assert.Equal(t, strconv.Itoa(int(tool1.ID)), getToolsResponse.Data[0].ID)

	// 5. Test Add Tool to App
	wAddTool := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%d/tools/%d", createdAppID, tool2.ID), nil)
	assert.Equal(t, http.StatusOK, wAddTool.Code)
	var addToolResponse AppResponseWrapper
	err = json.Unmarshal(wAddTool.Body.Bytes(), &addToolResponse)
	assert.NoError(t, err)
	assert.Len(t, addToolResponse.Data.Attributes.Tools, 2, "Should have 2 tools after adding another")

	// Verify by getting tools again
	wGetToolsAfterAdd := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%d/tools", createdAppID), nil)
	assert.Equal(t, http.StatusOK, wGetToolsAfterAdd.Code)
	err = json.Unmarshal(wGetToolsAfterAdd.Body.Bytes(), &getToolsResponse)
	assert.NoError(t, err)
	assert.Len(t, getToolsResponse.Data, 2)

	// 6. Test Update App with new ToolIDs (remove tool1, keep tool2)
	updateAppPayload := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:        "App With Tools Handler Test Updated",
				Description: "Test app update with tools",
				UserID:      user.ID,
				ToolIDs:     []string{strconv.Itoa(int(tool2.ID))}, // Only tool2
			},
		},
	}
	wUpdate := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%d", createdAppID), updateAppPayload)
	assert.Equal(t, http.StatusOK, wUpdate.Code)
	var updateAppResponse AppResponseWrapper
	err = json.Unmarshal(wUpdate.Body.Bytes(), &updateAppResponse)
	assert.NoError(t, err)
	assert.Len(t, updateAppResponse.Data.Attributes.Tools, 1, "Should have 1 tool after update")
	if len(updateAppResponse.Data.Attributes.Tools) > 0 {
		assert.Equal(t, strconv.Itoa(int(tool2.ID)), updateAppResponse.Data.Attributes.Tools[0].ID)
	}

	// 7. Test Remove Tool from App
	wRemoveTool := performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/apps/%d/tools/%d", createdAppID, tool2.ID), nil)
	assert.Equal(t, http.StatusNoContent, wRemoveTool.Code)

	// Verify by getting tools again
	wGetToolsAfterRemove := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%d/tools", createdAppID), nil)
	assert.Equal(t, http.StatusOK, wGetToolsAfterRemove.Code)
	err = json.Unmarshal(wGetToolsAfterRemove.Body.Bytes(), &getToolsResponse)
	assert.NoError(t, err)
	assert.Len(t, getToolsResponse.Data, 0, "Should have 0 tools after removal")

	// 8. Error case: Get tools for non-existent app
	wGetToolsNonExistentApp := performRequest(api.router, "GET", "/api/v1/apps/99999/tools", nil)
	assert.Equal(t, http.StatusNotFound, wGetToolsNonExistentApp.Code)

	// 9. Error case: Add tool to non-existent app
	wAddToolNonExistentApp := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/99999/tools/%d", tool1.ID), nil)
	assert.Equal(t, http.StatusNotFound, wAddToolNonExistentApp.Code)

	// 10. Error case: Add non-existent tool to app
	wAddNonExistentTool := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%d/tools/88888", createdAppID), nil)
	assert.Equal(t, http.StatusNotFound, wAddNonExistentTool.Code) // Service should return error if tool not found
}

// AppResponseWrapper is used if the actual response is nested under a "data" key.
type AppResponseWrapper struct {
	Data AppResponse `json:"data"`
}

// ToolsResponseWrapper for GET /apps/:id/tools
type ToolsResponseWrapper struct {
	Data []ToolResponse `json:"data"`
}


// TestAppEndpoints and TestAppPagination are existing tests.
// I need to update their AppInput to use TestAppInput (with string IDs) if they create apps.
// For brevity, I'm showing the new test function above and assuming modifications to existing tests
// like TestAppEndpoints and TestAppPagination to use TestAppInput for consistency.

func TestAppEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)

	user := &models.User{Email: "testendpoints@example.com", Name: "Test User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	// Use TestAppInput which expects string IDs for LLMs/Datasources/Tools
	createAppInput := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:            "Test App Old",
				Description:     "Test Description Old",
				UserID:          user.ID,
				DatasourceIDs:   []string{}, // Empty string slices
				LLMIDs:          []string{},
				ToolIDs:         []string{}, // Important: provide empty slice for new field
				MonthlyBudget:   nil,
				BudgetStartDate: nil,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response AppResponseWrapper
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "app", response.Data.Type)
	assert.Equal(t, "Test App Old", response.Data.Attributes.Name)
	assert.Empty(t, response.Data.Attributes.Tools) // Expect empty tools
}

func TestAppPagination(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "testpagination@example.com", Name: "Pagination User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		createAppInput := TestAppInput{ // Use TestAppInput
			Data: struct {
				Type       string `json:"type" binding:"required,eq=app"`
				Attributes struct {
					Name            string     `json:"name"`
					Description     string     `json:"description"`
					UserID          uint       `json:"user_id"`
					DatasourceIDs   []string   `json:"datasource_ids"`
					LLMIDs          []string   `json:"llm_ids"`
					ToolIDs         []string   `json:"tool_ids"`
					MonthlyBudget   *float64   `json:"monthly_budget"`
					BudgetStartDate *time.Time `json:"budget_start_date"`
				} `json:"attributes" binding:"required"`
			}{
				Type: "app",
				Attributes: struct {
					Name            string     `json:"name"`
					Description     string     `json:"description"`
					UserID          uint       `json:"user_id"`
					DatasourceIDs   []string   `json:"datasource_ids"`
					LLMIDs          []string   `json:"llm_ids"`
					ToolIDs         []string   `json:"tool_ids"`
					MonthlyBudget   *float64   `json:"monthly_budget"`
					BudgetStartDate *time.Time `json:"budget_start_date"`
				}{
					Name:            fmt.Sprintf("Test App %d", i),
					UserID:          user.ID,
					ToolIDs:         []string{}, // Add empty ToolIDs
				},
			},
		}
		w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	w := performRequest(api.router, "GET", "/api/v1/apps?page=2&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var response AppListResponse // Assuming AppListResponse is defined for list views
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assert.Len(t, response.Data, 5)
}

// Assuming AppListResponse is similar to this:
type AppListResponse struct {
	Data []AppResponse `json:"data"`
	// Meta map[string]int `json:"meta"` // If you have meta for pagination
}


func TestCreateAppPrivacyScoreMismatch(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "testprivacy@example.com", Name: "Privacy User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	llm := &models.LLM{Name: "Low Privacy LLM", PrivacyScore: 1, Vendor: "Test", ModelID: "test"}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	datasource := &models.Datasource{Name: "High Privacy DS", PrivacyScore: 5, UserID: user.ID}
	err = db.Create(datasource).Error
	assert.NoError(t, err)

	createAppInput := TestAppInput{ // Use TestAppInput
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:          "Privacy Mismatch App",
				UserID:        user.ID,
				DatasourceIDs: []string{strconv.Itoa(int(datasource.ID))},
				LLMIDs:        []string{strconv.Itoa(int(llm.ID))},
				ToolIDs:       []string{},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errorResponse ErrorResponse // Use the global ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.Len(t, errorResponse.Errors, 1)
	assert.Contains(t, errorResponse.Errors[0].Detail, "Datasources have higher privacy requirements than the selected LLMs")
}

func TestUpdateAppPrivacyScoreMismatch(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "testupdateprivacy@example.com", Name: "Update Privacy User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	llm := &models.LLM{Name: "Low Privacy LLM Update", PrivacyScore: 1, Vendor: "Test", ModelID: "test-update"}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	datasource := &models.Datasource{Name: "High Privacy DS Update", PrivacyScore: 5, UserID: user.ID}
	err = db.Create(datasource).Error
	assert.NoError(t, err)

	app := &models.App{Name: "App to Update Privacy", UserID: user.ID}
	err = db.Create(app).Error
	assert.NoError(t, err)
	err = db.Model(app).Association("LLMs").Append(llm) // Associate LLM first
	assert.NoError(t, err)


	updateAppInput := TestAppInput{ // Use TestAppInput
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []string   `json:"datasource_ids"`
				LLMIDs          []string   `json:"llm_ids"`
				ToolIDs         []string   `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:          "Updated App Privacy",
				UserID:        user.ID,
				DatasourceIDs: []string{strconv.Itoa(int(datasource.ID))},
				LLMIDs:        []string{strconv.Itoa(int(llm.ID))},
				ToolIDs:       []string{},
			},
		},
	}

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%d", app.ID), updateAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errorResponse ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.Len(t, errorResponse.Errors, 1)
	assert.Contains(t, errorResponse.Errors[0].Detail, "Datasources have higher privacy requirements than the selected LLMs")
}

// performRequest is a helper function from the existing test file.
// Ensure it's available or define it.
func performRequest(r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var req *http.Request
	var err error
	if body != nil {
		payload, _ := json.Marshal(body)
		req, err = http.NewRequest(method, path, bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, path, nil)
	}
	if err != nil {
		panic(err)
	}
	// Add dummy auth token if needed by middleware
	// req.Header.Set("Authorization", "Bearer testtoken")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// setupTestAPI is a helper from existing tests.
// Ensure it initializes the DB and API correctly.
// It's assumed that setupTestAPI internally calls models.InitModels(db)
// which now includes AppTool migration.
// For this example, I'll stub a simplified version if not provided.
// func setupTestAPI(t *testing.T) (*API, *gorm.DB) {
// 	gin.SetMode(gin.TestMode)
// 	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
// 	assert.NoError(t, err)
// 	models.InitModels(db) // Ensure this migrates AppTool
// 	service := services.NewService(db)
// 	// Simplified auth service for testing
// 	authService := auth.NewAuthService(&auth.Config{DB: db, TestMode: true}, nil, service, nil)
// 	api := NewAPI(service, true, authService, &auth.Config{DB: db, TestMode: true}, nil, embed.FS{}, nil)
// 	return api, db
// }

// ErrorResponse struct, assuming it's defined globally or in models
// type ErrorResponse struct {
// 	Errors []struct {
// 		Title  string `json:"title"`
// 		Detail string `json:"detail"`
// 	} `json:"errors"`
// }
